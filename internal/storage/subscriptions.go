package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	// SubscriptionCandidateUnread 表示候选尚未执行。
	SubscriptionCandidateUnread = "unread"
	// SubscriptionCandidateProcessing 表示候选已被执行器领取。
	SubscriptionCandidateProcessing = "processing"
	// SubscriptionCandidateProcessed 表示候选已完成处理。
	SubscriptionCandidateProcessed = "processed"
	// SubscriptionCandidateFailed 表示候选执行失败且等待显式处理。
	SubscriptionCandidateFailed = "failed"
)

// RecoverInterruptedSubscriptionWork 将上次进程退出时未完成的领取恢复为可重试状态。
func (s *SQLiteStore) RecoverInterruptedSubscriptionWork(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `
UPDATE download_tasks
SET status = CASE
		WHEN retry_count > 0 AND EXISTS (
			SELECT 1 FROM subscription_candidates c
			WHERE c.site_id = download_tasks.site_id
				AND c.torrent_id = download_tasks.torrent_id
				AND c.status = 'failed'
		) THEN 'failed'
		ELSE 'pending'
	END,
	reason_code = 'execution_recovered',
	error = 'recovered after interrupted execution',
	updated_at = CURRENT_TIMESTAMP
WHERE status = 'processing'
`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_candidates
SET status = 'unread', reason_code = 'execution_recovered', error = 'recovered after interrupted execution', updated_at = CURRENT_TIMESTAMP
WHERE status = 'processing'
`); err != nil {
		return err
	}
	return tx.Commit()
}

// SubscriptionRecord 表示引用筛选规则的自动下载订阅。
type SubscriptionRecord struct {
	ID               string
	Name             string
	RuleName         string
	Enabled          bool
	SiteIDs          []string
	Priority         int
	QBCategory       string
	SavePathTemplate string
	QBTags           []string
	FilenameTemplate string
	Paused           bool
	MaxConcurrent    int
	DailyLimit       int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SiteScheduleRecord 表示单个站点的周期抓取配置和最近状态。
type SiteScheduleRecord struct {
	SiteID          string
	Enabled         bool
	IntervalMinutes int
	LastRunAt       time.Time
	NextRunAt       time.Time
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SubscriptionCandidateRecord 表示已独占分配给订阅的种子候选。
type SubscriptionCandidateRecord struct {
	SiteID         string
	TorrentID      string
	SubscriptionID string
	RuleName       string
	SourceOrder    int
	Status         string
	ReasonCode     string
	Error          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ProcessedAt    time.Time
}

// SubscriptionCandidateQuery 描述订阅候选的分页筛选条件。
type SubscriptionCandidateQuery struct {
	SubscriptionID string
	SiteID         string
	Status         string
	Limit          int
	Offset         int
}

// SubscriptionIngestRecord 表示尚未完成首次订阅匹配的新入库种子。
type SubscriptionIngestRecord struct {
	SiteID      string
	TorrentID   string
	SourceOrder int
	CreatedAt   time.Time
}

// ListPendingSubscriptionIngest 按初始列表顺序返回站点尚未完成首次匹配的新种子。
func (s *SQLiteStore) ListPendingSubscriptionIngest(ctx context.Context, siteID string, limit int) ([]SubscriptionIngestRecord, error) {
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, source_order, created_at
FROM subscription_ingest_queue
WHERE site_id = ?
ORDER BY source_order ASC, torrent_id ASC
LIMIT ?
`, siteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []SubscriptionIngestRecord{}
	for rows.Next() {
		var record SubscriptionIngestRecord
		var createdAt string
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.SourceOrder, &createdAt); err != nil {
			return nil, err
		}
		record.CreatedAt = parseDBTime(createdAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

// DeletePendingSubscriptionIngest 在首次匹配持久化完成后删除对应 outbox 记录。
func (s *SQLiteStore) DeletePendingSubscriptionIngest(ctx context.Context, records []SubscriptionIngestRecord) error {
	if len(records) == 0 {
		return nil
	}
	clauses := make([]string, 0, len(records))
	args := make([]any, 0, len(records)*2)
	for _, record := range records {
		clauses = append(clauses, "(site_id = ? AND torrent_id = ?)")
		args = append(args, record.SiteID, record.TorrentID)
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscription_ingest_queue WHERE `+strings.Join(clauses, " OR "), args...)
	return err
}

// SubscriptionRunRecord 表示一次订阅调度、单次执行或手动批处理记录。
type SubscriptionRunRecord struct {
	ID             string
	SubscriptionID string
	SiteID         string
	Trigger        string
	Status         string
	FetchedCount   int
	InsertedCount  int
	MatchedCount   int
	AttemptedCount int
	SentCount      int
	ExistsCount    int
	FailedCount    int
	SkippedCount   int
	Error          string
	StartedAt      time.Time
	FinishedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SubscriptionRunQuery 描述订阅运行记录的分页筛选条件。
type SubscriptionRunQuery struct {
	SubscriptionID string
	SiteID         string
	Trigger        string
	Status         string
	Limit          int
	Offset         int
}

// SaveSubscription 新增或覆盖订阅配置。
func (s *SQLiteStore) SaveSubscription(ctx context.Context, record SubscriptionRecord) error {
	return s.saveSubscription(ctx, s.db, record)
}

type subscriptionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *SQLiteStore) saveSubscription(ctx context.Context, executor subscriptionExecutor, record SubscriptionRecord) error {
	siteIDs, err := json.Marshal(record.SiteIDs)
	if err != nil {
		return err
	}
	qbTags, err := json.Marshal(record.QBTags)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `
INSERT INTO subscriptions (
	id, name, rule_name, enabled, site_ids_json, priority, qb_category, save_path_template,
	qb_tags_json, filename_template, paused, max_concurrent, daily_limit, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	rule_name = excluded.rule_name,
	enabled = excluded.enabled,
	site_ids_json = excluded.site_ids_json,
	priority = excluded.priority,
	qb_category = excluded.qb_category,
	save_path_template = excluded.save_path_template,
	qb_tags_json = excluded.qb_tags_json,
	filename_template = excluded.filename_template,
	paused = excluded.paused,
	max_concurrent = excluded.max_concurrent,
	daily_limit = excluded.daily_limit,
	updated_at = CURRENT_TIMESTAMP
`, record.ID, record.Name, record.RuleName, boolInt(record.Enabled), string(siteIDs), record.Priority,
		record.QBCategory, record.SavePathTemplate, string(qbTags), record.FilenameTemplate,
		boolInt(record.Paused), record.MaxConcurrent, record.DailyLimit)
	return err
}

// SaveSubscriptionAndRequeueCandidates 原子保存订阅，并将旧配置下未完成的候选写入待匹配队列。
func (s *SQLiteStore) SaveSubscriptionAndRequeueCandidates(ctx context.Context, record SubscriptionRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if err := s.saveSubscription(ctx, tx, record); err != nil {
		return err
	}
	if err := requeueSubscriptionCandidates(ctx, tx, record.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSubscription 按 ID 读取订阅。
func (s *SQLiteStore) GetSubscription(ctx context.Context, id string) (SubscriptionRecord, bool, error) {
	record, err := scanSubscription(s.db.QueryRowContext(ctx, subscriptionSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return SubscriptionRecord{}, false, nil
	}
	if err != nil {
		return SubscriptionRecord{}, false, err
	}
	return record, true, nil
}

// ListSubscriptions 按优先级和 ID 稳定返回全部订阅。
func (s *SQLiteStore) ListSubscriptions(ctx context.Context) ([]SubscriptionRecord, error) {
	rows, err := s.db.QueryContext(ctx, subscriptionSelect+` ORDER BY priority DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

// ListEnabledSubscriptionsBySite 返回明确包含指定站点的已启用订阅。
func (s *SQLiteStore) ListEnabledSubscriptionsBySite(ctx context.Context, siteID string) ([]SubscriptionRecord, error) {
	rows, err := s.db.QueryContext(ctx, subscriptionSelect+`
WHERE enabled = 1
ORDER BY priority DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records, err := scanSubscriptions(rows)
	if err != nil {
		return nil, err
	}
	result := make([]SubscriptionRecord, 0, len(records))
	for _, record := range records {
		for _, candidateSiteID := range record.SiteIDs {
			if candidateSiteID == siteID {
				result = append(result, record)
				break
			}
		}
	}
	return result, nil
}

// DeleteSubscription 删除订阅；运行记录和下载任务由独立表保留。
func (s *SQLiteStore) DeleteSubscription(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// DeleteSubscriptionAndRequeueCandidates 原子删除订阅，并释放所有尚未成功处理的候选供当前配置重新匹配。
func (s *SQLiteStore) DeleteSubscriptionAndRequeueCandidates(ctx context.Context, id string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer rollbackUnlessCommitted(tx)
	if err := requeueSubscriptionCandidates(ctx, tx, id); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return count > 0, nil
}

func requeueSubscriptionCandidates(ctx context.Context, tx *sql.Tx, subscriptionID string) error {
	if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO subscription_ingest_queue (site_id, torrent_id, source_order, created_at)
SELECT site_id, torrent_id, source_order, CURRENT_TIMESTAMP
FROM subscription_candidates
WHERE subscription_id = ? AND status IN ('unread', 'processing', 'failed')
`, subscriptionID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
DELETE FROM subscription_candidates
WHERE subscription_id = ? AND status IN ('unread', 'processing', 'failed')
`, subscriptionID)
	return err
}

// SaveSiteSchedule 新增或覆盖站点周期配置。
func (s *SQLiteStore) SaveSiteSchedule(ctx context.Context, record SiteScheduleRecord) error {
	if record.IntervalMinutes <= 0 {
		record.IntervalMinutes = 15
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO site_schedules (
	site_id, enabled, interval_minutes, last_run_at, next_run_at, last_error, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(site_id) DO UPDATE SET
	enabled = excluded.enabled,
	interval_minutes = excluded.interval_minutes,
	last_run_at = excluded.last_run_at,
	next_run_at = excluded.next_run_at,
	last_error = excluded.last_error,
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, boolInt(record.Enabled), record.IntervalMinutes, formatDBTime(record.LastRunAt),
		formatDBTime(record.NextRunAt), record.LastError)
	return err
}

// GetSiteSchedule 按站点 ID 读取周期配置。
func (s *SQLiteStore) GetSiteSchedule(ctx context.Context, siteID string) (SiteScheduleRecord, bool, error) {
	record, err := scanSiteSchedule(s.db.QueryRowContext(ctx, siteScheduleSelect+` WHERE site_id = ?`, siteID))
	if err == sql.ErrNoRows {
		return SiteScheduleRecord{}, false, nil
	}
	if err != nil {
		return SiteScheduleRecord{}, false, err
	}
	return record, true, nil
}

// ListSiteSchedules 返回全部站点周期配置。
func (s *SQLiteStore) ListSiteSchedules(ctx context.Context) ([]SiteScheduleRecord, error) {
	rows, err := s.db.QueryContext(ctx, siteScheduleSelect+` ORDER BY site_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SiteScheduleRecord
	for rows.Next() {
		record, err := scanSiteSchedule(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ListDueSiteSchedules 返回当前应执行的已启用站点计划。
func (s *SQLiteStore) ListDueSiteSchedules(ctx context.Context, now time.Time, limit int) ([]SiteScheduleRecord, error) {
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	rows, err := s.db.QueryContext(ctx, siteScheduleSelect+`
WHERE enabled = 1 AND (next_run_at = '' OR next_run_at <= ?)
ORDER BY next_run_at ASC, site_id ASC
LIMIT ?`, formatDBTime(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SiteScheduleRecord
	for rows.Next() {
		record, err := scanSiteSchedule(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ClaimDueSiteSchedule 条件更新下次执行时间，避免同一到期计划被重复领取。
func (s *SQLiteStore) ClaimDueSiteSchedule(ctx context.Context, siteID string, now, nextRunAt time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE site_schedules
SET next_run_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND enabled = 1 AND (next_run_at = '' OR next_run_at <= ?)
`, formatDBTime(nextRunAt), siteID, formatDBTime(now))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// UpdateSiteScheduleResult 保存站点计划最近一次执行结果。
func (s *SQLiteStore) UpdateSiteScheduleResult(ctx context.Context, siteID string, lastRunAt, nextRunAt time.Time, lastError string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE site_schedules
SET last_run_at = ?, next_run_at = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ?
`, formatDBTime(lastRunAt), formatDBTime(nextRunAt), lastError, siteID)
	return err
}

// CreateSubscriptionCandidateIfAbsent 仅在种子尚未被任何订阅占用时创建候选。
func (s *SQLiteStore) CreateSubscriptionCandidateIfAbsent(ctx context.Context, record SubscriptionCandidateRecord) (bool, error) {
	if record.Status == "" {
		record.Status = SubscriptionCandidateUnread
	}
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO subscription_candidates (
	site_id, torrent_id, subscription_id, rule_name, source_order, status, reason_code, error,
	created_at, updated_at, processed_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
`, record.SiteID, record.TorrentID, record.SubscriptionID, record.RuleName, record.SourceOrder,
		record.Status, record.ReasonCode, record.Error, formatDBTime(record.ProcessedAt))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// GetSubscriptionCandidate 按种子联合键读取订阅候选。
func (s *SQLiteStore) GetSubscriptionCandidate(ctx context.Context, key TorrentKey) (SubscriptionCandidateRecord, bool, error) {
	record, err := scanSubscriptionCandidate(s.db.QueryRowContext(ctx, subscriptionCandidateSelect+`
WHERE site_id = ? AND torrent_id = ?`, key.SiteID, key.TorrentID))
	if err == sql.ErrNoRows {
		return SubscriptionCandidateRecord{}, false, nil
	}
	if err != nil {
		return SubscriptionCandidateRecord{}, false, err
	}
	return record, true, nil
}

// ListSubscriptionCandidates 按订阅、站点和状态分页读取候选。
func (s *SQLiteStore) ListSubscriptionCandidates(ctx context.Context, query SubscriptionCandidateQuery) ([]SubscriptionCandidateRecord, error) {
	clauses := []string{"1=1"}
	args := []any{}
	if query.SubscriptionID != "" {
		clauses = append(clauses, "subscription_id = ?")
		args = append(args, query.SubscriptionID)
	}
	if query.SiteID != "" {
		clauses = append(clauses, "site_id = ?")
		args = append(args, query.SiteID)
	}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	limit := query.Limit
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	args = append(args, limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, subscriptionCandidateSelect+`
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY source_order ASC, site_id ASC, torrent_id ASC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptionCandidates(rows)
}

// ClaimSubscriptionCandidates 原子领取指定订阅的一批未读候选。
func (s *SQLiteStore) ClaimSubscriptionCandidates(ctx context.Context, subscriptionID string, limit int) ([]SubscriptionCandidateRecord, error) {
	return s.ClaimSubscriptionCandidatesForSite(ctx, subscriptionID, "", limit)
}

// ClaimSubscriptionCandidatesForSite 原子领取订阅在指定站点的一批未读候选；站点为空时领取全部。
func (s *SQLiteStore) ClaimSubscriptionCandidatesForSite(ctx context.Context, subscriptionID, siteID string, limit int) ([]SubscriptionCandidateRecord, error) {
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	siteClause := ""
	selectArgs := []any{subscriptionID}
	if strings.TrimSpace(siteID) != "" {
		siteClause = " AND site_id = ?"
		selectArgs = append(selectArgs, siteID)
	}
	selectArgs = append(selectArgs, limit, subscriptionID)
	rows, err := s.db.QueryContext(ctx, `
UPDATE subscription_candidates
SET status = 'processing', error = '', updated_at = CURRENT_TIMESTAMP
WHERE (site_id, torrent_id) IN (
	SELECT site_id, torrent_id FROM subscription_candidates
	WHERE subscription_id = ? AND status = 'unread'`+siteClause+`
	ORDER BY source_order ASC, site_id ASC, torrent_id ASC
	LIMIT ?
)
AND subscription_id = ? AND status = 'unread'
RETURNING site_id, torrent_id, subscription_id, rule_name, source_order, status, reason_code,
	error, created_at, updated_at, processed_at
`, selectArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptionCandidates(rows)
}

// UpdateSubscriptionCandidateStatus 条件更新候选状态并保留原因。
func (s *SQLiteStore) UpdateSubscriptionCandidateStatus(ctx context.Context, key TorrentKey, fromStatus, status, reasonCode, errText string) (bool, error) {
	clauses := `site_id = ? AND torrent_id = ?`
	args := []any{status, reasonCode, errText, status, status, key.SiteID, key.TorrentID}
	if fromStatus != "" {
		clauses += ` AND status = ?`
		args = append(args, fromStatus)
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE subscription_candidates
SET status = ?, reason_code = ?, error = ?,
	processed_at = CASE WHEN ? IN ('processed', 'failed') THEN CURRENT_TIMESTAMP ELSE CASE WHEN ? = 'unread' THEN '' ELSE processed_at END END,
	updated_at = CURRENT_TIMESTAMP
WHERE `+clauses, args...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// SaveSubscriptionRun 新增或覆盖订阅运行记录。
func (s *SQLiteStore) SaveSubscriptionRun(ctx context.Context, record SubscriptionRunRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO subscription_runs (
	id, subscription_id, site_id, trigger, status, fetched_count, inserted_count, matched_count, attempted_count, sent_count,
	exists_count, failed_count, skipped_count, error, started_at, finished_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	subscription_id = excluded.subscription_id,
	site_id = excluded.site_id,
	trigger = excluded.trigger,
	status = excluded.status,
	fetched_count = excluded.fetched_count,
	inserted_count = excluded.inserted_count,
	matched_count = excluded.matched_count,
	attempted_count = excluded.attempted_count,
	sent_count = excluded.sent_count,
	exists_count = excluded.exists_count,
	failed_count = excluded.failed_count,
	skipped_count = excluded.skipped_count,
	error = excluded.error,
	started_at = excluded.started_at,
	finished_at = excluded.finished_at,
	updated_at = CURRENT_TIMESTAMP
`, record.ID, record.SubscriptionID, record.SiteID, record.Trigger, record.Status, record.FetchedCount,
		record.InsertedCount, record.MatchedCount, record.AttemptedCount, record.SentCount, record.ExistsCount, record.FailedCount, record.SkippedCount,
		record.Error, formatDBTime(record.StartedAt), formatDBTime(record.FinishedAt))
	return err
}

// GetSubscriptionRun 按 ID 读取订阅运行记录。
func (s *SQLiteStore) GetSubscriptionRun(ctx context.Context, id string) (SubscriptionRunRecord, bool, error) {
	record, err := scanSubscriptionRun(s.db.QueryRowContext(ctx, subscriptionRunSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return SubscriptionRunRecord{}, false, nil
	}
	if err != nil {
		return SubscriptionRunRecord{}, false, err
	}
	return record, true, nil
}

// ListSubscriptionRuns 分页读取订阅运行记录。
func (s *SQLiteStore) ListSubscriptionRuns(ctx context.Context, query SubscriptionRunQuery) ([]SubscriptionRunRecord, error) {
	clauses := []string{"1=1"}
	args := []any{}
	for _, filter := range []struct {
		column string
		value  string
	}{
		{"subscription_id", query.SubscriptionID},
		{"site_id", query.SiteID},
		{"trigger", query.Trigger},
		{"status", query.Status},
	} {
		if filter.value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	limit := query.Limit
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	args = append(args, limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, subscriptionRunSelect+`
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SubscriptionRunRecord
	for rows.Next() {
		record, err := scanSubscriptionRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

const subscriptionSelect = `
SELECT id, name, rule_name, enabled, site_ids_json, priority, qb_category, save_path_template,
	qb_tags_json, filename_template, paused, max_concurrent, daily_limit, created_at, updated_at
FROM subscriptions`

func scanSubscription(scanner rowScanner) (SubscriptionRecord, error) {
	var record SubscriptionRecord
	var enabled, paused int
	var siteIDsJSON, qbTagsJSON, createdAt, updatedAt string
	err := scanner.Scan(&record.ID, &record.Name, &record.RuleName, &enabled, &siteIDsJSON, &record.Priority,
		&record.QBCategory, &record.SavePathTemplate, &qbTagsJSON, &record.FilenameTemplate,
		&paused, &record.MaxConcurrent, &record.DailyLimit, &createdAt, &updatedAt)
	if err != nil {
		return SubscriptionRecord{}, err
	}
	record.Enabled = enabled != 0
	record.Paused = paused != 0
	_ = json.Unmarshal([]byte(siteIDsJSON), &record.SiteIDs)
	_ = json.Unmarshal([]byte(qbTagsJSON), &record.QBTags)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}

func scanSubscriptions(rows *sql.Rows) ([]SubscriptionRecord, error) {
	var records []SubscriptionRecord
	for rows.Next() {
		record, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

const siteScheduleSelect = `
SELECT site_id, enabled, interval_minutes, last_run_at, next_run_at, last_error, created_at, updated_at
FROM site_schedules`

func scanSiteSchedule(scanner rowScanner) (SiteScheduleRecord, error) {
	var record SiteScheduleRecord
	var enabled int
	var lastRunAt, nextRunAt, createdAt, updatedAt string
	err := scanner.Scan(&record.SiteID, &enabled, &record.IntervalMinutes, &lastRunAt, &nextRunAt,
		&record.LastError, &createdAt, &updatedAt)
	if err != nil {
		return SiteScheduleRecord{}, err
	}
	record.Enabled = enabled != 0
	record.LastRunAt = parseDBTime(lastRunAt)
	record.NextRunAt = parseDBTime(nextRunAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}

const subscriptionCandidateSelect = `
SELECT site_id, torrent_id, subscription_id, rule_name, source_order, status, reason_code,
	error, created_at, updated_at, processed_at
FROM subscription_candidates`

func scanSubscriptionCandidate(scanner rowScanner) (SubscriptionCandidateRecord, error) {
	var record SubscriptionCandidateRecord
	var createdAt, updatedAt, processedAt string
	err := scanner.Scan(&record.SiteID, &record.TorrentID, &record.SubscriptionID, &record.RuleName,
		&record.SourceOrder, &record.Status, &record.ReasonCode, &record.Error, &createdAt, &updatedAt, &processedAt)
	if err != nil {
		return SubscriptionCandidateRecord{}, err
	}
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	record.ProcessedAt = parseDBTime(processedAt)
	return record, nil
}

func scanSubscriptionCandidates(rows *sql.Rows) ([]SubscriptionCandidateRecord, error) {
	var records []SubscriptionCandidateRecord
	for rows.Next() {
		record, err := scanSubscriptionCandidate(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].SourceOrder != records[j].SourceOrder {
			return records[i].SourceOrder < records[j].SourceOrder
		}
		if records[i].SiteID != records[j].SiteID {
			return records[i].SiteID < records[j].SiteID
		}
		return records[i].TorrentID < records[j].TorrentID
	})
	return records, nil
}

const subscriptionRunSelect = `
SELECT id, subscription_id, site_id, trigger, status, fetched_count, inserted_count, matched_count,
	attempted_count, sent_count, exists_count, failed_count, skipped_count, error, started_at, finished_at, created_at, updated_at
FROM subscription_runs`

func scanSubscriptionRun(scanner rowScanner) (SubscriptionRunRecord, error) {
	var record SubscriptionRunRecord
	var startedAt, finishedAt, createdAt, updatedAt string
	err := scanner.Scan(&record.ID, &record.SubscriptionID, &record.SiteID, &record.Trigger, &record.Status,
		&record.FetchedCount, &record.InsertedCount, &record.MatchedCount, &record.AttemptedCount, &record.SentCount, &record.ExistsCount, &record.FailedCount,
		&record.SkippedCount, &record.Error, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if err != nil {
		return SubscriptionRunRecord{}, err
	}
	record.StartedAt = parseDBTime(startedAt)
	record.FinishedAt = parseDBTime(finishedAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}
