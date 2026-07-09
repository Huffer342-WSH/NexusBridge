package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

func TestTorrentDetail(t *testing.T) {
	settings := loadTestSettings(t)
	definition := loadSiteDefinition(t, settings)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	curlRequest := readCurlRequest(t, settings.CurlFile)
	cookies, err := fetcher.LoadCookiesFromHeader(curlRequest.CookieHeader)
	if err != nil {
		t.Fatalf("load cookies from curl: %v", err)
	}

	store, err := storage.OpenSQLite(ctx, settings.DBPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := store.SaveCookies(ctx, settings.BaseURL, cookies); err != nil {
		t.Fatalf("save cookies: %v", err)
	}
	if err := store.SaveSiteCredential(ctx, storage.SiteCredentialRecord{
		SiteID:    definition.ID,
		BaseURL:   settings.BaseURL,
		UserAgent: curlRequest.Headers.Get("User-Agent"),
	}); err != nil {
		t.Fatalf("save site credential: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close credential store: %v", err)
	}

	body := fetchConfiguredHTMLFromDB(t, ctx, settings, definition)
	parsed, err := parser.ParsePageWithDefinition(body, definition)
	if err != nil {
		t.Fatalf("parse torrents page: %v", err)
	}
	assertTorrents(t, parsed.Torrents)
	first := parsed.Torrents[0]
	record := storage.TorrentRecord{
		SiteID:      first.SiteID,
		TorrentID:   strconv.Itoa(first.ID),
		Title:       first.Title,
		Category:    first.Category,
		DetailURL:   first.DetailURL,
		DownloadURL: first.DownloadURL,
		CoverURL:    first.CoverURL,
		Tags:        first.Tags,
		Description: first.Description,
		Seeders:     first.Seeders,
		Leechers:    first.Leechers,
		Snatches:    first.Snatches,
		Promotion:   first.Promotion,
	}
	store, err = storage.OpenSQLite(ctx, settings.DBPath)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	if _, err := store.UpsertTorrents(ctx, []storage.TorrentRecord{record}); err != nil {
		t.Fatalf("upsert torrent: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store before app: %v", err)
	}

	siteDir := t.TempDir()
	writeSiteDefinition(t, siteDir, definition)

	cfg := config.Default()
	cfg.Storage.Path = settings.DBPath
	cfg.SitesDir = siteDir
	app, err := core.NewApp(ctx, cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	torrent, err := app.FetchTorrentDetail(ctx, first.SiteID, strconv.Itoa(first.ID))
	if err != nil {
		t.Fatalf("fetch real torrent detail: %v", err)
	}
	if torrent.DetailTitle == "" || torrent.DetailRawText == "" || torrent.DetailFetchedAt == "" {
		t.Fatalf("expected persisted real detail fields, got %#v", torrent)
	}
	if torrent.Subtitle == "" && torrent.DetailDescription == "" {
		t.Fatalf("expected subtitle or detail description from real detail page, got %#v", torrent)
	}
	t.Logf("fetched detail for %s/%s: title=%q subtitle=%q description_len=%d raw_len=%d",
		torrent.SiteID, torrent.ID, torrent.DetailTitle, torrent.Subtitle, len(torrent.DetailDescription), len(torrent.DetailRawText))
}

func writeSiteDefinition(t *testing.T, dir string, definition parser.SiteDefinition) {
	t.Helper()
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal site definition: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, definition.ID+".json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write site definition: %v", err)
	}
}
