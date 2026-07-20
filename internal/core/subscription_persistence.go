package core

import (
	"context"
	"fmt"

	"nexusbridge/internal/storage"
)

func (a *App) skipCandidate(ctx context.Context, item preparedSubscriptionCandidate, subscription Subscription, trigger, code string, blockErr error) error {
	task := plannedTaskRecord(item, subscription, trigger, "skipped", code, blockErr.Error())
	saved, disposition, err := a.persistTaskWithoutClaim(ctx, task)
	if err != nil {
		return err
	}
	key := storage.TorrentKey{SiteID: item.record.SiteID, TorrentID: item.record.TorrentID}
	switch disposition {
	case "processed":
		return a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateProcessed, saved.Status, "")
	case "failed":
		return a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateFailed, saved.ReasonCode, saved.Error)
	case "busy":
		return a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, "task_busy", "download task is already processing")
	}
	return a.transitionCandidate(ctx, key,
		storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, code, blockErr.Error())
}

func (a *App) failCandidateBeforeSend(ctx context.Context, candidate storage.SubscriptionCandidateRecord, torrent Torrent, plan DownloadPlan, trigger, code string, cause error) error {
	item := preparedSubscriptionCandidate{record: candidate, torrent: torrent, plan: plan}
	if item.torrent.SiteID == "" {
		item.torrent.SiteID = candidate.SiteID
		item.torrent.ID = candidate.TorrentID
	}
	task := plannedTaskRecord(item, Subscription{ID: candidate.SubscriptionID}, trigger, "failed", code, cause.Error())
	saved, disposition, err := a.persistTaskWithoutClaim(ctx, task)
	if err != nil {
		return err
	}
	key := storage.TorrentKey{SiteID: candidate.SiteID, TorrentID: candidate.TorrentID}
	switch disposition {
	case "processed":
		return a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateProcessed, saved.Status, "")
	case "busy":
		return a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, "task_busy", "download task is already processing")
	}
	return a.transitionCandidate(ctx, key,
		storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateFailed, code, cause.Error())
}

func (a *App) persistTaskWithoutClaim(ctx context.Context, desired storage.DownloadTaskRecord) (storage.DownloadTaskRecord, string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		existing, ok, err := a.store.GetDownloadTaskByRule(ctx, desired.SiteID, desired.TorrentID, desired.RuleName)
		if err != nil {
			return storage.DownloadTaskRecord{}, "", err
		}
		if !ok {
			created, err := a.store.CreateDownloadTaskIfAbsent(ctx, desired)
			if err != nil {
				return storage.DownloadTaskRecord{}, "", err
			}
			if created {
				return desired, desired.Status, nil
			}
			continue
		}
		switch disposition := plannedTaskDisposition(existing); disposition {
		case "processed", "failed", "busy":
			return existing, disposition, nil
		}
		desired.AttemptCount = existing.AttemptCount
		desired.RetryCount = existing.RetryCount
		desired.LastAttemptAt = existing.LastAttemptAt
		desired.SentAt = existing.SentAt
		updated, err := a.store.UpdateDownloadTaskRecordIfStatus(ctx, desired, []string{"pending", "skipped"})
		if err != nil {
			return storage.DownloadTaskRecord{}, "", err
		}
		if updated {
			return desired, desired.Status, nil
		}
	}
	latest, ok, err := a.store.GetDownloadTaskByRule(ctx, desired.SiteID, desired.TorrentID, desired.RuleName)
	if err != nil {
		return storage.DownloadTaskRecord{}, "", err
	}
	if !ok {
		return storage.DownloadTaskRecord{}, "", fmt.Errorf("download task %q changed repeatedly while saving", desired.ID)
	}
	return latest, "busy", nil
}

func (a *App) transitionCandidate(ctx context.Context, key storage.TorrentKey, fromStatus, status, reasonCode, errText string) error {
	updated, err := a.store.UpdateSubscriptionCandidateStatus(ctx, key, fromStatus, status, reasonCode, errText)
	if err != nil {
		return err
	}
	if !updated {
		return fmt.Errorf("subscription candidate %s/%s is no longer %s", key.SiteID, key.TorrentID, fromStatus)
	}
	return nil
}
