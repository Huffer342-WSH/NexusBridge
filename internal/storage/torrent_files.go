// Package storage 提供 torrent 文件和 info hash 的 SQLite 持久化。
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// TorrentFileRecord 表示数据库中的 torrent 原始文件及其 hash 元数据。
type TorrentFileRecord struct {
	SiteID     string
	TorrentID  string
	Data       []byte
	InfoHashV1 string
	InfoHashV2 string
	ByteSize   int64
	FetchedAt  time.Time
	LastError  string
	HasData    bool
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
	_, err := s.db.ExecContext(ctx, `
INSERT INTO torrent_files (site_id, torrent_id, data, info_hash_v1, info_hash_v2, byte_size, fetched_at, last_error, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, '', CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	data = excluded.data,
	info_hash_v1 = excluded.info_hash_v1,
	info_hash_v2 = excluded.info_hash_v2,
	byte_size = excluded.byte_size,
	fetched_at = excluded.fetched_at,
	last_error = '',
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, record.TorrentID, record.Data, strings.ToLower(record.InfoHashV1), strings.ToLower(record.InfoHashV2), len(record.Data), fetchedAt.UTC().Format(time.RFC3339))
	return err
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
	var fetchedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, torrent_id, data, info_hash_v1, info_hash_v2, byte_size, fetched_at, last_error
FROM torrent_files
WHERE site_id = ? AND torrent_id = ?
`, key.SiteID, key.TorrentID).Scan(&record.SiteID, &record.TorrentID, &record.Data, &record.InfoHashV1, &record.InfoHashV2, &record.ByteSize, &fetchedAt, &record.LastError)
	if err == sql.ErrNoRows {
		return TorrentFileRecord{}, false, nil
	}
	if err != nil {
		return TorrentFileRecord{}, false, err
	}
	record.HasData = len(record.Data) > 0
	record.FetchedAt = parseDBTime(fetchedAt)
	return record, true, nil
}

// ListTorrentFileMetadata 返回指定种子的文件元数据，不读取 BLOB。
func (s *SQLiteStore) ListTorrentFileMetadata(ctx context.Context, keys []TorrentKey) (map[TorrentKey]TorrentFileRecord, error) {
	result := make(map[TorrentKey]TorrentFileRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	query, args := torrentKeyQuery(`
SELECT site_id, torrent_id, info_hash_v1, info_hash_v2, byte_size, fetched_at, last_error,
	CASE WHEN data IS NOT NULL AND length(data) > 0 THEN 1 ELSE 0 END
FROM torrent_files WHERE `, keys)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var record TorrentFileRecord
		var fetchedAt string
		var hasData int
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.InfoHashV1, &record.InfoHashV2, &record.ByteSize, &fetchedAt, &record.LastError, &hasData); err != nil {
			return nil, err
		}
		record.HasData = hasData != 0
		record.FetchedAt = parseDBTime(fetchedAt)
		result[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = record
	}
	return result, rows.Err()
}

// ListTorrentHashes 返回全部可用于 qB 精确匹配的本地 torrent hash。
func (s *SQLiteStore) ListTorrentHashes(ctx context.Context) ([]TorrentFileRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, info_hash_v1, info_hash_v2, byte_size, fetched_at, last_error
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
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.InfoHashV1, &record.InfoHashV2, &record.ByteSize, &fetchedAt, &record.LastError); err != nil {
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
