package storage

import (
	"context"
	"database/sql"
	"time"
)

// SiteFetchJobRecord 表示一次站点列表扫描的持久化进度。
type SiteFetchJobRecord struct {
	ID             string
	SiteID         string
	Trigger        string
	Mode           string
	RequestedPages int
	Status         string
	CurrentPage    int
	PagesFetched   int
	FetchedCount   int
	InsertedCount  int
	ChangedCount   int
	MatchedCount   int
	SentCount      int
	FilesSaved     int
	FilesFailed    int
	StopReason     string
	Error          string
	StartedAt      time.Time
	FinishedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SaveSiteFetchJob 新增或覆盖扫描任务进度。
func (s *SQLiteStore) SaveSiteFetchJob(ctx context.Context, record SiteFetchJobRecord) error {
	_, err := s.execWriteContext(ctx, `
INSERT INTO site_fetch_jobs (
	id, site_id, trigger, mode, requested_pages, status, current_page, pages_fetched,
	fetched_count, inserted_count, changed_count, matched_count, sent_count, files_saved, files_failed,
	stop_reason, error, started_at, finished_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
	COALESCE(NULLIF(?, ''), CURRENT_TIMESTAMP), CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	trigger = excluded.trigger,
	mode = excluded.mode,
	requested_pages = excluded.requested_pages,
	status = excluded.status,
	current_page = excluded.current_page,
	pages_fetched = excluded.pages_fetched,
	fetched_count = excluded.fetched_count,
	inserted_count = excluded.inserted_count,
	changed_count = excluded.changed_count,
	matched_count = excluded.matched_count,
	sent_count = excluded.sent_count,
	files_saved = excluded.files_saved,
	files_failed = excluded.files_failed,
	stop_reason = excluded.stop_reason,
	error = excluded.error,
	started_at = excluded.started_at,
	finished_at = excluded.finished_at,
	updated_at = CURRENT_TIMESTAMP
`, record.ID, record.SiteID, record.Trigger, record.Mode, record.RequestedPages, record.Status,
		record.CurrentPage, record.PagesFetched, record.FetchedCount, record.InsertedCount, record.ChangedCount,
		record.MatchedCount, record.SentCount, record.FilesSaved, record.FilesFailed, record.StopReason, record.Error,
		formatDBTime(record.StartedAt), formatDBTime(record.FinishedAt), formatDBTime(record.CreatedAt))
	return err
}

// GetSiteFetchJob 返回指定扫描任务。
func (s *SQLiteStore) GetSiteFetchJob(ctx context.Context, id string) (SiteFetchJobRecord, bool, error) {
	record, err := scanSiteFetchJob(s.db.QueryRowContext(ctx, siteFetchJobSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return SiteFetchJobRecord{}, false, nil
	}
	return record, err == nil, err
}

// ListSiteFetchJobs 返回站点扫描任务，siteID 为空时返回全部站点。
func (s *SQLiteStore) ListSiteFetchJobs(ctx context.Context, siteID string, limit int) ([]SiteFetchJobRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := siteFetchJobSelect
	args := []any{}
	if siteID != "" {
		query += ` WHERE site_id = ?`
		args = append(args, siteID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SiteFetchJobRecord{}
	for rows.Next() {
		record, err := scanSiteFetchJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

// RecoverInterruptedSiteFetchJobs 标记上次进程未结束的扫描任务。
func (s *SQLiteStore) RecoverInterruptedSiteFetchJobs(ctx context.Context) error {
	_, err := s.execWriteContext(ctx, `
UPDATE site_fetch_jobs
SET status = 'failed', stop_reason = 'interrupted', error = 'recovered after interrupted site fetch',
	finished_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE status IN ('queued', 'running')`)
	return err
}

// PruneSiteFetchJobs 仅保留指定站点最近的已结束任务。
func (s *SQLiteStore) PruneSiteFetchJobs(ctx context.Context, siteID string, keep int) error {
	if keep <= 0 {
		keep = 100
	}
	_, err := s.execWriteContext(ctx, `
DELETE FROM site_fetch_jobs
WHERE site_id = ? AND status NOT IN ('queued', 'running') AND id NOT IN (
	SELECT id FROM site_fetch_jobs
	WHERE site_id = ? AND status NOT IN ('queued', 'running')
	ORDER BY created_at DESC, id DESC LIMIT ?
)`, siteID, siteID, keep)
	return err
}

const siteFetchJobSelect = `
SELECT id, site_id, trigger, mode, requested_pages, status, current_page, pages_fetched,
	fetched_count, inserted_count, changed_count, matched_count, sent_count, files_saved, files_failed,
	stop_reason, error, started_at, finished_at, created_at, updated_at
FROM site_fetch_jobs`

type siteFetchJobScanner interface {
	Scan(dest ...any) error
}

func scanSiteFetchJob(scanner siteFetchJobScanner) (SiteFetchJobRecord, error) {
	var record SiteFetchJobRecord
	var startedAt, finishedAt, createdAt, updatedAt string
	err := scanner.Scan(&record.ID, &record.SiteID, &record.Trigger, &record.Mode, &record.RequestedPages,
		&record.Status, &record.CurrentPage, &record.PagesFetched, &record.FetchedCount, &record.InsertedCount,
		&record.ChangedCount, &record.MatchedCount, &record.SentCount, &record.FilesSaved, &record.FilesFailed,
		&record.StopReason, &record.Error, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if err != nil {
		return SiteFetchJobRecord{}, err
	}
	record.StartedAt = parseDBTime(startedAt)
	record.FinishedAt = parseDBTime(finishedAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}
