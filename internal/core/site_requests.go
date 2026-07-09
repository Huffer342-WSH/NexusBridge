// site_requests.go 负责统一解析站点凭据并发起受策略约束的 HTTP 请求。
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/requestpolicy"
)

const defaultSiteRequestTimeout = 30 * time.Second

// siteRequestOptions 描述不同站点资源请求的请求头和 Cookie 要求。
type siteRequestOptions struct {
	Headers        http.Header
	Timeout        time.Duration
	RequireCookies bool
	LogRequest     bool
}

// fetchSiteResource 通过统一 Cookie 策略请求站点页面、种子文件或图片。
func (a *App) fetchSiteResource(ctx context.Context, site runtimeSite, targetURL string, opts siteRequestOptions) (fetcher.FetchResult, error) {
	credential, _, err := a.store.LoadSiteCredential(ctx, site.ID)
	if err != nil {
		return fetcher.FetchResult{}, err
	}
	cookies, err := a.store.LoadCookies(ctx, site.BaseURL)
	if err != nil {
		return fetcher.FetchResult{}, err
	}
	if opts.RequireCookies && len(cookies) == 0 {
		return fetcher.FetchResult{}, fmt.Errorf("no cookies found for %s", site.BaseURL)
	}
	headers := siteCredentialHeaders(credential.Headers, opts.Headers)
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", firstNonEmpty(credential.UserAgent, site.UserAgent))
	}
	resolver := func(requestURL string) ([]*http.Cookie, error) {
		resolution, err := requestpolicy.ResolveCookies(site.BaseURL, requestURL, site.Definition.RequestRules, cookies)
		if err != nil {
			return nil, err
		}
		if resolution.MatchedSuffix != "" || len(resolution.MissingNames) > 0 {
			slog.Info("site request cookie policy resolved",
				"site_id", site.ID,
				"target_host", resolution.TargetHost,
				"matched_suffix", resolution.MatchedSuffix,
				"cookie_names", resolution.SelectedNames,
				"missing_cookie_names", resolution.MissingNames,
			)
		}
		return resolution.Cookies, nil
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultSiteRequestTimeout
	}
	return fetcher.FetchTorrentsURL(ctx, targetURL, fetcher.FetchOptions{
		BaseURL: site.BaseURL, CookieResolver: resolver, Headers: headers, Timeout: timeout,
		Logger: slog.Default(), LogRequest: opts.LogRequest, LogSensitive: a.cfg.Logging.LogSensitiveFetch,
	})
}

// downloadRecoveryTorrentURL 按 URL 自动选择私站 Cookie 策略并解析 torrent。
func (a *App) downloadRecoveryTorrentURL(ctx context.Context, rawURL string, requestedSiteIDs []string) ([]byte, Torrent, error) {
	targetURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") || targetURL.Host == "" {
		return nil, Torrent{}, fmt.Errorf("torrent_url must be an absolute http or https url")
	}
	requestedSiteIDs = trimmedRecoverySiteIDs(requestedSiteIDs)
	if len(requestedSiteIDs) > 1 {
		return nil, Torrent{}, fmt.Errorf("torrent_url accepts at most one site")
	}

	var matchedSite *runtimeSite
	bestScore := -1
	candidates := a.siteIDs
	if len(requestedSiteIDs) == 1 {
		candidates = requestedSiteIDs
	}
	for _, siteID := range candidates {
		site, err := a.findSite(siteID)
		if err != nil {
			return nil, Torrent{}, err
		}
		resolution, err := requestpolicy.ResolveCookies(site.BaseURL, targetURL.String(), site.Definition.RequestRules, nil)
		if err != nil {
			return nil, Torrent{}, err
		}
		score := -1
		if resolution.SameSite {
			score = 10000
		} else if resolution.MatchedSuffix != "" {
			score = len(resolution.MatchedSuffix)
		}
		if score > bestScore {
			copySite := site
			matchedSite, bestScore = &copySite, score
		}
	}
	if len(requestedSiteIDs) == 1 && bestScore < 0 {
		return nil, Torrent{}, fmt.Errorf("torrent_url does not match site %s or its request_rules", requestedSiteIDs[0])
	}

	headers := http.Header{}
	headers.Set("Accept", "application/x-bittorrent,application/octet-stream;q=0.9,*/*;q=0.8")
	var result fetcher.FetchResult
	if matchedSite != nil && bestScore >= 0 {
		headers.Set("Referer", matchedSite.BaseURL)
		result, err = a.fetchSiteResource(ctx, *matchedSite, targetURL.String(), siteRequestOptions{
			Headers: headers, Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
		})
	} else {
		result, err = fetcher.FetchTorrentsURL(ctx, targetURL.String(), fetcher.FetchOptions{
			Headers: headers, Timeout: defaultSiteRequestTimeout, Logger: slog.Default(), LogRequest: true,
			LogSensitive: a.cfg.Logging.LogSensitiveFetch,
		})
	}
	if err != nil {
		return nil, Torrent{}, err
	}
	if err := validateTorrentData(result.Body); err != nil {
		return nil, Torrent{}, fmt.Errorf("downloaded torrent payload is invalid: %w", err)
	}
	metadata, err := qbittorrent.ParseTorrentMetadata(result.Body)
	if err != nil {
		return nil, Torrent{}, fmt.Errorf("parse downloaded torrent: %w", err)
	}
	siteID := "url"
	if matchedSite != nil && bestScore >= 0 {
		siteID = matchedSite.ID
	}
	torrentID := firstNonEmpty(metadata.Hashes.V1, metadata.Hashes.V2)
	return result.Body, Torrent{
		ID: torrentID, SiteID: siteID, Title: metadata.Name, DetailURL: targetURL.String(), DownloadURL: targetURL.String(),
		InfoHashV1: metadata.Hashes.V1, InfoHashV2: metadata.Hashes.V2,
	}, nil
}

// siteCredentialHeaders 合并站点持久化请求头与资源专用请求头并禁止 Cookie 绕过策略。
func siteCredentialHeaders(saved map[string]string, overrides http.Header) http.Header {
	headers := http.Header{}
	for name, value := range saved {
		if strings.EqualFold(name, "Cookie") || strings.TrimSpace(name) == "" {
			continue
		}
		headers.Set(name, value)
	}
	for name, values := range overrides {
		if strings.EqualFold(name, "Cookie") {
			continue
		}
		headers.Del(name)
		for _, value := range values {
			headers.Add(name, value)
		}
	}
	return headers
}
