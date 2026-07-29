package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SeriesRecord 表示一个本地剧集及其最近播放、扫描状态。
type SeriesRecord struct {
	ID               string
	Name             string
	LastSelectedPath string
	LastScannedAt    time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SeriesDirectoryRecord 表示剧集中的一个有序扫描根目录。
type SeriesDirectoryRecord struct {
	SeriesID      string
	Path          string
	SourceOrder   int
	Available     bool
	LastError     string
	LastScannedAt time.Time
}

// SeriesVideoRecord 表示剧集扫描后缓存的一个视频文件。
type SeriesVideoRecord struct {
	SeriesID      string
	DirectoryPath string
	Path          string
	RelativePath  string
	Name          string
	ByteSize      int64
	ModifiedAt    time.Time
	Available     bool
}

// SeriesBundleRecord 聚合一个剧集、目录和视频缓存。
type SeriesBundleRecord struct {
	Series      SeriesRecord
	Directories []SeriesDirectoryRecord
	Videos      []SeriesVideoRecord
}

// SaveSeries 保存剧集名称和有序目录；被移除目录的视频缓存由外键级联清理。
func (s *SQLiteStore) SaveSeries(ctx context.Context, record SeriesRecord, directories []SeriesDirectoryRecord) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO series (id, name, created_at, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	updated_at = CURRENT_TIMESTAMP
`, record.ID, record.Name); err != nil {
			return err
		}
		for _, directory := range directories {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO series_directories (
	series_id, path, source_order, available, last_error, last_scanned_at
)
VALUES (?, ?, ?, 1, '', '')
ON CONFLICT(series_id, path) DO UPDATE SET
	source_order = excluded.source_order
`, record.ID, directory.Path, directory.SourceOrder); err != nil {
				return err
			}
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(directories)), ",")
		args := make([]any, 0, len(directories)+1)
		args = append(args, record.ID)
		for _, directory := range directories {
			args = append(args, directory.Path)
		}
		_, err := tx.ExecContext(ctx,
			`DELETE FROM series_directories WHERE series_id = ? AND path NOT IN (`+placeholders+`)`,
			args...,
		)
		return err
	})
}

// ListSeries 返回全部剧集及其缓存，名称使用不区分大小写的稳定顺序。
func (s *SQLiteStore) ListSeries(ctx context.Context) ([]SeriesBundleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, last_selected_path, last_scanned_at, created_at, updated_at
FROM series
ORDER BY name COLLATE NOCASE, id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SeriesRecord
	for rows.Next() {
		record, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]SeriesBundleRecord, 0, len(records))
	for _, record := range records {
		bundle, ok, err := s.GetSeries(ctx, record.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, bundle)
		}
	}
	return result, nil
}

// GetSeries 按 ID 返回剧集及其目录和视频缓存。
func (s *SQLiteStore) GetSeries(ctx context.Context, id string) (SeriesBundleRecord, bool, error) {
	record, err := scanSeries(s.db.QueryRowContext(ctx, `
SELECT id, name, last_selected_path, last_scanned_at, created_at, updated_at
FROM series
WHERE id = ?
`, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return SeriesBundleRecord{}, false, nil
		}
		return SeriesBundleRecord{}, false, err
	}
	directories, err := s.listSeriesDirectories(ctx, id)
	if err != nil {
		return SeriesBundleRecord{}, false, err
	}
	videos, err := s.listSeriesVideos(ctx, id)
	if err != nil {
		return SeriesBundleRecord{}, false, err
	}
	return SeriesBundleRecord{Series: record, Directories: directories, Videos: videos}, true, nil
}

func (s *SQLiteStore) listSeriesDirectories(ctx context.Context, id string) ([]SeriesDirectoryRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT series_id, path, source_order, available, last_error, last_scanned_at
FROM series_directories
WHERE series_id = ?
ORDER BY source_order, path COLLATE NOCASE
`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SeriesDirectoryRecord{}
	for rows.Next() {
		var record SeriesDirectoryRecord
		var available int
		var lastScannedAt string
		if err := rows.Scan(&record.SeriesID, &record.Path, &record.SourceOrder, &available, &record.LastError, &lastScannedAt); err != nil {
			return nil, err
		}
		record.Available = available != 0
		record.LastScannedAt = parseDBTime(lastScannedAt)
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) listSeriesVideos(ctx context.Context, id string) ([]SeriesVideoRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT series_id, directory_path, path, relative_path, name, byte_size, modified_at, available
FROM series_videos
WHERE series_id = ?
`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SeriesVideoRecord{}
	for rows.Next() {
		var record SeriesVideoRecord
		var available int
		var modifiedAt string
		if err := rows.Scan(
			&record.SeriesID, &record.DirectoryPath, &record.Path, &record.RelativePath,
			&record.Name, &record.ByteSize, &modifiedAt, &available,
		); err != nil {
			return nil, err
		}
		record.ModifiedAt = parseDBTime(modifiedAt)
		record.Available = available != 0
		result = append(result, record)
	}
	return result, rows.Err()
}

// ReplaceSeriesDirectoryVideos 原子替换一个可读根目录的扫描结果。
func (s *SQLiteStore) ReplaceSeriesDirectoryVideos(
	ctx context.Context,
	seriesID, directoryPath string,
	videos []SeriesVideoRecord,
	scannedAt time.Time,
) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
UPDATE series_directories
SET available = 1, last_error = '', last_scanned_at = ?
WHERE series_id = ? AND path = ?
`, formatDBTime(scannedAt), seriesID, directoryPath); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
DELETE FROM series_videos
WHERE series_id = ? AND directory_path = ?
`, seriesID, directoryPath); err != nil {
			return err
		}
		for _, video := range videos {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO series_videos (
	series_id, directory_path, path, relative_path, name, byte_size, modified_at, available
)
VALUES (?, ?, ?, ?, ?, ?, ?, 1)
`, seriesID, directoryPath, video.Path, video.RelativePath, video.Name, video.ByteSize, formatDBTime(video.ModifiedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkSeriesDirectoryUnavailable 保留缓存并将一个暂时不可读目录标为不可用。
func (s *SQLiteStore) MarkSeriesDirectoryUnavailable(
	ctx context.Context,
	seriesID, directoryPath, lastError string,
	scannedAt time.Time,
) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE series_directories
SET available = 0, last_error = ?, last_scanned_at = ?
WHERE series_id = ? AND path = ?
`, lastError, formatDBTime(scannedAt), seriesID, directoryPath)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return fmt.Errorf("series directory not found")
		}
		_, err = tx.ExecContext(ctx, `
UPDATE series_videos
SET available = 0
WHERE series_id = ? AND directory_path = ?
`, seriesID, directoryPath)
		return err
	})
}

// FinalizeSeriesScan 更新剧集扫描时间，并在选集已从所有缓存消失时清空选择。
func (s *SQLiteStore) FinalizeSeriesScan(ctx context.Context, id string, scannedAt time.Time) error {
	_, err := s.execWriteContext(ctx, `
UPDATE series
SET
	last_scanned_at = ?,
	last_selected_path = CASE
		WHEN last_selected_path = '' THEN ''
		WHEN EXISTS (
			SELECT 1 FROM series_videos
			WHERE series_id = series.id AND path = series.last_selected_path
		) THEN last_selected_path
		ELSE ''
	END,
	updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, formatDBTime(scannedAt), id)
	return err
}

// SaveSeriesSelection 持久化剧集最后选择的视频。
func (s *SQLiteStore) SaveSeriesSelection(ctx context.Context, id, path string) error {
	result, err := s.execWriteContext(ctx, `
UPDATE series
SET last_selected_path = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, path, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("series not found")
	}
	return nil
}

// DeleteSeries 删除剧集配置和缓存，不操作任何源文件。
func (s *SQLiteStore) DeleteSeries(ctx context.Context, id string) (bool, error) {
	result, err := s.execWriteContext(ctx, `DELETE FROM series WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func scanSeries(scanner rowScanner) (SeriesRecord, error) {
	var record SeriesRecord
	var lastScannedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&record.ID, &record.Name, &record.LastSelectedPath,
		&lastScannedAt, &createdAt, &updatedAt,
	); err != nil {
		return SeriesRecord{}, err
	}
	record.LastScannedAt = parseDBTime(lastScannedAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}
