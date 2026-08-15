package storage

import (
	"context"
	"database/sql"
	"strings"
)

// SaveMediaLibrary 保存统一媒体库节点及其有序目录。
func (s *SQLiteStore) SaveMediaLibrary(
	ctx context.Context,
	record SeriesRecord,
	directories []SeriesDirectoryRecord,
) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO series (id, name, kind, parent_id, settings_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	kind = excluded.kind,
	parent_id = excluded.parent_id,
	settings_json = excluded.settings_json,
	updated_at = CURRENT_TIMESTAMP
`, record.ID, record.Name, record.Kind, record.ParentID, record.SettingsJSON); err != nil {
			return err
		}
		if record.Kind == "series" {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO series_options (series_id, episode_number_detection)
VALUES (?, ?)
ON CONFLICT(series_id) DO UPDATE SET
	episode_number_detection = excluded.episode_number_detection
`, record.ID, record.EpisodeNumberDetection); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `DELETE FROM series_options WHERE series_id = ?`, record.ID); err != nil {
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

// ListMediaLibraries 返回全部媒体库节点及其已有扫描缓存。
func (s *SQLiteStore) ListMediaLibraries(ctx context.Context) ([]SeriesBundleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.name, COALESCE(o.episode_number_detection, 0),
	s.kind, s.parent_id, s.settings_json,
	s.last_selected_path, s.last_scanned_at, s.created_at, s.updated_at
FROM series s
LEFT JOIN series_options o ON o.series_id = s.id
ORDER BY s.name COLLATE NOCASE, s.id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []SeriesRecord{}
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
		bundle, ok, err := s.GetMediaLibrary(ctx, record.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, bundle)
		}
	}
	return result, nil
}

// GetMediaLibrary 按 ID 返回一个媒体库节点。
func (s *SQLiteStore) GetMediaLibrary(ctx context.Context, id string) (SeriesBundleRecord, bool, error) {
	record, err := scanSeries(s.db.QueryRowContext(ctx, `
SELECT s.id, s.name, COALESCE(o.episode_number_detection, 0),
	s.kind, s.parent_id, s.settings_json,
	s.last_selected_path, s.last_scanned_at, s.created_at, s.updated_at
FROM series s
LEFT JOIN series_options o ON o.series_id = s.id
WHERE s.id = ?
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

// MediaLibraryHasChildren 判断节点是否仍有直接子媒体库。
func (s *SQLiteStore) MediaLibraryHasChildren(ctx context.Context, id string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM series WHERE parent_id = ?`, id).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteMediaLibrary 删除媒体库配置和缓存，不操作源文件。
func (s *SQLiteStore) DeleteMediaLibrary(ctx context.Context, id string) (bool, error) {
	return s.DeleteSeries(ctx, id)
}
