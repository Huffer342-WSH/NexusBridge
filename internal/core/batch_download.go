package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

type batchPlanContext struct {
	subscription     Subscription
	rule             Rule
	options          DownloadOptions
	subscriptionName string
}

// PreviewBatchDownload 只读计算手动批量下载的最终计划。
func (a *App) PreviewBatchDownload(ctx context.Context, request BatchDownloadRequest) (BatchDownloadPreview, error) {
	planContext, err := a.resolveBatchPlanContext(ctx, request)
	if err != nil {
		return BatchDownloadPreview{}, err
	}
	keys, err := validateBatchTorrentKeys(request.Torrents)
	if err != nil {
		return BatchDownloadPreview{}, err
	}
	storageKeys := make([]storage.TorrentKey, 0, len(keys))
	for _, key := range keys {
		storageKeys = append(storageKeys, storage.TorrentKey{SiteID: key.SiteID, TorrentID: key.TorrentID})
	}
	metadata, err := a.store.ListTorrentFileMetadata(ctx, storageKeys)
	if err != nil {
		return BatchDownloadPreview{}, err
	}

	qb, qbErr := a.qbClient(ctx)
	categoryExists := false
	var qbTorrents []qbittorrent.TorrentInfo
	if qbErr == nil {
		categories, err := qb.GetCategories(ctx)
		if err != nil {
			qbErr = err
		} else {
			for name := range categories {
				if strings.TrimSpace(name) == strings.TrimSpace(planContext.options.QBCategory) {
					categoryExists = true
					break
				}
			}
		}
	}
	if qbErr == nil {
		qbTorrents, qbErr = qb.ListTorrents(ctx)
	}

	result := BatchDownloadPreview{Items: make([]SubscriptionPreviewItem, 0, len(keys))}
	globalTags := a.effectiveQBConfig(ctx).Tags
	renameRegistry := newRenameRegistry(qbTorrents)
	for _, key := range keys {
		torrent, err := a.getTorrent(ctx, key.SiteID, key.TorrentID)
		if err != nil {
			return BatchDownloadPreview{}, err
		}
		file := metadata[storage.TorrentKey{SiteID: key.SiteID, TorrentID: key.TorrentID}]
		site := a.sites[torrent.SiteID]
		plan, planErr := BuildDownloadPlan(planContext.rule, planContext.subscriptionName, site.Name, planContext.options, torrent, file.OriginalName, globalTags)
		item := SubscriptionPreviewItem{Torrent: torrent, Matched: true, Eligible: planErr == nil, Plan: plan}
		if planErr != nil {
			item.Reasons = append(item.Reasons, RuleReason{Code: "plan_invalid", Message: planErr.Error()})
		}
		if !file.HasData {
			item.Eligible = false
			item.Reasons = append(item.Reasons, RuleReason{Code: "torrent_file_unavailable", Message: "torrent 文件尚未缓存"})
		} else if file.InfoHashV1 == "" && file.InfoHashV2 != "" {
			item.Eligible = false
			item.Reasons = append(item.Reasons, RuleReason{Code: "pure_v2_unsupported", Message: "pure v2 torrent 暂不支持精确验证"})
		}
		if qbErr != nil {
			item.Eligible = false
			item.Reasons = append(item.Reasons, RuleReason{Code: "qb_unavailable", Message: qbErr.Error()})
		} else if !categoryExists {
			item.Eligible = false
			item.Reasons = append(item.Reasons, RuleReason{Code: "qb_category_missing", Field: "qb_category", Message: "qB 分类不存在"})
		} else if renameRegistry.adjust(&item.Plan, firstNonEmpty(file.InfoHashV1, file.InfoHashV2, torrent.SiteID+":"+torrent.ID), torrent.SiteID, torrent.ID) {
			item.Plan.Reasons = append(item.Plan.Reasons, RuleReason{Code: "rename_conflict_adjusted", Field: "filename_template", Message: "已为同名不同 hash 任务追加稳定后缀"})
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// ExecuteBatchDownload 重新校验并执行手动批量下载。
func (a *App) ExecuteBatchDownload(ctx context.Context, request BatchDownloadRequest) (BatchDownloadResult, error) {
	planContext, err := a.resolveBatchPlanContext(ctx, request)
	if err != nil {
		return BatchDownloadResult{}, err
	}
	keys, err := validateBatchTorrentKeys(request.Torrents)
	if err != nil {
		return BatchDownloadResult{}, err
	}
	run := storage.SubscriptionRunRecord{
		ID:             newSubscriptionRunID(planContext.subscription.ID, "batch", "manual-batch"),
		SubscriptionID: planContext.subscription.ID, Trigger: "manual-batch", Status: "running", StartedAt: time.Now(),
	}
	if err := a.store.SaveSubscriptionRun(ctx, run); err != nil {
		return BatchDownloadResult{}, err
	}
	finishRun := func(result BatchDownloadResult, runErr error) (BatchDownloadResult, error) {
		run.AttemptedCount = result.Attempted
		run.SentCount = result.Sent
		run.ExistsCount = result.Exists
		run.FailedCount = result.Failed
		run.SkippedCount = result.Skipped
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
		return result, runErr
	}

	prepared := make([]preparedSubscriptionCandidate, 0, len(keys))
	result := BatchDownloadResult{Tasks: make([]DownloadTask, 0, len(keys))}
	globalTags := a.effectiveQBConfig(ctx).Tags
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return finishRun(result, err)
		}
		torrent, err := a.getTorrent(ctx, key.SiteID, key.TorrentID)
		if err != nil {
			return finishRun(result, err)
		}
		data, metadata, err := a.loadOrFetchTorrentMetadata(ctx, torrent)
		if err != nil {
			task, saveErr := a.saveBatchTerminalTask(ctx, planContext, torrent, DownloadPlan{}, "failed", "torrent_file_unavailable", err)
			if saveErr != nil {
				return finishRun(result, saveErr)
			}
			appendBatchTerminalOutcome(&result, task)
			continue
		}
		site := a.sites[torrent.SiteID]
		plan, err := BuildDownloadPlan(planContext.rule, planContext.subscriptionName, site.Name, planContext.options, torrent, metadata.Name, globalTags)
		if err != nil {
			task, saveErr := a.saveBatchTerminalTask(ctx, planContext, torrent, DownloadPlan{}, "failed", "plan_invalid", err)
			if saveErr != nil {
				return finishRun(result, saveErr)
			}
			appendBatchTerminalOutcome(&result, task)
			continue
		}
		prepared = append(prepared, preparedSubscriptionCandidate{
			record:  storage.SubscriptionCandidateRecord{SiteID: torrent.SiteID, TorrentID: torrent.ID, RuleName: planContext.rule.Name},
			torrent: torrent, data: data, metadata: metadata, plan: plan,
		})
	}
	if len(prepared) == 0 {
		return finishRun(result, nil)
	}

	qb, qbTorrents, blockCode, blockErr := a.prepareQBForPlans(ctx, planContext.options.QBCategory, prepared)
	if ctx.Err() != nil {
		return finishRun(result, ctx.Err())
	}
	if blockErr == nil {
		adjustPreparedRenameConflicts(prepared, qbTorrents)
	}
	for index := range prepared {
		if blockErr != nil {
			task, err := a.saveBatchTerminalTask(ctx, planContext, prepared[index].torrent, prepared[index].plan, "skipped", blockCode, blockErr)
			if err != nil {
				return finishRun(result, err)
			}
			appendBatchTerminalOutcome(&result, task)
		}
	}
	if blockErr != nil {
		return finishRun(result, nil)
	}

	for _, item := range prepared {
		if err := ctx.Err(); err != nil {
			return finishRun(result, err)
		}
		task, disposition, err := a.claimPlannedTask(ctx, item, planContext.subscription, "manual-batch", false)
		if err != nil {
			return finishRun(result, err)
		}
		if disposition != "claimed" {
			result.Tasks = append(result.Tasks, downloadTaskFromRecord(task))
			switch task.Status {
			case "sent":
				result.Sent++
			case "exists":
				result.Exists++
			case "failed":
				result.Failed++
			default:
				result.Skipped++
			}
			continue
		}
		result.Attempted++
		sent, sendErr := a.sendClaimedTask(ctx, qb, task, item.torrent, item.data, item.metadata, item.plan)
		result.Tasks = append(result.Tasks, downloadTaskFromRecord(sent))
		if sendErr != nil && sent.Status == "skipped" {
			result.Skipped++
		} else if sendErr != nil {
			result.Failed++
		} else if sent.Status == "sent" {
			result.Sent++
		} else {
			result.Exists++
		}
		if ctx.Err() != nil {
			return finishRun(result, ctx.Err())
		}
	}
	return finishRun(result, nil)
}

// RetryDownloadTask 使用已保存的计划快照重试真实发送失败任务。
func (a *App) RetryDownloadTask(ctx context.Context, id string) (DownloadTask, error) {
	task, ok, err := a.store.GetDownloadTask(ctx, strings.TrimSpace(id))
	if err != nil {
		return DownloadTask{}, err
	}
	if !ok {
		return DownloadTask{}, fmt.Errorf("download task %q not found", id)
	}
	if task.Status != "failed" && !(task.Status == "pending" && task.RetryCount > 0) {
		return DownloadTask{}, fmt.Errorf("download task %q is not failed", id)
	}
	torrent, err := a.getTorrent(ctx, task.SiteID, task.TorrentID)
	if err != nil {
		return DownloadTask{}, err
	}
	data, metadata, err := a.loadOrFetchTorrentMetadata(ctx, torrent)
	if err != nil {
		return DownloadTask{}, err
	}
	plan := DownloadPlan{Category: task.PlanCategory, SavePath: task.PlanSavePath, Tags: task.PlanTags, Rename: task.PlanRename, Paused: task.PlanPaused}
	if plan.SavePath != "" {
		autoTMM := false
		plan.AutoTMM = &autoTMM
	}
	item := preparedSubscriptionCandidate{record: storage.SubscriptionCandidateRecord{SiteID: task.SiteID, TorrentID: task.TorrentID, RuleName: task.RuleName}, torrent: torrent, data: data, metadata: metadata, plan: plan}
	qb, _, blockCode, err := a.prepareQBForPlans(ctx, plan.Category, []preparedSubscriptionCandidate{item})
	if err != nil {
		task.Status = "skipped"
		task.ReasonCode = blockCode
		task.Error = err.Error()
		updated, saveErr := a.store.UpdateDownloadTaskRecordIfStatus(ctx, task, []string{"failed", "pending"})
		if saveErr != nil {
			return DownloadTask{}, saveErr
		}
		if !updated {
			return DownloadTask{}, fmt.Errorf("download task %q changed while retry preflight was running", id)
		}
		if candidateErr := a.transitionRetryCandidate(ctx, task, storage.SubscriptionCandidateUnread, blockCode, err.Error()); candidateErr != nil {
			return downloadTaskFromRecord(task), candidateErr
		}
		return downloadTaskFromRecord(task), nil
	}
	claimed, claimedOK, err := a.store.ClaimDownloadTask(ctx, task.ID, []string{task.Status}, true)
	if err != nil {
		return DownloadTask{}, err
	}
	if !claimedOK {
		return DownloadTask{}, fmt.Errorf("download task %q is already being retried", id)
	}
	result, sendErr := a.sendClaimedTask(ctx, qb, claimed, torrent, data, metadata, plan)
	if sendErr == nil {
		if candidateErr := a.transitionRetryCandidate(ctx, result, storage.SubscriptionCandidateProcessed, result.Status, ""); candidateErr != nil {
			return downloadTaskFromRecord(result), candidateErr
		}
		return downloadTaskFromRecord(result), nil
	}
	if result.Status == "skipped" {
		if candidateErr := a.transitionRetryCandidate(ctx, result, storage.SubscriptionCandidateUnread, result.ReasonCode, result.Error); candidateErr != nil {
			return downloadTaskFromRecord(result), candidateErr
		}
		return downloadTaskFromRecord(result), nil
	}
	if result.Status == "pending" {
		result.Status = "failed"
		result.ReasonCode = "retry_interrupted"
		result.Error = sendErr.Error()
		_, _ = a.store.UpdateDownloadTaskRecordIfStatus(context.WithoutCancel(ctx), result, []string{"pending"})
	}
	return downloadTaskFromRecord(result), sendErr
}

func (a *App) transitionRetryCandidate(ctx context.Context, task storage.DownloadTaskRecord, status, reasonCode, errText string) error {
	key := storage.TorrentKey{SiteID: task.SiteID, TorrentID: task.TorrentID}
	candidate, ok, err := a.store.GetSubscriptionCandidate(ctx, key)
	if err != nil || !ok {
		return err
	}
	if candidate.Status != storage.SubscriptionCandidateFailed {
		return nil
	}
	return a.transitionCandidate(ctx, key, storage.SubscriptionCandidateFailed, status, reasonCode, errText)
}

func (a *App) resolveBatchPlanContext(ctx context.Context, request BatchDownloadRequest) (batchPlanContext, error) {
	hasSubscription := strings.TrimSpace(request.SubscriptionID) != ""
	hasOptions := request.Options != nil
	if hasSubscription == hasOptions {
		return batchPlanContext{}, fmt.Errorf("exactly one of subscription_id or options is required")
	}
	if hasSubscription {
		subscription, rule, err := a.loadSubscriptionAndRule(ctx, request.SubscriptionID)
		if err != nil {
			return batchPlanContext{}, err
		}
		return batchPlanContext{subscription: subscription, rule: rule, options: subscription.Download, subscriptionName: subscription.Name}, nil
	}
	options := *request.Options
	dummy := Subscription{ID: "manual-batch", Name: "manual-batch", Enabled: true, RuleName: "manual", SiteIDs: []string{"manual"}, Download: options}
	if err := ValidateSubscription(dummy); err != nil {
		return batchPlanContext{}, err
	}
	return batchPlanContext{
		subscription: Subscription{}, rule: Rule{Name: "manual", SortBy: defaultRuleSortBy, SortDirection: defaultRuleSortDirection},
		options: options, subscriptionName: "manual-batch",
	}, nil
}

func validateBatchTorrentKeys(keys []TorrentKey) ([]TorrentKey, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("at least one torrent is required")
	}
	if len(keys) > storage.MaxTorrentListLimit {
		return nil, fmt.Errorf("too many torrents")
	}
	seen := map[string]struct{}{}
	result := make([]TorrentKey, 0, len(keys))
	for _, key := range keys {
		key.SiteID = strings.TrimSpace(key.SiteID)
		key.TorrentID = strings.TrimSpace(key.TorrentID)
		if key.SiteID == "" || key.TorrentID == "" {
			return nil, fmt.Errorf("torrent site_id and torrent_id are required")
		}
		id := strings.ToLower(key.SiteID + "\x00" + key.TorrentID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, key)
	}
	return result, nil
}

func (a *App) saveBatchTerminalTask(ctx context.Context, planContext batchPlanContext, torrent Torrent, plan DownloadPlan, status, code string, cause error) (storage.DownloadTaskRecord, error) {
	item := preparedSubscriptionCandidate{
		record:  storage.SubscriptionCandidateRecord{SiteID: torrent.SiteID, TorrentID: torrent.ID, RuleName: planContext.rule.Name},
		torrent: torrent, plan: plan,
	}
	task := plannedTaskRecord(item, planContext.subscription, "manual-batch", status, code, cause.Error())
	saved, _, err := a.persistTaskWithoutClaim(ctx, task)
	if err != nil {
		return storage.DownloadTaskRecord{}, err
	}
	return saved, nil
}

func appendBatchTerminalOutcome(result *BatchDownloadResult, task storage.DownloadTaskRecord) {
	result.Tasks = append(result.Tasks, downloadTaskFromRecord(task))
	switch task.Status {
	case "sent":
		result.Sent++
	case "exists":
		result.Exists++
	case "failed":
		result.Failed++
	default:
		result.Skipped++
	}
}
