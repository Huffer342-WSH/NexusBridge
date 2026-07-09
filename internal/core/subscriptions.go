package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

const (
	defaultSiteScheduleIntervalSeconds = 15 * 60
	minSiteScheduleIntervalSeconds     = 60
	maxSiteScheduleIntervalSeconds     = 24 * 60 * 60
)

// DeleteRule 删除未被订阅引用的筛选规则。
func (a *App) DeleteRule(ctx context.Context, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("rule name is required")
	}
	subscriptions, err := a.store.ListSubscriptions(ctx)
	if err != nil {
		return false, err
	}
	for _, subscription := range subscriptions {
		if strings.EqualFold(subscription.RuleName, name) {
			return false, fmt.Errorf("rule %q is referenced by subscription %q", name, subscription.ID)
		}
	}
	return a.store.DeleteRule(ctx, name)
}

// ListSubscriptions 返回全部订阅配置。
func (a *App) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	records, err := a.store.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Subscription, 0, len(records))
	for _, record := range records {
		result = append(result, subscriptionFromRecord(record))
	}
	return result, nil
}

// SaveSubscription 校验并保存订阅配置。
func (a *App) SaveSubscription(ctx context.Context, subscription Subscription) (Subscription, error) {
	subscription = NormalizeSubscription(subscription)
	if err := ValidateSubscription(subscription); err != nil {
		return Subscription{}, err
	}
	ruleRecord, ok, err := a.store.GetRule(ctx, subscription.RuleName)
	if err != nil {
		return Subscription{}, err
	}
	if !ok {
		return Subscription{}, fmt.Errorf("rule %q not found", subscription.RuleName)
	}
	lock := a.subscriptionMutex(subscription.ID)
	if err := lock.Lock(ctx); err != nil {
		return Subscription{}, err
	}
	defer lock.Unlock()
	previous, hadPrevious, err := a.store.GetSubscription(ctx, subscription.ID)
	if err != nil {
		return Subscription{}, err
	}
	rule := ruleFromRecord(ruleRecord)
	for _, siteID := range subscription.SiteIDs {
		if _, err := a.findSite(siteID); err != nil {
			return Subscription{}, err
		}
		if len(rule.SiteIDs) > 0 && !containsFold(rule.SiteIDs, siteID) {
			return Subscription{}, fmt.Errorf("subscription site %q is outside rule site_ids", siteID)
		}
	}
	reassign := hadPrevious && (!strings.EqualFold(previous.RuleName, subscription.RuleName) || !sameStringSetFold(previous.SiteIDs, subscription.SiteIDs))
	if reassign {
		if err := a.store.SaveSubscriptionAndRequeueCandidates(ctx, subscriptionToRecord(subscription)); err != nil {
			return Subscription{}, err
		}
		if err := a.reassignPendingSites(ctx, previous.SiteIDs); err != nil {
			return Subscription{}, err
		}
	} else if err := a.store.SaveSubscription(ctx, subscriptionToRecord(subscription)); err != nil {
		return Subscription{}, err
	}
	record, _, err := a.store.GetSubscription(ctx, subscription.ID)
	if err != nil {
		return Subscription{}, err
	}
	a.wakeAutomation()
	return subscriptionFromRecord(record), nil
}

func (a *App) reassignPendingSites(ctx context.Context, siteIDs []string) error {
	seen := map[string]struct{}{}
	for _, siteID := range siteIDs {
		if _, ok := seen[siteID]; ok {
			continue
		}
		seen[siteID] = struct{}{}
		subscriptions, err := a.store.ListEnabledSubscriptionsBySite(ctx, siteID)
		if err != nil {
			return err
		}
		if _, err := a.assignPendingSiteTorrents(ctx, siteID, subscriptions); err != nil {
			return err
		}
	}
	return nil
}

func sameStringSetFold(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, value := range left {
		if !containsFold(right, value) {
			return false
		}
	}
	return true
}

// DeleteSubscription 删除订阅，保留历史任务并释放尚未处理的候选。
func (a *App) DeleteSubscription(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	lock := a.subscriptionMutex(id)
	if err := lock.Lock(ctx); err != nil {
		return false, err
	}
	defer lock.Unlock()
	previous, ok, err := a.store.GetSubscription(ctx, id)
	if err != nil || !ok {
		return false, err
	}
	deleted, err := a.store.DeleteSubscriptionAndRequeueCandidates(ctx, id)
	if err != nil || !deleted {
		return deleted, err
	}
	if err := a.reassignPendingSites(ctx, previous.SiteIDs); err != nil {
		return true, err
	}
	a.wakeAutomation()
	return true, nil
}

// PreviewSubscription 只读计算订阅命中和最终 qB 下载计划。
func (a *App) PreviewSubscription(ctx context.Context, id string, limit int) (SubscriptionPreview, error) {
	subscription, rule, err := a.loadSubscriptionAndRule(ctx, id)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	if limit <= 0 || limit > storage.MaxTorrentListLimit {
		limit = 100
	}
	torrents, err := a.listSubscriptionTorrents(ctx, subscription, rule, limit)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	SortTorrentsForRule(torrents, rule)
	if len(torrents) > limit {
		torrents = torrents[:limit]
	}
	keys := make([]storage.TorrentKey, 0, len(torrents))
	for _, torrent := range torrents {
		keys = append(keys, storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID})
	}
	metadata, err := a.store.ListTorrentFileMetadata(ctx, keys)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	categoryNames := map[string]struct{}{}
	qb, qbErr := a.qbClient(ctx)
	var qbTorrents []qbittorrent.TorrentInfo
	if qbErr == nil {
		categories, err := qb.GetCategories(ctx)
		if err != nil {
			qbErr = err
		} else {
			for name := range categories {
				categoryNames[strings.TrimSpace(name)] = struct{}{}
			}
		}
	}
	if qbErr == nil {
		qbTorrents, qbErr = qb.ListTorrents(ctx)
	}
	now := time.Now()
	localMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dailySent, err := a.store.CountSentDownloadTasksSince(ctx, subscription.ID, localMidnight)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	active := 0
	if qbErr == nil {
		active, err = a.countActiveSubscriptionTasks(ctx, subscription.ID, qbTorrents)
		if err != nil {
			return SubscriptionPreview{}, err
		}
	}
	globalTags := a.effectiveQBConfig(ctx).Tags
	renameRegistry := newRenameRegistry(qbTorrents)
	result := SubscriptionPreview{SubscriptionID: subscription.ID, Evaluated: len(torrents), Items: make([]SubscriptionPreviewItem, 0, len(torrents))}
	for _, torrent := range torrents {
		match := EvaluateRule(rule, torrent, now)
		item := SubscriptionPreviewItem{Torrent: torrent, Matched: match.Matched, Eligible: match.Matched, Reasons: append([]RuleReason{}, match.Reasons...)}
		if match.Matched {
			result.Matched++
			key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
			file := metadata[key]
			site := a.sites[torrent.SiteID]
			plan, planErr := BuildDownloadPlan(rule, subscription.Name, site.Name, subscription.Download, torrent, file.OriginalName, globalTags)
			if planErr != nil {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "plan_invalid", Message: planErr.Error()})
			} else {
				item.Plan = plan
			}
			if !file.HasData {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "torrent_file_unavailable", Message: "torrent 文件尚未缓存"})
			} else if file.InfoHashV1 == "" && file.InfoHashV2 != "" {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "pure_v2_unsupported", Message: "pure v2 torrent 暂不支持精确验证"})
			}
			if !subscription.Enabled {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "subscription_disabled", Field: "enabled", Message: "订阅未启用"})
			}
			if qbErr != nil {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "qb_unavailable", Message: qbErr.Error()})
			} else if _, ok := categoryNames[strings.TrimSpace(subscription.Download.QBCategory)]; !ok {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "qb_category_missing", Field: "qb_category", Message: "qB 分类不存在"})
			}
			if qbErr == nil && renameRegistry.adjust(&item.Plan, firstNonEmpty(file.InfoHashV1, file.InfoHashV2, torrent.SiteID+":"+torrent.ID), torrent.SiteID, torrent.ID) {
				item.Plan.Reasons = append(item.Plan.Reasons, RuleReason{Code: "rename_conflict_adjusted", Field: "filename_template", Message: "已为同名不同 hash 任务追加稳定后缀"})
			}
			_, alreadyInQB := findQBTorrentByHash(firstNonEmpty(file.InfoHashV1, file.InfoHashV2), qbTorrents)
			if item.Eligible && !alreadyInQB && subscription.Download.DailyLimit > 0 && dailySent >= subscription.Download.DailyLimit {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "quota_daily", Message: "订阅当日发送配额已用尽"})
			}
			if item.Eligible && !alreadyInQB && subscription.Download.MaxConcurrent > 0 && active >= subscription.Download.MaxConcurrent {
				item.Eligible = false
				item.Reasons = append(item.Reasons, RuleReason{Code: "quota_concurrent", Message: "订阅并发配额已用尽"})
			}
			if item.Eligible {
				result.Eligible++
				if !alreadyInQB {
					dailySent++
					active++
				}
			}
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// ListSubscriptionCandidates 返回订阅候选队列。
func (a *App) ListSubscriptionCandidates(ctx context.Context, subscriptionID, status string) ([]SubscriptionCandidate, error) {
	records, err := a.store.ListSubscriptionCandidates(ctx, storage.SubscriptionCandidateQuery{SubscriptionID: subscriptionID, Status: status, Limit: storage.MaxTorrentListLimit})
	if err != nil {
		return nil, err
	}
	result := make([]SubscriptionCandidate, 0, len(records))
	for _, record := range records {
		result = append(result, candidateFromRecord(record))
	}
	return result, nil
}

// ListSubscriptionRuns 返回最近订阅运行记录。
func (a *App) ListSubscriptionRuns(ctx context.Context, subscriptionID string) ([]SubscriptionRun, error) {
	records, err := a.store.ListSubscriptionRuns(ctx, storage.SubscriptionRunQuery{SubscriptionID: subscriptionID, Limit: 200})
	if err != nil {
		return nil, err
	}
	result := make([]SubscriptionRun, 0, len(records))
	for _, record := range records {
		result = append(result, subscriptionRunFromRecord(record))
	}
	return result, nil
}

// GetSiteSchedule 返回站点周期配置，未保存时返回关闭的默认值。
func (a *App) GetSiteSchedule(ctx context.Context, siteID string) (SiteSchedule, error) {
	if _, err := a.findSite(siteID); err != nil {
		return SiteSchedule{}, err
	}
	record, ok, err := a.store.GetSiteSchedule(ctx, siteID)
	if err != nil {
		return SiteSchedule{}, err
	}
	if !ok {
		return SiteSchedule{SiteID: siteID, IntervalSeconds: defaultSiteScheduleIntervalSeconds}, nil
	}
	return siteScheduleFromRecord(record), nil
}

// SaveSiteSchedule 保存站点周期并唤醒后台调度器。
func (a *App) SaveSiteSchedule(ctx context.Context, schedule SiteSchedule) (SiteSchedule, error) {
	if _, err := a.findSite(schedule.SiteID); err != nil {
		return SiteSchedule{}, err
	}
	if schedule.IntervalSeconds == 0 {
		schedule.IntervalSeconds = defaultSiteScheduleIntervalSeconds
	}
	if schedule.IntervalSeconds < minSiteScheduleIntervalSeconds || schedule.IntervalSeconds > maxSiteScheduleIntervalSeconds {
		return SiteSchedule{}, fmt.Errorf("site schedule interval_seconds must be between 60 and 86400")
	}
	if schedule.IntervalSeconds%60 != 0 {
		return SiteSchedule{}, fmt.Errorf("site schedule interval_seconds must use whole-minute increments")
	}
	now := time.Now()
	record := storage.SiteScheduleRecord{
		SiteID: schedule.SiteID, Enabled: schedule.Enabled, IntervalMinutes: max(1, schedule.IntervalSeconds/60),
		LastError: schedule.LastError, NextRunAt: now.Add(time.Duration(schedule.IntervalSeconds) * time.Second),
	}
	if schedule.LastRunAt != nil {
		record.LastRunAt = *schedule.LastRunAt
	}
	if err := a.store.SaveSiteSchedule(ctx, record); err != nil {
		return SiteSchedule{}, err
	}
	a.wakeAutomation()
	return a.GetSiteSchedule(ctx, schedule.SiteID)
}

func (a *App) loadSubscriptionAndRule(ctx context.Context, id string) (Subscription, Rule, error) {
	record, ok, err := a.store.GetSubscription(ctx, strings.TrimSpace(id))
	if err != nil {
		return Subscription{}, Rule{}, err
	}
	if !ok {
		return Subscription{}, Rule{}, fmt.Errorf("subscription %q not found", id)
	}
	ruleRecord, ok, err := a.store.GetRule(ctx, record.RuleName)
	if err != nil {
		return Subscription{}, Rule{}, err
	}
	if !ok {
		return Subscription{}, Rule{}, fmt.Errorf("rule %q not found", record.RuleName)
	}
	return subscriptionFromRecord(record), ruleFromRecord(ruleRecord), nil
}

func (a *App) listSubscriptionTorrents(ctx context.Context, subscription Subscription, rule Rule, limit int) ([]Torrent, error) {
	result := make([]Torrent, 0, limit*len(subscription.SiteIDs))
	for _, siteID := range subscription.SiteIDs {
		items, err := a.ListTorrents(ctx, TorrentQuery{
			SiteID: siteID, SortBy: rule.SortBy, SortDirection: rule.SortDirection, Limit: limit,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	return result, nil
}

func subscriptionToRecord(subscription Subscription) storage.SubscriptionRecord {
	return storage.SubscriptionRecord{
		ID: subscription.ID, Name: subscription.Name, RuleName: subscription.RuleName, Enabled: subscription.Enabled,
		SiteIDs: subscription.SiteIDs, Priority: subscription.Priority, QBCategory: subscription.Download.QBCategory,
		SavePathTemplate: subscription.Download.SavePathTemplate, QBTags: subscription.Download.QBTags,
		FilenameTemplate: subscription.Download.FilenameTemplate, Paused: subscription.Download.Paused,
		MaxConcurrent: subscription.Download.MaxConcurrent, DailyLimit: subscription.Download.DailyLimit,
	}
}

func subscriptionFromRecord(record storage.SubscriptionRecord) Subscription {
	return Subscription{
		ID: record.ID, Name: record.Name, RuleName: record.RuleName, Enabled: record.Enabled, SiteIDs: record.SiteIDs,
		Priority: record.Priority, Download: DownloadOptions{QBCategory: record.QBCategory, SavePathTemplate: record.SavePathTemplate,
			QBTags: record.QBTags, FilenameTemplate: record.FilenameTemplate, Paused: record.Paused,
			MaxConcurrent: record.MaxConcurrent, DailyLimit: record.DailyLimit},
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func candidateFromRecord(record storage.SubscriptionCandidateRecord) SubscriptionCandidate {
	return SubscriptionCandidate{SiteID: record.SiteID, TorrentID: record.TorrentID, SubscriptionID: record.SubscriptionID,
		RuleName: record.RuleName, Status: record.Status, SourceOrder: record.SourceOrder, ReasonCode: record.ReasonCode,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func subscriptionRunFromRecord(record storage.SubscriptionRunRecord) SubscriptionRun {
	result := SubscriptionRun{ID: record.ID, SubscriptionID: record.SubscriptionID, SiteID: record.SiteID, Trigger: record.Trigger,
		Status: record.Status, Fetched: record.FetchedCount, Inserted: record.InsertedCount, Matched: record.MatchedCount,
		Attempted: record.AttemptedCount, Sent: record.SentCount,
		Exists: record.ExistsCount, Failed: record.FailedCount, Skipped: record.SkippedCount, Error: record.Error,
		StartedAt: record.StartedAt}
	if !record.FinishedAt.IsZero() {
		value := record.FinishedAt
		result.FinishedAt = &value
	}
	return result
}

func siteScheduleFromRecord(record storage.SiteScheduleRecord) SiteSchedule {
	result := SiteSchedule{SiteID: record.SiteID, Enabled: record.Enabled, IntervalSeconds: record.IntervalMinutes * 60,
		LastError: record.LastError, UpdatedAt: record.UpdatedAt}
	if !record.LastRunAt.IsZero() {
		value := record.LastRunAt
		result.LastRunAt = &value
	}
	if !record.NextRunAt.IsZero() {
		value := record.NextRunAt
		result.NextRunAt = &value
	}
	return result
}

type renameRegistry map[string]map[string]struct{}

func newRenameRegistry(torrents []qbittorrent.TorrentInfo) renameRegistry {
	registry := renameRegistry{}
	for _, torrent := range torrents {
		registry.add(torrent.Name, torrent.Hash)
	}
	return registry
}

func (r renameRegistry) add(name, hash string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}
	hash = strings.ToLower(strings.TrimSpace(hash))
	if _, ok := r[name]; !ok {
		r[name] = map[string]struct{}{}
	}
	r[name][hash] = struct{}{}
}

func (r renameRegistry) adjust(plan *DownloadPlan, ownHash, siteID, torrentID string) bool {
	if strings.TrimSpace(plan.Rename) == "" {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(plan.Rename))
	ownHash = strings.ToLower(strings.TrimSpace(ownHash))
	conflict := false
	for hash := range r[key] {
		if hash != ownHash {
			conflict = true
			break
		}
	}
	if conflict {
		plan.Rename = ResolveRenameConflict(plan.Rename, siteID, torrentID)
	}
	r.add(plan.Rename, ownHash)
	return conflict
}
