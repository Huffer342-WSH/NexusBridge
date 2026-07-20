package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

// FetchTorrentDetail 抓取详情页并保存到种子记录。
func (a *App) FetchTorrentDetail(ctx context.Context, siteID, torrentID string) (Torrent, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return Torrent{}, err
	}
	if strings.TrimSpace(torrent.DetailURL) == "" {
		return Torrent{}, fmt.Errorf("torrent detail url is empty")
	}
	site, err := a.findSite(torrent.SiteID)
	if err != nil {
		return Torrent{}, err
	}
	headers := http.Header{}
	headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	headers.Set("Referer", site.BaseURL+"/torrents.php")
	slog.Info("torrent detail fetch started", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "url", torrent.DetailURL)
	result, err := a.fetchSiteResource(ctx, site, torrent.DetailURL, siteRequestOptions{
		Headers: headers, Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
	})
	if err != nil {
		return Torrent{}, err
	}
	detail, err := parser.ParseTorrentDetail(result.Body, parser.TorrentDetailParseOptions{
		SiteID:    torrent.SiteID,
		TorrentID: torrent.ID,
		BaseURL:   site.BaseURL,
		URL:       result.URL,
	})
	if err != nil {
		return Torrent{}, err
	}
	if err := a.store.UpdateTorrentDetail(ctx, storage.TorrentRecord{
		SiteID:            torrent.SiteID,
		TorrentID:         torrent.ID,
		DetailTitle:       detail.DetailTitle,
		Subtitle:          detail.Subtitle,
		ProductURL:        detail.ProductURL,
		DetailInfoHash:    detail.InfoHash,
		DetailDescription: detail.DetailDescription,
		DetailRawText:     detail.DetailRawText,
		DetailFetchedAt:   result.FetchedAt.Format(time.RFC3339Nano),
	}); err != nil {
		return Torrent{}, err
	}
	slog.Info("torrent detail fetch completed", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "bytes", len(result.Body))
	return a.getTorrent(ctx, torrent.SiteID, torrent.ID)
}
