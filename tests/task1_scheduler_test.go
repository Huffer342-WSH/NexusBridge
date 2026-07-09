// task1_scheduler_test.go 使用可控时钟验证站点周期的动态更新。
package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
)

type task1FakeAutomationClock struct {
	mu    sync.Mutex
	now   time.Time
	ticks chan time.Time
}

func (clock *task1FakeAutomationClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *task1FakeAutomationClock) NewTicker(time.Duration) core.AutomationTicker {
	return task1FakeAutomationTicker{ticks: clock.ticks}
}

func (clock *task1FakeAutomationClock) advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	now := clock.now
	clock.mu.Unlock()
	clock.ticks <- now
}

type task1FakeAutomationTicker struct{ ticks <-chan time.Time }

func (ticker task1FakeAutomationTicker) C() <-chan time.Time { return ticker.ticks }
func (task1FakeAutomationTicker) Stop()                      {}

// TestTask1SchedulerFakeClockDynamicInterval 验证 fake clock 驱动和运行中更新后的站点周期。
func TestTask1SchedulerFakeClockDynamicInterval(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("fixtures", "torrents_page.html"))
	if err != nil {
		t.Fatalf("read torrents fixture: %v", err)
	}
	const torrentData = "d4:infod6:lengthi0e4:name5:clock12:piece lengthi16384e6:pieces0:ee"
	fetches := make(chan struct{}, 4)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/torrents.php":
			fetches <- struct{}{}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
		case "/download.php":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write([]byte(torrentData))
		default:
			http.NotFound(w, request)
		}
	}))
	defer site.Close()

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

	cfg := config.Default()
	cfg.Storage.Path = filepath.Join(tempDir, "scheduler.db")
	cfg.SitesDir = sitesDir
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.SaveSiteCredential(t.Context(), core.SiteCredential{SiteID: "test-site", Cookie: "sid=test"}); err != nil {
		t.Fatalf("save site credential: %v", err)
	}
	schedule, err := app.SaveSiteSchedule(t.Context(), core.SiteSchedule{SiteID: "test-site", Enabled: true, IntervalSeconds: 60})
	if err != nil || schedule.NextRunAt == nil {
		t.Fatalf("save initial schedule: schedule=%#v err=%v", schedule, err)
	}
	clock := &task1FakeAutomationClock{now: schedule.NextRunAt.Add(time.Second), ticks: make(chan time.Time, 4)}
	app.StartAutomationWithClock(t.Context(), clock)
	waitTask1SchedulerFetch(t, fetches)
	waitTask1ScheduleInterval(t, app, 60)

	if _, err := app.SaveSiteSchedule(t.Context(), core.SiteSchedule{SiteID: "test-site", Enabled: true, IntervalSeconds: 120}); err != nil {
		t.Fatalf("update dynamic schedule interval: %v", err)
	}
	clock.advance(2 * time.Minute)
	waitTask1SchedulerFetch(t, fetches)
	waitTask1ScheduleInterval(t, app, 120)
}

// TestTask1SiteLockCancellation 验证同站抓取不重入，且等待锁的请求可由 context 立即取消。
func TestTask1SiteLockCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	var mu sync.Mutex
	requests := 0
	active := 0
	maxActive := 0
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/torrents.php" {
			http.NotFound(w, request)
			return
		}
		mu.Lock()
		requests++
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		started <- struct{}{}
		<-request.Context().Done()
		mu.Lock()
		active--
		mu.Unlock()
	}))
	defer site.Close()
	app := newTask1SiteApp(t, site.URL)

	firstContext, cancelFirst := context.WithCancel(t.Context())
	firstDone := make(chan error, 1)
	go func() {
		_, err := app.FetchSite(firstContext, "test-site")
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first site fetch")
	}

	secondContext, cancelSecond := context.WithCancel(t.Context())
	secondDone := make(chan error, 1)
	go func() {
		_, err := app.FetchSite(secondContext, "test-site")
		secondDone <- err
	}()
	cancelSecond()
	select {
	case err := <-secondDone:
		if !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("waiting site fetch must return context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting site fetch did not stop after context cancellation")
	}
	mu.Lock()
	gotRequests, gotMaxActive := requests, maxActive
	mu.Unlock()
	if gotRequests != 1 || gotMaxActive != 1 {
		t.Fatalf("same-site fetch must not re-enter: requests=%d max_active=%d", gotRequests, gotMaxActive)
	}
	cancelFirst()
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("active site fetch did not stop after context cancellation")
	}
}

func newTask1SiteApp(t *testing.T, siteURL string) *core.App {
	t.Helper()
	tempDir := t.TempDir()
	sitesDir := filepath.Join(tempDir, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatalf("create sites dir: %v", err)
	}
	definition, err := os.ReadFile(filepath.Join("fixtures", "site_parse_config.json"))
	if err != nil {
		t.Fatalf("read site definition: %v", err)
	}
	definition = []byte(strings.ReplaceAll(string(definition), "https://example.invalid", siteURL))
	if err := os.WriteFile(filepath.Join(sitesDir, "test-site.json"), definition, 0o600); err != nil {
		t.Fatalf("write site definition: %v", err)
	}
	cfg := config.Default()
	cfg.Storage.Path = filepath.Join(tempDir, "site-lock.db")
	cfg.SitesDir = sitesDir
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.SaveSiteCredential(t.Context(), core.SiteCredential{SiteID: "test-site", Cookie: "sid=test"}); err != nil {
		t.Fatalf("save site credential: %v", err)
	}
	return app
}

func waitTask1SchedulerFetch(t *testing.T, fetches <-chan struct{}) {
	t.Helper()
	select {
	case <-fetches:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for scheduled site fetch")
	}
}

func waitTask1ScheduleInterval(t *testing.T, app *core.App, seconds int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		schedule, err := app.GetSiteSchedule(t.Context(), "test-site")
		if err == nil && schedule.LastRunAt != nil && schedule.NextRunAt != nil &&
			int(schedule.NextRunAt.Sub(*schedule.LastRunAt).Seconds()) == seconds {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for persisted %d-second schedule interval", seconds)
}
