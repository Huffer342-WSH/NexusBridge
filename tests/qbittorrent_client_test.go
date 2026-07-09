package tests

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

var (
	qbRenameHash    = flag.String("hash", "", "existing qB torrent hash used by the rename experiment")
	qbRenameOldPath = flag.String("oldpath", "", "current qB relative file or folder path")
	qbRenameNewPath = flag.String("newpath", "", "new qB relative file or folder path")
	qbRenameKind    = flag.String("kind", "file", "rename endpoint to test: file or folder")
	qbRenameRestore = flag.Bool("restore", true, "restore the original path after the experiment")
)

func TestQBittorrentClientReadRealAPI(t *testing.T) {
	t.Parallel()

	client := realQBClient(t)
	if err := client.Test(t.Context()); err != nil {
		t.Fatalf("test qbittorrent api: %v", err)
	}
	if _, err := client.GetCategories(t.Context()); err != nil {
		t.Fatalf("get categories: %v", err)
	}
	if _, err := client.GetTags(t.Context()); err != nil {
		t.Fatalf("get tags: %v", err)
	}
	torrents, err := client.ListTorrentsWithOptions(t.Context(), qbittorrent.TorrentListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list torrents: %v", err)
	}
	if len(torrents) == 0 {
		t.Log("qBittorrent has no torrents; properties and contents read checks skipped")
		return
	}
	// 列表只打印前 3 项，避免任务较多时测试日志过长。
	sampleCount := min(3, len(torrents))
	logJSON(t, "qBittorrent torrent list sample", torrents[:sampleCount])

	// 选择列表第一项，完整打印属性和文件明细，便于核对 WebAPI 字段映射。
	hash := torrents[0].Hash
	properties, err := client.GetTorrentProperties(t.Context(), hash)
	if err != nil {
		t.Fatalf("get torrent properties: %v", err)
	}
	logJSON(t, "qBittorrent torrent properties for "+hash, properties)

	contents, err := client.GetTorrentContents(t.Context(), hash, nil)
	if err != nil {
		t.Fatalf("get torrent contents: %v", err)
	}
	logJSON(t, "qBittorrent torrent contents for "+hash, contents)
}

// TestQBittorrentTorrentDetailsRealAPI 打印环境变量指定任务的完整属性和文件明细。
func TestQBittorrentTorrentDetailsRealAPI(t *testing.T) {
	t.Parallel()

	client := realQBClient(t)
	targetHash := strings.ToLower(strings.TrimSpace(requiredEnv(t, "NEXUSBRIDGE_TEST_QB_HASH")))

	properties, err := client.GetTorrentProperties(t.Context(), targetHash)
	if err != nil {
		t.Fatalf("get torrent %s properties: %v", targetHash, err)
	}
	logJSON(t, "qBittorrent torrent properties for "+targetHash, properties)

	contents, err := client.GetTorrentContents(t.Context(), targetHash, nil)
	if err != nil {
		t.Fatalf("get torrent %s contents: %v", targetHash, err)
	}
	logJSON(t, "qBittorrent torrent contents for "+targetHash, contents)
}

// TestQBittorrentRenameRealAPI 使用命令行参数验证 qB 文件或目录重命名接口。
func TestQBittorrentRenameRealAPI(t *testing.T) {
	targetHash := strings.TrimSpace(*qbRenameHash)
	oldPath := strings.TrimSpace(*qbRenameOldPath)
	newPath := strings.TrimSpace(*qbRenameNewPath)
	if targetHash == "" || oldPath == "" || newPath == "" {
		t.Skip("set -hash, -oldpath and -newpath after go test -args")
	}
	kind := strings.ToLower(strings.TrimSpace(*qbRenameKind))
	if kind != "file" && kind != "folder" {
		t.Fatalf("invalid -kind %q: expected file or folder", kind)
	}

	client := realQBClient(t)
	before, err := client.GetTorrentContents(t.Context(), targetHash, nil)
	if err != nil {
		t.Fatalf("get torrent contents before rename: %v", err)
	}
	// 先打印并检查 qB 当前文件列表，防止参数写错后仍修改真实任务。
	logJSON(t, "qBittorrent torrent contents before rename", before)
	if !qbContentsContainPath(before, oldPath, kind) {
		t.Fatalf("%s path %q does not exist in torrent %s", kind, oldPath, targetHash)
	}

	rename := func(from, to string) error {
		if kind == "folder" {
			return client.RenameTorrentFolder(t.Context(), targetHash, from, to)
		}
		return client.RenameTorrentFile(t.Context(), targetHash, from, to)
	}
	renamed := false
	defer func() {
		if !renamed || !*qbRenameRestore {
			return
		}
		// 默认反向重命名，尽量不在真实 qB 任务中留下实验性修改。
		if err := rename(newPath, oldPath); err != nil {
			t.Logf("restore %s path %q to %q failed: %v", kind, newPath, oldPath, err)
			return
		}
		if _, err := waitQBContentsPath(t, client, targetHash, oldPath, kind); err != nil {
			t.Logf("wait restored %s path failed: %v", kind, err)
		}
	}()

	if err := rename(oldPath, newPath); err != nil {
		t.Fatalf("rename %s path %q to %q: %v", kind, oldPath, newPath, err)
	}
	renamed = true
	after, err := waitQBContentsPath(t, client, targetHash, newPath, kind)
	if err != nil {
		t.Fatalf("wait renamed %s path: %v", kind, err)
	}
	// qB 的 files.name 应显示重命名后的完整相对路径。
	logJSON(t, "qBittorrent torrent contents after rename", after)
}

func TestQBittorrentClientClosedLoopRealAPI(t *testing.T) {
	client := realQBClient(t)
	torrentURL := requiredEnv(t, "NEXUSBRIDGE_TEST_QB_TORRENT_URL")

	category := envOrDefault("NEXUSBRIDGE_TEST_QB_CATEGORY", "nexusbridge-test")
	tag := envOrDefault("NEXUSBRIDGE_TEST_QB_TAG", "nexusbridge-test")
	categoryPtr := &category

	categories, err := client.GetCategories(t.Context())
	if err != nil {
		t.Fatalf("initial get categories: %v", err)
	}
	tags, err := client.GetTags(t.Context())
	if err != nil {
		t.Fatalf("initial get tags: %v", err)
	}
	categoryExisted := categoryExists(categories, category)
	tagExisted := stringInSlice(tags, tag)

	initialTorrents, err := client.ListTorrentsWithOptions(t.Context(), qbittorrent.TorrentListOptions{Category: categoryPtr})
	if err != nil {
		t.Fatalf("initial list category torrents: %v", err)
	}
	initialHashes := torrentHashSet(initialTorrents)

	var addedHash string
	defer func() {
		if addedHash != "" {
			if err := client.DeleteTorrents(t.Context(), []string{addedHash}, false); err != nil {
				t.Logf("cleanup delete torrent %s failed: %v", addedHash, err)
			}
			if err := waitTorrentGone(t, client, addedHash); err != nil {
				t.Logf("cleanup wait torrent gone failed: %v", err)
			}
		}
		if !tagExisted {
			if err := client.DeleteTags(t.Context(), []string{tag}); err != nil {
				t.Logf("cleanup delete tag %q failed: %v", tag, err)
			}
		}
		if !categoryExisted {
			if err := client.RemoveCategories(t.Context(), []string{category}); err != nil {
				t.Logf("cleanup remove category %q failed: %v", category, err)
			}
		}
	}()

	if !categoryExisted {
		if err := client.CreateCategory(t.Context(), category, ""); err != nil {
			t.Fatalf("create category: %v", err)
		}
	}
	if !tagExisted {
		if err := client.CreateTags(t.Context(), []string{tag}); err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	if err := client.AddTorrentURL(t.Context(), qbittorrent.AddOptions{
		URL:      torrentURL,
		Category: category,
		Tags:     []string{tag},
		Paused:   true,
	}); err != nil {
		t.Fatalf("add torrent url: %v", err)
	}

	added, err := waitNewTorrent(t, client, category, tag, initialHashes)
	if err != nil {
		t.Fatalf("wait new torrent: %v", err)
	}
	addedHash = added.Hash
	if added.Category != category {
		t.Fatalf("expected category %q, got %q", category, added.Category)
	}
	if !torrentHasTag(added, tag) {
		t.Fatalf("expected tag %q in %q", tag, added.Tags)
	}

	if _, err := client.GetTorrentProperties(t.Context(), addedHash); err != nil {
		t.Fatalf("get added torrent properties: %v", err)
	}
	if _, err := client.GetTorrentContents(t.Context(), addedHash, nil); err != nil {
		t.Fatalf("get added torrent contents: %v", err)
	}

	if err := client.SetTorrentCategory(t.Context(), []string{addedHash}, category); err != nil {
		t.Fatalf("set torrent category: %v", err)
	}
	if err := client.AddTorrentTags(t.Context(), []string{addedHash}, []string{tag}); err != nil {
		t.Fatalf("add torrent tag: %v", err)
	}
	if err := client.RemoveTorrentTags(t.Context(), []string{addedHash}, []string{tag}); err != nil {
		t.Fatalf("remove torrent tag: %v", err)
	}
	if err := client.AddTorrentTags(t.Context(), []string{addedHash}, []string{tag}); err != nil {
		t.Fatalf("restore torrent tag: %v", err)
	}

	if err := client.DeleteTorrents(t.Context(), []string{addedHash}, false); err != nil {
		t.Fatalf("delete added torrent: %v", err)
	}
	if err := waitTorrentGone(t, client, addedHash); err != nil {
		t.Fatalf("wait deleted torrent gone: %v", err)
	}
	addedHash = ""

	if !tagExisted {
		if err := client.DeleteTags(t.Context(), []string{tag}); err != nil {
			t.Fatalf("delete test tag: %v", err)
		}
		assertTagAbsent(t, client, tag)
	}
	if !categoryExisted {
		if err := client.RemoveCategories(t.Context(), []string{category}); err != nil {
			t.Fatalf("remove test category: %v", err)
		}
		assertCategoryAbsent(t, client, category)
	}
}

func TestQBittorrentStatusRealAPI(t *testing.T) {
	client := realQBClient(t)
	torrentURL := requiredEnv(t, "NEXUSBRIDGE_TEST_QB_TORRENT_URL")
	category := envOrDefault("NEXUSBRIDGE_TEST_QB_CATEGORY", "nexusbridge-test")
	tag := envOrDefault("NEXUSBRIDGE_TEST_QB_TAG", "nexusbridge-test")
	categoryPtr := &category

	categories, err := client.GetCategories(t.Context())
	if err != nil {
		t.Fatalf("initial get categories: %v", err)
	}
	tags, err := client.GetTags(t.Context())
	if err != nil {
		t.Fatalf("initial get tags: %v", err)
	}
	categoryExisted := categoryExists(categories, category)
	tagExisted := stringInSlice(tags, tag)

	initialTorrents, err := client.ListTorrentsWithOptions(t.Context(), qbittorrent.TorrentListOptions{Category: categoryPtr})
	if err != nil {
		t.Fatalf("initial list category torrents: %v", err)
	}
	initialHashes := torrentHashSet(initialTorrents)

	var addedHash string
	defer func() {
		if addedHash != "" {
			if err := client.DeleteTorrents(t.Context(), []string{addedHash}, false); err != nil {
				t.Logf("cleanup delete torrent %s failed: %v", addedHash, err)
			}
			if err := waitTorrentGone(t, client, addedHash); err != nil {
				t.Logf("cleanup wait torrent gone failed: %v", err)
			}
		}
		if !tagExisted {
			if err := client.DeleteTags(t.Context(), []string{tag}); err != nil {
				t.Logf("cleanup delete tag %q failed: %v", tag, err)
			}
		}
		if !categoryExisted {
			if err := client.RemoveCategories(t.Context(), []string{category}); err != nil {
				t.Logf("cleanup remove category %q failed: %v", category, err)
			}
		}
	}()

	if !categoryExisted {
		if err := client.CreateCategory(t.Context(), category, ""); err != nil {
			t.Fatalf("create category: %v", err)
		}
	}
	if !tagExisted {
		if err := client.CreateTags(t.Context(), []string{tag}); err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}
	if err := client.AddTorrentURL(t.Context(), qbittorrent.AddOptions{
		URL:      torrentURL,
		Category: category,
		Tags:     []string{tag},
		Paused:   true,
	}); err != nil {
		t.Fatalf("add torrent url: %v", err)
	}
	added, err := waitNewTorrent(t, client, category, tag, initialHashes)
	if err != nil {
		t.Fatalf("wait new torrent: %v", err)
	}
	addedHash = added.Hash

	dbPath := filepath.Join(t.TempDir(), "nexusbridge.db")
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := store.UpsertTorrents(t.Context(), []storage.TorrentRecord{{
		SiteID:      "qb-real",
		TorrentID:   "1",
		Title:       added.Name,
		DetailURL:   torrentURL,
		DownloadURL: torrentURL,
	}}); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}
	if created, err := store.CreateDownloadTaskIfAbsent(t.Context(), storage.DownloadTaskRecord{
		ID:           "qb-real:1:manual",
		SiteID:       "qb-real",
		TorrentID:    "1",
		RuleName:     "manual",
		Status:       "sent",
		TorrentTitle: added.Name,
		DownloadURL:  torrentURL,
	}); err != nil {
		t.Fatalf("insert download task: %v", err)
	} else if !created {
		t.Fatalf("expected download task to be created")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	cfg := config.Default()
	cfg.Storage.Path = dbPath
	cfg.SitesDir = t.TempDir()
	cfg.QBittorrent = config.QBittorrentConfig{
		AuthMode: "api_key",
		URL:      requiredEnv(t, "NEXUSBRIDGE_TEST_QB_URL"),
		APIKey:   requiredEnv(t, "NEXUSBRIDGE_TEST_QB_API_KEY"),
		Tags:     []string{},
	}
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	status, err := app.GetTorrentQBStatus(t.Context(), "qb-real", "1", false)
	if err != nil {
		t.Fatalf("get qb status: %v", err)
	}
	if !status.Available || !status.Added || status.Hash != addedHash || status.Source != "task_title" {
		t.Fatalf("expected status linked by task title, got %#v", status)
	}
	torrents, err := app.ListTorrents(t.Context(), core.TorrentQuery{IncludeQB: true})
	if err != nil {
		t.Fatalf("list torrents with qb status: %v", err)
	}
	if len(torrents) != 1 || torrents[0].QBStatus == nil || torrents[0].QBStatus.Hash != addedHash {
		t.Fatalf("expected embedded qb status, got %#v", torrents)
	}
}

func realQBClient(t *testing.T) *qbittorrent.Client {
	t.Helper()
	client, err := qbittorrent.New(qbittorrent.Config{
		AuthMode: "api_key",
		URL:      requiredEnv(t, "NEXUSBRIDGE_TEST_QB_URL"),
		APIKey:   requiredEnv(t, "NEXUSBRIDGE_TEST_QB_API_KEY"),
	})
	if err != nil {
		t.Fatalf("new qbittorrent client: %v", err)
	}
	return client
}

func waitNewTorrent(t *testing.T, client *qbittorrent.Client, category, tag string, initial map[string]struct{}) (qbittorrent.TorrentInfo, error) {
	t.Helper()
	categoryPtr := &category
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		torrents, err := client.ListTorrentsWithOptions(t.Context(), qbittorrent.TorrentListOptions{Category: categoryPtr})
		if err != nil {
			return qbittorrent.TorrentInfo{}, err
		}
		for _, torrent := range torrents {
			if torrent.Hash == "" {
				continue
			}
			if _, ok := initial[torrent.Hash]; ok {
				continue
			}
			if torrent.Category == category && torrentHasTag(torrent, tag) {
				return torrent, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return qbittorrent.TorrentInfo{}, os.ErrDeadlineExceeded
}

func waitTorrentGone(t *testing.T, client *qbittorrent.Client, hash string) error {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		torrents, err := client.ListTorrentsWithOptions(t.Context(), qbittorrent.TorrentListOptions{Hashes: []string{hash}})
		if err != nil {
			return err
		}
		if len(torrents) == 0 {
			return nil
		}
		time.Sleep(time.Second)
	}
	return os.ErrDeadlineExceeded
}

func waitQBContentsPath(t *testing.T, client *qbittorrent.Client, hash, path, kind string) ([]qbittorrent.TorrentContent, error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := client.GetTorrentContents(t.Context(), hash, nil)
		if err != nil {
			return nil, err
		}
		if qbContentsContainPath(contents, path, kind) {
			return contents, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, os.ErrDeadlineExceeded
}

func qbContentsContainPath(contents []qbittorrent.TorrentContent, expected, kind string) bool {
	expected = normalizeQBContentPath(expected)
	for _, content := range contents {
		actual := normalizeQBContentPath(content.Name)
		if actual == expected || kind == "folder" && strings.HasPrefix(actual, expected+"/") {
			return true
		}
	}
	return false
}

func normalizeQBContentPath(value string) string {
	return strings.ToLower(strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/"))
}

func assertTagAbsent(t *testing.T, client *qbittorrent.Client, tag string) {
	t.Helper()
	tags, err := client.GetTags(t.Context())
	if err != nil {
		t.Fatalf("get tags after cleanup: %v", err)
	}
	if stringInSlice(tags, tag) {
		t.Fatalf("test tag %q still exists after cleanup", tag)
	}
}

func assertCategoryAbsent(t *testing.T, client *qbittorrent.Client, category string) {
	t.Helper()
	categories, err := client.GetCategories(t.Context())
	if err != nil {
		t.Fatalf("get categories after cleanup: %v", err)
	}
	if categoryExists(categories, category) {
		t.Fatalf("test category %q still exists after cleanup", category)
	}
}

func torrentHashSet(torrents []qbittorrent.TorrentInfo) map[string]struct{} {
	result := make(map[string]struct{}, len(torrents))
	for _, torrent := range torrents {
		if torrent.Hash != "" {
			result[torrent.Hash] = struct{}{}
		}
	}
	return result
}

func torrentHasTag(torrent qbittorrent.TorrentInfo, tag string) bool {
	for _, value := range strings.Split(torrent.Tags, ",") {
		if strings.EqualFold(strings.TrimSpace(value), tag) {
			return true
		}
	}
	return false
}

func categoryExists(categories map[string]qbittorrent.Category, name string) bool {
	if _, ok := categories[name]; ok {
		return true
	}
	for _, category := range categories {
		if category.Name == name {
			return true
		}
	}
	return false
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
