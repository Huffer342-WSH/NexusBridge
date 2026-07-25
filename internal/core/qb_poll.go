// qb_poll.go 负责由后端单一协调器维护 qB 实时状态。
package core

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"nexusbridge/internal/storage"
)

// PollQB 返回后端共享的 qB 实时状态；所有浏览器标签共用一个 qB RID。
func (a *App) PollQB(ctx context.Context, clientRevision int) (QBPollResult, error) {
	a.qbPollMu.Lock()
	defer a.qbPollMu.Unlock()

	a.qbConfigMu.RLock()
	syncIntervalSeconds := a.cfg.QBittorrent.SyncIntervalSeconds
	a.qbConfigMu.RUnlock()
	pollInterval := time.Duration(max(1, syncIntervalSeconds)) * time.Second
	if a.qbPollRevision == 0 || time.Since(a.qbPollLastAt) >= pollInterval {
		if err := a.refreshQBRuntime(ctx); err != nil {
			return QBPollResult{}, err
		}
	}
	result := QBPollResult{
		RID:       a.qbPollRevision,
		Connected: true,
		Updates:   []QBTorrentUpdate{},
	}
	if clientRevision == a.qbPollRevision && clientRevision != 0 {
		return result, nil
	}
	result.FullUpdate = true
	a.qbRuntimeMu.RLock()
	defer a.qbRuntimeMu.RUnlock()
	for _, record := range a.qbRuntime {
		result.Updates = append(result.Updates, qbUpdateFromSnapshot(record))
		if record.Added {
			result.Updated++
		} else {
			result.Removed++
		}
	}
	return result, nil
}

func (a *App) refreshQBRuntime(ctx context.Context) error {
	qb, err := a.qbClient(ctx)
	if err != nil {
		return err
	}
	mainData, err := qb.GetMainData(ctx, a.qbPollRID)
	if err != nil {
		return err
	}
	files, err := a.store.ListTorrentHashes(ctx)
	if err != nil {
		return err
	}
	keysByHash := make(map[string][]storage.TorrentKey, len(files))
	keys := make([]storage.TorrentKey, 0, len(files))
	currentKeys := make(map[storage.TorrentKey]struct{}, len(files))
	for _, file := range files {
		hash := strings.ToLower(strings.TrimSpace(file.InfoHashV1))
		if hash == "" {
			continue
		}
		key := storage.TorrentKey{SiteID: file.SiteID, TorrentID: file.TorrentID}
		keysByHash[hash] = append(keysByHash[hash], key)
		keys = append(keys, key)
		currentKeys[key] = struct{}{}
	}
	stable, err := a.store.ListQBSnapshots(ctx, keys)
	if err != nil {
		return err
	}
	now := time.Now()
	a.qbRuntimeMu.Lock()
	defer a.qbRuntimeMu.Unlock()
	for key := range a.qbRuntime {
		if _, ok := currentKeys[key]; !ok {
			delete(a.qbRuntime, key)
		}
	}
	seen := make(map[storage.TorrentKey]struct{}, len(keys))
	persist := make([]storage.QBSnapshotRecord, 0, len(mainData.Torrents)+len(mainData.TorrentsRemoved))
	for rawHash, patch := range mainData.Torrents {
		hash := strings.ToLower(strings.TrimSpace(rawHash))
		for _, key := range keysByHash[hash] {
			record, ok := a.qbRuntime[key]
			if !ok {
				record = stable[key]
			}
			previous := record
			record.SiteID, record.TorrentID = key.SiteID, key.TorrentID
			record.Added, record.QBHash, record.SyncedAt = true, hash, now
			if err := applyQBTorrentPatch(&record, patch); err != nil {
				return err
			}
			a.qbRuntime[key] = record
			seen[key] = struct{}{}
			if !stableQBAssociationEqual(previous, record) {
				persist = append(persist, record)
			}
		}
	}
	removedHashes := make(map[string]struct{}, len(mainData.TorrentsRemoved))
	for _, hash := range mainData.TorrentsRemoved {
		removedHashes[strings.ToLower(strings.TrimSpace(hash))] = struct{}{}
	}
	if mainData.FullUpdate {
		for hash, localKeys := range keysByHash {
			for _, key := range localKeys {
				if _, ok := seen[key]; !ok {
					removedHashes[hash] = struct{}{}
					break
				}
			}
		}
	}
	for hash := range removedHashes {
		for _, key := range keysByHash[hash] {
			if _, ok := seen[key]; ok {
				continue
			}
			record, ok := a.qbRuntime[key]
			if !ok {
				record = stable[key]
			}
			previous := record
			record.SiteID, record.TorrentID = key.SiteID, key.TorrentID
			record.Added, record.QBHash, record.SyncedAt = false, hash, now
			clearQBRuntimeFields(&record)
			a.qbRuntime[key] = record
			if !stableQBAssociationEqual(previous, record) {
				persist = append(persist, record)
			}
		}
	}
	if err := a.store.SaveQBSnapshots(ctx, persist); err != nil {
		return err
	}
	a.qbPollRID = mainData.RID
	a.qbPollLastAt = now
	if a.qbPollRevision == 0 || len(mainData.Torrents) > 0 || len(mainData.TorrentsRemoved) > 0 || mainData.FullUpdate {
		a.qbPollRevision++
	}
	return nil
}

func stableQBAssociationEqual(left, right storage.QBSnapshotRecord) bool {
	return left.SiteID == right.SiteID &&
		left.TorrentID == right.TorrentID &&
		left.Added == right.Added &&
		left.QBHash == right.QBHash &&
		left.Name == right.Name &&
		left.Category == right.Category &&
		slices.Equal(left.Tags, right.Tags) &&
		left.SavePath == right.SavePath &&
		left.ContentPath == right.ContentPath &&
		left.TotalSize == right.TotalSize &&
		left.Tracker == right.Tracker &&
		left.IsPrivate == right.IsPrivate &&
		left.AddedOn == right.AddedOn &&
		left.CompletionOn == right.CompletionOn &&
		left.CreationDate == right.CreationDate &&
		left.PieceSize == right.PieceSize &&
		left.Comment == right.Comment &&
		left.CreatedBy == right.CreatedBy
}

func clearQBRuntimeFields(record *storage.QBSnapshotRecord) {
	record.State = ""
	record.Progress = 0
	record.AmountLeft = 0
	record.Downloaded = 0
	record.Uploaded = 0
	record.DownloadSpeed = 0
	record.UploadSpeed = 0
	record.ETA = 0
	record.Ratio = 0
}

// applyQBTorrentPatch 将 qB 增量字段合并到内存状态。
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

func decodeQBField[T any](fields map[string]json.RawMessage, name string, target *T) bool {
	raw, ok := fields[name]
	if !ok || json.Unmarshal(raw, target) != nil {
		return false
	}
	return true
}

func qbUpdateFromSnapshot(record storage.QBSnapshotRecord) QBTorrentUpdate {
	return QBTorrentUpdate{SiteID: record.SiteID, TorrentID: record.TorrentID, QBStatus: qbStatusFromSnapshot(record)}
}
