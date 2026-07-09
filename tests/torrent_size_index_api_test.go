// torrent_size_index_api_test.go 验证恢复大小索引的状态和手动重建 HTTP 接口。
package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	httpserver "nexusbridge/internal/server"
	"nexusbridge/internal/storage"
)

// TestTorrentSizeIndexAPI 验证旧 BLOB 在手动重建前后的 API 状态。
func TestTorrentSizeIndexAPI(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "index-api.db")
	sitesDir := filepath.Join(root, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	const torrentData = "d4:infod6:lengthi0e4:name4:test12:piece lengthi16384e6:pieces0:ee"
	if _, err := store.UpsertTorrents(t.Context(), []storage.TorrentRecord{{SiteID: "demo", TorrentID: "1", Title: "test"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{
		SiteID: "demo", TorrentID: "1", Data: []byte(torrentData),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceQBCategories(t.Context(), []storage.QBCategoryRecord{{Name: "recovered", SavePath: root}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Storage.Path = dbPath
	cfg.SitesDir = sitesDir
	cfg.QBittorrent.AutoSync = false
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	handler := httpserver.New(cfg, app, "").Handler()
	targetPath := filepath.Join(root, "renamed-existing-file.bin")
	if err := os.WriteFile(targetPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	var before core.TorrentSizeIndexStatus
	task1JSONRequest(t, handler, http.MethodGet, "/api/qb/recovery/index", nil, http.StatusOK, &before)
	if before.Total != 1 || before.Indexed != 0 || before.Pending != 1 {
		t.Fatalf("unexpected pre-rebuild API status: %#v", before)
	}
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/recovery/preview", core.RecoveryPreviewRequest{
		Path: targetPath, SearchMode: core.RecoverySearchDatabase,
	}, http.StatusBadRequest, nil)
	var after core.TorrentSizeIndexStatus
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/recovery/index/rebuild", nil, http.StatusOK, &after)
	if after.Total != 1 || after.Indexed != 1 || after.Pending != 0 || after.Processed != 1 {
		t.Fatalf("unexpected post-rebuild API status: %#v", after)
	}
	var preview core.RecoveryPreview
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/recovery/preview", core.RecoveryPreviewRequest{
		Path: targetPath, SearchMode: core.RecoverySearchDatabase,
	}, http.StatusOK, &preview)
	if len(preview.Matches) != 1 || preview.Matches[0].Torrent.ID != "1" || preview.Matches[0].MatchMethod != "size" || !preview.Matches[0].MappingComplete || preview.Category != "recovered" {
		t.Fatalf("unexpected indexed recovery preview: %#v", preview)
	}
}
