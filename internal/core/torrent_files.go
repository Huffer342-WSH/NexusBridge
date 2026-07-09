// Package core 提供站点 torrent 文件下载、解析和持久化服务。
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

const torrentFileWorkers = 3

type torrentFileFetchResult struct {
	torrent Torrent
	data    []byte
	hashes  qbittorrent.TorrentHashes
	err     error
}

// ensureTorrentFiles 为尚未持久化文件的检索结果补齐 torrent BLOB 和 info hash。
func (a *App) ensureTorrentFiles(ctx context.Context, torrents []Torrent) (int, int, error) {
	keys := make([]storage.TorrentKey, 0, len(torrents))
	for _, torrent := range torrents {
		keys = append(keys, storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID})
	}
	metadata, err := a.store.ListTorrentFileMetadata(ctx, keys)
	if err != nil {
		return 0, 0, err
	}
	jobs := make(chan Torrent)
	results := make(chan torrentFileFetchResult)
	var workers sync.WaitGroup
	workerCount := min(torrentFileWorkers, len(torrents))
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for torrent := range jobs {
				data, hashes, err := a.downloadAndParseTorrentFile(ctx, torrent)
				results <- torrentFileFetchResult{torrent: torrent, data: data, hashes: hashes, err: err}
			}
		}()
	}
	go func() {
		defer close(results)
		for _, torrent := range torrents {
			key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
			file := metadata[key]
			if file.HasData && (file.InfoHashV1 != "" || file.InfoHashV2 != "") {
				continue
			}
			jobs <- torrent
		}
		close(jobs)
		workers.Wait()
	}()

	saved, failed := 0, 0
	var persistErr error
	for result := range results {
		key := storage.TorrentKey{SiteID: result.torrent.SiteID, TorrentID: result.torrent.ID}
		if result.err != nil {
			failed++
			if err := a.store.SaveTorrentFileError(ctx, key, result.err.Error()); err != nil && persistErr == nil {
				persistErr = err
			}
			slog.Warn("torrent file fetch failed", "site_id", key.SiteID, "torrent_id", key.TorrentID, "error", result.err)
			continue
		}
		if err := a.store.SaveTorrentFile(ctx, storage.TorrentFileRecord{
			SiteID: result.torrent.SiteID, TorrentID: result.torrent.ID, Data: result.data,
			InfoHashV1: result.hashes.V1, InfoHashV2: result.hashes.V2, FetchedAt: time.Now(),
		}); err != nil {
			failed++
			if persistErr == nil {
				persistErr = err
			}
			continue
		}
		saved++
	}
	return saved, failed, persistErr
}

// loadOrFetchTorrentFile 优先读取数据库 BLOB，缺失时下载、解析并保存。
func (a *App) loadOrFetchTorrentFile(ctx context.Context, torrent Torrent) ([]byte, qbittorrent.TorrentHashes, error) {
	key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
	stored, ok, err := a.store.GetTorrentFile(ctx, key)
	if err != nil {
		return nil, qbittorrent.TorrentHashes{}, err
	}
	if ok && stored.HasData && (stored.InfoHashV1 != "" || stored.InfoHashV2 != "") {
		return stored.Data, qbittorrent.TorrentHashes{V1: stored.InfoHashV1, V2: stored.InfoHashV2}, nil
	}
	data, hashes, err := a.downloadAndParseTorrentFile(ctx, torrent)
	if err != nil {
		_ = a.store.SaveTorrentFileError(ctx, key, err.Error())
		return nil, qbittorrent.TorrentHashes{}, err
	}
	if err := a.store.SaveTorrentFile(ctx, storage.TorrentFileRecord{
		SiteID: torrent.SiteID, TorrentID: torrent.ID, Data: data,
		InfoHashV1: hashes.V1, InfoHashV2: hashes.V2, FetchedAt: time.Now(),
	}); err != nil {
		return nil, qbittorrent.TorrentHashes{}, err
	}
	return data, hashes, nil
}

// downloadAndParseTorrentFile 下载 torrent 文件并解析原始 info hash。
func (a *App) downloadAndParseTorrentFile(ctx context.Context, torrent Torrent) ([]byte, qbittorrent.TorrentHashes, error) {
	data, err := a.downloadTorrentFile(ctx, torrent)
	if err != nil {
		return nil, qbittorrent.TorrentHashes{}, err
	}
	hashes, err := qbittorrent.ParseTorrentHashes(data)
	if err != nil {
		return nil, qbittorrent.TorrentHashes{}, fmt.Errorf("parse downloaded torrent: %w", err)
	}
	return data, hashes, nil
}

// downloadTorrentFile 使用站点 cookie 下载 torrent 原始文件。
func (a *App) downloadTorrentFile(ctx context.Context, torrent Torrent) ([]byte, error) {
	if strings.TrimSpace(torrent.DownloadURL) == "" {
		return nil, fmt.Errorf("torrent download url is empty")
	}
	site, err := a.findSite(torrent.SiteID)
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	headers.Set("Accept", "application/x-bittorrent,application/octet-stream;q=0.9,*/*;q=0.8")
	headers.Set("Referer", firstNonEmpty(torrent.DetailURL, site.BaseURL+"/torrents.php"))
	slog.Info("torrent file fetch started", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "url", torrent.DownloadURL)
	result, err := a.fetchSiteResource(ctx, site, torrent.DownloadURL, siteRequestOptions{
		Headers: headers, Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
	})
	if err != nil {
		return nil, err
	}
	if err := validateTorrentData(result.Body); err != nil {
		return nil, fmt.Errorf("downloaded torrent payload is invalid: %w", err)
	}
	slog.Info("torrent file fetch completed", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "bytes", len(result.Body))
	return result.Body, nil
}

// validateTorrentData 做最小 torrent bencode 载荷校验。
func validateTorrentData(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty response")
	}
	trimmed := strings.TrimSpace(string(data[:min(len(data), 64)]))
	if strings.HasPrefix(trimmed, "<") || strings.HasPrefix(strings.ToLower(trimmed), "<!doctype") {
		return fmt.Errorf("response looks like html")
	}
	if data[0] != 'd' {
		return fmt.Errorf("response does not start with bencoded dictionary")
	}
	return nil
}
