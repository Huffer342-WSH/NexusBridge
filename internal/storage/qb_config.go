package storage

import (
	"context"
	"database/sql"
	"time"
)

const (
	// QBCacheKindCategories 标识 qB 分类缓存。
	QBCacheKindCategories = "categories"
	// QBCacheKindTags 标识 qB 标签缓存。
	QBCacheKindTags = "tags"
)

// QBCategoryRecord 表示 qBittorrent 分类及其保存路径快照。
type QBCategoryRecord struct {
	Name      string
	SavePath  string
	LastError string
	SyncedAt  time.Time
	UpdatedAt time.Time
}

// QBTagRecord 表示 qBittorrent 标签快照。
type QBTagRecord struct {
	Name      string
	LastError string
	SyncedAt  time.Time
	UpdatedAt time.Time
}

// QBCacheStateRecord 表示一类 qB 配置快照的同步状态。
type QBCacheStateRecord struct {
	Kind      string
	LastError string
	SyncedAt  time.Time
	UpdatedAt time.Time
}

// ReplaceQBCategories 用一次成功同步的结果原子替换分类快照。
func (s *SQLiteStore) ReplaceQBCategories(ctx context.Context, records []QBCategoryRecord, syncedAt time.Time) error {
	if syncedAt.IsZero() {
		syncedAt = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM qb_categories`); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO qb_categories (name, save_path, last_error, synced_at, updated_at)
VALUES (?, ?, '', ?, CURRENT_TIMESTAMP)
`, record.Name, record.SavePath, formatDBTime(syncedAt)); err != nil {
			return err
		}
	}
	if err := saveQBCacheState(ctx, tx, QBCacheStateRecord{Kind: QBCacheKindCategories, SyncedAt: syncedAt}); err != nil {
		return err
	}
	return tx.Commit()
}

// ListQBCategories 返回最近一次成功同步的 qB 分类快照。
func (s *SQLiteStore) ListQBCategories(ctx context.Context) ([]QBCategoryRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name, save_path, last_error, synced_at, updated_at
FROM qb_categories
ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []QBCategoryRecord
	for rows.Next() {
		var record QBCategoryRecord
		var syncedAt, updatedAt string
		if err := rows.Scan(&record.Name, &record.SavePath, &record.LastError, &syncedAt, &updatedAt); err != nil {
			return nil, err
		}
		record.SyncedAt = parseDBTime(syncedAt)
		record.UpdatedAt = parseDBTime(updatedAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

// MarkQBCategoriesSyncError 保留分类快照并记录最近同步错误。
func (s *SQLiteStore) MarkQBCategoriesSyncError(ctx context.Context, errText string) error {
	return s.markQBCacheSyncError(ctx, QBCacheKindCategories, "qb_categories", errText)
}

// ReplaceQBTags 用一次成功同步的结果原子替换标签快照。
func (s *SQLiteStore) ReplaceQBTags(ctx context.Context, records []QBTagRecord, syncedAt time.Time) error {
	if syncedAt.IsZero() {
		syncedAt = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM qb_tags`); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO qb_tags (name, last_error, synced_at, updated_at)
VALUES (?, '', ?, CURRENT_TIMESTAMP)
`, record.Name, formatDBTime(syncedAt)); err != nil {
			return err
		}
	}
	if err := saveQBCacheState(ctx, tx, QBCacheStateRecord{Kind: QBCacheKindTags, SyncedAt: syncedAt}); err != nil {
		return err
	}
	return tx.Commit()
}

// ListQBTags 返回最近一次成功同步的 qB 标签快照。
func (s *SQLiteStore) ListQBTags(ctx context.Context) ([]QBTagRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name, last_error, synced_at, updated_at
FROM qb_tags
ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []QBTagRecord
	for rows.Next() {
		var record QBTagRecord
		var syncedAt, updatedAt string
		if err := rows.Scan(&record.Name, &record.LastError, &syncedAt, &updatedAt); err != nil {
			return nil, err
		}
		record.SyncedAt = parseDBTime(syncedAt)
		record.UpdatedAt = parseDBTime(updatedAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

// MarkQBTagsSyncError 保留标签快照并记录最近同步错误。
func (s *SQLiteStore) MarkQBTagsSyncError(ctx context.Context, errText string) error {
	return s.markQBCacheSyncError(ctx, QBCacheKindTags, "qb_tags", errText)
}

// GetQBCacheState 读取 qB 分类或标签缓存的同步状态。
func (s *SQLiteStore) GetQBCacheState(ctx context.Context, kind string) (QBCacheStateRecord, bool, error) {
	var record QBCacheStateRecord
	var syncedAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT kind, last_error, synced_at, updated_at
FROM qb_cache_state WHERE kind = ?
`, kind).Scan(&record.Kind, &record.LastError, &syncedAt, &updatedAt)
	if err == sql.ErrNoRows {
		return QBCacheStateRecord{}, false, nil
	}
	if err != nil {
		return QBCacheStateRecord{}, false, err
	}
	record.SyncedAt = parseDBTime(syncedAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, true, nil
}

func (s *SQLiteStore) markQBCacheSyncError(ctx context.Context, kind, table, errText string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET last_error = ?, updated_at = CURRENT_TIMESTAMP`, errText); err != nil {
		return err
	}
	state := QBCacheStateRecord{Kind: kind, LastError: errText}
	var syncedAt string
	err = tx.QueryRowContext(ctx, `SELECT synced_at FROM qb_cache_state WHERE kind = ?`, kind).Scan(&syncedAt)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	state.SyncedAt = parseDBTime(syncedAt)
	if err := saveQBCacheState(ctx, tx, state); err != nil {
		return err
	}
	return tx.Commit()
}

func saveQBCacheState(ctx context.Context, tx *sql.Tx, record QBCacheStateRecord) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO qb_cache_state (kind, last_error, synced_at, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(kind) DO UPDATE SET
	last_error = excluded.last_error,
	synced_at = CASE WHEN excluded.synced_at = '' THEN qb_cache_state.synced_at ELSE excluded.synced_at END,
	updated_at = CURRENT_TIMESTAMP
`, record.Kind, record.LastError, formatDBTime(record.SyncedAt))
	return err
}
