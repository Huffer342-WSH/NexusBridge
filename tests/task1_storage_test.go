// task1_storage_test.go 验证订阅持久化约束。
package tests

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"nexusbridge/internal/storage"
)

// TestTask1SubscriptionStorageSemantics 验证订阅优先级、独占候选、未读恢复和任务幂等。
func TestTask1SubscriptionStorageSemantics(t *testing.T) {
	store, err := storage.OpenSQLite(t.Context(), filepath.Join(t.TempDir(), "subscriptions.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	rule := storage.RuleRecord{
		Name: "Rule", SiteIDs: []string{"test-site"}, SiteCategories: []string{"anime"},
		SubtitleTags: []string{"4k"}, SiteTags: []string{"tag-1"}, TitleExpression: "wanted&!blocked",
		Promotions: []string{"pro_free"}, MinSize: 1, MaxSize: 2,
		MinSeeders: 3, MaxSeeders: 4, MinLeechers: 5, MaxLeechers: 6, MinSnatches: 7, MaxSnatches: 8,
		PublishedWithinMinutes: 90, SortBy: "seeders", SortDirection: "desc", Action: "download",
	}
	if err := store.SaveRule(t.Context(), rule); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	storedRule, ok, err := store.GetRule(t.Context(), rule.Name)
	if err != nil || !ok {
		t.Fatalf("get rule: ok=%t err=%v", ok, err)
	}
	if !reflect.DeepEqual(storedRule.SiteCategories, rule.SiteCategories) || !reflect.DeepEqual(storedRule.SiteTags, rule.SiteTags) ||
		!reflect.DeepEqual(storedRule.SubtitleTags, rule.SubtitleTags) || !reflect.DeepEqual(storedRule.Promotions, rule.Promotions) ||
		storedRule.MaxSeeders != 4 || storedRule.MinLeechers != 5 || storedRule.MaxLeechers != 6 ||
		storedRule.MinSnatches != 7 || storedRule.MaxSnatches != 8 || storedRule.PublishedWithinMinutes != 90 ||
		storedRule.SortBy != "seeders" || storedRule.SortDirection != "desc" {
		t.Fatalf("rule fields did not round trip: %#v", storedRule)
	}

	low := storage.SubscriptionRecord{
		ID: "low", Name: "Low", RuleName: rule.Name, Enabled: true, SiteIDs: []string{"test-site"}, Priority: 10,
		QBCategory: "PT/Low", SavePathTemplate: "D:/Low/{{title}}", QBTags: []string{"low"},
		FilenameTemplate: "{{title}}", Paused: true, MaxConcurrent: 1, DailyLimit: 2,
	}
	high := low
	high.ID, high.Name, high.Priority, high.QBCategory = "high", "High", 100, "PT/High"
	for _, subscription := range []storage.SubscriptionRecord{low, high} {
		if err := store.SaveSubscription(t.Context(), subscription); err != nil {
			t.Fatalf("save subscription %s: %v", subscription.ID, err)
		}
	}
	enabled, err := store.ListEnabledSubscriptionsBySite(t.Context(), "test-site")
	if err != nil {
		t.Fatalf("list enabled subscriptions: %v", err)
	}
	if len(enabled) != 2 || enabled[0].ID != "high" || enabled[1].ID != "low" {
		t.Fatalf("subscriptions must sort priority DESC, id ASC: %#v", enabled)
	}
	storedSubscription, ok, err := store.GetSubscription(t.Context(), "high")
	if err != nil || !ok {
		t.Fatalf("get subscription: ok=%t err=%v", ok, err)
	}
	if storedSubscription.QBCategory != "PT/High" || !reflect.DeepEqual(storedSubscription.QBTags, []string{"low"}) ||
		storedSubscription.SavePathTemplate != low.SavePathTemplate || storedSubscription.FilenameTemplate != low.FilenameTemplate ||
		!storedSubscription.Paused || storedSubscription.MaxConcurrent != 1 || storedSubscription.DailyLimit != 2 {
		t.Fatalf("subscription fields did not round trip: %#v", storedSubscription)
	}

	key := storage.TorrentKey{SiteID: "test-site", TorrentID: "1"}
	created, err := store.CreateSubscriptionCandidateIfAbsent(t.Context(), storage.SubscriptionCandidateRecord{
		SiteID: key.SiteID, TorrentID: key.TorrentID, SubscriptionID: "high", RuleName: rule.Name, SourceOrder: 5,
	})
	if err != nil || !created {
		t.Fatalf("create first candidate: created=%t err=%v", created, err)
	}
	created, err = store.CreateSubscriptionCandidateIfAbsent(t.Context(), storage.SubscriptionCandidateRecord{
		SiteID: key.SiteID, TorrentID: key.TorrentID, SubscriptionID: "low", RuleName: rule.Name, SourceOrder: 5,
	})
	if err != nil || created {
		t.Fatalf("lower priority subscription must not steal candidate: created=%t err=%v", created, err)
	}
	candidate, ok, err := store.GetSubscriptionCandidate(t.Context(), key)
	if err != nil || !ok || candidate.SubscriptionID != "high" || candidate.Status != storage.SubscriptionCandidateUnread {
		t.Fatalf("unexpected exclusive candidate: ok=%t err=%v candidate=%#v", ok, err, candidate)
	}
	claimed, err := store.ClaimSubscriptionCandidates(t.Context(), "high", 10)
	if err != nil || len(claimed) != 1 || claimed[0].Status != storage.SubscriptionCandidateProcessing {
		t.Fatalf("claim unread candidate: %#v err=%v", claimed, err)
	}
	updated, err := store.UpdateSubscriptionCandidateStatus(t.Context(), key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateUnread, "daily_limit", "quota reached")
	if err != nil || !updated {
		t.Fatalf("return quota-blocked candidate to unread: updated=%t err=%v", updated, err)
	}
	unread, err := store.ListSubscriptionCandidates(t.Context(), storage.SubscriptionCandidateQuery{SubscriptionID: "high", Status: storage.SubscriptionCandidateUnread, Limit: 10})
	if err != nil || len(unread) != 1 || unread[0].ReasonCode != "daily_limit" {
		t.Fatalf("quota-blocked candidate must remain unread with reason: %#v err=%v", unread, err)
	}
	claimed, err = store.ClaimSubscriptionCandidates(t.Context(), "high", 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim unread candidate: %#v err=%v", claimed, err)
	}
	updated, err = store.UpdateSubscriptionCandidateStatus(t.Context(), key, storage.SubscriptionCandidateProcessing, storage.SubscriptionCandidateProcessed, "sent", "")
	if err != nil || !updated {
		t.Fatalf("complete candidate: updated=%t err=%v", updated, err)
	}
	candidate, ok, err = store.GetSubscriptionCandidate(t.Context(), key)
	if err != nil || !ok || candidate.Status != storage.SubscriptionCandidateProcessed || candidate.ProcessedAt.IsZero() {
		t.Fatalf("processed candidate must keep completion time: ok=%t err=%v candidate=%#v", ok, err, candidate)
	}

	firstTask := storage.DownloadTaskRecord{
		ID: "task-1", SiteID: key.SiteID, TorrentID: key.TorrentID, RuleName: rule.Name, SubscriptionID: "high",
		Trigger: "scheduled", Status: "pending", TorrentTitle: "Title", DownloadURL: "https://example.invalid/1",
		PlanCategory: "PT/High", PlanSavePath: "D:/High", PlanTags: []string{"one", "two"}, PlanRename: "Title", PlanPaused: true,
	}
	created, err = store.CreateDownloadTaskIfAbsent(t.Context(), firstTask)
	if err != nil || !created {
		t.Fatalf("create download task: created=%t err=%v", created, err)
	}
	duplicate := firstTask
	duplicate.ID = "different-id"
	created, err = store.CreateDownloadTaskIfAbsent(t.Context(), duplicate)
	if err != nil || created {
		t.Fatalf("site_id+torrent_id+rule_name must be unique: created=%t err=%v", created, err)
	}
	claimedTask, ok, err := store.ClaimDownloadTask(t.Context(), firstTask.ID, []string{"pending"}, false)
	if err != nil || !ok || claimedTask.Status != "processing" || claimedTask.AttemptCount != 1 || claimedTask.RetryCount != 0 {
		t.Fatalf("unexpected first task claim: ok=%t err=%v task=%#v", ok, err, claimedTask)
	}
	if claimedTask.PlanCategory != firstTask.PlanCategory || claimedTask.PlanSavePath != firstTask.PlanSavePath ||
		!reflect.DeepEqual(claimedTask.PlanTags, firstTask.PlanTags) || claimedTask.PlanRename != firstTask.PlanRename || !claimedTask.PlanPaused {
		t.Fatalf("claim must preserve the planned qB snapshot: %#v", claimedTask)
	}
	claimedTask.Status = "failed"
	claimedTask.Error = "failure"
	claimedTask.ReasonCode = "send_failed"
	if err := store.UpdateDownloadTaskRecord(t.Context(), claimedTask); err != nil {
		t.Fatalf("save failed task: %v", err)
	}
	retriedTask, ok, err := store.ClaimDownloadTask(t.Context(), firstTask.ID, []string{"failed"}, true)
	if err != nil || !ok || retriedTask.AttemptCount != 2 || retriedTask.RetryCount != 1 || retriedTask.Status != "processing" {
		t.Fatalf("unexpected retry claim: ok=%t err=%v task=%#v", ok, err, retriedTask)
	}
	if retriedTask.PlanCategory != firstTask.PlanCategory || !reflect.DeepEqual(retriedTask.PlanTags, firstTask.PlanTags) {
		t.Fatalf("retry must reuse the saved plan snapshot: %#v", retriedTask)
	}
	retriedTask.Status = "sent"
	retriedTask.SentAt = time.Now()
	if err := store.UpdateDownloadTaskRecord(t.Context(), retriedTask); err != nil {
		t.Fatalf("save sent task: %v", err)
	}
	sentCount, err := store.CountSentDownloadTasksSince(t.Context(), "high", retriedTask.SentAt.Add(-time.Second))
	if err != nil || sentCount != 1 {
		t.Fatalf("count sent tasks since local-day boundary: count=%d err=%v", sentCount, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.ReplaceQBCategories(t.Context(), []storage.QBCategoryRecord{{Name: "PT/ASMR", SavePath: "D:/ASMR"}}, now); err != nil {
		t.Fatalf("replace qB categories: %v", err)
	}
	if err := store.ReplaceQBTags(t.Context(), []storage.QBTagRecord{{Name: "anime"}, {Name: "planned"}}, now); err != nil {
		t.Fatalf("replace qB tags: %v", err)
	}
	categories, err := store.ListQBCategories(t.Context())
	if err != nil || len(categories) != 1 || categories[0].Name != "PT/ASMR" || categories[0].SavePath != "D:/ASMR" {
		t.Fatalf("unexpected category snapshot: %#v err=%v", categories, err)
	}
	tags, err := store.ListQBTags(t.Context())
	if err != nil || len(tags) != 2 || tags[0].Name != "anime" || tags[1].Name != "planned" {
		t.Fatalf("unexpected tag snapshot: %#v err=%v", tags, err)
	}
	categoryState, ok, err := store.GetQBCacheState(t.Context(), storage.QBCacheKindCategories)
	if err != nil || !ok || categoryState.SyncedAt.IsZero() || categoryState.LastError != "" {
		t.Fatalf("unexpected category cache state: ok=%t err=%v state=%#v", ok, err, categoryState)
	}
	if err := store.MarkQBCategoriesSyncError(t.Context(), "offline"); err != nil {
		t.Fatalf("mark category sync error: %v", err)
	}
	categoryState, ok, err = store.GetQBCacheState(t.Context(), storage.QBCacheKindCategories)
	if err != nil || !ok || categoryState.LastError != "offline" || categoryState.SyncedAt.IsZero() {
		t.Fatalf("sync error must retain the last successful snapshot: ok=%t err=%v state=%#v", ok, err, categoryState)
	}

	dueAt := now.Add(-time.Minute)
	lastRunAt := now.Add(-2 * time.Minute)
	if err := store.SaveSiteSchedule(t.Context(), storage.SiteScheduleRecord{
		SiteID: "test-site", Enabled: true, IntervalMinutes: 15, LastRunAt: lastRunAt, NextRunAt: dueAt, LastError: "previous error",
	}); err != nil {
		t.Fatalf("save site schedule: %v", err)
	}
	storedSchedule, ok, err := store.GetSiteSchedule(t.Context(), "test-site")
	if err != nil || !ok || !storedSchedule.Enabled || storedSchedule.IntervalMinutes != 15 ||
		!storedSchedule.LastRunAt.Equal(lastRunAt) || !storedSchedule.NextRunAt.Equal(dueAt) || storedSchedule.LastError != "previous error" {
		t.Fatalf("site schedule did not round trip: ok=%t err=%v schedule=%#v", ok, err, storedSchedule)
	}
	due, err := store.ListDueSiteSchedules(t.Context(), now, 10)
	if err != nil || len(due) != 1 || due[0].SiteID != "test-site" {
		t.Fatalf("unexpected due schedule list: %#v err=%v", due, err)
	}
	nextRunAt := now.Add(15 * time.Minute)
	claimedSchedule, err := store.ClaimDueSiteSchedule(t.Context(), "test-site", now, nextRunAt)
	if err != nil || !claimedSchedule {
		t.Fatalf("claim due schedule: claimed=%t err=%v", claimedSchedule, err)
	}
	claimedSchedule, err = store.ClaimDueSiteSchedule(t.Context(), "test-site", now, nextRunAt)
	if err != nil || claimedSchedule {
		t.Fatalf("claimed schedule must not be claimed again: claimed=%t err=%v", claimedSchedule, err)
	}

	run := storage.SubscriptionRunRecord{
		ID: "run-1", SubscriptionID: "high", SiteID: "test-site", Trigger: "scheduled", Status: "completed",
		FetchedCount: 10, InsertedCount: 4, MatchedCount: 3, AttemptedCount: 2, SentCount: 1,
		ExistsCount: 1, FailedCount: 0, SkippedCount: 1, StartedAt: now, FinishedAt: now.Add(time.Second),
	}
	if err := store.SaveSubscriptionRun(t.Context(), run); err != nil {
		t.Fatalf("save subscription run: %v", err)
	}
	storedRun, ok, err := store.GetSubscriptionRun(t.Context(), run.ID)
	if err != nil || !ok || storedRun.InsertedCount != 4 || storedRun.AttemptedCount != 2 || storedRun.SentCount != 1 || storedRun.ExistsCount != 1 {
		t.Fatalf("subscription run fields did not round trip: ok=%t err=%v run=%#v", ok, err, storedRun)
	}

	requeueKey := storage.TorrentKey{SiteID: "test-site", TorrentID: "requeue"}
	created, err = store.CreateSubscriptionCandidateIfAbsent(t.Context(), storage.SubscriptionCandidateRecord{
		SiteID: requeueKey.SiteID, TorrentID: requeueKey.TorrentID, SubscriptionID: "high", RuleName: rule.Name,
		SourceOrder: 9, Status: storage.SubscriptionCandidateFailed,
	})
	if err != nil || !created {
		t.Fatalf("create failed candidate for requeue: created=%t err=%v", created, err)
	}
	high.Name = "High Updated"
	if err := store.SaveSubscriptionAndRequeueCandidates(t.Context(), high); err != nil {
		t.Fatalf("save subscription with atomic candidate requeue: %v", err)
	}
	if _, ok, err := store.GetSubscriptionCandidate(t.Context(), requeueKey); err != nil || ok {
		t.Fatalf("requeued candidate must leave exclusive candidate table: ok=%t err=%v", ok, err)
	}
	pendingIngest, err := store.ListPendingSubscriptionIngest(t.Context(), "test-site", 10)
	if err != nil || len(pendingIngest) != 1 || pendingIngest[0].TorrentID != requeueKey.TorrentID || pendingIngest[0].SourceOrder != 9 {
		t.Fatalf("candidate requeue must be durable: %#v err=%v", pendingIngest, err)
	}

	autoKey := storage.TorrentKey{SiteID: "test-site", TorrentID: "auto-interrupted"}
	if created, err := store.CreateSubscriptionCandidateIfAbsent(t.Context(), storage.SubscriptionCandidateRecord{
		SiteID: autoKey.SiteID, TorrentID: autoKey.TorrentID, SubscriptionID: "high", RuleName: rule.Name,
		Status: storage.SubscriptionCandidateProcessing,
	}); err != nil || !created {
		t.Fatalf("create interrupted automatic candidate: created=%t err=%v", created, err)
	}
	autoTask := storage.DownloadTaskRecord{ID: "auto-interrupted", SiteID: autoKey.SiteID, TorrentID: autoKey.TorrentID,
		RuleName: rule.Name, SubscriptionID: "high", Status: "processing", TorrentTitle: "auto", DownloadURL: "https://example.invalid/auto"}
	if created, err := store.CreateDownloadTaskIfAbsent(t.Context(), autoTask); err != nil || !created {
		t.Fatalf("create interrupted automatic task: created=%t err=%v", created, err)
	}
	retryKey := storage.TorrentKey{SiteID: "test-site", TorrentID: "retry-interrupted"}
	if created, err := store.CreateSubscriptionCandidateIfAbsent(t.Context(), storage.SubscriptionCandidateRecord{
		SiteID: retryKey.SiteID, TorrentID: retryKey.TorrentID, SubscriptionID: "high", RuleName: rule.Name,
		Status: storage.SubscriptionCandidateFailed,
	}); err != nil || !created {
		t.Fatalf("create interrupted retry candidate: created=%t err=%v", created, err)
	}
	retryTask := storage.DownloadTaskRecord{ID: "retry-interrupted", SiteID: retryKey.SiteID, TorrentID: retryKey.TorrentID,
		RuleName: rule.Name, SubscriptionID: "high", Status: "processing", RetryCount: 1,
		TorrentTitle: "retry", DownloadURL: "https://example.invalid/retry"}
	if created, err := store.CreateDownloadTaskIfAbsent(t.Context(), retryTask); err != nil || !created {
		t.Fatalf("create interrupted retry task: created=%t err=%v", created, err)
	}
	if err := store.RecoverInterruptedSubscriptionWork(t.Context()); err != nil {
		t.Fatalf("recover interrupted work: %v", err)
	}
	recoveredAutoTask, _, err := store.GetDownloadTask(t.Context(), autoTask.ID)
	if err != nil || recoveredAutoTask.Status != "pending" {
		t.Fatalf("automatic claim must recover to pending: task=%#v err=%v", recoveredAutoTask, err)
	}
	recoveredAutoCandidate, _, err := store.GetSubscriptionCandidate(t.Context(), autoKey)
	if err != nil || recoveredAutoCandidate.Status != storage.SubscriptionCandidateUnread {
		t.Fatalf("automatic candidate must recover to unread: candidate=%#v err=%v", recoveredAutoCandidate, err)
	}
	recoveredRetryTask, _, err := store.GetDownloadTask(t.Context(), retryTask.ID)
	if err != nil || recoveredRetryTask.Status != "failed" {
		t.Fatalf("explicit retry claim must remain retryable after recovery: task=%#v err=%v", recoveredRetryTask, err)
	}
	renamedRule := rule
	renamedRule.Name = "Rule Renamed"
	if err := store.RenameRule(t.Context(), rule.Name, renamedRule); err != nil {
		t.Fatalf("rename rule with cascade: %v", err)
	}
	if _, ok, err := store.GetRule(t.Context(), rule.Name); err != nil || ok {
		t.Fatalf("old rule name must no longer resolve: ok=%t err=%v", ok, err)
	}
	renamedSubscription, ok, err := store.GetSubscription(t.Context(), "high")
	if err != nil || !ok || renamedSubscription.RuleName != renamedRule.Name {
		t.Fatalf("subscription rule name was not cascaded: ok=%t err=%v subscription=%#v", ok, err, renamedSubscription)
	}
	renamedCandidate, ok, err := store.GetSubscriptionCandidate(t.Context(), autoKey)
	if err != nil || !ok || renamedCandidate.RuleName != renamedRule.Name {
		t.Fatalf("candidate rule name was not cascaded: ok=%t err=%v candidate=%#v", ok, err, renamedCandidate)
	}
	renamedTask, ok, err := store.GetDownloadTask(t.Context(), autoTask.ID)
	if err != nil || !ok || renamedTask.RuleName != renamedRule.Name {
		t.Fatalf("download task rule name was not cascaded: ok=%t err=%v task=%#v", ok, err, renamedTask)
	}
}
