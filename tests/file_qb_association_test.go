package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/qbittorrent"
	"os"
	"path/filepath"
	"testing"
)

// TestFileQBExactAssociation 验证文件清单匹配、目录聚合及读取失败时不发布部分结果。
func TestFileQBExactAssociation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "匹配文件"
		if fail {
			name = "清单失败"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"sites", "pack"} {
				if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"single.mp4", "pack/episode.mp4", "pack/unrelated.mp4"} {
				if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte("offline fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/torrents/info":
					_ = json.NewEncoder(w).Encode([]qbittorrent.TorrentInfo{
						{Hash: "single", Name: "single", SavePath: root, ContentPath: filepath.Join(root, "single.mp4")},
						{Hash: "pack", Name: "pack", SavePath: root, ContentPath: filepath.Join(root, "pack")},
						{Hash: "shared", Name: "shared", SavePath: root, ContentPath: filepath.Join(root, "pack")},
					})
				case "/api/v2/torrents/files":
					if fail && r.URL.Query().Get("hash") == "shared" {
						http.Error(w, "offline failure", 500)
						return
					}
					name := "pack/episode.mp4"
					if r.URL.Query().Get("hash") == "single" {
						name = "single.mp4"
					}
					_ = json.NewEncoder(w).Encode([]qbittorrent.TorrentContent{{Name: name}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.SitesDir = filepath.Join(root, "sites")
			cfg.Storage.Path = filepath.Join(root, "test.db")
			cfg.QBittorrent.AutoSync = false
			cfg.QBittorrent.URL = server.URL
			cfg.QBittorrent.AuthMode = "api_key"
			cfg.QBittorrent.APIKey = "offline-test-key"
			app, err := core.NewApp(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			for _, directory := range []string{root, filepath.Join(root, "pack")} {
				result, err := app.BrowseFiles(t.Context(), core.FileBrowseRequest{Path: directory})
				if err != nil {
					t.Fatal(err)
				}
				if fail {
					if result.QBConnected || result.QBError == "" {
						t.Fatal("expected unknown association")
					}
					for _, entry := range result.Entries {
						if len(entry.QBTasks) != 0 {
							t.Fatal("partial association published")
						}
					}
					continue
				}
				if !result.QBConnected {
					t.Fatal(result.QBError)
				}
				for _, entry := range result.Entries {
					want := 0
					switch entry.Name {
					case "single.mp4":
						want = 1
					case "pack", "episode.mp4":
						want = 2
					}
					if len(entry.QBTasks) != want {
						t.Fatalf("%s: got %d tasks, want %d", entry.Name, len(entry.QBTasks), want)
					}
				}
			}
		})
	}
}
