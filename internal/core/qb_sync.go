// Package core 提供基于 info hash 的 qBittorrent 状态与稳定关联同步服务。
package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

// syncTorrentQBSnapshots 按 v1 info hash 匹配实时状态并批量保存稳定关联。
func (a *App) syncTorrentQBSnapshots(ctx context.Context, qb *qbittorrent.Client, qbTorrents []qbittorrent.TorrentInfo) (int, int, int, int, error) {
	files, err := a.store.ListTorrentHashes(ctx)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	keysByHash := make(map[string][]storage.TorrentKey)
	for _, file := range files {
		if hash := strings.ToLower(strings.TrimSpace(file.InfoHashV1)); hash != "" {
			keysByHash[hash] = append(keysByHash[hash], storage.TorrentKey{SiteID: file.SiteID, TorrentID: file.TorrentID})
		}
	}
	syncedAt := time.Now()
	snapshots := make([]storage.QBSnapshotRecord, 0)
	matchedKeys := make(map[storage.TorrentKey]struct{})
	matched, detailFailed := 0, 0
	for _, torrent := range qbTorrents {
		keys := keysByHash[strings.ToLower(strings.TrimSpace(torrent.Hash))]
		if len(keys) == 0 {
			continue
		}
		properties, propErr := qb.GetTorrentProperties(ctx, torrent.Hash)
		if propErr != nil {
			detailFailed++
		}
		for _, key := range keys {
			snapshots = append(snapshots, qbSnapshotFromTorrent(key, torrent, properties, propErr == nil, syncedAt))
			matchedKeys[key] = struct{}{}
			matched++
		}
	}
	for _, file := range files {
		if strings.TrimSpace(file.InfoHashV1) == "" {
			continue
		}
		key := storage.TorrentKey{SiteID: file.SiteID, TorrentID: file.TorrentID}
		if _, ok := matchedKeys[key]; ok {
			continue
		}
		snapshots = append(snapshots, storage.QBSnapshotRecord{
			SiteID: key.SiteID, TorrentID: key.TorrentID, Added: false,
			QBHash: file.InfoHashV1, State: "missing", SyncedAt: syncedAt,
		})
	}
	removed, err := a.store.ReplaceQBSnapshots(ctx, snapshots)
	if err != nil {
		return matched, 0, 0, detailFailed, err
	}
	a.qbRuntimeMu.Lock()
	for _, snapshot := range snapshots {
		a.qbRuntime[storage.TorrentKey{SiteID: snapshot.SiteID, TorrentID: snapshot.TorrentID}] = snapshot
	}
	a.qbRuntimeMu.Unlock()
	return matched, len(snapshots), removed, detailFailed, nil
}

// refreshTorrentQBSnapshot 实时查询单个种子，并仅在稳定关联变化时写数据库。
func (a *App) refreshTorrentQBSnapshot(ctx context.Context, torrent Torrent, weakMatch bool) (QBTorrentStatus, error) {
	localHash, err := a.torrentQBHash(ctx, torrent)
	if err != nil {
		return QBTorrentStatus{}, err
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return QBTorrentStatus{}, err
	}
	var matches []qbittorrent.TorrentInfo
	if strings.TrimSpace(localHash) != "" {
		matches, err = qb.ListTorrentsWithOptions(ctx, qbittorrent.TorrentListOptions{Hashes: []string{localHash}})
	} else if weakMatch {
		matches, err = qb.ListTorrents(ctx)
		if err == nil {
			matches = filterQBTorrentsByName(matches, torrent.Title)
		}
	} else {
		return QBTorrentStatus{Available: true, Added: false, Source: "no_hash", FetchedAt: time.Now()}, nil
	}
	if err != nil {
		return QBTorrentStatus{}, err
	}
	key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
	if len(matches) == 0 {
		snapshot := storage.QBSnapshotRecord{SiteID: key.SiteID, TorrentID: key.TorrentID, Added: false, State: "missing", SyncedAt: time.Now()}
		snapshot.QBHash = localHash
		if err := a.store.SaveQBSnapshot(ctx, snapshot); err != nil {
			return QBTorrentStatus{}, err
		}
		a.qbRuntimeMu.Lock()
		a.qbRuntime[key] = snapshot
		a.qbRuntimeMu.Unlock()
		return qbStatusFromSnapshot(snapshot), nil
	}
	matched := matches[0]
	properties, propErr := qb.GetTorrentProperties(ctx, matched.Hash)
	snapshot := qbSnapshotFromTorrent(key, matched, properties, propErr == nil, time.Now())
	if err := a.store.SaveQBSnapshot(ctx, snapshot); err != nil {
		return QBTorrentStatus{}, err
	}
	a.qbRuntimeMu.Lock()
	a.qbRuntime[key] = snapshot
	a.qbRuntimeMu.Unlock()
	return qbStatusFromSnapshot(snapshot), nil
}

// ControlTorrentQB 按本地 info hash 暂停或恢复单个 qB 任务并刷新快照。
func (a *App) ControlTorrentQB(ctx context.Context, siteID, torrentID, action string) (QBTorrentStatus, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return QBTorrentStatus{}, err
	}
	hash, err := a.torrentQBHash(ctx, torrent)
	if err != nil {
		return QBTorrentStatus{}, err
	}
	if strings.TrimSpace(hash) == "" {
		return QBTorrentStatus{}, fmt.Errorf("torrent has no qB hash")
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return QBTorrentStatus{}, err
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "stop":
		err = qb.StopTorrents(ctx, []string{hash})
	case "start":
		err = qb.StartTorrents(ctx, []string{hash})
	default:
		return QBTorrentStatus{}, fmt.Errorf("unsupported qB action %q", action)
	}
	if err != nil {
		return QBTorrentStatus{}, err
	}
	return a.refreshTorrentQBSnapshot(ctx, torrent, false)
}

// torrentQBHash 优先读取 torrent 文件 v1 hash，并兼容已有下载任务 hash。
func (a *App) torrentQBHash(ctx context.Context, torrent Torrent) (string, error) {
	key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
	files, err := a.store.ListTorrentFileMetadata(ctx, []storage.TorrentKey{key})
	if err != nil {
		return "", err
	}
	if file, ok := files[key]; ok && strings.TrimSpace(file.InfoHashV1) != "" {
		return file.InfoHashV1, nil
	}
	tasks, err := a.store.ListDownloadTasksByTorrents(ctx, []storage.TorrentKey{key})
	if err != nil {
		return "", err
	}
	for _, task := range tasks[torrentKey(torrent)] {
		if strings.TrimSpace(task.QBHash) != "" {
			return task.QBHash, nil
		}
	}
	return "", nil
}

// qbSnapshotFromTorrent 将 qB 列表和属性响应合并为运行时记录。
func qbSnapshotFromTorrent(key storage.TorrentKey, torrent qbittorrent.TorrentInfo, properties qbittorrent.TorrentProperties, hasProperties bool, syncedAt time.Time) storage.QBSnapshotRecord {
	totalSize := torrent.TotalSize
	if totalSize == 0 {
		totalSize = torrent.Size
	}
	record := storage.QBSnapshotRecord{
		SiteID: key.SiteID, TorrentID: key.TorrentID, Added: true, QBHash: torrent.Hash,
		Name: torrent.Name, State: torrent.State, Progress: torrent.Progress, Category: torrent.Category,
		Tags: splitQBTags(torrent.Tags), SavePath: torrent.SavePath, ContentPath: torrent.ContentPath,
		TotalSize: totalSize, AmountLeft: torrent.AmountLeft, Downloaded: torrent.Downloaded,
		Uploaded: torrent.Uploaded, DownloadSpeed: torrent.DownloadSpeed, UploadSpeed: torrent.UploadSpeed,
		ETA: torrent.ETA, Ratio: torrent.Ratio, Tracker: torrent.Tracker, IsPrivate: torrent.IsPrivate,
		AddedOn: torrent.AddedOn, CompletionOn: torrent.CompletionOn, SyncedAt: syncedAt,
	}
	if hasProperties {
		record.SavePath = firstNonEmpty(properties.SavePath, record.SavePath)
		record.CreationDate = properties.CreationDate
		record.PieceSize = properties.PieceSize
		record.Comment = properties.Comment
		record.CreatedBy = properties.CreatedBy
		if record.TotalSize == 0 {
			record.TotalSize = properties.TotalSize
		}
	}
	return record
}

// qbStatusFromSnapshot 将运行时或稳定关联记录转换为 API 状态模型。
func qbStatusFromSnapshot(snapshot storage.QBSnapshotRecord) QBTorrentStatus {
	completed := snapshot.TotalSize - snapshot.AmountLeft
	if completed < 0 {
		completed = 0
	}
	return QBTorrentStatus{
		Available: true, Added: snapshot.Added, Source: "hash", FetchedAt: snapshot.SyncedAt,
		Hash: snapshot.QBHash, Name: snapshot.Name, State: snapshot.State, Progress: snapshot.Progress,
		Category: snapshot.Category, Tags: strings.Join(snapshot.Tags, ","), SavePath: snapshot.SavePath,
		ContentPath: snapshot.ContentPath, DownloadSpeed: snapshot.DownloadSpeed, UploadSpeed: snapshot.UploadSpeed,
		ETA: snapshot.ETA, Ratio: snapshot.Ratio, Size: snapshot.TotalSize,
		Completed: completed, AmountLeft: snapshot.AmountLeft,
	}
}

// filterQBTorrentsByName 仅为显式弱匹配筛选同名 qB 任务。
func filterQBTorrentsByName(torrents []qbittorrent.TorrentInfo, title string) []qbittorrent.TorrentInfo {
	title = normalizeQBName(title)
	if title == "" {
		return nil
	}
	for _, torrent := range torrents {
		if normalizeQBName(torrent.Name) == title {
			return []qbittorrent.TorrentInfo{torrent}
		}
	}
	return nil
}

// splitQBTags 解析 qB WebAPI 返回的逗号分隔标签。
func splitQBTags(raw string) []string {
	var tags []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			tags = append(tags, value)
		}
	}
	return tags
}
