package storage

import (
	"context"
	"strings"
)

// ListPendingSubscriptionIngest 按初始列表顺序返回尚未完成订阅匹配的种子。
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
