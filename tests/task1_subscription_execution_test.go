// task1_subscription_execution_test.go 验证自动订阅的优先级独占和配额恢复。
package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/storage"
)

// TestTask1SubscriptionPriorityQuotaAndNextDayRecovery 验证首个订阅独占、配额未读和下一自然日恢复。
func TestTask1SubscriptionPriorityQuotaAndNextDayRecovery(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("fixtures", "torrents_page.html"))
	if err != nil {
		t.Fatalf("read torrents fixture: %v", err)
	}
	const (
		firstData  = "d4:infod6:lengthi0e4:name5:first12:piece lengthi16384e6:pieces0:ee"
		secondData = "d4:infod6:lengthi0e4:name6:second12:piece lengthi16384e6:pieces0:ee"
	)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/torrents.php":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
		case "/download.php":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			if request.URL.Query().Get("id") == "43042" {
				_, _ = w.Write([]byte(firstData))
			} else {
				_, _ = w.Write([]byte(secondData))
			}
		default:
			http.NotFound(w, request)
		}
	}))
	defer site.Close()

	fakeQB := newTask1FakeQB(t)
	tempDir := t.TempDir()
	sitesDir := filepath.Join(tempDir, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatalf("create sites dir: %v", err)
	}
	definition, err := os.ReadFile(filepath.Join("fixtures", "site_parse_config.json"))
	if err != nil {
		t.Fatalf("read site definition: %v", err)
	}
	definition = []byte(strings.ReplaceAll(string(definition), "https://example.invalid", site.URL))
	if err := os.WriteFile(filepath.Join(sitesDir, "test-site.json"), definition, 0o600); err != nil {
		t.Fatalf("write site definition: %v", err)
	}
	dbPath := filepath.Join(tempDir, "subscriptions.db")
	cfg := config.Default()
	cfg.Storage.Path = dbPath
	cfg.SitesDir = sitesDir
	cfg.QBittorrent = config.QBittorrentConfig{AuthMode: "api_key", URL: fakeQB.server.URL, APIKey: "test-key", AutoSync: false}
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.SaveSiteCredential(t.Context(), core.SiteCredential{SiteID: "test-site", Cookie: "sid=test"}); err != nil {
		t.Fatalf("save site credential: %v", err)
	}
	rule, err := app.SaveRule(t.Context(), core.Rule{Name: "Auto", SiteIDs: []string{"test-site"}})
	if err != nil {
		t.Fatalf("save rule: %v", err)
	}
	baseSubscription := core.Subscription{
		Name: "Low", RuleName: rule.Name, Enabled: true, SiteIDs: []string{"test-site"}, Priority: 10,
		Download: core.DownloadOptions{QBCategory: "PT/ASMR", FilenameTemplate: "{{title}}"},
	}
	low := baseSubscription
	low.ID = "low"
	if _, err := app.SaveSubscription(t.Context(), low); err != nil {
		t.Fatalf("save low-priority subscription: %v", err)
	}
	high := baseSubscription
	high.ID, high.Name, high.Priority, high.Download.DailyLimit = "high", "High", 100, 1
	if _, err := app.SaveSubscription(t.Context(), high); err != nil {
		t.Fatalf("save high-priority subscription: %v", err)
	}

	firstRun, err := app.RunOnce(t.Context(), "test-site")
	if err != nil || firstRun.Matched != 2 || firstRun.DownloadSent != 1 {
		t.Fatalf("unexpected first subscription run: result=%#v err=%v", firstRun, err)
	}
	if got := len(fakeQB.addFormsSnapshot()); got != 1 {
		t.Fatalf("daily_limit=1 must send exactly one torrent, got %d", got)
	}
	highCandidates, err := app.ListSubscriptionCandidates(t.Context(), high.ID, "")
	if err != nil || len(highCandidates) != 2 {
		t.Fatalf("high-priority subscription must own both candidates: %#v err=%v", highCandidates, err)
	}
	lowCandidates, err := app.ListSubscriptionCandidates(t.Context(), low.ID, "")
	if err != nil || len(lowCandidates) != 0 {
		t.Fatalf("lower-priority subscription must not receive fallback candidates: %#v err=%v", lowCandidates, err)
	}
	if task1CandidateStatusCount(highCandidates, storage.SubscriptionCandidateProcessed) != 1 ||
		task1CandidateStatusCount(highCandidates, storage.SubscriptionCandidateUnread) != 1 {
		t.Fatalf("quota-blocked candidate must remain unread: %#v", highCandidates)
	}

	secondRun, err := app.RunOnce(t.Context(), "test-site")
	if err != nil || secondRun.DownloadSent != 0 || len(fakeQB.addFormsSnapshot()) != 1 {
		t.Fatalf("same-day replay must keep the unread candidate quota-blocked: result=%#v err=%v", secondRun, err)
	}

	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open sqlite for day-boundary fixture: %v", err)
	}
	tasks, err := store.ListDownloadTasks(t.Context())
	if err != nil {
		_ = store.Close()
		t.Fatalf("list tasks for day-boundary fixture: %v", err)
	}
	for _, task := range tasks {
		if task.Status == "sent" {
			task.SentAt = time.Now().Add(-25 * time.Hour)
			if err := store.UpdateDownloadTaskRecord(t.Context(), task); err != nil {
				_ = store.Close()
				t.Fatalf("move sent task to previous local day: %v", err)
			}
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close day-boundary sqlite: %v", err)
	}

	thirdRun, err := app.RunOnce(t.Context(), "test-site")
	if err != nil || thirdRun.DownloadSent != 1 || len(fakeQB.addFormsSnapshot()) != 2 {
		t.Fatalf("next-day quota must release the unread candidate: result=%#v err=%v", thirdRun, err)
	}
	highCandidates, err = app.ListSubscriptionCandidates(t.Context(), high.ID, "")
	if err != nil || task1CandidateStatusCount(highCandidates, storage.SubscriptionCandidateProcessed) != 2 {
		t.Fatalf("both high-priority candidates must be processed after quota recovery: %#v err=%v", highCandidates, err)
	}
	fakeQB.assertNoContractErrors(t)
}

func task1CandidateStatusCount(candidates []core.SubscriptionCandidate, status string) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.Status == status {
			count++
		}
	}
	return count
}
