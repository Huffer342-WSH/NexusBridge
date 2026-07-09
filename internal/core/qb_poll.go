// qb_poll.go 负责使用 qB sync/maindata 增量刷新本地状态快照。
package core

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

// PollQB 获取 qB 增量数据、按 hash 更新数据库快照并返回前端更新项。
func (a *App) PollQB(ctx context.Context, rid int) (QBPollResult, error) {
	qb, err := a.qbClient(ctx)
	if err != nil {
		return QBPollResult{}, err
	}
	mainData, err := qb.GetMainData(ctx, rid)
	if err != nil {
		return QBPollResult{}, err
	}
	files, err := a.store.ListTorrentHashes(ctx)
	if err != nil {
		return QBPollResult{}, err
	}
	keysByHash := map[string][]storage.TorrentKey{}
	keys := make([]storage.TorrentKey, 0, len(files))
	for _, file := range files {
		hash := strings.ToLower(strings.TrimSpace(file.InfoHashV1))
		if hash == "" {
			continue
		}
		key := storage.TorrentKey{SiteID: file.SiteID, TorrentID: file.TorrentID}
		keysByHash[hash] = append(keysByHash[hash], key)
		keys = append(keys, key)
	}
	existing, err := a.store.ListQBSnapshots(ctx, keys)
	if err != nil {
		return QBPollResult{}, err
	}
	result := QBPollResult{RID: mainData.RID, FullUpdate: mainData.FullUpdate, Connected: true, Updates: []QBTorrentUpdate{}}
	seen := map[storage.TorrentKey]struct{}{}
	syncedAt := time.Now()
	for rawHash, patch := range mainData.Torrents {
		hash := strings.ToLower(strings.TrimSpace(rawHash))
		for _, key := range keysByHash[hash] {
			record := existing[key]
			record.SiteID, record.TorrentID = key.SiteID, key.TorrentID
			record.Added, record.QBHash, record.SyncedAt = true, hash, syncedAt
			if mainData.FullUpdate {
				var torrent qbittorrent.TorrentInfo
				if err := json.Unmarshal(patch, &torrent); err != nil {
					return result, err
				}
				torrent.Hash = hash
				record = qbSnapshotFromTorrent(key, torrent, qbittorrent.TorrentProperties{}, false, syncedAt)
			} else if err := applyQBTorrentPatch(&record, patch); err != nil {
				return result, err
			}
			if err := a.store.SaveQBSnapshot(ctx, record); err != nil {
				return result, err
			}
			seen[key] = struct{}{}
			result.Updates = append(result.Updates, qbUpdateFromSnapshot(record))
			result.Updated++
		}
	}
	removedHashes := map[string]struct{}{}
	for _, hash := range mainData.TorrentsRemoved {
		removedHashes[strings.ToLower(strings.TrimSpace(hash))] = struct{}{}
	}
	if mainData.FullUpdate {
		for hash, localKeys := range keysByHash {
			present := false
			for _, key := range localKeys {
				if _, ok := seen[key]; ok {
					present = true
					break
				}
			}
			if present {
				continue
			}
			for _, key := range localKeys {
				if existing[key].Added {
					removedHashes[hash] = struct{}{}
				}
			}
		}
	}
	for hash := range removedHashes {
		for _, key := range keysByHash[hash] {
			if _, updated := seen[key]; updated {
				continue
			}
			record := existing[key]
			record.SiteID, record.TorrentID = key.SiteID, key.TorrentID
			record.Added, record.QBHash, record.State, record.SyncedAt = false, hash, "missing", syncedAt
			if err := a.store.SaveQBSnapshot(ctx, record); err != nil {
				return result, err
			}
			result.Updates = append(result.Updates, qbUpdateFromSnapshot(record))
			result.Removed++
		}
	}
	return result, nil
}

// applyQBTorrentPatch 将 qB 增量字段合并到已有数据库快照。
func applyQBTorrentPatch(record *storage.QBSnapshotRecord, data json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	decodeQBField(fields, "name", &record.Name)
	decodeQBField(fields, "state", &record.State)
	decodeQBField(fields, "progress", &record.Progress)
	decodeQBField(fields, "category", &record.Category)
	var tags string
	if decodeQBField(fields, "tags", &tags) {
		record.Tags = splitQBTags(tags)
	}
	decodeQBField(fields, "save_path", &record.SavePath)
	decodeQBField(fields, "content_path", &record.ContentPath)
	var size int64
	if decodeQBField(fields, "size", &size) {
		record.TotalSize = size
	}
	decodeQBField(fields, "total_size", &record.TotalSize)
	decodeQBField(fields, "amount_left", &record.AmountLeft)
	var completed int64
	if decodeQBField(fields, "completed", &completed) && record.TotalSize >= completed {
		record.AmountLeft = record.TotalSize - completed
	}
	decodeQBField(fields, "downloaded", &record.Downloaded)
	decodeQBField(fields, "uploaded", &record.Uploaded)
	decodeQBField(fields, "dlspeed", &record.DownloadSpeed)
	decodeQBField(fields, "upspeed", &record.UploadSpeed)
	decodeQBField(fields, "eta", &record.ETA)
	decodeQBField(fields, "ratio", &record.Ratio)
	decodeQBField(fields, "tracker", &record.Tracker)
	decodeQBField(fields, "isPrivate", &record.IsPrivate)
	decodeQBField(fields, "added_on", &record.AddedOn)
	decodeQBField(fields, "completion_on", &record.CompletionOn)
	return nil
}

// decodeQBField 仅在增量响应包含字段时覆盖目标值。
func decodeQBField[T any](fields map[string]json.RawMessage, name string, target *T) bool {
	raw, ok := fields[name]
	if !ok || json.Unmarshal(raw, target) != nil {
		return false
	}
	return true
}

// qbUpdateFromSnapshot 生成前端可直接合并的单种子状态更新。
func qbUpdateFromSnapshot(record storage.QBSnapshotRecord) QBTorrentUpdate {
	return QBTorrentUpdate{SiteID: record.SiteID, TorrentID: record.TorrentID, QBStatus: qbStatusFromSnapshot(record)}
}
