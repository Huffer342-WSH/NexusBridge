// Package tests 验证 torrent 文件和 qB 快照的 SQLite 持久化行为。
package tests

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/storage"
)

// TestTorrentFileAndQBSnapshotPersistence 验证 BLOB、hash、快照更新和移除标记。
func TestTorrentFileAndQBSnapshotPersistence(t *testing.T) {
	store, err := storage.OpenSQLite(t.Context(), filepath.Join(t.TempDir(), "persistence.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	key1 := storage.TorrentKey{SiteID: "demo", TorrentID: "1"}
	key2 := storage.TorrentKey{SiteID: "demo", TorrentID: "2"}
	data := []byte("d4:infod4:name4:teste")
	if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{
		SiteID: key1.SiteID, TorrentID: key1.TorrentID, Data: data,
		InfoHashV1: "ABCDEF", InfoHashV2: "123456", FetchedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save torrent file: %v", err)
	}
	stored, ok, err := store.GetTorrentFile(t.Context(), key1)
	if err != nil || !ok {
		t.Fatalf("get torrent file: ok=%t err=%v", ok, err)
	}
	if string(stored.Data) != string(data) || stored.InfoHashV1 != "abcdef" {
		t.Fatalf("unexpected stored torrent file: %#v", stored)
	}
	metadata, err := store.ListTorrentFileMetadata(t.Context(), []storage.TorrentKey{key1})
	if err != nil {
		t.Fatalf("list torrent metadata: %v", err)
	}
	if !metadata[key1].HasData || len(metadata[key1].Data) != 0 {
		t.Fatalf("metadata query must report data without loading blob: %#v", metadata[key1])
	}

	for _, record := range []storage.QBSnapshotRecord{
		{SiteID: key1.SiteID, TorrentID: key1.TorrentID, Added: true, QBHash: "abcdef", State: "downloading", Category: "movies", Tags: []string{"hd"}},
		{SiteID: key2.SiteID, TorrentID: key2.TorrentID, Added: true, QBHash: "fedcba", State: "seeding"},
	} {
		if err := store.SaveQBSnapshot(t.Context(), record); err != nil {
			t.Fatalf("save qB snapshot: %v", err)
		}
	}
	removed, err := store.ReplaceQBSnapshots(t.Context(), []storage.QBSnapshotRecord{{
		SiteID: key1.SiteID, TorrentID: key1.TorrentID, Added: true, QBHash: "abcdef",
		State: "stalledDL", Progress: 0.5, Category: "movies", Tags: []string{"hd", "demo"},
	}})
	if err != nil {
		t.Fatalf("replace qB snapshots: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one removed snapshot, got %d", removed)
	}
	snapshots, err := store.ListQBSnapshots(t.Context(), []storage.TorrentKey{key1, key2})
	if err != nil {
		t.Fatalf("list qB snapshots: %v", err)
	}
	if !snapshots[key1].Added || snapshots[key1].Progress != 0.5 || len(snapshots[key1].Tags) != 2 {
		t.Fatalf("unexpected matched snapshot: %#v", snapshots[key1])
	}
	if snapshots[key2].Added || snapshots[key2].State != "missing" {
		t.Fatalf("expected second snapshot marked missing: %#v", snapshots[key2])
	}
}

// TestManualTorrentSizeIndexRebuild 验证旧 BLOB 只在显式调用时解析并重建，失败项不会阻止其他项。
func TestManualTorrentSizeIndexRebuild(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "manual-rebuild.db")
	sitesDir := filepath.Join(root, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	const validTorrent = "d4:infod6:lengthi0e4:name4:test12:piece lengthi16384e6:pieces0:ee"
	for _, record := range []storage.TorrentFileRecord{
		{SiteID: "demo", TorrentID: "valid", Data: []byte(validTorrent)},
		{SiteID: "demo", TorrentID: "broken", Data: []byte("not-bencode")},
	} {
		if err := store.SaveTorrentFile(t.Context(), record); err != nil {
			t.Fatalf("save legacy torrent %s: %v", record.TorrentID, err)
		}
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
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	before, err := app.GetTorrentSizeIndexStatus(t.Context())
	if err != nil || before.Total != 2 || before.Pending != 2 {
		t.Fatalf("unexpected pre-rebuild status: status=%#v err=%v", before, err)
	}
	after, err := app.RebuildTorrentSizeIndex(t.Context())
	if err != nil {
		t.Fatalf("manual rebuild: %v", err)
	}
	if after.Processed != 1 || after.Indexed != 1 || after.Pending != 0 || after.Failed != 1 || after.LastError == "" {
		t.Fatalf("unexpected post-rebuild status: %#v", after)
	}
}

// TestTorrentFileSizeIndexLifecycle 验证倒排行、完整多重集合查询以及保存、覆盖、删除时的自动维护。
func TestTorrentFileSizeIndexLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "size-index.db")
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	records := []storage.TorrentFileRecord{
		{SiteID: "demo", TorrentID: "A", Data: []byte("A"), SizeCounts: map[int64]int{100: 1, 20: 1}},
		{SiteID: "demo", TorrentID: "B", Data: []byte("B"), SizeCounts: map[int64]int{100: 1, 10: 1}},
		{SiteID: "demo", TorrentID: "C", Data: []byte("C"), SizeCounts: map[int64]int{200: 1}},
		{SiteID: "other", TorrentID: "D", Data: []byte("D"), SizeCounts: map[int64]int{200: 1}},
	}
	for _, record := range records {
		if err := store.SaveTorrentFile(t.Context(), record); err != nil {
			t.Fatalf("save torrent %s: %v", record.TorrentID, err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("inspect sqlite: %v", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(t.Context(), `
SELECT file_size, torrent_id, occurrence_count
FROM torrent_file_size_index
ORDER BY file_size, torrent_id
`)
	if err != nil {
		t.Fatalf("list inverted rows: %v", err)
	}
	var actual []string
	for rows.Next() {
		var size int64
		var torrentID string
		var count int
		if err := rows.Scan(&size, &torrentID, &count); err != nil {
			t.Fatalf("scan inverted row: %v", err)
		}
		actual = append(actual, fmt.Sprintf("%d:%s:%d", size, torrentID, count))
	}
	_ = rows.Close()
	expected := []string{"10:B:1", "20:A:1", "100:A:1", "100:B:1", "200:C:1", "200:D:1"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("unexpected inverted index rows: got %v want %v", actual, expected)
	}

	assertSizeMatch := func(query map[int64]int, siteIDs []string, wantIDs []string) {
		t.Helper()
		files, err := store.ListTorrentFilesBySizes(t.Context(), query, siteIDs)
		if err != nil {
			t.Fatalf("query size index %v: %v", query, err)
		}
		ids := make([]string, 0, len(files))
		for _, file := range files {
			ids = append(ids, file.TorrentID)
		}
		if !reflect.DeepEqual(ids, wantIDs) {
			t.Fatalf("query %v returned %v, want %v", query, ids, wantIDs)
		}
	}
	assertSizeMatch(map[int64]int{100: 1, 20: 1}, nil, []string{"A"})
	assertSizeMatch(map[int64]int{100: 1}, nil, []string{})
	assertSizeMatch(map[int64]int{200: 1}, nil, []string{"C", "D"})
	assertSizeMatch(map[int64]int{200: 1}, []string{"demo"}, []string{"C"})
	assertSizeMatch(map[int64]int{200: 1}, []string{"other"}, []string{"D"})

	if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{
		SiteID: "demo", TorrentID: "A", Data: []byte("A2"), SizeCounts: map[int64]int{300: 2},
	}); err != nil {
		t.Fatalf("replace torrent A: %v", err)
	}
	assertSizeMatch(map[int64]int{100: 1, 20: 1}, nil, []string{})
	assertSizeMatch(map[int64]int{300: 2}, nil, []string{"A"})
	applied, err := store.ReplaceTorrentSizeIndex(t.Context(), storage.TorrentKey{SiteID: "demo", TorrentID: "A"}, []byte("A"), map[int64]int{100: 1, 20: 1}, "")
	if err != nil {
		t.Fatalf("attempt stale index replacement: %v", err)
	}
	if applied {
		t.Fatal("stale BLOB rebuild unexpectedly replaced the current size index")
	}
	assertSizeMatch(map[int64]int{300: 2}, nil, []string{"A"})
	if err := store.DeleteTorrentFile(t.Context(), storage.TorrentKey{SiteID: "demo", TorrentID: "B"}); err != nil {
		t.Fatalf("delete torrent B: %v", err)
	}
	var deletedRows int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM torrent_file_size_index WHERE site_id = 'demo' AND torrent_id = 'B'`).Scan(&deletedRows); err != nil {
		t.Fatalf("count deleted index rows: %v", err)
	}
	if deletedRows != 0 {
		t.Fatalf("deleted torrent retained %d index rows", deletedRows)
	}
	if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{
		SiteID: "demo", TorrentID: "trigger", Data: []byte("trigger"), SizeCounts: map[int64]int{400: 1},
	}); err != nil {
		t.Fatalf("save trigger torrent: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM torrent_files WHERE site_id = 'demo' AND torrent_id = 'trigger'`); err != nil {
		t.Fatalf("delete torrent row directly: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM torrent_file_size_index WHERE site_id = 'demo' AND torrent_id = 'trigger'`).Scan(&deletedRows); err != nil {
		t.Fatalf("count trigger-cleaned index rows: %v", err)
	}
	if deletedRows != 0 {
		t.Fatalf("torrent_files delete trigger retained %d index rows", deletedRows)
	}

	if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{SiteID: "demo", TorrentID: "legacy", Data: []byte("legacy")}); err != nil {
		t.Fatalf("save unindexed legacy torrent: %v", err)
	}
	status, err := store.GetTorrentSizeIndexStatus(t.Context())
	if err != nil {
		t.Fatalf("get size index status: %v", err)
	}
	if status.Total != 4 || status.Indexed != 3 || status.Pending != 1 {
		t.Fatalf("unexpected size index status: %#v", status)
	}
}
