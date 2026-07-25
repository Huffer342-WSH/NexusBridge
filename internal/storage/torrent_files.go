// Package storage 提供 torrent 文件和恢复索引的本地持久化。
package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const TorrentSizeIndexVersion = 2

// TorrentFileRecord 表示 torrent 文件的数据库元数据和按需读取的数据。
type TorrentFileRecord struct {
	SiteID           string
	TorrentID        string
	Data             []byte
	RelativePath     string
	PayloadSHA256    string
	InfoHashV1       string
	InfoHashV2       string
	OriginalName     string
	ByteSize         int64
	FetchedAt        time.Time
	LastError        string
	HasData          bool
	ContentFileCount int
	ContentTotalSize int64
	SizeSignature    string
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

// SaveTorrentFile 先原子保存内容寻址文件，再批量更新元数据和恢复签名。
func (s *SQLiteStore) SaveTorrentFile(ctx context.Context, record TorrentFileRecord) error {
	if strings.TrimSpace(record.SiteID) == "" || strings.TrimSpace(record.TorrentID) == "" {
		return fmt.Errorf("torrent file key is required")
	}
	if len(record.Data) == 0 {
		return fmt.Errorf("torrent file data is required")
	}
	payloadHash := sha256.Sum256(record.Data)
	payloadSHA256 := hex.EncodeToString(payloadHash[:])
	relativePath := filepath.Join(payloadSHA256[:2], payloadSHA256+".torrent")
	absolutePath := filepath.Join(s.torrentDir, relativePath)
	if err := writeFileOnce(absolutePath, record.Data); err != nil {
		return fmt.Errorf("save torrent payload: %w", err)
	}
	fetchedAt := record.FetchedAt
	if fetchedAt.IsZero() {
		fetchedAt = time.Now()
	}
	fileCount, totalSize, signature, indexVersion, indexedAt, err := torrentSizeSummary(record.SizeCounts)
	if err != nil {
		return err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO torrent_files (
	site_id, torrent_id, relative_path, payload_sha256, info_hash_v1, info_hash_v2, original_name,
	byte_size, fetched_at, last_error, content_file_count, content_total_size, size_signature,
	size_index_version, size_indexed_at, size_index_error, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, '', CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	relative_path = excluded.relative_path,
	payload_sha256 = excluded.payload_sha256,
	info_hash_v1 = excluded.info_hash_v1,
	info_hash_v2 = excluded.info_hash_v2,
	original_name = excluded.original_name,
	byte_size = excluded.byte_size,
	fetched_at = excluded.fetched_at,
	last_error = '',
	content_file_count = excluded.content_file_count,
	content_total_size = excluded.content_total_size,
	size_signature = excluded.size_signature,
	size_index_version = excluded.size_index_version,
	size_indexed_at = excluded.size_indexed_at,
	size_index_error = '',
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, record.TorrentID, filepath.ToSlash(relativePath), payloadSHA256,
			strings.ToLower(record.InfoHashV1), strings.ToLower(record.InfoHashV2), record.OriginalName,
			len(record.Data), fetchedAt.UTC().Format(time.RFC3339), fileCount, totalSize, signature, indexVersion, indexedAt)
		return err
	})
	if err != nil {
		return err
	}
	return s.replaceSizeSignature(ctx, TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}, signature, fileCount, totalSize, indexVersion)
}

func writeFileOnce(path string, data []byte) error {
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(data)) {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".torrent-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		if info, statErr := os.Stat(path); statErr == nil && info.Size() == int64(len(data)) {
			_ = os.Remove(tempPath)
			return nil
		}
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func torrentSizeSummary(sizeCounts map[int64]int) (int, int64, string, int, string, error) {
	if sizeCounts == nil {
		return 0, 0, "", 0, "", nil
	}
	sizes := make([]int64, 0, len(sizeCounts))
	for size, count := range sizeCounts {
		if size < 0 || count <= 0 {
			return 0, 0, "", 0, "", fmt.Errorf("invalid torrent file size index entry: size=%d count=%d", size, count)
		}
		sizes = append(sizes, size)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	hash := sha256.New()
	var fileCount int
	var totalSize int64
	for _, size := range sizes {
		count := sizeCounts[size]
		fileCount += count
		totalSize += size * int64(count)
		_, _ = fmt.Fprintf(hash, "%d:%d;", size, count)
	}
	return fileCount, totalSize, hex.EncodeToString(hash.Sum(nil)), TorrentSizeIndexVersion,
		time.Now().UTC().Format(time.RFC3339), nil
}

func (s *SQLiteStore) replaceSizeSignature(ctx context.Context, key TorrentKey, signature string, fileCount int, totalSize int64, version int) error {
	return s.withIndexWriteTx(ctx, func(tx *sql.Tx) error {
		if signature == "" || version == 0 {
			_, err := tx.ExecContext(ctx, `DELETE FROM torrent_size_signatures WHERE site_id = ? AND torrent_id = ?`, key.SiteID, key.TorrentID)
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO torrent_size_signatures (site_id, torrent_id, signature, file_count, total_size, version, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	signature = excluded.signature,
	file_count = excluded.file_count,
	total_size = excluded.total_size,
	version = excluded.version,
	updated_at = CURRENT_TIMESTAMP
`, key.SiteID, key.TorrentID, signature, fileCount, totalSize, version)
		return err
	})
}

// ReplaceTorrentSizeIndex 在 torrent 内容未变化时重建单行大小签名。
func (s *SQLiteStore) ReplaceTorrentSizeIndex(ctx context.Context, key TorrentKey, expectedData []byte, sizeCounts map[int64]int, indexErr string) (bool, error) {
	if len(expectedData) == 0 {
		return false, fmt.Errorf("expected torrent file data is required")
	}
	expectedHash := sha256.Sum256(expectedData)
	payloadSHA256 := hex.EncodeToString(expectedHash[:])
	fileCount, totalSize, signature, version, indexedAt, err := torrentSizeSummary(sizeCounts)
	if err != nil {
		return false, err
	}
	updated := false
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if strings.TrimSpace(indexErr) != "" {
			result, err := tx.ExecContext(ctx, `
UPDATE torrent_files
SET size_index_version = 0, size_indexed_at = '', size_index_error = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ? AND payload_sha256 = ?
`, indexErr, key.SiteID, key.TorrentID, payloadSHA256)
			if err != nil {
				return err
			}
			affected, err := result.RowsAffected()
			updated = affected > 0
			return err
		}
		result, err := tx.ExecContext(ctx, `
UPDATE torrent_files
SET content_file_count = ?, content_total_size = ?, size_signature = ?, size_index_version = ?,
	size_indexed_at = ?, size_index_error = '', updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ? AND payload_sha256 = ?
`, fileCount, totalSize, signature, version, indexedAt, key.SiteID, key.TorrentID, payloadSHA256)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		updated = affected > 0
		return err
	})
	if err != nil || !updated {
		return updated, err
	}
	if strings.TrimSpace(indexErr) != "" {
		signature, version = "", 0
	}
	return true, s.replaceSizeSignature(ctx, key, signature, fileCount, totalSize, version)
}

// DeleteTorrentFile 删除关联元数据和派生索引，内容寻址文件由后续垃圾回收统一处理。
func (s *SQLiteStore) DeleteTorrentFile(ctx context.Context, key TorrentKey) error {
	if err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM torrent_files WHERE site_id = ? AND torrent_id = ?`, key.SiteID, key.TorrentID)
		return err
	}); err != nil {
		return err
	}
	return s.replaceSizeSignature(ctx, key, "", 0, 0, 0)
}

// SaveTorrentFileError 记录下载或解析失败信息并保留已有文件。
func (s *SQLiteStore) SaveTorrentFileError(ctx context.Context, key TorrentKey, errText string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO torrent_files (site_id, torrent_id, last_error, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	last_error = excluded.last_error,
	updated_at = CURRENT_TIMESTAMP
`, key.SiteID, key.TorrentID, errText)
		return err
	})
}

// GetTorrentFile 按需读取单个 torrent 文件；文件缺失时返回明确错误。
func (s *SQLiteStore) GetTorrentFile(ctx context.Context, key TorrentKey) (TorrentFileRecord, bool, error) {
	record, found, err := s.getTorrentFileMetadata(ctx, key)
	if err != nil || !found {
		return record, found, err
	}
	if record.RelativePath == "" {
		return record, true, nil
	}
	data, err := os.ReadFile(filepath.Join(s.torrentDir, filepath.FromSlash(record.RelativePath)))
	if err != nil {
		return record, true, fmt.Errorf("read torrent payload: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != record.PayloadSHA256 {
		return record, true, fmt.Errorf("torrent payload checksum mismatch")
	}
	record.Data = data
	record.HasData = true
	return record, true, nil
}

func (s *SQLiteStore) getTorrentFileMetadata(ctx context.Context, key TorrentKey) (TorrentFileRecord, bool, error) {
	var record TorrentFileRecord
	var fetchedAt, sizeIndexedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, torrent_id, relative_path, payload_sha256, info_hash_v1, info_hash_v2, original_name,
	byte_size, fetched_at, last_error, content_file_count, content_total_size, size_signature,
	size_index_version, size_indexed_at, size_index_error
FROM torrent_files WHERE site_id = ? AND torrent_id = ?
`, key.SiteID, key.TorrentID).Scan(
		&record.SiteID, &record.TorrentID, &record.RelativePath, &record.PayloadSHA256, &record.InfoHashV1,
		&record.InfoHashV2, &record.OriginalName, &record.ByteSize, &fetchedAt, &record.LastError,
		&record.ContentFileCount, &record.ContentTotalSize, &record.SizeSignature, &record.SizeIndexVersion,
		&sizeIndexedAt, &record.SizeIndexError,
	)
	if err == sql.ErrNoRows {
		return TorrentFileRecord{}, false, nil
	}
	if err != nil {
		return TorrentFileRecord{}, false, err
	}
	record.HasData = record.RelativePath != ""
	record.FetchedAt = parseDBTime(fetchedAt)
	record.SizeIndexedAt = parseDBTime(sizeIndexedAt)
	return record, true, nil
}

// ListPersistedTorrentFileKeys 返回全部已有文件引用，不读取文件内容。
func (s *SQLiteStore) ListPersistedTorrentFileKeys(ctx context.Context) ([]TorrentKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id FROM torrent_files
WHERE relative_path <> '' ORDER BY site_id, torrent_id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TorrentKey
	for rows.Next() {
		var key TorrentKey
		if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
			return nil, err
		}
		result = append(result, key)
	}
	return result, rows.Err()
}

// GetTorrentSizeIndexStatus 返回已保存 torrent 的签名覆盖情况。
func (s *SQLiteStore) GetTorrentSizeIndexStatus(ctx context.Context) (TorrentSizeIndexStatusRecord, error) {
	result := TorrentSizeIndexStatusRecord{Version: TorrentSizeIndexVersion}
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*),
	COALESCE(SUM(CASE WHEN size_index_version = ? AND size_index_error = '' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN size_index_error <> '' THEN 1 ELSE 0 END), 0)
FROM torrent_files WHERE relative_path <> ''
`, TorrentSizeIndexVersion).Scan(&result.Total, &result.Indexed, &result.Failed)
	if err != nil {
		return TorrentSizeIndexStatusRecord{}, err
	}
	result.Pending = result.Total - result.Indexed - result.Failed
	return result, nil
}

// ListTorrentFilesBySizes 使用完整多重集签名查找大小集合完全一致的 torrent。
func (s *SQLiteStore) ListTorrentFilesBySizes(ctx context.Context, sizeCounts map[int64]int, siteIDs []string) ([]TorrentFileRecord, error) {
	fileCount, totalSize, signature, version, _, err := torrentSizeSummary(sizeCounts)
	if err != nil {
		return nil, err
	}
	if signature == "" {
		return []TorrentFileRecord{}, nil
	}
	args := []any{signature, fileCount, totalSize, version}
	siteClause := ""
	if len(siteIDs) > 0 {
		placeholders := make([]string, 0, len(siteIDs))
		for _, siteID := range siteIDs {
			placeholders = append(placeholders, "?")
			args = append(args, siteID)
		}
		siteClause = " AND site_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	rows, err := s.indexDB.QueryContext(ctx, `
SELECT site_id, torrent_id FROM torrent_size_signatures
WHERE signature = ? AND file_count = ? AND total_size = ? AND version = ?`+siteClause+`
ORDER BY site_id, torrent_id
`, args...)
	if err != nil {
		return nil, err
	}
	var keys []TorrentKey
	for rows.Next() {
		var key TorrentKey
		if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]TorrentFileRecord, 0, len(keys))
	for _, key := range keys {
		record, found, err := s.GetTorrentFile(ctx, key)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if found && record.HasData {
			result = append(result, record)
		}
	}
	return result, nil
}

// ListTorrentFileMetadata 返回指定种子的文件元数据，不访问 torrent 文件。
func (s *SQLiteStore) ListTorrentFileMetadata(ctx context.Context, keys []TorrentKey) (map[TorrentKey]TorrentFileRecord, error) {
	result := make(map[TorrentKey]TorrentFileRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	query, args := torrentKeyQuery(`
SELECT site_id, torrent_id, relative_path, payload_sha256, info_hash_v1, info_hash_v2, original_name,
	byte_size, fetched_at, last_error, content_file_count, content_total_size, size_signature,
	size_index_version, size_indexed_at, size_index_error
FROM torrent_files WHERE `, keys)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var record TorrentFileRecord
		var fetchedAt, indexedAt string
		if err := rows.Scan(
			&record.SiteID, &record.TorrentID, &record.RelativePath, &record.PayloadSHA256, &record.InfoHashV1,
			&record.InfoHashV2, &record.OriginalName, &record.ByteSize, &fetchedAt, &record.LastError,
			&record.ContentFileCount, &record.ContentTotalSize, &record.SizeSignature, &record.SizeIndexVersion,
			&indexedAt, &record.SizeIndexError,
		); err != nil {
			return nil, err
		}
		record.HasData = record.RelativePath != ""
		record.FetchedAt = parseDBTime(fetchedAt)
		record.SizeIndexedAt = parseDBTime(indexedAt)
		result[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = record
	}
	return result, rows.Err()
}

// ListTorrentHashes 返回全部可用于 qB 精确匹配的本地 torrent hash。
func (s *SQLiteStore) ListTorrentHashes(ctx context.Context) ([]TorrentFileRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, relative_path, payload_sha256, info_hash_v1, info_hash_v2,
	original_name, byte_size, fetched_at, last_error
FROM torrent_files WHERE info_hash_v1 <> '' OR info_hash_v2 <> ''
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []TorrentFileRecord
	for rows.Next() {
		var record TorrentFileRecord
		var fetchedAt string
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.RelativePath, &record.PayloadSHA256,
			&record.InfoHashV1, &record.InfoHashV2, &record.OriginalName, &record.ByteSize, &fetchedAt,
			&record.LastError); err != nil {
			return nil, err
		}
		record.HasData = record.RelativePath != ""
		record.FetchedAt = parseDBTime(fetchedAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

func torrentKeyQuery(prefix string, keys []TorrentKey) (string, []any) {
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		clauses = append(clauses, "(site_id = ? AND torrent_id = ?)")
		args = append(args, key.SiteID, key.TorrentID)
	}
	return prefix + "(" + strings.Join(clauses, " OR ") + ")", args
}
