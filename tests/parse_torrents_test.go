package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nexusbridge/internal/parser"
)

func TestParseTorrentsFromFixtureHTML(t *testing.T) {
	definition := loadFixtureSiteDefinition(t)
	body := readFixtureHTML(t)

	parsed, err := parser.ParsePageWithDefinition(body, definition)
	if err != nil {
		t.Fatalf("parse fixture page: %v", err)
	}
	assertTorrents(t, parsed.Torrents)
	logParsedTorrents(t, parsed.Torrents)
	first := parsed.Torrents[0]
	if first.ID != 43042 || first.Title != "Fixture Torrent One" {
		t.Fatalf("expected fixture first torrent id and title: %#v", first)
	}
	if first.Category != "音声" || first.CategoryQuery != "cat=410" {
		t.Fatalf("expected fixture first torrent category: %#v", first)
	}
	if first.Subtitle != "Fixture description text ThisTokenIsDefinitelyLongerThanThirtyTwoCharactersForTags" {
		t.Fatalf("expected fixture first torrent subtitle from br text: %#v", first)
	}
	if len(first.Tags) != 3 || first.Tags[0] != "Fixture" || first.Tags[1] != "description" || first.Tags[2] != "text" {
		t.Fatalf("expected fixture first torrent tags from subtitle words with long token filtered: %#v", first.Tags)
	}
	if len(first.TagIDs) != 2 || first.TagIDs[0] != "禁转" || first.TagIDs[1] != "自购" {
		t.Fatalf("expected fixture first torrent tag_ids from colored spans: %#v", first.TagIDs)
	}
	if first.Description != "" {
		t.Fatalf("expected fixture list description to stay empty: %#v", first)
	}
}

func TestParseTorrentsFromSavedKamePTHTML(t *testing.T) {
	definition, err := parser.LoadSiteDefinition(filepath.Join("..", "internal", "builtin", "sites", "kamept.json"))
	if err != nil {
		t.Fatalf("load kamept site definition: %v", err)
	}
	body, err := os.ReadFile(filepath.Join("..", "data", "tests", "torrents_page.html"))
	if os.IsNotExist(err) {
		t.Skip("data/tests/torrents_page.html is not available")
	}
	if err != nil {
		t.Fatalf("read saved kamept html: %v", err)
	}

	parsed, err := parser.ParsePageWithDefinition(body, definition)
	if err != nil {
		t.Fatalf("parse saved kamept page: %v", err)
	}
	assertTorrents(t, parsed.Torrents)
	logParsedTorrents(t, parsed.Torrents)

	first := parsed.Torrents[0]
	if first.ID != 43143 || first.Subtitle != "哥伦比娅 原神" {
		t.Fatalf("expected saved kamept first torrent subtitle, got %#v", first)
	}
	if !hasTorrentTag(first.Tags, "哥伦比娅") || !hasTorrentTag(first.Tags, "原神") {
		t.Fatalf("expected saved kamept first torrent tags from subtitle words: %#v", first.Tags)
	}
	if !hasTorrentTag(first.TagIDs, "原盘") || !hasTorrentTag(first.TagIDs, "禁转") || !hasTorrentTag(first.TagIDs, "自购") {
		t.Fatalf("expected saved kamept first torrent tag_ids from colored spans: %#v", first.TagIDs)
	}
	if first.Description != "" {
		t.Fatalf("expected saved kamept list description to stay empty: %#v", first)
	}
}

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
	logParsedTorrents(t, parsed.Torrents)
}

func logParsedTorrents(t *testing.T, torrents []parser.TorrentEntry) {
	t.Helper()
	data, err := json.MarshalIndent(torrents, "", "  ")
	if err != nil {
		t.Fatalf("marshal torrents: %v", err)
	}
	t.Logf("parsed torrents:\n%s", data)
}

func hasTorrentTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
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
	if first.Seeders == 0 || first.Snatches == 0 {
		t.Fatalf("expected first torrent statistics: %#v", first)
	}
}
