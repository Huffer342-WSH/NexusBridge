// Package tests 验证 torrent 文件和 qB 快照的 SQLite 持久化行为。
package tests

import (
	"path/filepath"
	"testing"
	"time"

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
