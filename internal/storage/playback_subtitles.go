package storage

import (
	"context"
	"database/sql"
)

// PlaybackExternalSubtitleRecord 保存视频与一个外挂字幕文件的持久关联。
type PlaybackExternalSubtitleRecord struct {
	VideoPath    string
	SubtitlePath string
}

// GetPlaybackExternalSubtitle 按规范化视频路径读取外挂字幕关联。
func (s *SQLiteStore) GetPlaybackExternalSubtitle(ctx context.Context, videoPath string) (PlaybackExternalSubtitleRecord, bool, error) {
	var record PlaybackExternalSubtitleRecord
	err := s.db.QueryRowContext(ctx, `SELECT video_path, subtitle_path FROM playback_external_subtitles WHERE video_path = ?`, videoPath).
		Scan(&record.VideoPath, &record.SubtitlePath)
	if err == sql.ErrNoRows {
		return PlaybackExternalSubtitleRecord{}, false, nil
	}
	return record, err == nil, err
}

// SavePlaybackExternalSubtitle 新增或替换视频的外挂字幕关联。
func (s *SQLiteStore) SavePlaybackExternalSubtitle(ctx context.Context, videoPath, subtitlePath string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO playback_external_subtitles (video_path, subtitle_path)
VALUES (?, ?)
ON CONFLICT(video_path) DO UPDATE SET subtitle_path = excluded.subtitle_path, updated_at = CURRENT_TIMESTAMP`, videoPath, subtitlePath)
	return err
}

// DeletePlaybackExternalSubtitle 只删除关联配置，不删除源字幕文件。
func (s *SQLiteStore) DeletePlaybackExternalSubtitle(ctx context.Context, videoPath string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM playback_external_subtitles WHERE video_path = ?`, videoPath)
	return err
}
