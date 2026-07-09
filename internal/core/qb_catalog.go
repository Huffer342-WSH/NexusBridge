package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"nexusbridge/internal/storage"
)

// GetQBCategories 返回 qB 分类；刷新失败时回退到最近持久化快照。
func (a *App) GetQBCategories(ctx context.Context, refresh bool) (QBCategoriesResult, error) {
	cached, err := a.store.ListQBCategories(ctx)
	if err != nil {
		return QBCategoriesResult{}, err
	}
	state, hasSnapshot, err := a.store.GetQBCacheState(ctx, storage.QBCacheKindCategories)
	if err != nil {
		return QBCategoriesResult{}, err
	}
	if !refresh && hasSnapshot {
		return categoriesFromRecords(cached, state, state.LastError != ""), nil
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		_ = a.store.MarkQBCategoriesSyncError(ctx, err.Error())
		if hasSnapshot {
			state.LastError = err.Error()
			return categoriesFromRecords(cached, state, true), nil
		}
		return QBCategoriesResult{}, err
	}
	categories, err := qb.GetCategories(ctx)
	if err != nil {
		_ = a.store.MarkQBCategoriesSyncError(ctx, err.Error())
		if hasSnapshot {
			state.LastError = err.Error()
			return categoriesFromRecords(cached, state, true), nil
		}
		return QBCategoriesResult{}, err
	}
	syncedAt := time.Now()
	records := make([]storage.QBCategoryRecord, 0, len(categories))
	for name, category := range categories {
		resolvedName := strings.TrimSpace(category.Name)
		if resolvedName == "" {
			resolvedName = strings.TrimSpace(name)
		}
		if resolvedName == "" {
			continue
		}
		records = append(records, storage.QBCategoryRecord{Name: resolvedName, SavePath: category.SavePath, SyncedAt: syncedAt})
	}
	if err := a.store.ReplaceQBCategories(ctx, records, syncedAt); err != nil {
		return QBCategoriesResult{}, err
	}
	return categoriesFromRecords(records, storage.QBCacheStateRecord{SyncedAt: syncedAt}, false), nil
}

// CreateQBCategory 在 qB 中创建完整分类名称并刷新本地快照。
func (a *App) CreateQBCategory(ctx context.Context, category QBCategory) (QBCategoriesResult, error) {
	category.Name = strings.TrimSpace(category.Name)
	category.SavePath = strings.TrimSpace(category.SavePath)
	if category.Name == "" {
		return QBCategoriesResult{}, fmt.Errorf("qb category name is required")
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return QBCategoriesResult{}, err
	}
	if err := qb.CreateCategory(ctx, category.Name, category.SavePath); err != nil {
		return QBCategoriesResult{}, err
	}
	return a.GetQBCategories(ctx, true)
}

// GetQBTags 返回 qB 标签；刷新失败时回退到最近持久化快照。
func (a *App) GetQBTags(ctx context.Context, refresh bool) (QBTagsResult, error) {
	cached, err := a.store.ListQBTags(ctx)
	if err != nil {
		return QBTagsResult{}, err
	}
	state, hasSnapshot, err := a.store.GetQBCacheState(ctx, storage.QBCacheKindTags)
	if err != nil {
		return QBTagsResult{}, err
	}
	if !refresh && hasSnapshot {
		return tagsFromRecords(cached, state, state.LastError != ""), nil
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		_ = a.store.MarkQBTagsSyncError(ctx, err.Error())
		if hasSnapshot {
			state.LastError = err.Error()
			return tagsFromRecords(cached, state, true), nil
		}
		return QBTagsResult{}, err
	}
	tags, err := qb.GetTags(ctx)
	if err != nil {
		_ = a.store.MarkQBTagsSyncError(ctx, err.Error())
		if hasSnapshot {
			state.LastError = err.Error()
			return tagsFromRecords(cached, state, true), nil
		}
		return QBTagsResult{}, err
	}
	tags = normalizeStrings(tags)
	syncedAt := time.Now()
	records := make([]storage.QBTagRecord, 0, len(tags))
	for _, tag := range tags {
		records = append(records, storage.QBTagRecord{Name: tag, SyncedAt: syncedAt})
	}
	if err := a.store.ReplaceQBTags(ctx, records, syncedAt); err != nil {
		return QBTagsResult{}, err
	}
	return tagsFromRecords(records, storage.QBCacheStateRecord{SyncedAt: syncedAt}, false), nil
}

// CreateQBTags 在 qB 中创建标签并刷新本地快照。
func (a *App) CreateQBTags(ctx context.Context, tags []string) (QBTagsResult, error) {
	tags = mergeTags(tags)
	if len(tags) == 0 {
		return QBTagsResult{}, fmt.Errorf("at least one qb tag is required")
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return QBTagsResult{}, err
	}
	if err := qb.CreateTags(ctx, tags); err != nil {
		return QBTagsResult{}, err
	}
	return a.GetQBTags(ctx, true)
}

func categoriesFromRecords(records []storage.QBCategoryRecord, state storage.QBCacheStateRecord, stale bool) QBCategoriesResult {
	items := make([]QBCategory, 0, len(records))
	for _, record := range records {
		segments := []string{}
		for _, segment := range strings.Split(record.Name, "/") {
			if segment = strings.TrimSpace(segment); segment != "" {
				segments = append(segments, segment)
			}
		}
		items = append(items, QBCategory{Name: record.Name, SavePath: record.SavePath, PathSegments: segments, SyncedAt: record.SyncedAt})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	result := QBCategoriesResult{Items: items, Stale: stale, Connected: !stale, Error: state.LastError}
	if !state.SyncedAt.IsZero() {
		value := state.SyncedAt
		result.SyncedAt = &value
	}
	return result
}

func tagsFromRecords(records []storage.QBTagRecord, state storage.QBCacheStateRecord, stale bool) QBTagsResult {
	items := make([]string, 0, len(records))
	for _, record := range records {
		items = append(items, record.Name)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i]) < strings.ToLower(items[j]) })
	result := QBTagsResult{Items: items, Stale: stale, Connected: !stale, Error: state.LastError}
	if !state.SyncedAt.IsZero() {
		value := state.SyncedAt
		result.SyncedAt = &value
	}
	return result
}
