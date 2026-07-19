// covers.go 负责通过应用凭据代理站点封面图片。
package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"nexusbridge/internal/core/covercache"
)

const maxCoverImageBytes = 10 << 20

// CoverImage 表示可直接写入 HTTP 响应的封面图片。
type CoverImage struct {
	Path        string
	ContentType string
	SHA256      string
	FileSize    int64
	ModTime     time.Time
	FromCache   bool
}

// FetchTorrentCover 返回本地持久缓存中的封面，缓存未命中时同步下载。
func (a *App) FetchTorrentCover(ctx context.Context, siteID, torrentID string) (CoverImage, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return CoverImage{}, err
	}
	if strings.TrimSpace(torrent.CoverURL) == "" {
		return CoverImage{}, fmt.Errorf("torrent cover url is empty")
	}
	result, err := a.coverCache.GetCover(ctx, covercache.Source{
		SiteID: torrent.SiteID, TorrentID: torrent.ID, SourceURL: torrent.CoverURL,
	})
	if err != nil {
		return CoverImage{}, err
	}
	return CoverImage{
		Path: result.Path, ContentType: result.MIMEType, SHA256: result.SHA256,
		FileSize: result.FileSize, ModTime: result.ModTime, FromCache: result.FromCache,
	}, nil
}

type coverDownloader struct {
	app *App
}

// DownloadCover 使用站点 Cookie、User-Agent 和 Referer 下载并校验远程封面。
func (d coverDownloader) DownloadCover(ctx context.Context, source covercache.Source) (covercache.Download, error) {
	site, err := d.app.findSite(source.SiteID)
	if err != nil {
		return covercache.Download{}, err
	}
	headers, err := coverRequestHeaders(site.BaseURL, source.SourceURL)
	if err != nil {
		return covercache.Download{}, err
	}
	result, err := d.app.fetchSiteResource(ctx, site, source.SourceURL, siteRequestOptions{
		Headers: headers, Timeout: defaultSiteRequestTimeout,
	})
	if err != nil {
		return covercache.Download{}, err
	}
	if len(result.Body) == 0 || len(result.Body) > maxCoverImageBytes {
		return covercache.Download{}, fmt.Errorf("invalid cover image size: %d", len(result.Body))
	}
	contentType := http.DetectContentType(result.Body)
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return covercache.Download{}, fmt.Errorf("unexpected cover content type: %s", contentType)
	}
	return covercache.Download{Data: result.Body, MIMEType: contentType}, nil
}

// coverRequestHeaders 根据站点页面与封面地址的关系构造浏览器图片请求头。
func coverRequestHeaders(siteBaseURL, sourceURL string) (http.Header, error) {
	siteURL, err := url.Parse(strings.TrimSpace(siteBaseURL))
	if err != nil || siteURL.Scheme == "" || siteURL.Host == "" {
		return nil, fmt.Errorf("invalid site base url")
	}
	coverURL, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || coverURL.Scheme == "" || coverURL.Host == "" {
		return nil, fmt.Errorf("invalid cover url")
	}
	fetchSite := coverFetchSite(siteURL, coverURL)
	referer := strings.TrimRight(siteBaseURL, "/") + "/"
	priority := "i"
	if fetchSite == "same-origin" {
		referer = strings.TrimRight(siteBaseURL, "/") + "/torrents.php"
	} else if fetchSite == "same-site" {
		priority = "u=1, i"
	}

	headers := http.Header{}
	headers.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	headers.Set("Priority", priority)
	headers.Set("Referer", referer)
	headers.Set("Sec-Fetch-Dest", "image")
	headers.Set("Sec-Fetch-Mode", "no-cors")
	headers.Set("Sec-Fetch-Site", fetchSite)
	if fetchSite == "cross-site" {
		headers.Set("Sec-Fetch-Storage-Access", "active")
	} else {
		headers["Sec-Fetch-Storage-Access"] = nil
	}
	for _, name := range []string{"Sec-Fetch-User", "Upgrade-Insecure-Requests"} {
		headers[name] = nil
	}
	return headers, nil
}

// coverFetchSite 返回浏览器 Sec-Fetch-Site 使用的来源关系。
func coverFetchSite(siteURL, coverURL *url.URL) string {
	if sameCoverOrigin(siteURL, coverURL) {
		return "same-origin"
	}
	if !strings.EqualFold(siteURL.Scheme, coverURL.Scheme) {
		return "cross-site"
	}
	siteHost := strings.TrimSuffix(strings.ToLower(siteURL.Hostname()), ".")
	coverHost := strings.TrimSuffix(strings.ToLower(coverURL.Hostname()), ".")
	if siteHost == coverHost {
		return "same-site"
	}
	if net.ParseIP(siteHost) != nil || net.ParseIP(coverHost) != nil {
		return "cross-site"
	}
	siteDomain, siteErr := publicsuffix.EffectiveTLDPlusOne(siteHost)
	coverDomain, coverErr := publicsuffix.EffectiveTLDPlusOne(coverHost)
	if siteErr == nil && coverErr == nil && strings.EqualFold(siteDomain, coverDomain) {
		return "same-site"
	}
	return "cross-site"
}

// sameCoverOrigin 判断两个 URL 是否具有相同协议、主机和有效端口。
func sameCoverOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(strings.TrimSuffix(left.Hostname(), "."), strings.TrimSuffix(right.Hostname(), ".")) &&
		coverURLPort(left) == coverURLPort(right)
}

// coverURLPort 返回 URL 的显式端口或协议默认端口。
func coverURLPort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
