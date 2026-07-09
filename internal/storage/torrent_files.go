// Package storage 提供 torrent 文件和 info hash 的 SQLite 持久化。
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

const TorrentSizeIndexVersion = 1

// TorrentFileRecord 表示数据库中的 torrent 原始文件及其 hash 元数据。
type TorrentFileRecord struct {
	SiteID           string
	TorrentID        string
	Data             []byte
	InfoHashV1       string
	InfoHashV2       string
	OriginalName     string
	ByteSize         int64
	FetchedAt        time.Time
	LastError        string
	HasData          bool
	ContentFileCount int
	ContentTotalSize int64
	SizeIndexVersion int
	SizeIndexedAt    time.Time
	SizeIndexError   string
	SizeCounts       map[int64]int
}

// TorrentSizeIndexStatusRecord 汇总 torrent 文件大小索引状态。
type TorrentSizeIndexStatusRecord struct {
	Version int
	Total   int
	Indexed int
	Pending int
	Failed  int
}

// SaveTorrentFile 保存已验证的 torrent 原始文件和 info hash。
func (s *SQLiteStore) SaveTorrentFile(ctx context.Context, record TorrentFileRecord) error {
	if strings.TrimSpace(record.SiteID) == "" || strings.TrimSpace(record.TorrentID) == "" {
		return fmt.Errorf("torrent file key is required")
	}
	if len(record.Data) == 0 {
		return fmt.Errorf("torrent file data is required")
	}
	fetchedAt := record.FetchedAt
	if fetchedAt.IsZero() {
		fetchedAt = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	fileCount, totalSize, indexVersion, indexedAt := torrentSizeSummary(record.SizeCounts)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO torrent_files (
	site_id, torrent_id, data, info_hash_v1, info_hash_v2, original_name, byte_size, fetched_at, last_error,
	content_file_count, content_total_size, size_index_version, size_indexed_at, size_index_error, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, '', CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	data = excluded.data,
	info_hash_v1 = excluded.info_hash_v1,
	info_hash_v2 = excluded.info_hash_v2,
	original_name = excluded.original_name,
	byte_size = excluded.byte_size,
	fetched_at = excluded.fetched_at,
	last_error = '',
	content_file_count = excluded.content_file_count,
	content_total_size = excluded.content_total_size,
	size_index_version = excluded.size_index_version,
	size_indexed_at = excluded.size_indexed_at,
	size_index_error = '',
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, record.TorrentID, record.Data, strings.ToLower(record.InfoHashV1), strings.ToLower(record.InfoHashV2), record.OriginalName,
		len(record.Data), fetchedAt.UTC().Format(time.RFC3339), fileCount, totalSize, indexVersion, indexedAt); err != nil {
		return err
	}
	if err := replaceTorrentSizeIndexTx(ctx, tx, TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}, record.SizeCounts); err != nil {
		return err
	}
	return tx.Commit()
}

func torrentSizeSummary(sizeCounts map[int64]int) (int, int64, int, string) {
	if sizeCounts == nil {
		return 0, 0, 0, ""
	}
	var fileCount int
	var totalSize int64
	for size, count := range sizeCounts {
		if size < 0 || count <= 0 {
			continue
		}
		fileCount += count
		totalSize += size * int64(count)
	}
	return fileCount, totalSize, TorrentSizeIndexVersion, time.Now().UTC().Format(time.RFC3339)
}

func replaceTorrentSizeIndexTx(ctx context.Context, tx *sql.Tx, key TorrentKey, sizeCounts map[int64]int) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM torrent_file_size_index WHERE site_id = ? AND torrent_id = ?`, key.SiteID, key.TorrentID); err != nil {
		return err
	}
	if sizeCounts == nil {
		return nil
	}
	sizes := make([]int64, 0, len(sizeCounts))
	for size := range sizeCounts {
		sizes = append(sizes, size)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	for _, size := range sizes {
		count := sizeCounts[size]
		if size < 0 || count <= 0 {
			return fmt.Errorf("invalid torrent file size index entry: size=%d count=%d", size, count)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO torrent_file_size_index (site_id, torrent_id, file_size, occurrence_count)
VALUES (?, ?, ?, ?)
`, key.SiteID, key.TorrentID, size, count); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceTorrentSizeIndex 仅在 BLOB 未变化时重建单个 torrent 的大小索引。
func (s *SQLiteStore) ReplaceTorrentSizeIndex(ctx context.Context, key TorrentKey, expectedData []byte, sizeCounts map[int64]int, indexErr string) (bool, error) {
	if len(expectedData) == 0 {
		return false, fmt.Errorf("expected torrent file data is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if strings.TrimSpace(indexErr) != "" {
		result, err := tx.ExecContext(ctx, `
UPDATE torrent_files SET size_index_version = 0, size_indexed_at = '', size_index_error = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ? AND data = ?
`, indexErr, key.SiteID, key.TorrentID, expectedData)
		if err != nil {
			return false, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if affected == 0 {
			return false, nil
		}
		if err := replaceTorrentSizeIndexTx(ctx, tx, key, nil); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	fileCount, totalSize, version, indexedAt := torrentSizeSummary(sizeCounts)
	result, err := tx.ExecContext(ctx, `
UPDATE torrent_files SET content_file_count = ?, content_total_size = ?, size_index_version = ?, size_indexed_at = ?, size_index_error = '', updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ? AND data = ?
`, fileCount, totalSize, version, indexedAt, key.SiteID, key.TorrentID, expectedData)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	if err := replaceTorrentSizeIndexTx(ctx, tx, key, sizeCounts); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// DeleteTorrentFile 删除 torrent BLOB 及其大小索引。
func (s *SQLiteStore) DeleteTorrentFile(ctx context.Context, key TorrentKey) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM torrent_file_size_index WHERE site_id = ? AND torrent_id = ?`, key.SiteID, key.TorrentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM torrent_files WHERE site_id = ? AND torrent_id = ?`, key.SiteID, key.TorrentID); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveTorrentFileError 记录 torrent 文件下载或解析失败信息并保留已有文件。
func (s *SQLiteStore) SaveTorrentFileError(ctx context.Context, key TorrentKey, errText string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO torrent_files (site_id, torrent_id, last_error, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	last_error = excluded.last_error,
	updated_at = CURRENT_TIMESTAMP
`, key.SiteID, key.TorrentID, errText)
	return err
}

// GetTorrentFile 读取单个 torrent 文件；未找到时返回 false。
func (s *SQLiteStore) GetTorrentFile(ctx context.Context, key TorrentKey) (TorrentFileRecord, bool, error) {
	var record TorrentFileRecord
	var fetchedAt, sizeIndexedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, torrent_id, data, info_hash_v1, info_hash_v2, original_name, byte_size, fetched_at, last_error,
	content_file_count, content_total_size, size_index_version, size_indexed_at, size_index_error
FROM torrent_files
WHERE site_id = ? AND torrent_id = ?
`, key.SiteID, key.TorrentID).Scan(
		&record.SiteID, &record.TorrentID, &record.Data, &record.InfoHashV1, &record.InfoHashV2, &record.OriginalName,
		&record.ByteSize, &fetchedAt, &record.LastError, &record.ContentFileCount, &record.ContentTotalSize,
		&record.SizeIndexVersion, &sizeIndexedAt, &record.SizeIndexError,
	)
	if err == sql.ErrNoRows {
		return TorrentFileRecord{}, false, nil
	}
	if err != nil {
		return TorrentFileRecord{}, false, err
	}
	record.HasData = len(record.Data) > 0
	record.FetchedAt = parseDBTime(fetchedAt)
	record.SizeIndexedAt = parseDBTime(sizeIndexedAt)
	return record, true, nil
}

// ListPersistedTorrentFileKeys 返回全部有 BLOB 的 torrent 联合键，不读取 BLOB 内容。
func (s *SQLiteStore) ListPersistedTorrentFileKeys(ctx context.Context) ([]TorrentKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id
FROM torrent_files
WHERE data IS NOT NULL AND length(data) > 0
ORDER BY site_id ASC, torrent_id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TorrentKey{}
	for rows.Next() {
		var key TorrentKey
		if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
			return nil, err
		}
		result = append(result, key)
	}
	return result, rows.Err()
}

// GetTorrentSizeIndexStatus 返回当前已保存 torrent 的大小索引覆盖情况。
func (s *SQLiteStore) GetTorrentSizeIndexStatus(ctx context.Context) (TorrentSizeIndexStatusRecord, error) {
	result := TorrentSizeIndexStatusRecord{Version: TorrentSizeIndexVersion}
	err := s.db.QueryRowContext(ctx, `
SELECT
	COUNT(*),
	COALESCE(SUM(CASE WHEN size_index_version = ? AND size_index_error = '' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN size_index_error <> '' THEN 1 ELSE 0 END), 0)
FROM torrent_files
WHERE data IS NOT NULL AND length(data) > 0
`, TorrentSizeIndexVersion).Scan(&result.Total, &result.Indexed, &result.Failed)
	if err != nil {
		return TorrentSizeIndexStatusRecord{}, err
	}
	result.Pending = result.Total - result.Indexed - result.Failed
	return result, nil
}

// ListTorrentFilesBySizes 使用倒排索引查找文件大小集合完全一致的 torrent。
func (s *SQLiteStore) ListTorrentFilesBySizes(ctx context.Context, sizeCounts map[int64]int, siteIDs []string) ([]TorrentFileRecord, error) {
	if len(sizeCounts) == 0 {
		return []TorrentFileRecord{}, nil
	}
	sizes := make([]int64, 0, len(sizeCounts))
	fileCount := 0
	var totalSize int64
	for size, count := range sizeCounts {
		if size < 0 || count <= 0 {
			return nil, fmt.Errorf("invalid recovery size query: size=%d count=%d", size, count)
		}
		sizes = append(sizes, size)
		fileCount += count
		totalSize += size * int64(count)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	values := make([]string, 0, len(sizes))
	args := make([]any, 0, len(sizes)*2+len(siteIDs)+4)
	for _, size := range sizes {
		values = append(values, "(?, ?)")
		args = append(args, size, sizeCounts[size])
	}
	siteClause := ""
	if len(siteIDs) > 0 {
		placeholders := make([]string, 0, len(siteIDs))
		for _, siteID := range siteIDs {
			placeholders = append(placeholders, "?")
			args = append(args, siteID)
		}
		siteClause = " AND i.site_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	args = append(args, TorrentSizeIndexVersion, fileCount, totalSize, len(sizes))
	query := `
WITH target_sizes(file_size, occurrence_count) AS (VALUES ` + strings.Join(values, ",") + `),
matched AS (
	SELECT i.site_id, i.torrent_id
	FROM target_sizes target
	JOIN torrent_file_size_index i
		ON i.file_size = target.file_size AND i.occurrence_count = target.occurrence_count
	JOIN torrent_files summary ON summary.site_id = i.site_id AND summary.torrent_id = i.torrent_id
	WHERE 1 = 1` + siteClause + `
		AND summary.size_index_version = ?
		AND summary.content_file_count = ?
		AND summary.content_total_size = ?
	GROUP BY i.site_id, i.torrent_id
	HAVING COUNT(*) = ?
)
SELECT tf.site_id, tf.torrent_id, tf.data, tf.info_hash_v1, tf.info_hash_v2, tf.original_name, tf.byte_size,
	tf.fetched_at, tf.last_error, tf.content_file_count, tf.content_total_size, tf.size_index_version,
	tf.size_indexed_at, tf.size_index_error
FROM matched
JOIN torrent_files tf ON tf.site_id = matched.site_id AND tf.torrent_id = matched.torrent_id
ORDER BY tf.site_id, tf.torrent_id
`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TorrentFileRecord{}
	for rows.Next() {
		var record TorrentFileRecord
		var fetchedAt, indexedAt string
		if err := rows.Scan(
			&record.SiteID, &record.TorrentID, &record.Data, &record.InfoHashV1, &record.InfoHashV2, &record.OriginalName,
			&record.ByteSize, &fetchedAt, &record.LastError, &record.ContentFileCount, &record.ContentTotalSize,
			&record.SizeIndexVersion, &indexedAt, &record.SizeIndexError,
		); err != nil {
			return nil, err
		}
		record.HasData = len(record.Data) > 0
		record.FetchedAt = parseDBTime(fetchedAt)
		record.SizeIndexedAt = parseDBTime(indexedAt)
		result = append(result, record)
	}
	return result, rows.Err()
}

// ListTorrentFileMetadata 返回指定种子的文件元数据，不读取 BLOB。
func (s *SQLiteStore) ListTorrentFileMetadata(ctx context.Context, keys []TorrentKey) (map[TorrentKey]TorrentFileRecord, error) {
	result := make(map[TorrentKey]TorrentFileRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	query, args := torrentKeyQuery(`
SELECT site_id, torrent_id, info_hash_v1, info_hash_v2, original_name, byte_size, fetched_at, last_error,
	CASE WHEN data IS NOT NULL AND length(data) > 0 THEN 1 ELSE 0 END,
	content_file_count, content_total_size, size_index_version, size_indexed_at, size_index_error
FROM torrent_files WHERE `, keys)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var record TorrentFileRecord
		var fetchedAt, indexedAt string
		var hasData int
		if err := rows.Scan(
			&record.SiteID, &record.TorrentID, &record.InfoHashV1, &record.InfoHashV2, &record.OriginalName,
			&record.ByteSize, &fetchedAt, &record.LastError, &hasData, &record.ContentFileCount,
			&record.ContentTotalSize, &record.SizeIndexVersion, &indexedAt, &record.SizeIndexError,
		); err != nil {
			return nil, err
		}
		record.HasData = hasData != 0
		record.FetchedAt = parseDBTime(fetchedAt)
		record.SizeIndexedAt = parseDBTime(indexedAt)
		result[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = record
	}
	return result, rows.Err()
}

// ListTorrentHashes 返回全部可用于 qB 精确匹配的本地 torrent hash。
func (s *SQLiteStore) ListTorrentHashes(ctx context.Context) ([]TorrentFileRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, info_hash_v1, info_hash_v2, original_name, byte_size, fetched_at, last_error
FROM torrent_files
WHERE info_hash_v1 <> '' OR info_hash_v2 <> ''
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []TorrentFileRecord
	for rows.Next() {
		var record TorrentFileRecord
		var fetchedAt string
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.InfoHashV1, &record.InfoHashV2, &record.OriginalName, &record.ByteSize, &fetchedAt, &record.LastError); err != nil {
			return nil, err
		}
		record.HasData = true
		record.FetchedAt = parseDBTime(fetchedAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

// torrentKeyQuery 构造一组站点和种子联合键的查询条件。
func torrentKeyQuery(prefix string, keys []TorrentKey) (string, []any) {
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		clauses = append(clauses, "(site_id = ? AND torrent_id = ?)")
		args = append(args, key.SiteID, key.TorrentID)
	}
	return prefix + "(" + strings.Join(clauses, " OR ") + ")", args
}
