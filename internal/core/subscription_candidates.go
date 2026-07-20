package core

import (
	"context"
	"fmt"
	"time"

	"nexusbridge/internal/storage"
)

func (a *App) runSubscriptionCandidates(ctx context.Context, subscription Subscription, rule Rule, trigger, siteID string) (subscriptionExecutionCounts, error) {
	var counts subscriptionExecutionCounts
	lock := a.subscriptionMutex(subscription.ID)
	if err := lock.Lock(ctx); err != nil {
		return counts, err
	}
	defer lock.Unlock()

	claimed, err := a.store.ClaimSubscriptionCandidatesForSite(ctx, subscription.ID, siteID, storage.MaxTorrentListLimit)
	if err != nil || len(claimed) == 0 {
		return counts, err
	}
	remaining := make(map[storage.TorrentKey]struct{}, len(claimed))
	for _, candidate := range claimed {
		remaining[storage.TorrentKey{SiteID: candidate.SiteID, TorrentID: candidate.TorrentID}] = struct{}{}
	}
	defer func() {
		for key := range remaining {
			reason, message := "execution_interrupted", "subscription execution ended before this candidate completed"
			if ctx.Err() != nil {
				reason, message = "cancelled", ctx.Err().Error()
			}
			_, _ = a.store.UpdateSubscriptionCandidateStatus(context.Background(), key, storage.SubscriptionCandidateProcessing,
				storage.SubscriptionCandidateUnread, reason, message)
		}
	}()

	prepared := make([]preparedSubscriptionCandidate, 0, len(claimed))
	globalTags := a.effectiveQBConfig(ctx).Tags
	for _, candidate := range claimed {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		key := storage.TorrentKey{SiteID: candidate.SiteID, TorrentID: candidate.TorrentID}
		torrent, err := a.getTorrent(ctx, candidate.SiteID, candidate.TorrentID)
		if err != nil {
			if markErr := a.failCandidateBeforeSend(ctx, candidate, Torrent{}, DownloadPlan{}, trigger, "torrent_missing", err); markErr != nil {
				return counts, markErr
			}
			delete(remaining, key)
			counts.failed++
			continue
		}
		data, metadata, err := a.loadOrFetchTorrentMetadata(ctx, torrent)
		if err != nil {
			if ctx.Err() != nil {
				return counts, ctx.Err()
			}
			if markErr := a.failCandidateBeforeSend(ctx, candidate, torrent, DownloadPlan{}, trigger, "torrent_file_unavailable", err); markErr != nil {
				return counts, markErr
			}
			delete(remaining, key)
			counts.failed++
			continue
		}
		site := a.sites[torrent.SiteID]
		plan, err := BuildDownloadPlan(rule, subscription.Name, site.Name, subscription.Download, torrent, metadata.Name, globalTags)
		if err != nil {
			if markErr := a.failCandidateBeforeSend(ctx, candidate, torrent, DownloadPlan{}, trigger, "plan_invalid", err); markErr != nil {
				return counts, markErr
			}
			delete(remaining, key)
			counts.failed++
			continue
		}
		prepared = append(prepared, preparedSubscriptionCandidate{record: candidate, torrent: torrent, data: data, metadata: metadata, plan: plan})
	}
	if len(prepared) == 0 {
		return counts, nil
	}

	sortPreparedForRule(prepared, rule)
	qb, qbTorrents, blockCode, blockErr := a.prepareQBForPlans(ctx, subscription.Download.QBCategory, prepared)
	if ctx.Err() != nil {
		return counts, ctx.Err()
	}
	if blockErr == nil {
		adjustPreparedRenameConflicts(prepared, qbTorrents)
	}
	if blockErr != nil {
		for _, item := range prepared {
			if err := a.skipCandidate(ctx, item, subscription, trigger, blockCode, blockErr); err != nil {
				return counts, err
			}
			delete(remaining, storage.TorrentKey{SiteID: item.record.SiteID, TorrentID: item.record.TorrentID})
			counts.skipped++
		}
		return counts, nil
	}

	active, err := a.countActiveSubscriptionTasks(ctx, subscription.ID, qbTorrents)
	if err != nil {
		return counts, err
	}
	now := time.Now()
	localMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dailySent, err := a.store.CountSentDownloadTasksSince(ctx, subscription.ID, localMidnight)
	if err != nil {
		return counts, err
	}
	for _, item := range prepared {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		key := storage.TorrentKey{SiteID: item.record.SiteID, TorrentID: item.record.TorrentID}
		current := time.Now()
		currentMidnight := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
		if !currentMidnight.Equal(localMidnight) {
			localMidnight = currentMidnight
			dailySent, err = a.store.CountSentDownloadTasksSince(ctx, subscription.ID, localMidnight)
			if err != nil {
				return counts, err
			}
		}
		existing, exists, err := a.store.GetDownloadTaskByRule(ctx, item.torrent.SiteID, item.torrent.ID, rule.Name)
		if err != nil {
			return counts, err
		}
		reserved := exists && (existing.Status == "pending" || existing.Status == "processing")
		_, alreadyInQB := findQBTorrentByHash(firstNonEmpty(item.metadata.Hashes.V1, item.metadata.Hashes.V2), qbTorrents)
		if !alreadyInQB && subscription.Download.DailyLimit > 0 && dailySent >= subscription.Download.DailyLimit {
			if err := a.skipCandidate(ctx, item, subscription, trigger, "quota_daily", fmt.Errorf("subscription daily limit reached")); err != nil {
				return counts, err
			}
			if reserved && active > 0 {
				active--
			}
			delete(remaining, key)
			counts.skipped++
			continue
		}
		if !alreadyInQB && !reserved && subscription.Download.MaxConcurrent > 0 && active >= subscription.Download.MaxConcurrent {
			if err := a.skipCandidate(ctx, item, subscription, trigger, "quota_concurrent", fmt.Errorf("subscription concurrent limit reached")); err != nil {
				return counts, err
			}
			delete(remaining, key)
			counts.skipped++
			continue
		}

		task, disposition, err := a.claimPlannedTask(ctx, item, subscription, trigger, false)
		if err != nil {
			return counts, err
		}
		switch disposition {
		case "claimed":
			// 只有原子领取成功的任务才能进入 qB 写入阶段。
		case "processed":
			if err := a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateProcessed, task.Status, ""); err != nil {
				return counts, err
			}
			delete(remaining, key)
			continue
		case "failed":
			if err := a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateFailed, task.ReasonCode, task.Error); err != nil {
				return counts, err
			}
			delete(remaining, key)
			continue
		case "busy":
			if err := a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, "task_busy", "download task is already processing"); err != nil {
				return counts, err
			}
			delete(remaining, key)
			continue
		default:
			return counts, fmt.Errorf("download task %q returned invalid claim disposition %q", task.ID, disposition)
		}

		counts.attempted++
		result, err := a.sendClaimedTask(ctx, qb, task, item.torrent, item.data, item.metadata, item.plan)
		if ctx.Err() != nil {
			if result.Status == "sent" || result.Status == "exists" {
				_ = a.transitionCandidate(context.Background(), key, storage.SubscriptionCandidateProcessing,
					storage.SubscriptionCandidateProcessed, result.Status, "")
				delete(remaining, key)
				if result.Status == "sent" {
					counts.sent++
				} else {
					counts.exists++
				}
				return counts, ctx.Err()
			}
			return counts, ctx.Err()
		}
		if err != nil {
			if result.Status == "skipped" {
				if transitionErr := a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, result.ReasonCode, result.Error); transitionErr != nil {
					return counts, transitionErr
				}
				delete(remaining, key)
				counts.skipped++
				continue
			}
			if result.Status == "pending" {
				if transitionErr := a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, result.ReasonCode, result.Error); transitionErr != nil {
					return counts, transitionErr
				}
				delete(remaining, key)
				return counts, err
			}
			if transitionErr := a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateFailed, result.ReasonCode, result.Error); transitionErr != nil {
				return counts, transitionErr
			}
			delete(remaining, key)
			counts.failed++
			continue
		}
		if err = a.transitionCandidate(ctx, key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateProcessed, result.Status, ""); err != nil {
			return counts, err
		}
		delete(remaining, key)
		if result.Status == "sent" {
			counts.sent++
			dailySent++
			if !reserved {
				active++
			}
		} else {
			counts.exists++
			if !reserved && !qbTorrentComplete(result.QBHash, qbTorrents) {
				active++
			}
		}
	}
	return counts, nil
}
