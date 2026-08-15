package storage

import (
	"context"
	"database/sql"
	"encoding/json"
)

const (
	// MaxTorrentListLimit 是单次种子列表查询允许的最大记录数。
	MaxTorrentListLimit     = 500
	defaultTorrentListLimit = 100
	QBittorrentSettingKey   = "qbittorrent"
	LLMSettingKey           = "llm"
	NetworkSettingKey       = "network"
	// FetchSettingKey 是全局站点抓取设置的存储键。
	FetchSettingKey = "fetch"
	// MihomoSettingKey 是 Mihomo 配置目录偏好的存储键。
	MihomoSettingKey = "mihomo"
	// SubtitleScanSettingKey 是所有媒体库共用的字幕扫描设置存储键。
	SubtitleScanSettingKey = "subtitle_scan"
)

// SaveSetting 将 JSON 设置按键新增或更新。
func (s *SQLiteStore) SaveSetting(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.execWriteContext(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
`, key, string(data))
	return err
}

// LoadSetting 按键读取 JSON 设置到 value。
func (s *SQLiteStore) LoadSetting(ctx context.Context, key string, value any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return false, err
	}
	return true, nil
}
