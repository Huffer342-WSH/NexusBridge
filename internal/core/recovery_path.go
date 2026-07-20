package core

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
)

func scanRecoveryPath(value string) (recoveryTarget, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return recoveryTarget{}, fmt.Errorf("recovery path is required")
	}
	target, err := filepath.Abs(value)
	if err != nil {
		return recoveryTarget{}, err
	}
	target = filepath.Clean(target)
	info, err := os.Stat(target)
	if err != nil {
		return recoveryTarget{}, fmt.Errorf("inspect recovery path: %w", err)
	}
	if info.Mode().IsRegular() {
		relative, ok := normalizeRecoveryRelativePath(filepath.Base(target))
		if !ok {
			return recoveryTarget{}, fmt.Errorf("invalid recovery file name: %s", filepath.Base(target))
		}
		return recoveryTarget{
			Path: target, SearchName: filepath.Base(target), DiskFiles: map[string]int64{relative: info.Size()},
			DiskFilePaths: map[string]string{relative: filepath.Base(target)}, IsFile: true,
		}, nil
	}
	if !info.IsDir() {
		return recoveryTarget{}, fmt.Errorf("recovery path must be a regular file or directory")
	}
	files := map[string]int64{}
	filePaths := map[string]string{}
	err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("recovery path contains non-regular file: %s", path)
		}
		relative, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		normalized, ok := normalizeRecoveryRelativePath(filepath.ToSlash(relative))
		if !ok {
			return fmt.Errorf("invalid recovery relative path: %s", relative)
		}
		files[normalized] = info.Size()
		filePaths[normalized] = filepath.ToSlash(relative)
		return nil
	})
	if err != nil {
		return recoveryTarget{}, err
	}
	return recoveryTarget{Path: target, SearchName: filepath.Base(target), DiskFiles: files, DiskFilePaths: filePaths}, nil
}

func normalizeRecoveryRelativePath(value string) (string, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") {
		return "", false
	}
	cleaned := pathpkg.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, true
}

func recoveryNameEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
