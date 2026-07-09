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
	torrent  Torrent
	data     []byte
	metadata qbittorrent.TorrentMetadata
	err      error
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
				data, metadata, err := a.downloadAndParseTorrentFileMetadata(ctx, torrent)
				results <- torrentFileFetchResult{torrent: torrent, data: data, metadata: metadata, err: err}
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
			InfoHashV1: result.metadata.Hashes.V1, InfoHashV2: result.metadata.Hashes.V2,
			OriginalName: result.metadata.Name, FetchedAt: time.Now(), SizeCounts: torrentMetadataSizeCounts(result.metadata),
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
	data, metadata, err := a.loadOrFetchTorrentMetadata(ctx, torrent)
	return data, metadata.Hashes, err
}

// loadOrFetchTorrentMetadata 优先读取数据库 BLOB，并返回 hash 与原始名称。
func (a *App) loadOrFetchTorrentMetadata(ctx context.Context, torrent Torrent) ([]byte, qbittorrent.TorrentMetadata, error) {
	key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
	stored, ok, err := a.store.GetTorrentFile(ctx, key)
	if err != nil {
		return nil, qbittorrent.TorrentMetadata{}, err
	}
	if ok && stored.HasData && (stored.InfoHashV1 != "" || stored.InfoHashV2 != "") {
		return stored.Data, qbittorrent.TorrentMetadata{
			Hashes: qbittorrent.TorrentHashes{V1: stored.InfoHashV1, V2: stored.InfoHashV2},
			Name:   stored.OriginalName,
		}, nil
	}
	data, metadata, err := a.downloadAndParseTorrentFileMetadata(ctx, torrent)
	if err != nil {
		_ = a.store.SaveTorrentFileError(ctx, key, err.Error())
		return nil, qbittorrent.TorrentMetadata{}, err
	}
	if err := a.store.SaveTorrentFile(ctx, storage.TorrentFileRecord{
		SiteID: torrent.SiteID, TorrentID: torrent.ID, Data: data,
		InfoHashV1: metadata.Hashes.V1, InfoHashV2: metadata.Hashes.V2,
		OriginalName: metadata.Name, FetchedAt: time.Now(), SizeCounts: torrentMetadataSizeCounts(metadata),
	}); err != nil {
		return nil, qbittorrent.TorrentMetadata{}, err
	}
	return data, metadata, nil
}

// torrentMetadataSizeCounts 汇总非 padding torrent 文件大小及重复次数。
func torrentMetadataSizeCounts(metadata qbittorrent.TorrentMetadata) map[int64]int {
	result := map[int64]int{}
	for _, file := range metadata.Files {
		if file.Padding {
			continue
		}
		result[file.Size]++
	}
	return result
}

// downloadAndParseTorrentFile 下载 torrent 文件并解析原始 info hash。
func (a *App) downloadAndParseTorrentFile(ctx context.Context, torrent Torrent) ([]byte, qbittorrent.TorrentHashes, error) {
	data, metadata, err := a.downloadAndParseTorrentFileMetadata(ctx, torrent)
	return data, metadata.Hashes, err
}

// downloadAndParseTorrentFileMetadata 下载并解析 torrent 完整元数据。
func (a *App) downloadAndParseTorrentFileMetadata(ctx context.Context, torrent Torrent) ([]byte, qbittorrent.TorrentMetadata, error) {
	data, err := a.downloadTorrentFile(ctx, torrent)
	if err != nil {
		return nil, qbittorrent.TorrentMetadata{}, err
	}
	metadata, err := qbittorrent.ParseTorrentMetadata(data)
	if err != nil {
		return nil, qbittorrent.TorrentMetadata{}, fmt.Errorf("parse downloaded torrent: %w", err)
	}
	return data, metadata, nil
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
