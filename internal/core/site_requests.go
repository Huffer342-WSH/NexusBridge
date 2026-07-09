// site_requests.go 负责统一解析站点凭据并发起受策略约束的 HTTP 请求。
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"nexusbridge/internal/fetcher"
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
