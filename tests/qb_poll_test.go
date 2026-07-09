// qb_poll_test.go 验证 qB sync/maindata 增量同步与本地快照合并。
package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

// TestQBPollUpdatesTorrentSnapshot 验证完整响应和后续部分响应都会更新媒体状态。
func TestQBPollUpdatesTorrentSnapshot(t *testing.T) {
	const hash = "444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("rid") == "8" {
			_, _ = fmt.Fprintf(w, `{"rid":9,"full_update":false,"torrents":{"%s":{"state":"stoppedDL","completed":60,"dlspeed":0}}}`, hash)
			return
		}
		_, _ = fmt.Fprintf(w, `{"rid":8,"full_update":true,"torrents":{"%s":{"name":"Demo","state":"downloading","progress":0.5,"total_size":100,"amount_left":50,"completed":50,"dlspeed":10}}}`, hash)
	}))
	defer server.Close()

	root := t.TempDir()
	sitesDir := filepath.Join(root, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSiteDefinition(t, sitesDir, parser.SiteDefinition{ID: "demo", Name: "Demo", Domain: "https://example.invalid"})
	dbPath := filepath.Join(root, "poll.db")
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertTorrents(t.Context(), []storage.TorrentRecord{{SiteID: "demo", TorrentID: "1", Title: "Demo"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{SiteID: "demo", TorrentID: "1", Data: []byte("torrent"), InfoHashV1: hash}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Storage.Path = dbPath
	cfg.SitesDir = sitesDir
	cfg.QBittorrent = config.QBittorrentConfig{AuthMode: "api_key", URL: server.URL, APIKey: "test-key"}
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	first, err := app.PollQB(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.RID != 8 || !first.FullUpdate || len(first.Updates) != 1 || first.Updates[0].QBStatus.Completed != 50 {
		t.Fatalf("unexpected full qB poll: %#v", first)
	}
	second, err := app.PollQB(t.Context(), first.RID)
	if err != nil {
		t.Fatal(err)
	}
	if second.RID != 9 || second.FullUpdate || len(second.Updates) != 1 {
		t.Fatalf("unexpected incremental qB poll: %#v", second)
	}
	status := second.Updates[0].QBStatus
	if status.State != "stoppedDL" || status.Completed != 60 || status.Size != 100 {
		t.Fatalf("incremental patch was not merged: %#v", status)
	}
}

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
