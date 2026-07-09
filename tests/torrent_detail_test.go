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

func TestParseTorrentDetailFromFixtureHTML(t *testing.T) {
	body := []byte(`<!doctype html>
<html>
<head><title>Fixture Detail :: Test Site</title></head>
<body>
<h1>Fixture Detail Title</h1>
<table>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">Fixture subtitle</td></tr>
  <tr><td class="rowhead">商品链接</td><td class="rowfollow"><a href="https://example.com/product/RJ000001">Product</a></td></tr>
  <tr><td class="rowhead">Hash码</td><td class="rowfollow">0123456789abcdef0123456789abcdef01234567</td></tr>
  <tr><td class="rowhead">简介</td><td class="rowfollow"><div id="kdescr">Fixture detail description</div></td></tr>
</table>
</body>
</html>`)
	detail, err := parser.ParseTorrentDetail(body, parser.TorrentDetailParseOptions{
		SiteID:    "test-site",
		TorrentID: "43042",
		BaseURL:   "https://example.invalid",
		URL:       "/details.php?id=43042",
	})
	if err != nil {
		t.Fatalf("parse fixture detail: %v", err)
	}
	if detail.DetailTitle != "Fixture Detail Title" || detail.Subtitle != "Fixture subtitle" {
		t.Fatalf("expected detail title and subtitle: %#v", detail)
	}
	if detail.ProductURL != "https://example.com/product/RJ000001" {
		t.Fatalf("expected product url: %#v", detail)
	}
	if detail.InfoHash != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("expected detail info hash: %#v", detail)
	}
	if detail.DetailDescription != "Fixture detail description" {
		t.Fatalf("expected detail description: %#v", detail)
	}
}

func TestParseTorrentDetailFromFetchedHTML(t *testing.T) {
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
	logJSON(t, "first parsed torrent before detail", first)
	record := storage.TorrentRecord{
		SiteID:      first.SiteID,
		TorrentID:   strconv.Itoa(first.ID),
		Title:       first.Title,
		Category:    first.Category,
		DetailURL:   first.DetailURL,
		DownloadURL: first.DownloadURL,
		CoverURL:    first.CoverURL,
		Tags:        first.Tags,
		TagIDs:      first.TagIDs,
		Subtitle:    first.Subtitle,
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
	if torrent.ProductURL == "" || torrent.DetailInfoHash == "" {
		t.Fatalf("expected product url and detail info hash from real detail page, got %#v", torrent)
	}
	logJSON(t, "torrent after detail fetch", torrent)
	t.Logf("fetched detail for %s/%s: title=%q subtitle=%q product_url=%q detail_info_hash=%q description_len=%d raw_len=%d",
		torrent.SiteID, torrent.ID, torrent.DetailTitle, torrent.Subtitle, torrent.ProductURL, torrent.DetailInfoHash, len(torrent.DetailDescription), len(torrent.DetailRawText))
}

func logJSON(t *testing.T, label string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	t.Logf("%s:\n%s", label, data)
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
