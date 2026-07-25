// qb_poll_test.go 验证 qB 轮询配置默认值。
package tests

import (
	"testing"

	"nexusbridge/internal/config"
)

// TestQBPollingConfigDefaults 验证旧配置默认启用轮询且显式关闭配置保持关闭。
func TestQBPollingConfigDefaults(t *testing.T) {
	legacy := config.NormalizeQBittorrentConfig(config.QBittorrentConfig{})
	if !legacy.AutoSync || legacy.SyncIntervalSeconds != 3 || legacy.InactiveSyncIntervalSeconds != 30 || legacy.DisconnectedSyncIntervalSeconds != 60 {
		t.Fatalf("unexpected legacy polling defaults: %#v", legacy)
	}
	disabled := config.NormalizeQBittorrentConfig(config.QBittorrentConfig{
		AutoSync: false, SyncIntervalSeconds: 1, InactiveSyncIntervalSeconds: 999, DisconnectedSyncIntervalSeconds: 1,
	})
	if disabled.AutoSync || disabled.SyncIntervalSeconds != 2 || disabled.InactiveSyncIntervalSeconds != 300 || disabled.DisconnectedSyncIntervalSeconds != 15 {
		t.Fatalf("unexpected normalized polling settings: %#v", disabled)
	}
}
