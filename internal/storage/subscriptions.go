package storage

import (
	"context"
	"database/sql"
	"encoding/json"
)

// SaveSubscription 新增或更新自动下载订阅。
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
