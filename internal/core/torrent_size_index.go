// Package core 中的本文件负责恢复用 torrent 文件大小签名索引。
package core

import (
	"context"
	"fmt"
	"strings"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

// GetTorrentSizeIndexStatus 返回当前大小索引覆盖情况。
func (a *App) GetTorrentSizeIndexStatus(ctx context.Context) (TorrentSizeIndexStatus, error) {
	record, err := a.store.GetTorrentSizeIndexStatus(ctx)
	if err != nil {
		return TorrentSizeIndexStatus{}, err
	}
	return torrentSizeIndexStatusFromRecord(record), nil
}

// RebuildTorrentSizeIndex 手动解析全部已保存 torrent 文件并重建大小签名。
func (a *App) RebuildTorrentSizeIndex(ctx context.Context) (TorrentSizeIndexStatus, error) {
	keys, err := a.store.ListPersistedTorrentFileKeys(ctx)
	if err != nil {
		return TorrentSizeIndexStatus{}, err
	}
	processed := 0
	errors := make([]string, 0)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return TorrentSizeIndexStatus{}, err
		}
		file, ok, err := a.store.GetTorrentFile(ctx, key)
		if err != nil {
			return TorrentSizeIndexStatus{}, err
		}
		if !ok || !file.HasData {
			continue
		}
		metadata, parseErr := qbittorrent.ParseTorrentMetadata(file.Data)
		if parseErr != nil {
			message := fmt.Sprintf("%s/%s: %v", key.SiteID, key.TorrentID, parseErr)
			applied, err := a.store.ReplaceTorrentSizeIndex(ctx, key, file.Data, nil, message)
			if err != nil {
				return TorrentSizeIndexStatus{}, err
			}
			if applied {
				errors = append(errors, message)
			}
			continue
		}
		applied, err := a.store.ReplaceTorrentSizeIndex(ctx, key, file.Data, torrentMetadataSizeCounts(metadata), "")
		if err != nil {
			return TorrentSizeIndexStatus{}, err
		}
		if applied {
			processed++
		}
	}
	status, err := a.GetTorrentSizeIndexStatus(ctx)
	if err != nil {
		return TorrentSizeIndexStatus{}, err
	}
	status.Processed = processed
	if len(errors) > 0 {
		const maxReportedErrors = 5
		shown := errors
		if len(shown) > maxReportedErrors {
			shown = shown[:maxReportedErrors]
		}
		status.LastError = strings.Join(shown, "; ")
		if len(errors) > len(shown) {
			status.LastError += fmt.Sprintf("; and %d more", len(errors)-len(shown))
		}
	}
	return status, nil
}

func torrentSizeIndexStatusFromRecord(record storage.TorrentSizeIndexStatusRecord) TorrentSizeIndexStatus {
	return TorrentSizeIndexStatus{
		Version: record.Version, Total: record.Total, Indexed: record.Indexed, Pending: record.Pending, Failed: record.Failed,
	}
}
