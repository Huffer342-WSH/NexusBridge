// task1_storage_test.go 验证任务 1 的新库结构和订阅持久化约束。
package tests

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"nexusbridge/internal/storage"
)

// TestTask1FreshDatabaseColumns 验证全库重建后的规则自然键和自动化表结构。
func TestTask1FreshDatabaseColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	_, err = db.ExecContext(t.Context(), `
CREATE TABLE torrents (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	title TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	category_query TEXT NOT NULL DEFAULT '',
	detail_url TEXT NOT NULL DEFAULT '',
	download_url TEXT NOT NULL DEFAULT '',
	cover_url TEXT NOT NULL DEFAULT '',
	tags_json TEXT NOT NULL DEFAULT '[]',
	tag_ids_json TEXT NOT NULL DEFAULT '[]',
	promotion TEXT NOT NULL DEFAULT '',
	promotion_class TEXT NOT NULL DEFAULT '',
	promotion_ends_at TEXT NOT NULL DEFAULT '',
	promotion_remaining TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	detail_title TEXT NOT NULL DEFAULT '',
	subtitle TEXT NOT NULL DEFAULT '',
	product_url TEXT NOT NULL DEFAULT '',
	detail_info_hash TEXT NOT NULL DEFAULT '',
	detail_description TEXT NOT NULL DEFAULT '',
	detail_raw_text TEXT NOT NULL DEFAULT '',
	detail_fetched_at TEXT NOT NULL DEFAULT '',
	size_text TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	seeders INTEGER NOT NULL DEFAULT 0,
	leechers INTEGER NOT NULL DEFAULT 0,
	snatches INTEGER NOT NULL DEFAULT 0,
	comments INTEGER NOT NULL DEFAULT 0,
	published_at TEXT NOT NULL DEFAULT '',
	published_text TEXT NOT NULL DEFAULT '',
	sticky_level INTEGER NOT NULL DEFAULT 0,
	bookmarked INTEGER NOT NULL DEFAULT 0,
	first_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);
CREATE TABLE torrent_files (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	data BLOB,
	info_hash_v1 TEXT NOT NULL DEFAULT '',
	info_hash_v2 TEXT NOT NULL DEFAULT '',
	byte_size INTEGER NOT NULL DEFAULT 0,
	fetched_at TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);
CREATE TABLE rules (
	name TEXT PRIMARY KEY COLLATE NOCASE,
	site_ids_json TEXT NOT NULL DEFAULT '[]',
	site_categories_json TEXT NOT NULL DEFAULT '[]',
	site_tags_json TEXT NOT NULL DEFAULT '[]',
	subtitle_tags_json TEXT NOT NULL DEFAULT '[]',
	title_expression TEXT NOT NULL DEFAULT '',
	promotions_json TEXT NOT NULL DEFAULT '[]',
	min_size INTEGER NOT NULL DEFAULT 0,
	max_size INTEGER NOT NULL DEFAULT 0,
	min_seeders INTEGER NOT NULL DEFAULT 0,
	max_seeders INTEGER NOT NULL DEFAULT 0,
	min_leechers INTEGER NOT NULL DEFAULT 0,
	max_leechers INTEGER NOT NULL DEFAULT 0,
	min_snatches INTEGER NOT NULL DEFAULT 0,
	max_snatches INTEGER NOT NULL DEFAULT 0,
	published_within_minutes INTEGER NOT NULL DEFAULT 0,
	sort_by TEXT NOT NULL DEFAULT 'source_order',
	sort_direction TEXT NOT NULL DEFAULT 'asc',
	action TEXT NOT NULL DEFAULT 'download',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE download_tasks (
	id TEXT PRIMARY KEY,
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	rule_name TEXT NOT NULL COLLATE NOCASE,
	status TEXT NOT NULL,
	torrent_title TEXT NOT NULL,
	download_url TEXT NOT NULL,
	qb_hash TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	content_path TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO torrents (site_id, torrent_id, title, category, tags_json)
VALUES ('legacy-site', '100', 'legacy torrent', 'movie', '["legacy"]');
INSERT INTO torrent_files (site_id, torrent_id, data, info_hash_v1, byte_size)
VALUES ('legacy-site', '100', X'010203', 'ABCDEF', 3);
INSERT INTO rules (name, site_ids_json, site_categories_json, subtitle_tags_json, title_expression, action)
VALUES ('Legacy Rule', '["legacy-site"]', '["movie"]', '["legacy"]', 'wanted', 'download');
INSERT INTO download_tasks (id, site_id, torrent_id, rule_name, status, torrent_title, download_url, qb_hash)
VALUES ('legacy-task', 'legacy-site', '100', 'Legacy Rule', 'sent', 'legacy torrent', 'https://example.invalid/100', 'ABCDEF');
`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy sqlite: %v", err)
	}

	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("migrate legacy sqlite: %v", err)
	}
	defer func() { _ = store.Close() }()
	legacyTorrent, ok, err := store.GetTorrent(t.Context(), "legacy-site", "100")
	if err != nil || !ok || legacyTorrent.Title != "legacy torrent" || legacyTorrent.SourceOrder != 0 {
		t.Fatalf("legacy torrent was not preserved: ok=%t err=%v torrent=%#v", ok, err, legacyTorrent)
	}
	legacyFile, ok, err := store.GetTorrentFile(t.Context(), storage.TorrentKey{SiteID: "legacy-site", TorrentID: "100"})
	if err != nil || !ok || !strings.EqualFold(legacyFile.InfoHashV1, "abcdef") || legacyFile.OriginalName != "" || !reflect.DeepEqual(legacyFile.Data, []byte{1, 2, 3}) {
		t.Fatalf("legacy torrent file was not preserved: ok=%t err=%v file=%#v", ok, err, legacyFile)
	}
	legacyRule, ok, err := store.GetRule(t.Context(), "legacy rule")
	if err != nil || !ok || legacyRule.Name != "Legacy Rule" || legacyRule.TitleExpression != "wanted" || legacyRule.SortBy != "source_order" {
		t.Fatalf("legacy rule was not preserved: ok=%t err=%v rule=%#v", ok, err, legacyRule)
	}
	legacyTask, ok, err := store.GetDownloadTask(t.Context(), "legacy-task")
	if err != nil || !ok || legacyTask.Status != "sent" || legacyTask.QBHash != "ABCDEF" || legacyTask.AttemptCount != 0 {
		t.Fatalf("legacy download task was not preserved: ok=%t err=%v task=%#v", ok, err, legacyTask)
	}
	duplicateLegacy := legacyTask
	duplicateLegacy.ID = "legacy-task-duplicate"
	created, err := store.CreateDownloadTaskIfAbsent(t.Context(), duplicateLegacy)
	if err != nil || created {
		t.Fatalf("migrated legacy table must enforce task triple idempotency: created=%t err=%v", created, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated sqlite: %v", err)
	}

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen migrated sqlite: %v", err)
	}
	defer db.Close()
	for table, columns := range map[string][]string{
		"torrents":       {"source_order"},
		"torrent_files":  {"original_name", "content_file_count", "content_total_size", "size_index_version", "size_indexed_at", "size_index_error"},
		"rules":          {"name", "site_categories_json", "site_tags_json", "subtitle_tags_json", "title_expression", "promotions_json", "max_seeders", "min_leechers", "max_leechers", "min_snatches", "max_snatches", "published_within_minutes", "sort_by", "sort_direction"},
		"download_tasks": {"subscription_id", "trigger", "plan_category", "plan_save_path", "plan_tags_json", "plan_rename", "plan_paused", "reason_code", "attempt_count", "retry_count", "last_attempt_at", "sent_at"},
	} {
		existing := task1TableColumns(t, db, table)
		for _, column := range columns {
			if _, ok := existing[column]; !ok {
				t.Fatalf("migration did not add %s.%s; columns=%v", table, column, existing)
			}
		}
	}
	for _, table := range []string{"subscriptions", "site_schedules", "subscription_candidates", "subscription_ingest_queue", "subscription_runs", "qb_categories", "qb_tags", "qb_cache_state", "torrent_file_size_index"} {
		if len(task1TableColumns(t, db, table)) == 0 {
			t.Fatalf("migration did not create %s", table)
		}
	}
}

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

func task1TableColumns(t *testing.T, db *sql.DB, table string) map[string]struct{} {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "PRAGMA table_info("+table+")")
	if err != nil {
		t.Fatalf("inspect %s columns: %v", table, err)
	}
	defer rows.Close()
	result := map[string]struct{}{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan %s columns: %v", table, err)
		}
		result[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s columns: %v", table, err)
	}
	return result
}
