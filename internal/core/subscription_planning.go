package core

import (
	"context"
	"fmt"
	"strings"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

func (a *App) prepareQBForPlans(ctx context.Context, category string, items []preparedSubscriptionCandidate) (*qbittorrent.Client, []qbittorrent.TorrentInfo, string, error) {
	qb, err := a.qbClient(ctx)
	if err != nil {
		return nil, nil, "qb_unavailable", err
	}
	categories, err := qb.GetCategories(ctx)
	if err != nil {
		return qb, nil, "qb_unavailable", err
	}
	foundCategory := false
	for name := range categories {
		if strings.TrimSpace(name) == strings.TrimSpace(category) {
			foundCategory = true
			break
		}
	}
	if strings.TrimSpace(category) == "" || !foundCategory {
		return qb, nil, "qb_category_missing", fmt.Errorf("qb category %q does not exist", category)
	}
	existingTags, err := qb.GetTags(ctx)
	if err != nil {
		return qb, nil, "qb_unavailable", err
	}
	existing := map[string]struct{}{}
	for _, tag := range existingTags {
		existing[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	missing := []string{}
	seenMissing := map[string]struct{}{}
	for _, item := range items {
		for _, tag := range item.plan.Tags {
			key := strings.ToLower(strings.TrimSpace(tag))
			if _, ok := existing[key]; ok {
				continue
			}
			if _, ok := seenMissing[key]; ok {
				continue
			}
			seenMissing[key] = struct{}{}
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		if err := qb.CreateTags(ctx, missing); err != nil {
			return qb, nil, "qb_unavailable", fmt.Errorf("create qb tags: %w", err)
		}
	}
	torrents, err := qb.ListTorrents(ctx)
	if err != nil {
		return qb, nil, "qb_unavailable", err
	}
	return qb, torrents, "", nil
}

func (a *App) countActiveSubscriptionTasks(ctx context.Context, subscriptionID string, qbTorrents []qbittorrent.TorrentInfo) (int, error) {
	tasks, err := a.store.ListDownloadTasksByStatus(ctx, subscriptionID, []string{"pending", "processing", "sent", "exists"}, storage.MaxTorrentListLimit)
	if err != nil {
		return 0, err
	}
	active := 0
	for _, task := range tasks {
		if task.Status == "pending" || task.Status == "processing" {
			active++
			continue
		}
		if !qbTorrentComplete(task.QBHash, qbTorrents) {
			active++
		}
	}
	return active, nil
}

func qbTorrentComplete(hash string, torrents []qbittorrent.TorrentInfo) bool {
	if torrent, ok := findQBTorrentByHash(hash, torrents); ok {
		return torrent.Progress >= 1 || (torrent.Size > 0 && torrent.AmountLeft == 0)
	}
	// 本地历史任务已不在 qB 中时，不再占用订阅并发名额。
	return true
}

func findQBTorrentByHash(hash string, torrents []qbittorrent.TorrentInfo) (qbittorrent.TorrentInfo, bool) {
	for _, torrent := range torrents {
		if strings.EqualFold(strings.TrimSpace(torrent.Hash), strings.TrimSpace(hash)) {
			return torrent, true
		}
	}
	return qbittorrent.TorrentInfo{}, false
}

func (a *App) claimPlannedTask(ctx context.Context, item preparedSubscriptionCandidate, subscription Subscription, trigger string, incrementRetry bool) (storage.DownloadTaskRecord, string, error) {
	id := downloadTaskID(item.torrent.SiteID, item.torrent.ID, item.record.RuleName)
	task, exists, err := a.store.GetDownloadTaskByRule(ctx, item.torrent.SiteID, item.torrent.ID, item.record.RuleName)
	if err != nil {
		return storage.DownloadTaskRecord{}, "", err
	}
	if !exists {
		task = plannedTaskRecord(item, subscription, trigger, "pending", "", "")
		created, err := a.store.CreateDownloadTaskIfAbsent(ctx, task)
		if err != nil {
			return storage.DownloadTaskRecord{}, "", err
		}
		if !created {
			task, exists, err = a.store.GetDownloadTaskByRule(ctx, item.torrent.SiteID, item.torrent.ID, item.record.RuleName)
			if err != nil {
				return storage.DownloadTaskRecord{}, "", err
			}
			if !exists {
				return storage.DownloadTaskRecord{}, "", fmt.Errorf("download task %q disappeared during claim", id)
			}
		}
	}
	if disposition := plannedTaskDisposition(task); disposition != "claimable" {
		return task, disposition, nil
	}
	claimed, ok, err := a.store.ClaimDownloadTask(ctx, task.ID, []string{"pending", "skipped"}, incrementRetry)
	if err != nil {
		return storage.DownloadTaskRecord{}, "", err
	}
	if !ok {
		latest, exists, getErr := a.store.GetDownloadTask(ctx, task.ID)
		if getErr != nil {
			return storage.DownloadTaskRecord{}, "", getErr
		}
		if !exists {
			return task, "busy", nil
		}
		disposition := plannedTaskDisposition(latest)
		if disposition == "claimable" {
			disposition = "busy"
		}
		return latest, disposition, nil
	}
	claimed.SubscriptionID = subscription.ID
	claimed.Trigger = trigger
	claimed.TorrentTitle = item.torrent.Title
	claimed.DownloadURL = item.torrent.DownloadURL
	claimed.PlanCategory = item.plan.Category
	claimed.PlanSavePath = item.plan.SavePath
	claimed.PlanTags = item.plan.Tags
	claimed.PlanRename = item.plan.Rename
	claimed.PlanPaused = item.plan.Paused
	claimed.ReasonCode = ""
	claimed.Error = ""
	updated, err := a.store.UpdateDownloadTaskRecordIfStatus(ctx, claimed, []string{"processing"})
	if err != nil || !updated {
		if err == nil {
			err = fmt.Errorf("download task %q claim was lost before its plan snapshot was saved", claimed.ID)
		}
		claimed.Status = "pending"
		claimed.Error = err.Error()
		claimed.ReasonCode = "claim_failed"
		_, _ = a.store.UpdateDownloadTaskRecordIfStatus(context.WithoutCancel(ctx), claimed, []string{"processing"})
		return storage.DownloadTaskRecord{}, "", err
	}
	return claimed, "claimed", nil
}

func plannedTaskDisposition(task storage.DownloadTaskRecord) string {
	switch task.Status {
	case "pending", "skipped":
		return "claimable"
	case "sent", "exists":
		return "processed"
	case "failed":
		return "failed"
	default:
		return "busy"
	}
}

func plannedTaskRecord(item preparedSubscriptionCandidate, subscription Subscription, trigger, status, reasonCode, errText string) storage.DownloadTaskRecord {
	return storage.DownloadTaskRecord{
		ID: downloadTaskID(item.torrent.SiteID, item.torrent.ID, item.record.RuleName), SiteID: item.torrent.SiteID,
		TorrentID: item.torrent.ID, RuleName: item.record.RuleName, SubscriptionID: subscription.ID,
		Trigger: trigger, Status: status, TorrentTitle: item.torrent.Title, DownloadURL: item.torrent.DownloadURL,
		PlanCategory: item.plan.Category, PlanSavePath: item.plan.SavePath, PlanTags: item.plan.Tags,
		PlanRename: item.plan.Rename, PlanPaused: item.plan.Paused, ReasonCode: reasonCode, Error: errText,
	}
}
