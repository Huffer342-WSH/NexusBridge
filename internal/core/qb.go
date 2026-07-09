// Package core 提供 qBittorrent 配置、发送和状态协调服务。
package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

// GetTorrentQBStatus 实时查询本地种子对应的 qBittorrent 状态。
func (a *App) GetTorrentQBStatus(ctx context.Context, siteID, torrentID string, weakMatch bool) (QBTorrentStatus, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return QBTorrentStatus{}, err
	}
	return a.refreshTorrentQBSnapshot(ctx, torrent, weakMatch)
}

// SyncQB 同步 qBittorrent 已完成任务。
func (a *App) SyncQB(ctx context.Context) (QBSyncResult, error) {
	qb, err := a.qbClient(ctx)
	if err != nil {
		return QBSyncResult{}, err
	}
	qbTorrents, err := qb.ListTorrents(ctx)
	if err != nil {
		return QBSyncResult{}, err
	}
	matched, updated, removed, detailFailed, err := a.syncTorrentQBSnapshots(ctx, qb, qbTorrents)
	if err != nil {
		return QBSyncResult{}, err
	}
	tasks, err := a.store.ListDownloadTasks(ctx)
	if err != nil {
		return QBSyncResult{}, err
	}
	result := QBSyncResult{TorrentMatched: matched, TorrentUpdated: updated, TorrentRemoved: removed, DetailFailed: detailFailed}
	for _, task := range tasks {
		if task.Status != "sent" && task.Status != "pending" {
			continue
		}
		result.Checked++
		qbTorrent, matched := matchDownloadTaskToQB(task, qbTorrents)
		if !matched {
			result.Missing++
			continue
		}
		if task.QBHash == "" && qbTorrent.Hash != "" {
			if err := a.store.UpdateDownloadTaskHash(ctx, task.ID, qbTorrent.Hash); err != nil {
				return result, err
			}
			result.Linked++
		}
		if qbittorrent.IsCompleted(qbTorrent) {
			if err := a.store.UpdateDownloadTask(ctx, task.ID, "completed", qbTorrent.Hash, qbTorrent.ContentPath, ""); err != nil {
				return result, err
			}
			result.Completed++
			created, err := a.store.CreateOrganizeTaskIfAbsent(ctx, storage.OrganizeTaskRecord{
				ID:             "org:" + task.ID,
				DownloadTaskID: task.ID,
				Status:         "pending",
				Title:          task.TorrentTitle,
				SourcePath:     qbTorrent.ContentPath,
			})
			if err != nil {
				return result, err
			}
			if created {
				result.OrganizeCreated++
			}
		}
	}
	return result, nil
}

// GetQBittorrentConfig 读取 qBittorrent 配置。
func (a *App) GetQBittorrentConfig(ctx context.Context) config.QBittorrentConfig {
	return a.effectiveQBConfig(ctx)
}

// SaveQBittorrentConfig 保存 qBittorrent 配置。
func (a *App) SaveQBittorrentConfig(ctx context.Context, cfg config.QBittorrentConfig) (config.QBittorrentConfig, error) {
	cfg = config.NormalizeQBittorrentConfig(cfg)
	if cfg.AuthMode == "" {
		cfg.AuthMode = "uid"
	}
	if cfg.Username == "" && cfg.UserID != "" {
		cfg.Username = cfg.UserID
	}
	if cfg.UserID == "" && cfg.Username != "" {
		cfg.UserID = cfg.Username
	}
	if err := a.store.SaveSetting(ctx, storage.QBittorrentSettingKey, cfg); err != nil {
		return config.QBittorrentConfig{}, err
	}
	a.qbMu.Lock()
	a.qbCached, a.qbCacheKey = nil, ""
	a.qbMu.Unlock()
	return cfg, nil
}

// qbStatusesForTorrents 为兼容查询实时组装一批 qB 状态。
func (a *App) qbStatusesForTorrents(ctx context.Context, torrents []Torrent, weakMatch bool) (map[string]*QBTorrentStatus, error) {
	statuses := make(map[string]*QBTorrentStatus, len(torrents))
	fetchedAt := time.Now()
	if len(torrents) == 0 {
		return statuses, nil
	}
	keys := make([]storage.TorrentKey, 0, len(torrents))
	for _, torrent := range torrents {
		key := torrentKey(torrent)
		statuses[key] = &QBTorrentStatus{Available: true, Added: false, Source: "none", FetchedAt: fetchedAt}
		keys = append(keys, storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID})
	}
	tasksByTorrent, err := a.store.ListDownloadTasksByTorrents(ctx, keys)
	if err != nil {
		return nil, err
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return unavailableQBStatuses(torrents, fetchedAt, err), nil
	}
	qbTorrents, err := qb.ListTorrents(ctx)
	if err != nil {
		return unavailableQBStatuses(torrents, fetchedAt, err), nil
	}
	index := newQBTorrentIndex(qbTorrents)
	for _, torrent := range torrents {
		key := torrentKey(torrent)
		status, err := a.resolveQBStatus(ctx, torrent, tasksByTorrent[key], index, weakMatch, fetchedAt)
		if err != nil {
			return nil, err
		}
		statuses[key] = &status
	}
	return statuses, nil
}

// unavailableQBStatuses 构造 qB 不可用时的批量状态。
func unavailableQBStatuses(torrents []Torrent, fetchedAt time.Time, err error) map[string]*QBTorrentStatus {
	statuses := make(map[string]*QBTorrentStatus, len(torrents))
	for _, torrent := range torrents {
		statuses[torrentKey(torrent)] = &QBTorrentStatus{Available: false, Added: false, Source: "unavailable", Error: err.Error(), FetchedAt: fetchedAt}
	}
	return statuses
}

// resolveQBStatus 按任务 hash 或显式弱匹配解析单个 qB 状态。
func (a *App) resolveQBStatus(ctx context.Context, torrent Torrent, tasks []storage.DownloadTaskRecord, index qbTorrentIndex, weakMatch bool, fetchedAt time.Time) (QBTorrentStatus, error) {
	base := QBTorrentStatus{Available: true, Added: false, Source: "none", FetchedAt: fetchedAt}
	for _, task := range tasks {
		if task.QBHash == "" {
			continue
		}
		if qbTorrent, ok := index.byHash[strings.ToLower(task.QBHash)]; ok {
			return statusFromQBTorrent(qbTorrent, task, "hash", fetchedAt), nil
		}
	}
	for _, task := range tasks {
		if qbTorrent, ok := index.byName[normalizeQBName(task.TorrentTitle)]; ok {
			if task.QBHash == "" && qbTorrent.Hash != "" {
				if err := a.store.UpdateDownloadTaskHash(ctx, task.ID, qbTorrent.Hash); err != nil {
					return QBTorrentStatus{}, err
				}
			}
			return statusFromQBTorrent(qbTorrent, task, "task_title", fetchedAt), nil
		}
	}
	if len(tasks) > 0 {
		task := tasks[0]
		base.Source = "task_missing"
		base.TaskID = task.ID
		base.TaskStatus = task.Status
		base.RuleID = task.RuleID
		base.Hash = task.QBHash
		return base, nil
	}
	if weakMatch {
		if qbTorrent, ok := index.byName[normalizeQBName(torrent.Title)]; ok {
			return statusFromQBTorrent(qbTorrent, storage.DownloadTaskRecord{}, "weak_title", fetchedAt), nil
		}
	}
	return base, nil
}

// statusFromQBTorrent 将 qB 原生任务转换为领域状态。
func statusFromQBTorrent(qbTorrent qbittorrent.TorrentInfo, task storage.DownloadTaskRecord, source string, fetchedAt time.Time) QBTorrentStatus {
	return QBTorrentStatus{
		Available: true, Added: true, Source: source, FetchedAt: fetchedAt,
		TaskID: task.ID, TaskStatus: task.Status, RuleID: task.RuleID,
		Hash: qbTorrent.Hash, Name: qbTorrent.Name, State: qbTorrent.State, Progress: qbTorrent.Progress,
		Category: qbTorrent.Category, Tags: qbTorrent.Tags, SavePath: qbTorrent.SavePath, ContentPath: qbTorrent.ContentPath,
		DownloadSpeed: qbTorrent.DownloadSpeed, UploadSpeed: qbTorrent.UploadSpeed, ETA: qbTorrent.ETA, Ratio: qbTorrent.Ratio,
		Size: qbTorrent.Size, Completed: qbTorrent.Completed, AmountLeft: qbTorrent.AmountLeft,
	}
}

// matchDownloadTaskToQB 匹配历史下载任务与 qB 原生任务。
func matchDownloadTaskToQB(task storage.DownloadTaskRecord, torrents []qbittorrent.TorrentInfo) (qbittorrent.TorrentInfo, bool) {
	if task.QBHash != "" {
		for _, torrent := range torrents {
			if strings.EqualFold(task.QBHash, torrent.Hash) {
				return torrent, true
			}
		}
	}
	taskName := normalizeQBName(task.TorrentTitle)
	for _, torrent := range torrents {
		if taskName != "" && taskName == normalizeQBName(torrent.Name) {
			return torrent, true
		}
	}
	return qbittorrent.TorrentInfo{}, false
}

type qbTorrentIndex struct {
	byHash map[string]qbittorrent.TorrentInfo
	byName map[string]qbittorrent.TorrentInfo
}

// newQBTorrentIndex 创建按 hash 和名称查询的 qB 索引。
func newQBTorrentIndex(torrents []qbittorrent.TorrentInfo) qbTorrentIndex {
	index := qbTorrentIndex{byHash: make(map[string]qbittorrent.TorrentInfo, len(torrents)), byName: make(map[string]qbittorrent.TorrentInfo, len(torrents))}
	for _, torrent := range torrents {
		if torrent.Hash != "" {
			index.byHash[strings.ToLower(torrent.Hash)] = torrent
		}
		if name := normalizeQBName(torrent.Name); name != "" {
			index.byName[name] = torrent
		}
	}
	return index
}

// normalizeQBName 规范化用于兼容匹配的 qB 名称。
func normalizeQBName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// qbClient 按当前有效配置创建 qBittorrent 客户端。
func (a *App) qbClient(ctx context.Context) (*qbittorrent.Client, error) {
	cfg := a.effectiveQBConfig(ctx)
	cacheKey := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s", cfg.AuthMode, cfg.URL, cfg.APIKey, cfg.Username, cfg.UserID, cfg.Password)
	a.qbMu.Lock()
	defer a.qbMu.Unlock()
	if a.qbCached != nil && a.qbCacheKey == cacheKey {
		return a.qbCached, nil
	}
	client, err := qbittorrent.New(qbittorrent.Config{
		AuthMode: cfg.AuthMode,
		URL:      cfg.URL,
		APIKey:   cfg.APIKey,
		Username: cfg.Username,
		UserID:   cfg.UserID,
		Password: cfg.Password,
		Category: cfg.Category,
		Tags:     cfg.Tags,
	})
	if err != nil {
		return nil, err
	}
	a.qbCached, a.qbCacheKey = client, cacheKey
	return client, nil
}

// effectiveQBConfig 合并配置文件和数据库中的 qB 设置。
func (a *App) effectiveQBConfig(ctx context.Context) config.QBittorrentConfig {
	cfg := a.cfg.QBittorrent
	var stored config.QBittorrentConfig
	if ok, err := a.store.LoadSetting(ctx, storage.QBittorrentSettingKey, &stored); err == nil && ok {
		cfg = stored
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = "uid"
	}
	if cfg.Username == "" && cfg.UserID != "" {
		cfg.Username = cfg.UserID
	}
	if cfg.UserID == "" && cfg.Username != "" {
		cfg.UserID = cfg.Username
	}
	if cfg.Tags == nil {
		cfg.Tags = []string{}
	}
	return config.NormalizeQBittorrentConfig(cfg)
}

// addTorrentToQB 下载私站 torrent 文件，上传到 qBittorrent 并按 info hash 验证。
func (a *App) addTorrentToQB(ctx context.Context, qb *qbittorrent.Client, torrent Torrent) (qbittorrent.TorrentInfo, error) {
	data, _, err := a.loadOrFetchTorrentFile(ctx, torrent)
	if err != nil {
		return qbittorrent.TorrentInfo{}, err
	}
	qbCfg := a.effectiveQBConfig(ctx)
	return qb.AddTorrentFileVerified(ctx, qbittorrent.AddOptions{
		Name:     torrentFileName(torrent),
		Data:     data,
		Category: qbCfg.Category,
		Tags:     qbCfg.Tags,
	})
}

// addTorrentToQBAndLink 添加种子、验证本地 info hash 并回写下载任务。
func (a *App) addTorrentToQBAndLink(ctx context.Context, qb *qbittorrent.Client, torrent Torrent, taskID string) error {
	added, err := a.addTorrentToQB(ctx, qb, torrent)
	if err != nil {
		return err
	}
	if strings.TrimSpace(taskID) != "" && strings.TrimSpace(added.Hash) != "" {
		if err := a.store.UpdateDownloadTaskHash(ctx, taskID, added.Hash); err != nil {
			return err
		}
	}
	if _, err := a.refreshTorrentQBSnapshot(ctx, torrent, false); err != nil {
		slog.Warn("refresh qB snapshot after add failed", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "error", err)
	}
	return nil
}

// torrentFileName 生成上传给 qB 的 torrent 文件名。
func torrentFileName(torrent Torrent) string {
	name := strings.TrimSpace(torrent.SiteID + "-" + torrent.ID)
	if name == "-" {
		name = "nexusbridge"
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(name) + ".torrent"
}
