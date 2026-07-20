package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

func (a *App) sendClaimedTask(ctx context.Context, qb *qbittorrent.Client, task storage.DownloadTaskRecord, torrent Torrent, data []byte, metadata qbittorrent.TorrentMetadata, plan DownloadPlan) (storage.DownloadTaskRecord, error) {
	hashLock := a.hashMutex(firstNonEmpty(metadata.Hashes.V1, metadata.Hashes.V2, task.ID))
	if err := hashLock.Lock(ctx); err != nil {
		task.Status = "pending"
		task.ReasonCode = "cancelled"
		task.Error = err.Error()
		_, _ = a.store.UpdateDownloadTaskRecordIfStatus(context.WithoutCancel(ctx), task, []string{"processing"})
		return task, err
	}
	defer hashLock.Unlock()
	result, err := qb.AddTorrentFileVerifiedResult(ctx, qbittorrent.AddOptions{
		Name: torrentFileName(torrent), Data: data, Category: plan.Category, SavePath: plan.SavePath,
		Tags: plan.Tags, Rename: plan.Rename, Paused: plan.Paused, AutoTMM: plan.AutoTMM,
	})
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		task.ReasonCode = "send_failed"
		if ctx.Err() != nil {
			task.Status = "pending"
			task.ReasonCode = "cancelled"
		} else if strings.Contains(strings.ToLower(err.Error()), "pure v2") {
			task.ReasonCode = "pure_v2_unsupported"
		} else if code := classifyQBBlockAfterSend(ctx, qb, plan.Category); code != "" {
			task.Status = "skipped"
			task.ReasonCode = code
		}
		updated, saveErr := a.store.UpdateDownloadTaskRecordIfStatus(context.WithoutCancel(ctx), task, []string{"processing"})
		if saveErr != nil {
			return task, saveErr
		}
		if !updated {
			return task, fmt.Errorf("download task %q claim was lost while saving qB error: %w", task.ID, err)
		}
		return task, err
	}
	task.QBHash = firstNonEmpty(result.Torrent.Hash, metadata.Hashes.V1)
	task.ContentPath = result.Torrent.ContentPath
	task.Error = ""
	task.ReasonCode = ""
	if result.Added {
		task.Status = "sent"
		task.SentAt = time.Now()
	} else {
		task.Status = "exists"
	}
	updated, err := a.store.UpdateDownloadTaskRecordIfStatus(ctx, task, []string{"processing"})
	if err != nil || !updated {
		if err == nil {
			err = fmt.Errorf("download task %q claim was lost while saving qB result", task.ID)
		}
		task.Status = "pending"
		task.ReasonCode = "persist_failed"
		task.Error = err.Error()
		_, _ = a.store.UpdateDownloadTaskRecordIfStatus(context.WithoutCancel(ctx), task, []string{"processing"})
		return task, err
	}
	if _, err := a.refreshTorrentQBSnapshot(ctx, torrent, false); err != nil && ctx.Err() == nil {
		// 发送结果已经通过 hash 验证；快照刷新失败不回滚成功状态。
	}
	return task, nil
}

func adjustPreparedRenameConflicts(items []preparedSubscriptionCandidate, torrents []qbittorrent.TorrentInfo) {
	registry := newRenameRegistry(torrents)
	for index := range items {
		hash := firstNonEmpty(items[index].metadata.Hashes.V1, items[index].metadata.Hashes.V2,
			items[index].torrent.SiteID+":"+items[index].torrent.ID)
		if registry.adjust(&items[index].plan, hash, items[index].torrent.SiteID, items[index].torrent.ID) {
			items[index].plan.Reasons = append(items[index].plan.Reasons, RuleReason{
				Code: "rename_conflict_adjusted", Field: "filename_template", Message: "已为同名不同 hash 任务追加稳定后缀",
			})
		}
	}
}

func classifyQBBlockAfterSend(ctx context.Context, qb *qbittorrent.Client, category string) string {
	if ctx.Err() != nil {
		return ""
	}
	categories, err := qb.GetCategories(ctx)
	if err != nil {
		return "qb_unavailable"
	}
	for name := range categories {
		if strings.TrimSpace(name) == strings.TrimSpace(category) {
			return ""
		}
	}
	return "qb_category_missing"
}

func newSubscriptionRunID(subscriptionID, siteID, trigger string) string {
	return fmt.Sprintf("%s:%s:%s:%d", subscriptionID, siteID, trigger, time.Now().UnixNano())
}

func sortPreparedForRule(items []preparedSubscriptionCandidate, rule Rule) {
	ordered := make([]Torrent, 0, len(items))
	byKey := make(map[string][]preparedSubscriptionCandidate, len(items))
	for _, item := range items {
		ordered = append(ordered, item.torrent)
		key := item.torrent.SiteID + "\x00" + item.torrent.ID
		byKey[key] = append(byKey[key], item)
	}
	SortTorrentsForRule(ordered, rule)
	result := make([]preparedSubscriptionCandidate, 0, len(items))
	for _, torrent := range ordered {
		key := torrent.SiteID + "\x00" + torrent.ID
		result = append(result, byKey[key][0])
		byKey[key] = byKey[key][1:]
	}
	copy(items, result)
}
