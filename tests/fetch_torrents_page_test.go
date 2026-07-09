package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/storage"
)

func TestFetchTorrentsPageHTML(t *testing.T) {
	settings := loadTestSettings(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	curlRequest := readCurlRequest(t, settings.CurlFile)
	cookies, err := fetcher.LoadCookiesFromHeader(curlRequest.CookieHeader)
	if err != nil {
		t.Fatalf("load cookies from curl input file: %v", err)
	}

	store, err := storage.OpenSQLite(ctx, settings.DBPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	if err := store.SaveCookies(ctx, settings.BaseURL, cookies); err != nil {
		t.Fatalf("save cookies: %v", err)
	}
	dbCookies, err := store.LoadCookies(ctx, settings.BaseURL)
	if err != nil {
		t.Fatalf("load cookies: %v", err)
	}
	if len(dbCookies) == 0 {
		t.Fatal("expected cookies loaded from database")
	}

	result, err := fetcher.FetchTorrentsPage(ctx, fetcher.FetchOptions{
		BaseURL: settings.BaseURL,
		Cookies: dbCookies,
		Headers: curlRequest.Headers,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("fetch torrents page with database cookies: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if len(result.Body) == 0 {
		t.Fatal("expected non-empty HTML body")
	}

	if err := saveHTML(settings.SaveHTMLPath, result.Body); err != nil {
		t.Fatalf("save html: %v", err)
	}
	t.Logf("saved %d bytes from %s to %s; cookie database: %s", len(result.Body), result.URL, settings.SaveHTMLPath, settings.DBPath)

	if os.Getenv("NEXUSBRIDGE_TEST_PRINT_HTML") == "1" {
		fmt.Println(string(result.Body))
	}
}

func saveHTML(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
