package core

import (
	"context"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

func (a *App) recoveryMatch(ctx context.Context, target recoveryTarget, file storage.TorrentFileRecord, source string) (RecoveryMatch, []recoveryFileMapping, bool, error) {
	torrent, err := a.getTorrent(ctx, file.SiteID, file.TorrentID)
	if err != nil {
		return RecoveryMatch{}, nil, false, err
	}
	return a.recoveryMatchData(target, file.Data, torrent, source)
}

// recoveryMatchData 按完整文件大小多重集合匹配，并生成不改动磁盘的 qB 文件路径映射。
func (a *App) recoveryMatchData(target recoveryTarget, data []byte, torrent Torrent, source string) (RecoveryMatch, []recoveryFileMapping, bool, error) {
	metadata, err := qbittorrent.ParseTorrentMetadata(data)
	if err != nil {
		return RecoveryMatch{}, nil, false, err
	}
	type torrentFile struct {
		path       string
		normalized string
		size       int64
	}
	torrentFiles := make([]torrentFile, 0, len(metadata.Files))
	var totalSize int64
	for _, item := range metadata.Files {
		if item.Padding {
			continue
		}
		normalized, ok := normalizeRecoveryRelativePath(item.Path)
		if !ok {
			return RecoveryMatch{}, nil, false, nil
		}
		torrentFiles = append(torrentFiles, torrentFile{path: filepath.ToSlash(item.Path), normalized: normalized, size: item.Size})
		totalSize += item.Size
	}
	if len(target.DiskFiles) == 0 || len(target.DiskFiles) != len(torrentFiles) {
		return RecoveryMatch{}, nil, false, nil
	}
	if target.IsFile && len(torrentFiles) != 1 {
		return RecoveryMatch{}, nil, false, nil
	}

	diskPaths := make([]string, 0, len(target.DiskFiles))
	for path := range target.DiskFiles {
		diskPaths = append(diskPaths, path)
	}
	sort.Strings(diskPaths)
	used := make(map[int]struct{}, len(diskPaths))
	selectedByDisk := make(map[string]int, len(diskPaths))
	assignUnique := func(matches func(string, torrentFile) bool) bool {
		changed := false
		for _, diskPath := range diskPaths {
			if _, assigned := selectedByDisk[diskPath]; assigned {
				continue
			}
			candidates := make([]int, 0)
			for index, item := range torrentFiles {
				if _, exists := used[index]; !exists && item.size == target.DiskFiles[diskPath] && matches(diskPath, item) {
					candidates = append(candidates, index)
				}
			}
			if len(candidates) == 1 {
				selectedByDisk[diskPath] = candidates[0]
				used[candidates[0]] = struct{}{}
				changed = true
			}
		}
		return changed
	}
	assignUnique(func(diskPath string, item torrentFile) bool { return item.normalized == diskPath })
	assignUnique(func(diskPath string, item torrentFile) bool {
		return recoveryNameEqual(pathpkg.Base(item.normalized), pathpkg.Base(diskPath))
	})
	for assignUnique(func(_ string, _ torrentFile) bool { return true }) {
	}
	// 没有路径或文件名线索时，同尺寸文件仍可建立稳定的一一映射；最终由 qB 强制校验确认内容。
	for _, diskPath := range diskPaths {
		if _, assigned := selectedByDisk[diskPath]; assigned {
			continue
		}
		for index, item := range torrentFiles {
			if _, exists := used[index]; !exists && item.size == target.DiskFiles[diskPath] {
				selectedByDisk[diskPath] = index
				used[index] = struct{}{}
				break
			}
		}
	}
	if len(selectedByDisk) != len(diskPaths) {
		return RecoveryMatch{}, nil, false, nil
	}

	mappings := make([]recoveryFileMapping, 0, len(torrentFiles))
	for _, diskPath := range diskPaths {
		selected := selectedByDisk[diskPath]
		size := target.DiskFiles[diskPath]
		originalDiskPath := firstNonEmpty(target.DiskFilePaths[diskPath], diskPath)
		newPath := originalDiskPath
		if !target.IsFile {
			newPath = filepath.ToSlash(filepath.Join(filepath.Base(target.Path), filepath.FromSlash(originalDiskPath)))
		}
		if normalized, ok := normalizeRecoveryRelativePath(newPath); !ok || normalized == "" {
			return RecoveryMatch{}, nil, false, nil
		}
		mappings = append(mappings, recoveryFileMapping{
			TorrentPath: torrentFiles[selected].path, NewPath: filepath.ToSlash(newPath), Size: size,
		})
	}
	return RecoveryMatch{
		Torrent: torrent, Source: source, OriginalName: metadata.Name,
		SavePath: filepath.Dir(target.Path), RootFolder: false,
		InfoHashV1: metadata.Hashes.V1, InfoHashV2: metadata.Hashes.V2,
		FileCount: len(torrentFiles), TotalSize: totalSize, MatchMethod: "size", MappingComplete: true,
	}, mappings, true, nil
}

func recoveryTargetSizeCounts(target recoveryTarget) map[int64]int {
	result := make(map[int64]int, len(target.DiskFiles))
	for _, size := range target.DiskFiles {
		result[size]++
	}
	return result
}

func recoveryTargetTotalSize(target recoveryTarget) int64 {
	var result int64
	for _, size := range target.DiskFiles {
		result += size
	}
	return result
}

func uniqueRecoveryCandidate(candidates []int, matches func(int) bool) int {
	selected := -1
	for _, candidate := range candidates {
		if !matches(candidate) {
			continue
		}
		if selected >= 0 {
			return -1
		}
		selected = candidate
	}
	return selected
}

// applyRecoveryFileMappings 使用 qB 返回的完整相对路径直接重命名文件。
func (a *App) applyRecoveryFileMappings(ctx context.Context, qb *qbittorrent.Client, hash string, mappings []recoveryFileMapping) error {
	contents, err := waitRecoveryTorrentContents(ctx, qb, hash)
	if err != nil {
		return err
	}
	used := make(map[int]struct{}, len(mappings))
	type rename struct {
		contentIndex int
		oldPath      string
		newPath      string
	}
	renames := make([]rename, 0, len(mappings))
	newPaths := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		selected := selectRecoveryQBContent(mapping, contents, used)
		if selected < 0 {
			return fmt.Errorf("cannot identify qB file for torrent path %s", mapping.TorrentPath)
		}
		used[selected] = struct{}{}
		oldPath := filepath.ToSlash(contents[selected].Name)
		newNormalized, newOK := normalizeRecoveryRelativePath(mapping.NewPath)
		if !newOK {
			return fmt.Errorf("invalid mapped qB file path %s", mapping.NewPath)
		}
		if _, exists := newPaths[newNormalized]; exists {
			return fmt.Errorf("multiple existing files map to qB path %s", mapping.NewPath)
		}
		newPaths[newNormalized] = struct{}{}
		if oldNormalized, oldOK := normalizeRecoveryRelativePath(oldPath); oldOK {
			if oldNormalized == newNormalized {
				continue
			}
		}
		renames = append(renames, rename{contentIndex: selected, oldPath: oldPath, newPath: mapping.NewPath})
	}
	for _, item := range renames {
		newPath, _ := normalizeRecoveryRelativePath(item.newPath)
		for index, content := range contents {
			if index == item.contentIndex {
				continue
			}
			contentPath, ok := normalizeRecoveryRelativePath(content.Name)
			if ok && contentPath == newPath {
				return fmt.Errorf("mapped qB path conflicts with torrent file %s", content.Name)
			}
		}
	}
	for _, item := range renames {
		if err := qb.RenameTorrentFile(ctx, hash, item.oldPath, item.newPath); err != nil {
			return fmt.Errorf("rename %s to %s: %w", item.oldPath, item.newPath, err)
		}
		if err := waitRecoveryTorrentFilePath(ctx, qb, hash, item.newPath); err != nil {
			return fmt.Errorf("confirm renamed qB file %s: %w", item.newPath, err)
		}
	}
	return nil
}

// waitRecoveryTorrentFilePath 等待 qB 文件列表反映重命名结果。
func waitRecoveryTorrentFilePath(ctx context.Context, qb *qbittorrent.Client, hash, expectedPath string) error {
	expected, ok := normalizeRecoveryRelativePath(expectedPath)
	if !ok {
		return fmt.Errorf("invalid qB file path %s", expectedPath)
	}
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		contents, err := qb.GetTorrentContents(ctx, hash, nil)
		if err == nil {
			for _, content := range contents {
				if actual, valid := normalizeRecoveryRelativePath(content.Name); valid && actual == expected {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("timed out waiting for qB file path %s", expectedPath)
}

func waitRecoveryTorrentContents(ctx context.Context, qb *qbittorrent.Client, hash string) ([]qbittorrent.TorrentContent, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		contents, err := qb.GetTorrentContents(ctx, hash, nil)
		if err == nil && len(contents) > 0 {
			return contents, nil
		}
		lastErr = err
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("wait for qB torrent files: %w", lastErr)
	}
	return nil, fmt.Errorf("wait for qB torrent files timed out")
}

func selectRecoveryQBContent(mapping recoveryFileMapping, contents []qbittorrent.TorrentContent, used map[int]struct{}) int {
	torrentPath, _ := normalizeRecoveryRelativePath(mapping.TorrentPath)
	candidates := make([]int, 0)
	for index, content := range contents {
		if _, exists := used[index]; exists || content.Size != mapping.Size {
			continue
		}
		candidates = append(candidates, index)
	}
	selected := uniqueRecoveryCandidate(candidates, func(index int) bool {
		contentPath, ok := normalizeRecoveryRelativePath(contents[index].Name)
		return ok && (contentPath == torrentPath || strings.HasSuffix(contentPath, "/"+torrentPath))
	})
	if selected < 0 && len(candidates) == 1 {
		selected = candidates[0]
	}
	return selected
}
