package storage

import (
	"context"
	"database/sql"
	"time"
)

// CoverCacheRecord 保存单个站点种子的封面缓存状态和本地文件元数据。
type CoverCacheRecord struct {
	SiteID         string
	TorrentID      string
	SourceURL      string
	LocalPath      string
	MIMEType       string
	FileSize       int64
	SHA256         string
	Status         string
	LastError      string
	LastSuccessAt  time.Time
	LastFailedAt   time.Time
	FailCount      int
	AttemptVersion int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GetCoverCache 按站点和种子 ID 读取封面缓存记录。
func (s *SQLiteStore) GetCoverCache(ctx context.Context, siteID, torrentID string) (CoverCacheRecord, bool, error) {
	var record CoverCacheRecord
	var lastSuccessAt, lastFailedAt, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, torrent_id, source_url, local_path, mime_type, file_size, sha256, status, last_error,
       last_success_at, last_failed_at, fail_count, attempt_version, created_at, updated_at
FROM cover_cache
WHERE site_id = ? AND torrent_id = ?
`, siteID, torrentID).Scan(
		&record.SiteID, &record.TorrentID, &record.SourceURL, &record.LocalPath, &record.MIMEType,
		&record.FileSize, &record.SHA256, &record.Status, &record.LastError, &lastSuccessAt,
		&lastFailedAt, &record.FailCount, &record.AttemptVersion, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return CoverCacheRecord{}, false, nil
		}
		return CoverCacheRecord{}, false, err
	}
	record.LastSuccessAt = parseDBTime(lastSuccessAt)
	record.LastFailedAt = parseDBTime(lastFailedAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, true, nil
}

// UpsertCoverCache 写入封面缓存状态并保留记录的首次创建时间。
func (s *SQLiteStore) UpsertCoverCache(ctx context.Context, record CoverCacheRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO cover_cache (
    site_id, torrent_id, source_url, local_path, mime_type, file_size, sha256, status, last_error,
    last_success_at, last_failed_at, fail_count, attempt_version, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
    source_url = excluded.source_url,
    local_path = excluded.local_path,
    mime_type = excluded.mime_type,
    file_size = excluded.file_size,
    sha256 = excluded.sha256,
    status = excluded.status,
    last_error = excluded.last_error,
    last_success_at = excluded.last_success_at,
    last_failed_at = excluded.last_failed_at,
    fail_count = excluded.fail_count,
    attempt_version = excluded.attempt_version,
    updated_at = CURRENT_TIMESTAMP
`, record.SiteID, record.TorrentID, record.SourceURL, record.LocalPath, record.MIMEType,
		record.FileSize, record.SHA256, record.Status, record.LastError, formatDBTime(record.LastSuccessAt),
		formatDBTime(record.LastFailedAt), record.FailCount, record.AttemptVersion)
	return err
}
