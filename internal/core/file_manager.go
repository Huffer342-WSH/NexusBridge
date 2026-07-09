package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"nexusbridge/internal/qbittorrent"
)

const (
	defaultRecoveryScanDepth = 1
	maxRecoveryScanDepth     = 5
	defaultRecoveryScanLimit = 200
	maxRecoveryScanLimit     = 500
)

// BrowseFiles 返回本机目录内容，并从 qB 任务的唯一内容根开始标记归属。
func (a *App) BrowseFiles(ctx context.Context, request FileBrowseRequest) (FileBrowseResult, error) {
	qbTasks, qbConnected, qbError := a.fileManagerQBTorrents(ctx)
	if strings.TrimSpace(request.Path) == "" {
		entries := filesystemRootEntries()
		attachQBTasks(entries, qbTasks)
		return FileBrowseResult{
			IsRoot: true, QBConnected: qbConnected, QBError: qbError, Entries: entries,
		}, nil
	}

	target, err := filepath.Abs(strings.TrimSpace(request.Path))
	if err != nil {
		return FileBrowseResult{}, err
	}
	target = filepath.Clean(target)
	info, err := os.Stat(target)
	if err != nil {
		return FileBrowseResult{}, fmt.Errorf("inspect browse path: %w", err)
	}
	if !info.IsDir() {
		return FileBrowseResult{}, fmt.Errorf("browse path must be a directory")
	}
	diskEntries, err := os.ReadDir(target)
	if err != nil {
		return FileBrowseResult{}, fmt.Errorf("read browse path: %w", err)
	}
	entries := make([]FileEntry, 0, len(diskEntries))
	for _, entry := range diskEntries {
		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}
		entries = append(entries, FileEntry{
			Name: entry.Name(), Path: filepath.Join(target, entry.Name()), IsDir: entry.IsDir(),
			Size: entryInfo.Size(), ModifiedAt: entryInfo.ModTime(), QBTasks: []FileQBTask{},
		})
	}
	sortFileEntries(entries)
	attachQBTasks(entries, qbTasks)
	parent := filepath.Dir(target)
	if sameFilesystemPath(parent, target) {
		parent = ""
	}
	return FileBrowseResult{
		Path: target, Parent: parent, QBConnected: qbConnected, QBError: qbError, Entries: entries,
	}, nil
}

// ScanRecoveryCandidates 递归预览不属于现有 qB 任务的文件和目录，不执行恢复。
func (a *App) ScanRecoveryCandidates(ctx context.Context, request RecoveryScanRequest) (RecoveryScanResult, error) {
	root, err := filepath.Abs(strings.TrimSpace(request.Path))
	if err != nil {
		return RecoveryScanResult{}, err
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return RecoveryScanResult{}, fmt.Errorf("inspect recovery scan path: %w", err)
	}
	if !info.IsDir() {
		return RecoveryScanResult{}, fmt.Errorf("recovery scan path must be a directory")
	}
	maxDepth := request.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultRecoveryScanDepth
	}
	if maxDepth > maxRecoveryScanDepth {
		return RecoveryScanResult{}, fmt.Errorf("recovery scan max_depth must not exceed %d", maxRecoveryScanDepth)
	}
	limit := request.Limit
	if limit <= 0 {
		limit = defaultRecoveryScanLimit
	}
	if limit > maxRecoveryScanLimit {
		return RecoveryScanResult{}, fmt.Errorf("recovery scan limit must not exceed %d", maxRecoveryScanLimit)
	}
	searchMode := strings.TrimSpace(request.SearchMode)
	if searchMode == "" {
		searchMode = RecoverySearchDatabase
	}
	if _, err := normalizeRecoverySearchMode(searchMode); err != nil {
		return RecoveryScanResult{}, err
	}
	if searchMode != RecoverySearchSite {
		status, err := a.store.GetTorrentSizeIndexStatus(ctx)
		if err != nil {
			return RecoveryScanResult{}, err
		}
		if status.Pending > 0 {
			return RecoveryScanResult{}, fmt.Errorf("torrent size index is incomplete: %d of %d torrent files indexed; rebuild it first", status.Indexed, status.Total)
		}
	}

	qbTorrents, _, _ := a.fileManagerQBTorrents(ctx)
	qbRoots := make([]string, 0, len(qbTorrents))
	for _, torrent := range qbTorrents {
		if path := qbTorrentManagedRoot(torrent); path != "" {
			qbRoots = append(qbRoots, path)
		}
	}
	result := RecoveryScanResult{Path: root, Items: []RecoveryScanItem{}}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if sameFilesystemPath(path, root) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(relative), "/") + 1
		if depth > maxDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if result.Evaluated >= limit {
			result.Truncated = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if filesystemPathManagedByRoots(path, qbRoots) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		result.Evaluated++
		preview, previewErr := a.PreviewRecovery(ctx, RecoveryPreviewRequest{
			Path: path, SiteIDs: request.SiteIDs, SearchMode: searchMode,
		})
		item := RecoveryScanItem{Path: path, Name: entry.Name(), IsDir: entry.IsDir(), Preview: preview}
		switch {
		case previewErr != nil:
			item.Status, item.Reason = "error", previewErr.Error()
		case len(preview.Matches) == 1:
			item.Status = "matched"
			result.Matched++
		case len(preview.Matches) > 1:
			item.Status, item.Reason = "ambiguous", "multiple torrents match this path"
		default:
			item.Status, item.Reason = "none", "no torrent has the same complete file-size set"
		}
		result.Items = append(result.Items, item)
		if entry.IsDir() && item.Status == "matched" {
			return filepath.SkipDir
		}
		if entry.IsDir() && depth >= maxDepth {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return RecoveryScanResult{}, err
	}
	return result, nil
}

func (a *App) fileManagerQBTorrents(ctx context.Context) ([]qbittorrent.TorrentInfo, bool, string) {
	qb, err := a.qbClient(ctx)
	if err != nil {
		return nil, false, err.Error()
	}
	torrents, err := qb.ListTorrents(ctx)
	if err != nil {
		return nil, false, err.Error()
	}
	return torrents, true, ""
}

func attachQBTasks(entries []FileEntry, torrents []qbittorrent.TorrentInfo) {
	type managedTask struct {
		root string
		task FileQBTask
	}
	managed := make([]managedTask, 0, len(torrents))
	for _, torrent := range torrents {
		root := qbTorrentManagedRoot(torrent)
		if root == "" {
			continue
		}
		managed = append(managed, managedTask{
			root: root,
			task: FileQBTask{
				Hash: torrent.Hash, Name: torrent.Name, State: torrent.State, Category: torrent.Category,
				SavePath: torrent.SavePath, ContentPath: torrent.ContentPath,
			},
		})
	}
	for index := range entries {
		entryPath := normalizedFilesystemPath(entries[index].Path)
		for _, item := range managed {
			if filesystemPathWithinRoot(entryPath, item.root) {
				entries[index].QBTasks = append(entries[index].QBTasks, item.task)
			}
		}
	}
}

// qbTorrentManagedRoot 返回 save_path 下属于任务的第一个文件或目录。
func qbTorrentManagedRoot(torrent qbittorrent.TorrentInfo) string {
	contentPath := normalizedFilesystemPath(torrent.ContentPath)
	savePath := normalizedFilesystemPath(torrent.SavePath)
	if contentPath == "" && savePath != "" && strings.TrimSpace(torrent.Name) != "" {
		contentPath = normalizedFilesystemPath(filepath.Join(savePath, filepath.FromSlash(torrent.Name)))
	}
	if contentPath == "" || savePath == "" || contentPath == savePath {
		return contentPath
	}
	relative, err := filepath.Rel(savePath, contentPath)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return contentPath
	}
	firstPart := strings.Split(filepath.Clean(relative), string(filepath.Separator))[0]
	if firstPart == "" || firstPart == "." || firstPart == ".." {
		return contentPath
	}
	return normalizedFilesystemPath(filepath.Join(savePath, firstPart))
}

func filesystemPathManagedByRoots(path string, roots []string) bool {
	path = normalizedFilesystemPath(path)
	for _, root := range roots {
		if filesystemPathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func filesystemPathWithinRoot(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func filesystemRootEntries() []FileEntry {
	entries := []FileEntry{}
	if runtime.GOOS == "windows" {
		for letter := 'A'; letter <= 'Z'; letter++ {
			path := string(letter) + `:\`
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				entries = append(entries, FileEntry{Name: path, Path: path, IsDir: true, QBTasks: []FileQBTask{}})
			}
		}
		return entries
	}
	return []FileEntry{{Name: "/", Path: "/", IsDir: true, QBTasks: []FileQBTask{}}}
}

func sortFileEntries(entries []FileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

func normalizedFilesystemPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	abs, err := filepath.Abs(value)
	if err == nil {
		value = abs
	}
	value = filepath.Clean(value)
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}

func sameFilesystemPath(left, right string) bool {
	return normalizedFilesystemPath(left) == normalizedFilesystemPath(right)
}
