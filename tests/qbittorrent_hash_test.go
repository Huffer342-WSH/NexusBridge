package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexusbridge/internal/qbittorrent"
)

func TestParseTorrentHashes(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		wantV1 string
		wantV2 string
	}{
		{
			name:   "v1",
			data:   "d4:infod6:lengthi0e4:name4:test12:piece lengthi16384e6:pieces0:ee",
			wantV1: "444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4",
		},
		{
			name:   "v2",
			data:   "d4:infod9:file treede12:meta versioni2e4:name4:test12:piece lengthi16384eee",
			wantV2: "ecececfea6c100a6fdd280c6f24f89253950456689131f411b6c4d58ed53f1cb",
		},
		{
			name: "hybrid",
			data: "d4:infod9:file treede6:lengthi1e12:meta versioni2e4:name4:test12:piece lengthi16384eee",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashes, err := qbittorrent.ParseTorrentHashes([]byte(tt.data))
			if err != nil {
				t.Fatalf("parse torrent hashes: %v", err)
			}
			if tt.wantV1 != "" && hashes.V1 != tt.wantV1 {
				t.Fatalf("expected v1 %s, got %s", tt.wantV1, hashes.V1)
			}
			if tt.wantV2 != "" && hashes.V2 != tt.wantV2 {
				t.Fatalf("expected v2 %s, got %s", tt.wantV2, hashes.V2)
			}
			if tt.name == "v2" && hashes.V1 != "" {
				t.Fatalf("expected pure v2 torrent, got v1 %s", hashes.V1)
			}
			if tt.name == "hybrid" && (hashes.V1 == "" || hashes.V2 == "") {
				t.Fatalf("expected hybrid hashes, got %#v", hashes)
			}
		})
	}
}

func TestParseTorrentHashesRejectsInvalidData(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not-bencode"), []byte("de"), []byte("d4:infode")} {
		if _, err := qbittorrent.ParseTorrentHashes(data); err == nil {
			t.Fatalf("expected invalid torrent data %q to fail", data)
		}
	}
}

func TestAddTorrentFileVerifiedUsesResponseHash(t *testing.T) {
	const (
		data         = "d4:infod6:lengthi0e4:name4:test12:piece lengthi16384e6:pieces0:ee"
		expectedHash = "444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/add":
			if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("paused") != "true" {
				http.Error(w, "invalid multipart request", http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("torrents")
			if err != nil {
				http.Error(w, "torrent file is required", http.StatusBadRequest)
				return
			}
			defer file.Close()
			uploaded, _ := io.ReadAll(file)
			if string(uploaded) != data {
				http.Error(w, "unexpected torrent data", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"added_torrent_ids":["` + expectedHash + `"],"failure_count":0,"pending_count":0,"success_count":1}`))
		case "/api/v2/torrents/info":
			http.Error(w, "response hash should avoid polling", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := qbittorrent.New(qbittorrent.Config{AuthMode: "api_key", URL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new qBittorrent client: %v", err)
	}
	added, err := client.AddTorrentFileVerified(t.Context(), qbittorrent.AddOptions{Data: []byte(data), Paused: true})
	if err != nil {
		t.Fatalf("add verified torrent: %v", err)
	}
	if added.Hash != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, added.Hash)
	}
}

func TestAddTorrentFileVerifiedReconcilesConflict(t *testing.T) {
	const (
		data         = "d4:infod6:lengthi0e4:name4:test12:piece lengthi16384e6:pieces0:ee"
		expectedHash = "444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/add":
			http.Error(w, "Conflict", http.StatusConflict)
		case "/api/v2/torrents/info":
			if r.URL.Query().Get("hashes") != expectedHash {
				http.Error(w, "unexpected hash", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"hash":"` + expectedHash + `"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := qbittorrent.New(qbittorrent.Config{AuthMode: "api_key", URL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new qBittorrent client: %v", err)
	}
	added, err := client.AddTorrentFileVerified(t.Context(), qbittorrent.AddOptions{Data: []byte(data)})
	if err != nil {
		t.Fatalf("reconcile duplicate torrent: %v", err)
	}
	if added.Hash != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, added.Hash)
	}
}

func TestAddTorrentFileVerifiedRejectsPureV2(t *testing.T) {
	const pureV2 = "d4:infod9:file treede12:meta versioni2e4:name4:test12:piece lengthi16384eee"
	client, err := qbittorrent.New(qbittorrent.Config{AuthMode: "api_key", URL: "http://127.0.0.1:1", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new qBittorrent client: %v", err)
	}
	_, err = client.AddTorrentFileVerified(t.Context(), qbittorrent.AddOptions{Data: []byte(pureV2)})
	if err == nil || !strings.Contains(err.Error(), "pure v2") {
		t.Fatalf("expected pure v2 rejection, got %v", err)
	}
}

func TestQBittorrentTorrentFileHashRealAPI(t *testing.T) {
	client := realQBClient(t)
	if err := client.Test(t.Context()); err != nil {
		t.Fatalf("test qBittorrent api: %v", err)
	}
	torrentURL := requiredEnv(t, "NEXUSBRIDGE_TEST_QB_TORRENT_URL")
	data := downloadTestTorrent(t, torrentURL)
	hashes, err := qbittorrent.ParseTorrentHashes(data)
	if err != nil {
		t.Fatalf("parse downloaded torrent hash: %v", err)
	}
	if hashes.V1 == "" {
		t.Fatalf("real qB hash test requires a v1 or hybrid torrent")
	}
	t.Logf("calculated torrent hashes: v1=%s v2=%s", hashes.V1, hashes.V2)

	existing, err := client.ListTorrentsWithOptions(t.Context(), qbittorrent.TorrentListOptions{Hashes: []string{hashes.V1}})
	if err != nil {
		t.Fatalf("query existing torrent by hash: %v", err)
	}
	if len(existing) > 0 {
		t.Skipf("torrent %s already exists in qBittorrent", hashes.V1)
	}

	addOptions := qbittorrent.AddOptions{
		Name:   "nexusbridge-hash-test.torrent",
		Data:   data,
		Paused: true,
	}
	added, err := client.AddTorrentFileVerified(t.Context(), addOptions)
	if err != nil {
		t.Fatalf("add and verify torrent file: %v", err)
	}
	defer func() {
		if err := client.DeleteTorrents(t.Context(), []string{hashes.V1}, false); err != nil {
			t.Logf("cleanup delete torrent %s failed: %v", hashes.V1, err)
		}
	}()
	if !strings.EqualFold(added.Hash, hashes.V1) {
		t.Fatalf("expected local hash %s, got qB hash %s", hashes.V1, added.Hash)
	}
	if _, err := client.GetTorrentProperties(t.Context(), hashes.V1); err != nil {
		t.Fatalf("query added torrent by local hash: %v", err)
	}

	t.Logf("repeat adding torrent with local v1 hash %s", hashes.V1)
	repeated, err := client.AddTorrentFileVerified(t.Context(), addOptions)
	if err != nil {
		t.Fatalf("reconcile repeat add: %v", err)
	}
	if !strings.EqualFold(repeated.Hash, hashes.V1) {
		t.Fatalf("expected repeat add hash %s, got %s", hashes.V1, repeated.Hash)
	}
	t.Logf("repeat add reconciled as existing torrent hash %s", repeated.Hash)
}

func downloadTestTorrent(t *testing.T, rawURL string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("create torrent request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("download torrent: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("download torrent status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read torrent response: %v", err)
	}
	return data
}
