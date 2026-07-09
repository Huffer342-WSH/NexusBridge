package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"nexusbridge/internal/parser"
)

func TestParseTorrentsFromFetchedHTML(t *testing.T) {
	settings := loadTestSettings(t)
	definition := loadSiteDefinition(t, settings)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	body := fetchConfiguredHTMLFromDB(t, ctx, settings, definition)
	parsed, err := parser.ParsePageWithDefinition(body, definition)
	if err != nil {
		t.Fatalf("parse configured page: %v", err)
	}
	assertTorrents(t, parsed.Torrents)

	data, err := json.MarshalIndent(parsed.Torrents, "", "  ")
	if err != nil {
		t.Fatalf("marshal torrents: %v", err)
	}
	t.Logf("parsed torrents:\n%s", data)
}

func assertTorrents(t *testing.T, torrents []parser.TorrentEntry) {
	t.Helper()
	if len(torrents) == 0 {
		t.Fatal("expected torrents")
	}
	first := torrents[0]
	if first.ID == 0 || first.Title == "" {
		t.Fatalf("expected first torrent id and title: %#v", first)
	}
	if first.DetailURL == "" || first.DownloadURL == "" {
		t.Fatalf("expected first torrent urls: %#v", first)
	}
	if first.Category == "" || first.CoverURL == "" {
		t.Fatalf("expected first torrent category and cover: %#v", first)
	}
	if first.SizeText == "" || first.SizeBytes == 0 {
		t.Fatalf("expected first torrent size: %#v", first)
	}
	if first.Promotion == "" || len(first.Tags) == 0 {
		t.Fatalf("expected first torrent promotion and tags: %#v", first)
	}
	if first.Seeders == 0 || first.Snatches == 0 {
		t.Fatalf("expected first torrent statistics: %#v", first)
	}
}
