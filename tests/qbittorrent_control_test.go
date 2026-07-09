// qbittorrent_control_test.go 验证 qBittorrent 单种子暂停和恢复原生接口。
package tests

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"nexusbridge/internal/qbittorrent"
)

// TestQBittorrentStartStopTorrents 验证 start/stop 请求路径和 hashes 参数。
func TestQBittorrentStartStopTorrents(t *testing.T) {
	type requestRecord struct {
		Path   string
		Hashes string
	}
	records := []requestRecord{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		records = append(records, requestRecord{Path: r.URL.Path, Hashes: r.Form.Get("hashes")})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := qbittorrent.New(qbittorrent.Config{AuthMode: "api_key", URL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	hashes := []string{"abc", "def"}
	if err := client.StopTorrents(t.Context(), hashes); err != nil {
		t.Fatal(err)
	}
	if err := client.StartTorrents(t.Context(), hashes); err != nil {
		t.Fatal(err)
	}
	want := []requestRecord{
		{Path: "/api/v2/torrents/stop", Hashes: "abc|def"},
		{Path: "/api/v2/torrents/start", Hashes: "abc|def"},
	}
	if !slices.Equal(records, want) {
		t.Fatalf("unexpected qB control requests: %#v", records)
	}
}

// TestQBittorrentUIDSessionIsReused 验证高频增量请求不会重复登录 qB。
func TestQBittorrentUIDSessionIsReused(t *testing.T) {
	loginCount, syncCount := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			loginCount++
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "session", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			syncCount++
			cookie, err := r.Cookie("SID")
			if err != nil || cookie.Value != "session" {
				t.Errorf("missing reused SID cookie: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rid":1,"full_update":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := qbittorrent.New(qbittorrent.Config{AuthMode: "uid", URL: server.URL, Username: "demo", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMainData(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMainData(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if loginCount != 1 || syncCount != 2 {
		t.Fatalf("expected one login and two sync calls, got login=%d sync=%d", loginCount, syncCount)
	}
}
