package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func normalizeRecoverySearchMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return RecoverySearchDatabaseThenSite, nil
	}
	switch value {
	case RecoverySearchDatabase, RecoverySearchSite, RecoverySearchDatabaseThenSite:
		return value, nil
	default:
		return "", fmt.Errorf("invalid recovery search_mode %q", value)
	}
}

// recoveryCategory 使用本地分类快照为预览快速推断分类。
func (a *App) recoveryCategory(ctx context.Context, requested, savePath string) string {
	return a.recoveryCategoryFromCatalog(ctx, requested, savePath, false)
}

// recoveryCategoryForExecution 刷新 qB 分类后按最终保存目录推断恢复分类。
func (a *App) recoveryCategoryForExecution(ctx context.Context, requested, savePath string) string {
	return a.recoveryCategoryFromCatalog(ctx, requested, savePath, true)
}

// recoveryCategoryFromCatalog 按分类的有效保存目录精确匹配最深层分类。
func (a *App) recoveryCategoryFromCatalog(ctx context.Context, requested, savePath string, refresh bool) string {
	if category := strings.TrimSpace(requested); category != "" {
		return category
	}
	if strings.TrimSpace(savePath) == "" {
		return ""
	}
	categories, err := a.GetQBCategories(ctx, refresh)
	if err != nil {
		return ""
	}
	defaultSavePath := ""
	needsDefaultSavePath := false
	for _, category := range categories.Items {
		if strings.TrimSpace(category.SavePath) == "" && !qbCategoryHasExplicitParentPath(category, categories.Items) {
			needsDefaultSavePath = true
			break
		}
	}
	if needsDefaultSavePath {
		if qb, qbErr := a.qbClient(ctx); qbErr == nil {
			defaultSavePath, _ = qb.GetDefaultSavePath(ctx)
		}
	}
	matchedName := ""
	matchedDepth := -1
	for _, category := range categories.Items {
		effectivePath := qbCategoryEffectiveSavePath(category, categories.Items, defaultSavePath)
		if effectivePath == "" || !sameFilesystemPath(effectivePath, savePath) {
			continue
		}
		depth := len(qbCategorySegments(category.Name))
		if depth > matchedDepth {
			matchedName = category.Name
			matchedDepth = depth
		}
	}
	return matchedName
}

// qbCategoryEffectiveSavePath 解析分类自身、最近父分类或全局默认目录继承后的有效保存路径。
func qbCategoryEffectiveSavePath(category QBCategory, categories []QBCategory, defaultSavePath string) string {
	if savePath := strings.TrimSpace(category.SavePath); savePath != "" {
		return filepath.Clean(savePath)
	}
	segments := qbCategorySegments(category.Name)
	if len(segments) == 0 {
		return ""
	}
	byName := make(map[string]QBCategory, len(categories))
	for _, candidate := range categories {
		name := strings.ToLower(strings.Join(qbCategorySegments(candidate.Name), "/"))
		if name != "" {
			byName[name] = candidate
		}
	}
	for depth := len(segments) - 1; depth > 0; depth-- {
		parent, exists := byName[strings.ToLower(strings.Join(segments[:depth], "/"))]
		if !exists || strings.TrimSpace(parent.SavePath) == "" {
			continue
		}
		parts := append([]string{strings.TrimSpace(parent.SavePath)}, segments[depth:]...)
		return filepath.Clean(filepath.Join(parts...))
	}
	if strings.TrimSpace(defaultSavePath) == "" {
		return ""
	}
	parts := append([]string{strings.TrimSpace(defaultSavePath)}, segments...)
	return filepath.Clean(filepath.Join(parts...))
}

// qbCategoryHasExplicitParentPath 判断空路径分类能否从任一父分类继承保存目录。
func qbCategoryHasExplicitParentPath(category QBCategory, categories []QBCategory) bool {
	segments := qbCategorySegments(category.Name)
	if len(segments) < 2 {
		return false
	}
	parents := make(map[string]struct{}, len(categories))
	for _, candidate := range categories {
		if strings.TrimSpace(candidate.SavePath) == "" {
			continue
		}
		parents[strings.ToLower(strings.Join(qbCategorySegments(candidate.Name), "/"))] = struct{}{}
	}
	for depth := len(segments) - 1; depth > 0; depth-- {
		if _, exists := parents[strings.ToLower(strings.Join(segments[:depth], "/"))]; exists {
			return true
		}
	}
	return false
}

// qbCategorySegments 返回去除空白层级后的 qB 分类路径片段。
func qbCategorySegments(name string) []string {
	segments := make([]string, 0, strings.Count(name, "/")+1)
	for _, segment := range strings.Split(name, "/") {
		if segment = strings.TrimSpace(segment); segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func trimmedRecoverySiteIDs(siteIDs []string) []string {
	result := make([]string, 0, len(siteIDs))
	seen := map[string]struct{}{}
	for _, siteID := range siteIDs {
		siteID = strings.TrimSpace(siteID)
		if siteID == "" {
			continue
		}
		if _, exists := seen[siteID]; exists {
			continue
		}
		seen[siteID] = struct{}{}
		result = append(result, siteID)
	}
	return result
}
