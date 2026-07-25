package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// DownloadTaskRecord 表示下载任务的数据库记录。
type DownloadTaskRecord struct {
	ID             string
	SiteID         string
	TorrentID      string
	RuleName       string
	SubscriptionID string
	Trigger        string
	Status         string
	TorrentTitle   string
	DownloadURL    string
	QBHash         string
	Error          string
	ContentPath    string
	PlanCategory   string
	PlanSavePath   string
	PlanTags       []string
	PlanRename     string
	PlanPaused     bool
	ReasonCode     string
	AttemptCount   int
	RetryCount     int
	LastAttemptAt  time.Time
	SentAt         time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreateDownloadTaskIfAbsent 在任务不存在时创建下载任务。
func (s *SQLiteStore) CreateDownloadTaskIfAbsent(ctx context.Context, task DownloadTaskRecord) (bool, error) {
	planTags, err := json.Marshal(task.PlanTags)
	if err != nil {
		return false, err
	}
	result, err := s.execWriteContext(ctx, `
INSERT INTO download_tasks (
	id, site_id, torrent_id, rule_name, subscription_id, trigger, status, torrent_title, download_url,
	qb_hash, error, content_path, plan_category, plan_save_path, plan_tags_json, plan_rename,
	plan_paused, reason_code, attempt_count, retry_count, last_attempt_at, sent_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, task.ID, task.SiteID, task.TorrentID, task.RuleName, task.SubscriptionID, task.Trigger, task.Status, task.TorrentTitle,
		task.DownloadURL, task.QBHash, task.Error, task.ContentPath, task.PlanCategory, task.PlanSavePath,
		string(planTags), task.PlanRename, boolInt(task.PlanPaused), task.ReasonCode, task.AttemptCount, task.RetryCount,
		formatDBTime(task.LastAttemptAt), formatDBTime(task.SentAt))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "constraint") {
			return false, nil
		}
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// UpdateDownloadTask 更新下载任务的主要执行状态。
func (s *SQLiteStore) UpdateDownloadTask(ctx context.Context, id, status, qbHash, contentPath, errText string) error {
	_, err := s.execWriteContext(ctx, `
UPDATE download_tasks
SET status = ?, qb_hash = COALESCE(NULLIF(?, ''), qb_hash), content_path = COALESCE(NULLIF(?, ''), content_path), error = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, qbHash, contentPath, errText, id)
	return err
}

// UpdateDownloadTaskHash 回写下载任务关联的 qBittorrent hash。
func (s *SQLiteStore) UpdateDownloadTaskHash(ctx context.Context, id, qbHash string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(qbHash) == "" {
		return nil
	}
	_, err := s.execWriteContext(ctx, `
UPDATE download_tasks
SET qb_hash = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, qbHash, id)
	return err
}

// ListDownloadTasks 按创建时间倒序返回下载任务。
func (s *SQLiteStore) ListDownloadTasks(ctx context.Context) ([]DownloadTaskRecord, error) {
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+` ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDownloadTasks(rows)
}

// ListDownloadTasksByTorrents 批量查询指定本地种子的下载任务。
func (s *SQLiteStore) ListDownloadTasksByTorrents(ctx context.Context, keys []TorrentKey) (map[string][]DownloadTaskRecord, error) {
	result := make(map[string][]DownloadTaskRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		if strings.TrimSpace(key.SiteID) == "" || strings.TrimSpace(key.TorrentID) == "" {
			continue
		}
		clauses = append(clauses, "(site_id = ? AND torrent_id = ?)")
		args = append(args, key.SiteID, key.TorrentID)
	}
	if len(clauses) == 0 {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+`
WHERE `+strings.Join(clauses, " OR ")+`
ORDER BY updated_at DESC
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanDownloadTasks(rows)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		key := task.SiteID + ":" + task.TorrentID
		result[key] = append(result[key], task)
	}
	return result, nil
}

// GetDownloadTask 按 ID 读取下载任务。
func (s *SQLiteStore) GetDownloadTask(ctx context.Context, id string) (DownloadTaskRecord, bool, error) {
	task, err := scanDownloadTask(s.db.QueryRowContext(ctx, downloadTaskSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return DownloadTaskRecord{}, false, nil
	}
	if err != nil {
		return DownloadTaskRecord{}, false, err
	}
	return task, true, nil
}

// GetDownloadTaskByRule 按种子联合键和规则名称读取幂等任务。
func (s *SQLiteStore) GetDownloadTaskByRule(ctx context.Context, siteID, torrentID, ruleName string) (DownloadTaskRecord, bool, error) {
	task, err := scanDownloadTask(s.db.QueryRowContext(ctx, downloadTaskSelect+`
WHERE site_id = ? AND torrent_id = ? AND rule_name = ? COLLATE NOCASE
`, siteID, torrentID, ruleName))
	if err == sql.ErrNoRows {
		return DownloadTaskRecord{}, false, nil
	}
	if err != nil {
		return DownloadTaskRecord{}, false, err
	}
	return task, true, nil
}

// ListDownloadTasksByStatus 按订阅和状态分页读取下载任务。
func (s *SQLiteStore) ListDownloadTasksByStatus(ctx context.Context, subscriptionID string, statuses []string, limit int) ([]DownloadTaskRecord, error) {
	clauses := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(subscriptionID) != "" {
		clauses = append(clauses, "subscription_id = ?")
		args = append(args, subscriptionID)
	}
	if len(statuses) > 0 {
		placeholders := make([]string, 0, len(statuses))
		for _, status := range statuses {
			placeholders = append(placeholders, "?")
			args = append(args, status)
		}
		clauses = append(clauses, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+`
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDownloadTasks(rows)
}

// ClaimDownloadTask 以单条条件更新安全领取任务，并记录本次尝试。
func (s *SQLiteStore) ClaimDownloadTask(ctx context.Context, id string, fromStatuses []string, incrementRetry bool) (DownloadTaskRecord, bool, error) {
	if len(fromStatuses) == 0 {
		fromStatuses = []string{"pending"}
	}
	placeholders := make([]string, 0, len(fromStatuses))
	args := []any{boolInt(incrementRetry), id}
	for _, status := range fromStatuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}
	result, err := s.execWriteContext(ctx, `
UPDATE download_tasks
SET status = 'processing', attempt_count = attempt_count + 1,
	retry_count = retry_count + ?, last_attempt_at = CURRENT_TIMESTAMP,
	error = '', reason_code = '', updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND status IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return DownloadTaskRecord{}, false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return DownloadTaskRecord{}, false, err
	}
	return s.GetDownloadTask(ctx, id)
}

// UpdateDownloadTaskRecord 覆盖下载任务的执行状态和下载计划快照。
func (s *SQLiteStore) UpdateDownloadTaskRecord(ctx context.Context, task DownloadTaskRecord) error {
	_, err := s.UpdateDownloadTaskRecordIfStatus(ctx, task, nil)
	return err
}

// UpdateDownloadTaskRecordIfStatus 仅在任务仍处于指定旧状态时更新执行状态和计划快照。
func (s *SQLiteStore) UpdateDownloadTaskRecordIfStatus(ctx context.Context, task DownloadTaskRecord, fromStatuses []string) (bool, error) {
	planTags, err := json.Marshal(task.PlanTags)
	if err != nil {
		return false, err
	}
	where := "id = ?"
	args := []any{task.SubscriptionID, task.Trigger, task.Status, task.TorrentTitle, task.DownloadURL, task.QBHash,
		task.Error, task.ContentPath, task.PlanCategory, task.PlanSavePath, string(planTags), task.PlanRename,
		boolInt(task.PlanPaused), task.ReasonCode, task.AttemptCount, task.RetryCount,
		formatDBTime(task.LastAttemptAt), formatDBTime(task.SentAt), task.ID}
	if len(fromStatuses) > 0 {
		placeholders := make([]string, 0, len(fromStatuses))
		for _, status := range fromStatuses {
			placeholders = append(placeholders, "?")
			args = append(args, status)
		}
		where += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	result, err := s.execWriteContext(ctx, `
UPDATE download_tasks
SET subscription_id = ?, trigger = ?, status = ?, torrent_title = ?, download_url = ?, qb_hash = ?,
	error = ?, content_path = ?, plan_category = ?, plan_save_path = ?, plan_tags_json = ?,
	plan_rename = ?, plan_paused = ?, reason_code = ?, attempt_count = ?, retry_count = ?,
	last_attempt_at = ?, sent_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE `+where, args...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// CountSentDownloadTasksSince 统计订阅从指定时间起成功发送的任务数。
func (s *SQLiteStore) CountSentDownloadTasksSince(ctx context.Context, subscriptionID string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM download_tasks
WHERE subscription_id = ? AND status = 'sent' AND sent_at >= ?
`, subscriptionID, formatDBTime(since)).Scan(&count)
	return count, err
}

const downloadTaskSelect = `
SELECT id, site_id, torrent_id, rule_name, subscription_id, trigger, status, torrent_title, download_url,
	qb_hash, error, content_path, plan_category, plan_save_path, plan_tags_json, plan_rename,
	plan_paused, reason_code, attempt_count, retry_count, last_attempt_at, sent_at, created_at, updated_at
FROM download_tasks`

func scanDownloadTask(scanner rowScanner) (DownloadTaskRecord, error) {
	var task DownloadTaskRecord
	var planTagsJSON, lastAttemptAt, sentAt, createdAt, updatedAt string
	var planPaused int
	err := scanner.Scan(&task.ID, &task.SiteID, &task.TorrentID, &task.RuleName, &task.SubscriptionID,
		&task.Trigger, &task.Status, &task.TorrentTitle, &task.DownloadURL, &task.QBHash, &task.Error,
		&task.ContentPath, &task.PlanCategory, &task.PlanSavePath, &planTagsJSON, &task.PlanRename,
		&planPaused, &task.ReasonCode, &task.AttemptCount, &task.RetryCount, &lastAttemptAt, &sentAt,
		&createdAt, &updatedAt)
	if err != nil {
		return DownloadTaskRecord{}, err
	}
	_ = json.Unmarshal([]byte(planTagsJSON), &task.PlanTags)
	task.PlanPaused = planPaused != 0
	task.LastAttemptAt = parseDBTime(lastAttemptAt)
	task.SentAt = parseDBTime(sentAt)
	task.CreatedAt = parseDBTime(createdAt)
	task.UpdatedAt = parseDBTime(updatedAt)
	return task, nil
}
