package storage

import (
	"context"
	"database/sql"
	"time"
)

// SiteAttendanceScheduleRecord 表示单个站点的每日签到配置和最近状态。
type SiteAttendanceScheduleRecord struct {
	SiteID    string
	Enabled   bool
	TimeOfDay string
	Timezone  string
	LastRunAt time.Time
	NextRunAt time.Time
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const siteAttendanceScheduleSelect = `
SELECT site_id, enabled, time_of_day, timezone, last_run_at, next_run_at, last_error, created_at, updated_at
FROM site_attendance_schedules`

// SaveSiteAttendanceSchedule 新增签到配置或更新已有配置，保留最近执行结果。
func (s *SQLiteStore) SaveSiteAttendanceSchedule(ctx context.Context, record SiteAttendanceScheduleRecord) error {
	_, err := s.execWriteContext(ctx, `
INSERT INTO site_attendance_schedules (
	site_id, enabled, time_of_day, timezone, last_run_at, next_run_at, last_error, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(site_id) DO UPDATE SET
	enabled = excluded.enabled,
	time_of_day = excluded.time_of_day,
	timezone = excluded.timezone,
	next_run_at = excluded.next_run_at,
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, boolInt(record.Enabled), record.TimeOfDay, record.Timezone,
		formatDBTime(record.LastRunAt), formatDBTime(record.NextRunAt), record.LastError)
	return err
}

// GetSiteAttendanceSchedule 按站点 ID 读取签到配置。
func (s *SQLiteStore) GetSiteAttendanceSchedule(ctx context.Context, siteID string) (SiteAttendanceScheduleRecord, bool, error) {
	record, err := scanSiteAttendanceSchedule(s.db.QueryRowContext(ctx, siteAttendanceScheduleSelect+` WHERE site_id = ?`, siteID))
	if err == sql.ErrNoRows {
		return SiteAttendanceScheduleRecord{}, false, nil
	}
	if err != nil {
		return SiteAttendanceScheduleRecord{}, false, err
	}
	return record, true, nil
}

// ListDueSiteAttendanceSchedules 返回当前应执行的已启用签到配置。
func (s *SQLiteStore) ListDueSiteAttendanceSchedules(ctx context.Context, now time.Time, limit int) ([]SiteAttendanceScheduleRecord, error) {
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	rows, err := s.db.QueryContext(ctx, siteAttendanceScheduleSelect+`
WHERE enabled = 1 AND (next_run_at = '' OR next_run_at <= ?)
ORDER BY next_run_at ASC, site_id ASC
LIMIT ?`, formatDBTime(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]SiteAttendanceScheduleRecord, 0)
	for rows.Next() {
		record, err := scanSiteAttendanceSchedule(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ClaimDueSiteAttendanceSchedule 条件更新下次执行时间，避免重复领取到期签到。
func (s *SQLiteStore) ClaimDueSiteAttendanceSchedule(ctx context.Context, siteID string, now, nextRunAt time.Time) (bool, error) {
	result, err := s.execWriteContext(ctx, `
UPDATE site_attendance_schedules
SET next_run_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND enabled = 1 AND (next_run_at = '' OR next_run_at <= ?)
`, formatDBTime(nextRunAt), siteID, formatDBTime(now))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// UpdateSiteAttendanceResult 保存签到结果，并避免覆盖执行期间修改的配置。
func (s *SQLiteStore) UpdateSiteAttendanceResult(
	ctx context.Context,
	siteID string,
	lastRunAt, claimedNextRunAt, nextRunAt time.Time,
	lastError string,
) error {
	_, err := s.execWriteContext(ctx, `
UPDATE site_attendance_schedules
SET last_run_at = ?,
	last_error = ?,
	next_run_at = CASE WHEN enabled = 1 AND next_run_at = ? THEN ? ELSE next_run_at END,
	updated_at = CURRENT_TIMESTAMP
WHERE site_id = ?
`, formatDBTime(lastRunAt), lastError, formatDBTime(claimedNextRunAt), formatDBTime(nextRunAt), siteID)
	return err
}

func scanSiteAttendanceSchedule(scanner rowScanner) (SiteAttendanceScheduleRecord, error) {
	var record SiteAttendanceScheduleRecord
	var enabled int
	var lastRunAt, nextRunAt, createdAt, updatedAt string
	err := scanner.Scan(
		&record.SiteID,
		&enabled,
		&record.TimeOfDay,
		&record.Timezone,
		&lastRunAt,
		&nextRunAt,
		&record.LastError,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return SiteAttendanceScheduleRecord{}, err
	}
	record.Enabled = enabled != 0
	record.LastRunAt = parseDBTime(lastRunAt)
	record.NextRunAt = parseDBTime(nextRunAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}
