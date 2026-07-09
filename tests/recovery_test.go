package tests

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexusbridge/internal/core"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/runtimeconfig"
)

// TestRecoverRealAPI 验证文件管理器、扫描、四种恢复来源及自动批量入口。
func TestRecoverRealAPI(t *testing.T) {
	if strings.TrimSpace(os.Getenv("NEXUSBRIDGE_TEST_RECOVERY_REAL")) != "1" {
		t.Skip("set NEXUSBRIDGE_TEST_RECOVERY_REAL=1 to enable the destructive recovery experiment")
	}
	targetHash := strings.ToLower(requiredEnv(t, "NEXUSBRIDGE_TEST_RECOVERY_HASH"))
	decodedHash, err := hex.DecodeString(targetHash)
	if err != nil || len(decodedHash) != 20 {
		t.Fatalf("NEXUSBRIDGE_TEST_RECOVERY_HASH must be a 40-character v1 info hash")
	}

	// 使用开发数据库中已保存的 qB 和站点配置，确保试验走真实服务链路。
	prepared, err := runtimeconfig.Load(runtimeconfig.Options{ExplicitConfig: filepath.Join("..", "data", "config.json")})
	if err != nil {
		t.Fatalf("load development config: %v", err)
	}
	app, err := core.NewApp(t.Context(), prepared.Config)
	if err != nil {
		t.Fatalf("new recovery app: %v", err)
	}
	defer app.Close()
	qbCfg := app.GetQBittorrentConfig(t.Context())
	client, err := qbittorrent.New(qbittorrent.Config{
		AuthMode: qbCfg.AuthMode, URL: qbCfg.URL, APIKey: qbCfg.APIKey,
		Username: qbCfg.Username, UserID: qbCfg.UserID, Password: qbCfg.Password,
	})
	if err != nil {
		t.Fatalf("new real qB client: %v", err)
	}
	if err := client.Test(t.Context()); err != nil {
		t.Fatalf("test real qB connection: %v", err)
	}
	// 旧数据库只在用户明确操作时重建一次；后续新保存和删除的 torrent 会自动维护索引。
	indexStatus, err := app.RebuildTorrentSizeIndex(t.Context())
	if err != nil {
		t.Fatalf("rebuild torrent size index: %v", err)
	}
	if indexStatus.Pending != 0 {
		t.Fatalf("torrent size index is still incomplete after rebuild: %#v", indexStatus)
	}

	// 删除前先从 qB 读取 content_path；目录和单文件现在都可以直接作为恢复输入。
	existing, found, err := client.FindTorrentByHash(t.Context(), targetHash)
	if err != nil {
		t.Fatalf("find target qB torrent: %v", err)
	}
	if !found {
		t.Fatalf("target qB torrent %s does not exist before experiment", targetHash)
	}
	targetPath := filepath.Clean(existing.ContentPath)
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("target content path must be readable: %s: %v", targetPath, err)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		t.Fatalf("target content path must be a regular file or directory: %s", targetPath)
	}

	// 文件管理器必须从 save_path 下的唯一内容根开始标记，而不是只标记最深层 content_path。
	managedRoot := recoveryTestManagedRoot(existing)
	browsePath := filepath.Dir(managedRoot)
	browsed, err := app.BrowseFiles(t.Context(), core.FileBrowseRequest{Path: browsePath})
	if err != nil {
		t.Fatalf("browse target parent: %v", err)
	}
	if !browsed.QBConnected {
		t.Fatalf("file manager did not connect to qB: %s", browsed.QBError)
	}
	foundTargetEntry := false
	for _, entry := range browsed.Entries {
		if !strings.EqualFold(filepath.Clean(entry.Path), managedRoot) {
			continue
		}
		for _, task := range entry.QBTasks {
			if strings.EqualFold(task.Hash, targetHash) {
				foundTargetEntry = true
				break
			}
		}
	}
	if !foundTargetEntry {
		t.Fatalf("file manager did not associate managed root %s with qB hash %s (save_path=%s content_path=%s)", managedRoot, targetHash, existing.SavePath, existing.ContentPath)
	}
	expectedCategory := ""
	if existing.Category != "" {
		catalog, err := app.GetQBCategories(t.Context(), true)
		if err != nil {
			t.Fatalf("load qB categories for recovery: %v", err)
		}
		for _, category := range catalog.Items {
			if category.Name == existing.Category && category.SavePath != "" && sameRecoveryTestPath(category.SavePath, existing.SavePath) {
				expectedCategory = category.Name
				break
			}
		}
	}

	tests := []struct {
		name           string
		searchMode     string
		expectedSource string
		expectsWeb     bool
	}{
		{name: "site", searchMode: core.RecoverySearchSite, expectedSource: "site_search", expectsWeb: true},
		{name: "database", searchMode: core.RecoverySearchDatabase, expectedSource: "database"},
		{name: "database_then_site", searchMode: core.RecoverySearchDatabaseThenSite, expectedSource: "database"},
		{name: "torrent_url", searchMode: core.RecoverySearchURL, expectedSource: "url"},
	}
	var recoveryFallback core.RecoveryMatch
	var directTorrentURL string
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 删除前先按当前模式预览；未指定站点时，site 模式会遍历全部已配置站点。
			previewRequest := core.RecoveryPreviewRequest{Path: targetPath, SearchMode: test.searchMode}
			if test.searchMode == core.RecoverySearchURL {
				if directTorrentURL == "" {
					t.Fatal("site recovery did not provide a torrent download url for the url subtest")
				}
				previewRequest.TorrentURL = directTorrentURL
			}
			preview, err := app.PreviewRecovery(t.Context(), previewRequest)
			if err != nil {
				t.Fatalf("preview %s recovery: %v", test.searchMode, err)
			}
			var selected core.RecoveryMatch
			for _, match := range preview.Matches {
				if strings.EqualFold(match.InfoHashV1, targetHash) {
					selected = match
					break
				}
			}
			if selected.Torrent.ID == "" {
				t.Fatalf("no exact %s match for target hash %s: %#v", test.searchMode, targetHash, preview)
			}
			if test.searchMode == core.RecoverySearchSite {
				recoveryFallback = selected
				directTorrentURL = selected.Torrent.DownloadURL
			}
			if preview.Source != test.expectedSource || preview.SearchMode != test.searchMode {
				t.Fatalf("unexpected recovery source/mode: %#v", preview)
			}
			if expectedCategory != "" && preview.Category != expectedCategory {
				t.Fatalf("expected inferred category %q for save path %s, got %#v", expectedCategory, preview.SavePath, preview)
			}
			if (len(preview.SearchAttempts) > 0) != test.expectsWeb {
				t.Fatalf("unexpected web search attempts for %s: %#v", test.searchMode, preview.SearchAttempts)
			}

			// 破坏性步骤开始后保留已知候选的数据库恢复兜底，避免断言失败时遗留缺失任务。
			recovered := false
			defer func() {
				if recovered {
					return
				}
				fallback := selected
				if test.searchMode == core.RecoverySearchURL {
					fallback = recoveryFallback
				}
				if _, err := app.RecoverFolder(t.Context(), core.RecoveryRequest{
					Path: targetPath, SiteID: fallback.Torrent.SiteID, TorrentID: fallback.Torrent.ID,
					SearchMode: core.RecoverySearchDatabase,
				}); err != nil {
					t.Logf("best-effort recovery after failed experiment also failed: %v", err)
				}
			}()

			// 只删除 qB 任务，deleteFiles=false 是本真实试验的安全边界。
			if err := client.DeleteTorrents(t.Context(), []string{targetHash}, false); err != nil {
				t.Fatalf("delete target qB task while preserving files: %v", err)
			}
			if err := waitTorrentGone(t, client, targetHash); err != nil {
				t.Fatalf("wait target qB task deletion: %v", err)
			}
			if test.searchMode == core.RecoverySearchSite {
				// 高级扫描只读地跳过现有 qB 任务；删除后应在父目录中重新发现该精确候选。
				scan, err := app.ScanRecoveryCandidates(t.Context(), core.RecoveryScanRequest{
					Path: filepath.Dir(targetPath), SearchMode: core.RecoverySearchDatabase, MaxDepth: 1, Limit: 500,
				})
				if err != nil {
					t.Fatalf("scan deleted qB task candidate: %v", err)
				}
				foundCandidate := false
				for _, item := range scan.Items {
					if sameRecoveryTestPath(item.Path, targetPath) && item.Status == "matched" {
						foundCandidate = true
						break
					}
				}
				if !foundCandidate {
					t.Fatalf("recovery scan did not find deleted target %s: %#v", targetPath, scan)
				}
			}

			// 执行入口只提供目录和模式，不依赖调用方预先知道 torrent_id 或 site_id。
			recoverRequest := core.RecoveryRequest{Path: targetPath, SearchMode: test.searchMode}
			if test.searchMode == core.RecoverySearchURL {
				recoverRequest.TorrentURL = directTorrentURL
			}
			result, err := app.RecoverFolder(t.Context(), recoverRequest)
			if err != nil {
				t.Fatalf("recover deleted qB task with %s: %v", test.searchMode, err)
			}
			if !strings.EqualFold(result.QBStatus.Hash, targetHash) {
				t.Fatalf("expected recovered hash %s, got %#v", targetHash, result)
			}
			if result.VerificationStatus != "verified" || !result.Started || result.RecheckAttempts < 1 {
				t.Fatalf("expected verified and started recovery after forced recheck, got %#v", result)
			}
			recoveredTorrent, found, err := client.FindTorrentByHash(t.Context(), targetHash)
			if err != nil || !found {
				t.Fatalf("recovered qB task is not visible: found=%v err=%v", found, err)
			}
			if expectedCategory != "" {
				if result.Category != expectedCategory {
					t.Fatalf("expected recovery result category %q, got %#v", expectedCategory, result)
				}
				if recoveredTorrent.Category != expectedCategory {
					t.Fatalf("expected recovered qB category %q, got %#v", expectedCategory, recoveredTorrent)
				}
			}
			assertRecoveredQBFilePaths(t, client, targetHash, targetPath, result.SavePath)
			recovered = true
			t.Logf("recovered qB torrent %s via %s with state %s", targetHash, test.searchMode, result.QBStatus.State)
		})
	}

	// 自动批量入口仍使用真实 qB 删除/添加闭环；网页搜索开关分别映射到数据库模式和数据库优先模式。
	for _, webSearch := range []bool{false, true} {
		name := "batch_database_only"
		if webSearch {
			name = "batch_with_web_fallback"
		}
		t.Run(name, func(t *testing.T) {
			preview, err := app.PreviewRecovery(t.Context(), core.RecoveryPreviewRequest{
				Path: targetPath, SearchMode: core.RecoverySearchDatabase,
			})
			if err != nil {
				t.Fatalf("preview batch recovery candidate: %v", err)
			}
			var selected core.RecoveryMatch
			for _, match := range preview.Matches {
				if strings.EqualFold(match.InfoHashV1, targetHash) {
					selected = match
					break
				}
			}
			if selected.Torrent.ID == "" {
				t.Fatalf("no exact database candidate for batch recovery: %#v", preview)
			}

			recovered := false
			defer func() {
				if recovered {
					return
				}
				if _, err := app.RecoverFolder(t.Context(), core.RecoveryRequest{
					Path: targetPath, SiteID: selected.Torrent.SiteID, TorrentID: selected.Torrent.ID,
					SearchMode: core.RecoverySearchDatabase,
				}); err != nil {
					t.Logf("best-effort recovery after failed batch experiment also failed: %v", err)
				}
			}()

			// 批量试验同样只移除 qB 任务，绝不删除磁盘文件。
			if err := client.DeleteTorrents(t.Context(), []string{targetHash}, false); err != nil {
				t.Fatalf("delete target qB task before batch recovery: %v", err)
			}
			if err := waitTorrentGone(t, client, targetHash); err != nil {
				t.Fatalf("wait target qB task deletion before batch recovery: %v", err)
			}

			result, err := app.RecoverFolders(t.Context(), core.RecoveryBatchRequest{
				WebSearch: webSearch,
				Items: []core.RecoveryBatchItemRequest{{
					Path: targetPath, SiteID: selected.Torrent.SiteID, TorrentID: selected.Torrent.ID, Category: preview.Category,
				}},
			})
			if err != nil {
				t.Fatalf("batch recover deleted qB task: %v", err)
			}
			if result.Attempted != 1 || result.Recovered != 1 || result.NeedsAttention != 0 || result.Failed != 0 || len(result.Items) != 1 {
				t.Fatalf("unexpected batch recovery result: %#v", result)
			}
			if result.Items[0].Status != "recovered" || result.Items[0].Result == nil || !strings.EqualFold(result.Items[0].Result.QBStatus.Hash, targetHash) {
				t.Fatalf("batch recovery did not restore target hash %s: %#v", targetHash, result.Items[0])
			}
			assertRecoveredQBFilePaths(t, client, targetHash, targetPath, result.Items[0].Result.SavePath)
			recovered = true
		})
	}
}

func assertRecoveredQBFilePaths(t *testing.T, client *qbittorrent.Client, hash, targetPath, savePath string) {
	t.Helper()
	contents, err := client.GetTorrentContents(t.Context(), hash, nil)
	if err != nil {
		t.Fatalf("get URL-recovered qB files: %v", err)
	}
	existing := map[string]int64{}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("inspect URL recovery target: %v", err)
	}
	if info.Mode().IsRegular() {
		existing[filepath.Clean(targetPath)] = info.Size()
	} else {
		err = filepath.WalkDir(targetPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			entryInfo, err := entry.Info()
			if err != nil {
				return err
			}
			existing[filepath.Clean(path)] = entryInfo.Size()
			return nil
		})
		if err != nil {
			t.Fatalf("walk URL recovery target: %v", err)
		}
	}
	qBFiles := make(map[string]int64, len(contents))
	for _, content := range contents {
		path := filepath.Clean(filepath.Join(savePath, filepath.FromSlash(content.Name)))
		qBFiles[path] = content.Size
	}
	for path, size := range existing {
		if qBSize, ok := qBFiles[path]; !ok || qBSize != size {
			t.Fatalf("URL recovery did not map existing file %s (%d bytes); qB files=%#v", path, size, qBFiles)
		}
	}
}

func recoveryTestManagedRoot(torrent qbittorrent.TorrentInfo) string {
	contentPath := filepath.Clean(torrent.ContentPath)
	savePath := filepath.Clean(torrent.SavePath)
	if torrent.ContentPath == "" || torrent.SavePath == "" || sameRecoveryTestPath(contentPath, savePath) {
		return contentPath
	}
	relative, err := filepath.Rel(savePath, contentPath)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return contentPath
	}
	firstPart := strings.Split(filepath.Clean(relative), string(filepath.Separator))[0]
	if firstPart == "" || firstPart == "." || firstPart == ".." {
		return contentPath
	}
	return filepath.Join(savePath, firstPart)
}

func sameRecoveryTestPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
