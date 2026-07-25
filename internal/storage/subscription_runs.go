package storage

import (
	"context"
	"database/sql"
	"strings"
)

// SaveSubscriptionRun 新增或更新订阅运行记录。
func (s *SQLiteStore) SaveSubscriptionRun(ctx context.Context, record SubscriptionRunRecord) error {
	_, err := s.execWriteContext(ctx, `
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
