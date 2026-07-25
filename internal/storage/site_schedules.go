package storage

import (
	"context"
	"database/sql"
	"time"
)

// SaveSiteSchedule 新增或更新站点调度配置。
func (s *SQLiteStore) SaveSiteSchedule(ctx context.Context, record SiteScheduleRecord) error {
	if record.IntervalMinutes <= 0 {
		record.IntervalMinutes = 15
	}
	_, err := s.execWriteContext(ctx, `
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
	result, err := s.execWriteContext(ctx, `
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
	_, err := s.execWriteContext(ctx, `
UPDATE site_schedules
SET last_run_at = ?, next_run_at = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ?
`, formatDBTime(lastRunAt), formatDBTime(nextRunAt), lastError, siteID)
	return err
}

// CreateSubscriptionCandidateIfAbsent 仅在种子尚未被任何订阅占用时创建候选。
