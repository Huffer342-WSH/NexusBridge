// covers.go 负责通过应用凭据代理站点封面图片。
package core

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const maxCoverImageBytes = 10 << 20

// CoverImage 表示可直接写入 HTTP 响应的封面图片。
type CoverImage struct {
	Data        []byte
	ContentType string
}

// FetchTorrentCover 使用站点 cookie 获取封面，解决 WebP 等受鉴权或防盗链限制的图片无法直连问题。
func (a *App) FetchTorrentCover(ctx context.Context, siteID, torrentID string) (CoverImage, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return CoverImage{}, err
	}
	if strings.TrimSpace(torrent.CoverURL) == "" {
		return CoverImage{}, fmt.Errorf("torrent cover url is empty")
	}
	site, err := a.findSite(torrent.SiteID)
	if err != nil {
		return CoverImage{}, err
	}
	headers := http.Header{}
	headers.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	headers.Set("Referer", firstNonEmpty(torrent.DetailURL, site.BaseURL+"/torrents.php"))
	result, err := a.fetchSiteResource(ctx, site, torrent.CoverURL, siteRequestOptions{
		Headers: headers, Timeout: defaultSiteRequestTimeout,
	})
	if err != nil {
		return CoverImage{}, err
	}
	if len(result.Body) == 0 || len(result.Body) > maxCoverImageBytes {
		return CoverImage{}, fmt.Errorf("invalid cover image size: %d", len(result.Body))
	}
	contentType := http.DetectContentType(result.Body)
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return CoverImage{}, fmt.Errorf("unexpected cover content type: %s", contentType)
	}
	return CoverImage{Data: result.Body, ContentType: contentType}, nil
}
