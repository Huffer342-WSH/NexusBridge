package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

type subscriptionExecutionCounts struct {
	attempted int
	sent      int
	exists    int
	failed    int
	skipped   int
}

type preparedSubscriptionCandidate struct {
	record   storage.SubscriptionCandidateRecord
	torrent  Torrent
	data     []byte
	metadata qbittorrent.TorrentMetadata
	plan     DownloadPlan
}

// RunSubscriptionOnce 抓取订阅涉及的站点并按同一配额规则执行一次。
func (a *App) RunSubscriptionOnce(ctx context.Context, id string) (SubscriptionRun, error) {
	subscription, rule, err := a.loadSubscriptionAndRule(ctx, id)
	if err != nil {
		return SubscriptionRun{}, err
	}

	run := storage.SubscriptionRunRecord{
		ID: newSubscriptionRunID(subscription.ID, "all", "run-once"), SubscriptionID: subscription.ID,
		Trigger: "run-once", Status: "running", StartedAt: time.Now(),
	}
	if err := a.store.SaveSubscriptionRun(ctx, run); err != nil {
		return SubscriptionRun{}, err
	}
	var fetchErrors []error
	finish := func(runErr error) (SubscriptionRun, error) {
		run.FinishedAt = time.Now()
		if runErr != nil {
			run.Status = "failed"
			run.Error = runErr.Error()
		} else {
			run.Status = "completed"
		}
		if saveErr := a.store.SaveSubscriptionRun(context.WithoutCancel(ctx), run); saveErr != nil && runErr == nil {
			runErr = saveErr
		}
		return subscriptionRunFromRecord(run), runErr
	}

	for _, siteID := range subscription.SiteIDs {
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
		lock := a.siteMutex(siteID)
		if err := lock.Lock(ctx); err != nil {
			return finish(err)
		}
		fetch, inserted, fetchErr := a.refreshSite(ctx, siteID)
		run.FetchedCount += fetch.Fetched
		run.InsertedCount += fetch.Inserted
		if ctx.Err() != nil {
			lock.Unlock()
			return finish(ctx.Err())
		}

		candidates, err := a.store.ListEnabledSubscriptionsBySite(ctx, siteID)
		if err != nil {
			lock.Unlock()
			return finish(err)
		}
		if !subscription.Enabled {
			candidates = append(candidates, subscriptionToRecord(subscription))
			sort.SliceStable(candidates, func(i, j int) bool {
				if candidates[i].Priority != candidates[j].Priority {
					return candidates[i].Priority > candidates[j].Priority
				}
				return candidates[i].ID < candidates[j].ID
			})
		}
		assigned, err := a.assignPendingSiteTorrents(ctx, siteID, candidates)
		lock.Unlock()
		if err != nil {
			return finish(err)
		}
		run.MatchedCount += assigned[subscription.ID]
		if fetchErr != nil {
			fetchErrors = append(fetchErrors, fetchErr)
		}
		_ = inserted // inserted 仅用于抓取统计；首次匹配由持久化 ingest 队列驱动。
	}

	counts, err := a.runSubscriptionCandidates(ctx, subscription, rule, "run-once", "")
	run.AttemptedCount += counts.attempted
	run.SentCount += counts.sent
	run.ExistsCount += counts.exists
	run.FailedCount += counts.failed
	run.SkippedCount += counts.skipped
	return finish(errors.Join(append(fetchErrors, err)...))
}

// runSiteSubscriptions 抓取站点、按优先级独占分配新种子并执行全部启用订阅。
func (a *App) runSiteSubscriptions(ctx context.Context, siteID, trigger string) (FetchResult, error) {
	lock := a.siteMutex(siteID)
	if err := lock.Lock(ctx); err != nil {
		return FetchResult{SiteID: siteID, Status: "failed"}, err
	}

	result, inserted, fetchErr := a.refreshSite(ctx, siteID)
	if ctx.Err() != nil {
		lock.Unlock()
		return result, ctx.Err()
	}
	records, err := a.store.ListEnabledSubscriptionsBySite(ctx, siteID)
	if err != nil {
		lock.Unlock()
		return result, err
	}
	assigned, err := a.assignPendingSiteTorrents(ctx, siteID, records)
	lock.Unlock()
	if err != nil {
		return result, err
	}
	_ = inserted // inserted 仅用于抓取统计；首次匹配由持久化 ingest 队列驱动。
	for _, count := range assigned {
		result.Matched += count
	}

	var runErrors []error
	if fetchErr != nil {
		runErrors = append(runErrors, fetchErr)
	}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		subscription := subscriptionFromRecord(record)
		ruleRecord, ok, err := a.store.GetRule(ctx, subscription.RuleName)
		if err != nil {
			runErrors = append(runErrors, err)
			continue
		}
		if !ok {
			continue
		}
		run := storage.SubscriptionRunRecord{
			ID: newSubscriptionRunID(subscription.ID, siteID, trigger), SubscriptionID: subscription.ID,
			SiteID: siteID, Trigger: trigger, Status: "running", FetchedCount: result.Fetched,
			InsertedCount: result.Inserted, MatchedCount: assigned[subscription.ID], StartedAt: time.Now(),
		}
		if err := a.store.SaveSubscriptionRun(ctx, run); err != nil {
			runErrors = append(runErrors, err)
			continue
		}
		counts, runErr := a.runSubscriptionCandidates(ctx, subscription, ruleFromRecord(ruleRecord), trigger, siteID)
		run.AttemptedCount = counts.attempted
		run.SentCount = counts.sent
		run.ExistsCount = counts.exists
		run.FailedCount = counts.failed
		run.SkippedCount = counts.skipped
		run.FinishedAt = time.Now()
		if runErr != nil {
			run.Status = "failed"
			run.Error = runErr.Error()
			runErrors = append(runErrors, fmt.Errorf("subscription %s: %w", subscription.ID, runErr))
		} else {
			run.Status = "completed"
		}
		if err := a.store.SaveSubscriptionRun(context.WithoutCancel(ctx), run); err != nil {
			runErrors = append(runErrors, err)
		}
		result.DownloadSent += counts.sent
	}
	if len(runErrors) > 0 {
		result.Status = "failed"
		return result, errors.Join(runErrors...)
	}
	result.Status = "ok"
	return result, nil
}

func (a *App) assignInsertedTorrents(ctx context.Context, torrents []Torrent, subscriptions []storage.SubscriptionRecord) (map[string]int, error) {
	assigned := map[string]int{}
	rules := make(map[string]Rule, len(subscriptions))
	for _, subscription := range subscriptions {
		record, ok, err := a.store.GetRule(ctx, subscription.RuleName)
		if err != nil {
			return nil, err
		}
		if ok {
			rules[subscription.ID] = ruleFromRecord(record)
		}
	}
	for _, torrent := range torrents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, subscription := range subscriptions {
			rule, ok := rules[subscription.ID]
			if !ok || !MatchRule(rule, torrent).Matched {
				continue
			}
			created, err := a.store.CreateSubscriptionCandidateIfAbsent(ctx, storage.SubscriptionCandidateRecord{
				SiteID: torrent.SiteID, TorrentID: torrent.ID, SubscriptionID: subscription.ID,
				RuleName: rule.Name, SourceOrder: torrent.SourceOrder, Status: storage.SubscriptionCandidateUnread,
				ReasonCode: "matched",
			})
			if err != nil {
				return nil, err
			}
			if created {
				assigned[subscription.ID]++
			}
			// 无论是否已存在候选，同一种子都不能回退到低优先级订阅。
			break
		}
	}
	return assigned, nil
}

func (a *App) assignPendingSiteTorrents(ctx context.Context, siteID string, subscriptions []storage.SubscriptionRecord) (map[string]int, error) {
	records, err := a.store.ListPendingSubscriptionIngest(ctx, siteID, storage.MaxTorrentListLimit)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return map[string]int{}, nil
	}
	torrents := make([]Torrent, 0, len(records))
	for _, record := range records {
		torrent, err := a.getTorrent(ctx, record.SiteID, record.TorrentID)
		if err != nil {
			return nil, err
		}
		// 候选顺序固定为首次入库时的页面位置，不随后续抓取重排。
		torrent.SourceOrder = record.SourceOrder
		torrents = append(torrents, torrent)
	}
	assigned, err := a.assignInsertedTorrents(ctx, torrents, subscriptions)
	if err != nil {
		return nil, err
	}
	if err := a.store.DeletePendingSubscriptionIngest(ctx, records); err != nil {
		return nil, err
	}
	return assigned, nil
}

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
