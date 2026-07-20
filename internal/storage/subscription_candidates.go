package storage

import (
	"context"
	"database/sql"
	"strings"
)

// CreateSubscriptionCandidateIfAbsent 在候选不存在时建立独占订阅分配。
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
