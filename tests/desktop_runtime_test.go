// desktop_runtime_test.go 验证桌面数据初始化和共享 HTTP API 接入。
package tests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexusbridge/internal/desktop"
)

// TestDesktopPrepareData 验证用户目录初始化、相对路径解析和站点文件保护。
func TestDesktopPrepareData(t *testing.T) {
	dataDir := t.TempDir()
	prepared, err := desktop.PrepareData(desktop.Options{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{prepared.ConfigPath, prepared.Config.Storage.Path, prepared.Config.SitesDir, prepared.Config.Logging.File} {
		if !filepath.IsAbs(path) {
			t.Fatalf("expected absolute desktop path, got %q", path)
		}
	}
	sitePath := filepath.Join(prepared.Config.SitesDir, "kamept.json")
	if _, err := os.Stat(sitePath); err != nil {
		t.Fatalf("builtin site was not installed: %v", err)
	}
	if err := os.WriteFile(sitePath, []byte(`{"custom":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.PrepareData(desktop.Options{DataDir: dataDir}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sitePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"custom":true}` {
		t.Fatalf("existing site definition was overwritten: %s", data)
	}
}

// TestDesktopRuntimeServesSharedAPI 验证桌面中间件复用现有 API 并保留静态资源回退。
func TestDesktopRuntimeServesSharedAPI(t *testing.T) {
	runtime, err := desktop.Start(context.Background(), desktop.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "frontend")
	})
	handler := desktop.APIMiddleware(runtime.Handler())(fallback)

	session := httptest.NewRecorder()
	handler.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if session.Code != http.StatusOK || !strings.Contains(session.Body.String(), `"requires_login":false`) {
		t.Fatalf("unexpected desktop session response: status=%d body=%s", session.Code, session.Body.String())
	}

	sites := httptest.NewRecorder()
	handler.ServeHTTP(sites, httptest.NewRequest(http.MethodGet, "/api/sites", nil))
	if sites.Code != http.StatusOK || !strings.Contains(sites.Body.String(), `"id":"kamept"`) {
		t.Fatalf("unexpected desktop sites response: status=%d body=%s", sites.Code, sites.Body.String())
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/index.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "frontend" {
		t.Fatalf("static request did not reach fallback: status=%d body=%s", asset.Code, asset.Body.String())
	}
}
