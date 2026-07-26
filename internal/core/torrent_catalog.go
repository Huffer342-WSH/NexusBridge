package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"nexusbridge/internal/storage"
)

// ErrQBRuntimeNotReady 表示 progress 筛选尚无完整的 qB 运行态可用。
var ErrQBRuntimeNotReady = errors.New("qBittorrent runtime status is not ready")

// ErrInvalidMediaFilter 表示媒体 checkbox 或促销条件不符合单站点定义。
var ErrInvalidMediaFilter = errors.New("invalid media filter")

// ListTorrents 查询本地种子缓存。
func (a *App) ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error) {
	if query.QBTask != "" || len(query.Categories) > 0 || len(query.SiteCheckboxes) > 0 || len(query.Promotions) > 0 {
		page, err := a.ListTorrentPage(ctx, query)
		return page.Items, err
	}
	searchSiteIDs := a.searchSiteIDs(query.Search)
	pinned := a.pinnedSnapshot(query.SiteID)
	excluded := []storage.TorrentKey(nil)
	if query.ExcludePinned {
		excluded = pinnedKeys(pinned)
	}
	records, err := a.store.ListTorrents(ctx, storage.TorrentListQuery{
		SiteID: query.SiteID, Search: query.Search, SortBy: query.SortBy, SortDirection: query.SortDirection,
		Limit: query.Limit, Offset: query.Offset, SearchSiteIDs: searchSiteIDs, ExcludeKeys: excluded,
	})
	if err != nil {
		return nil, err
	}
	applyPinnedLevels(records, pinned)
	return a.torrentsFromRecords(ctx, records, query)
}

func (a *App) torrentsFromRecords(ctx context.Context, records []storage.TorrentRecord, query TorrentQuery) ([]Torrent, error) {
	torrents := make([]Torrent, 0, len(records))
	keys := make([]storage.TorrentKey, 0, len(records))
	for _, record := range records {
		torrents = append(torrents, torrentFromRecord(record))
		keys = append(keys, storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID})
	}
	files, err := a.store.ListTorrentFileMetadata(ctx, keys)
	if err != nil {
		return nil, err
	}
	snapshots, err := a.store.ListQBSnapshots(ctx, keys)
	if err != nil {
		return nil, err
	}
	for i := range torrents {
		key := storage.TorrentKey{SiteID: torrents[i].SiteID, TorrentID: torrents[i].ID}
		if file, ok := files[key]; ok {
			torrents[i].TorrentFileSaved = file.HasData
			torrents[i].InfoHashV1 = file.InfoHashV1
			torrents[i].InfoHashV2 = file.InfoHashV2
			torrents[i].TorrentFileError = file.LastError
		}
		if snapshot, ok := snapshots[key]; ok {
			status := qbStatusFromSnapshot(snapshot)
			torrents[i].QBStatus = &status
		}
	}
	a.qbRuntimeMu.RLock()
	for i := range torrents {
		key := storage.TorrentKey{SiteID: torrents[i].SiteID, TorrentID: torrents[i].ID}
		if snapshot, ok := a.qbRuntime[key]; ok {
			status := qbStatusFromSnapshot(snapshot)
			torrents[i].QBStatus = &status
		}
	}
	a.qbRuntimeMu.RUnlock()
	if query.IncludeQB {
		statuses, err := a.qbStatusesForTorrents(ctx, torrents, query.QBWeakMatch)
		if err != nil {
			return nil, err
		}
		for i := range torrents {
			if status, ok := statuses[torrentKey(torrents[i])]; ok {
				torrents[i].QBStatus = status
			}
		}
	}
	return torrents, nil
}

// ListTorrentPage 返回媒体页所需的范围数据和筛选后总数。
func (a *App) ListTorrentPage(ctx context.Context, query TorrentQuery) (TorrentPage, error) {
	if query.Offset < 0 {
		return TorrentPage{}, fmt.Errorf("torrent offset must not be negative")
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 100 {
		return TorrentPage{}, fmt.Errorf("torrent limit must not exceed 100")
	}
	if strings.TrimSpace(query.SortBy) == "" {
		query.SortBy = "published_at"
	}
	if strings.TrimSpace(query.SortDirection) == "" {
		query.SortDirection = "desc"
	}
	if err := a.normalizeMediaFilters(ctx, &query); err != nil {
		return TorrentPage{}, err
	}
	progressKeys, err := a.qbProgressFilterKeys(query)
	if err != nil {
		return TorrentPage{}, err
	}
	searchSiteIDs := a.searchSiteIDs(query.Search)
	pinned := a.pinnedSnapshot(query.SiteID)
	pinnedRecords, err := a.matchingPinnedRecords(ctx, pinned, query, searchSiteIDs, progressKeys)
	if err != nil {
		return TorrentPage{}, err
	}
	if query.ExcludePinned {
		pinnedRecords = nil
	}
	normalOffset := 0
	pinnedOffset := query.Offset
	if query.ExcludePinned {
		normalOffset = query.Offset
	} else if query.Offset >= len(pinnedRecords) {
		normalOffset = query.Offset - len(pinnedRecords)
		pinnedOffset = len(pinnedRecords)
	}
	pinnedPage := []storage.TorrentRecord{}
	if pinnedOffset < len(pinnedRecords) {
		end := min(len(pinnedRecords), pinnedOffset+query.Limit)
		pinnedPage = append(pinnedPage, pinnedRecords[pinnedOffset:end]...)
	}
	normalLimit := query.Limit - len(pinnedPage)
	normalQuery := storage.TorrentListQuery{
		SiteID: query.SiteID, Search: query.Search, SortBy: query.SortBy, SortDirection: query.SortDirection,
		QBTask: query.QBTask, QBProgressKeys: progressKeys, Categories: query.Categories,
		SiteCheckboxes: storageCheckboxFilters(query.SiteCheckboxes), Promotions: query.Promotions,
		Limit: max(1, normalLimit), Offset: normalOffset, SearchSiteIDs: searchSiteIDs, ExcludeKeys: pinnedKeys(pinned),
	}
	normalRecords, normalTotal, err := a.store.ListTorrentPage(ctx, normalQuery)
	if err != nil {
		return TorrentPage{}, err
	}
	if normalLimit == 0 {
		normalRecords = nil
	}
	records := append(pinnedPage, normalRecords...)
	total := normalTotal
	if !query.ExcludePinned {
		total += len(pinnedRecords)
	}
	items, err := a.torrentsFromRecords(ctx, records, query)
	if err != nil {
		return TorrentPage{}, err
	}
	return TorrentPage{Items: items, Offset: query.Offset, Limit: query.Limit, Total: total}, nil
}

type pinnedTorrent struct {
	Key   storage.TorrentKey
	Level int
}

func (a *App) replaceSitePinned(siteID string, records []storage.TorrentRecord) {
	next := make(map[storage.TorrentKey]int)
	for _, record := range records {
		if record.StickyLevel <= 0 {
			continue
		}
		key := storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}
		next[key] = record.StickyLevel
	}
	a.pinnedMu.Lock()
	a.pinnedBySite[siteID] = next
	a.pinnedMu.Unlock()
}

func (a *App) pinnedSnapshot(siteID string) []pinnedTorrent {
	a.pinnedMu.RLock()
	defer a.pinnedMu.RUnlock()
	result := []pinnedTorrent{}
	appendSite := func(items map[storage.TorrentKey]int) {
		for key, level := range items {
			result = append(result, pinnedTorrent{Key: key, Level: level})
		}
	}
	if siteID != "" {
		appendSite(a.pinnedBySite[siteID])
	} else {
		for _, items := range a.pinnedBySite {
			appendSite(items)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Level != result[j].Level {
			return result[i].Level > result[j].Level
		}
		if result[i].Key.SiteID != result[j].Key.SiteID {
			return result[i].Key.SiteID < result[j].Key.SiteID
		}
		return result[i].Key.TorrentID < result[j].Key.TorrentID
	})
	return result
}

func pinnedKeys(pinned []pinnedTorrent) []storage.TorrentKey {
	keys := make([]storage.TorrentKey, 0, len(pinned))
	for _, item := range pinned {
		keys = append(keys, item.Key)
	}
	return keys
}

func applyPinnedLevels(records []storage.TorrentRecord, pinned []pinnedTorrent) {
	levels := make(map[storage.TorrentKey]int, len(pinned))
	for _, item := range pinned {
		levels[item.Key] = item.Level
	}
	for i := range records {
		records[i].StickyLevel = levels[storage.TorrentKey{SiteID: records[i].SiteID, TorrentID: records[i].TorrentID}]
	}
}

func (a *App) matchingPinnedRecords(
	ctx context.Context,
	pinned []pinnedTorrent,
	query TorrentQuery,
	searchSiteIDs []string,
	progressKeys map[storage.TorrentKey]struct{},
) ([]storage.TorrentRecord, error) {
	records, err := a.store.ListTorrentsByKeys(ctx, pinnedKeys(pinned))
	if err != nil {
		return nil, err
	}
	qbSnapshots := map[storage.TorrentKey]storage.QBSnapshotRecord{}
	if query.QBTask != "" {
		qbSnapshots, err = a.store.ListQBSnapshots(ctx, pinnedKeys(pinned))
		if err != nil {
			return nil, err
		}
		a.qbRuntimeMu.RLock()
		for _, item := range pinned {
			if snapshot, ok := a.qbRuntime[item.Key]; ok {
				qbSnapshots[item.Key] = snapshot
			}
		}
		a.qbRuntimeMu.RUnlock()
	}
	siteMatches := make(map[string]struct{}, len(searchSiteIDs))
	for _, siteID := range searchSiteIDs {
		siteMatches[siteID] = struct{}{}
	}
	levels := make(map[storage.TorrentKey]int, len(pinned))
	for _, item := range pinned {
		levels[item.Key] = item.Level
	}
	search := strings.ToLower(strings.TrimSpace(query.Search))
	result := make([]storage.TorrentRecord, 0, len(records))
	for _, record := range records {
		key := storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}
		if query.SiteID != "" && record.SiteID != query.SiteID {
			continue
		}
		if search != "" {
			textMatch := strings.Contains(strings.ToLower(record.Title), search) ||
				strings.Contains(strings.ToLower(record.Category), search) ||
				strings.Contains(strings.ToLower(record.Promotion), search) ||
				strings.Contains(strings.ToLower(record.SiteID), search)
			_, siteMatch := siteMatches[record.SiteID]
			if !textMatch && !siteMatch {
				continue
			}
		}
		if len(query.Categories) > 0 && !containsFold(query.Categories, record.Category) {
			continue
		}
		checkboxesMatch := true
		for _, group := range query.SiteCheckboxes {
			if !containsAnyFold(record.TagIDs, group.Values) {
				checkboxesMatch = false
				break
			}
		}
		if !checkboxesMatch {
			continue
		}
		if len(query.Promotions) > 0 && !containsFold(query.Promotions, recordPromotionKey(record)) {
			continue
		}
		if query.QBTask != "" {
			added := qbSnapshots[key].Added
			if (query.QBTask == "present" && !added) || (query.QBTask == "absent" && added) {
				continue
			}
			if progressKeys != nil {
				if _, matches := progressKeys[key]; !matches {
					continue
				}
			}
		}
		record.StickyLevel = levels[key]
		result = append(result, record)
	}
	return result, nil
}

func (a *App) normalizeMediaFilters(ctx context.Context, query *TorrentQuery) error {
	if len(query.Categories) == 0 && len(query.SiteCheckboxes) == 0 && len(query.Promotions) == 0 {
		return nil
	}
	query.SiteID = strings.TrimSpace(query.SiteID)
	if query.SiteID == "" {
		return fmt.Errorf("%w: site checkbox and promotion filters require site_id", ErrInvalidMediaFilter)
	}
	options, err := a.MediaFilterOptions(ctx, query.SiteID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidMediaFilter, err)
	}
	categories, err := canonicalMediaFilterValues(query.Categories, options.Categories)
	if err != nil {
		return fmt.Errorf("%w: category %v", ErrInvalidMediaFilter, err)
	}
	promotions, err := canonicalMediaFilterValues(query.Promotions, options.Promotions)
	if err != nil {
		return fmt.Errorf("%w: promotion %v", ErrInvalidMediaFilter, err)
	}
	checkboxGroups := make(map[string]MediaFilterGroup, len(options.Checkboxes))
	for _, group := range options.Checkboxes {
		checkboxGroups[strings.ToLower(strings.TrimSpace(group.Name))] = group
	}
	checkboxes := make([]MediaCheckboxFilter, 0, len(query.SiteCheckboxes))
	checkboxIndexes := map[string]int{}
	for _, selected := range query.SiteCheckboxes {
		key := strings.ToLower(strings.TrimSpace(selected.Name))
		group, ok := checkboxGroups[key]
		if !ok {
			return fmt.Errorf("%w: site checkbox group %q is not configured", ErrInvalidMediaFilter, selected.Name)
		}
		values, err := canonicalMediaFilterValues(selected.Values, group.Options)
		if err != nil {
			return fmt.Errorf("%w: site checkbox %s %v", ErrInvalidMediaFilter, group.Name, err)
		}
		if len(values) == 0 {
			continue
		}
		if index, exists := checkboxIndexes[key]; exists {
			merged, err := canonicalMediaFilterValues(append(checkboxes[index].Values, values...), group.Options)
			if err != nil {
				return fmt.Errorf("%w: site checkbox %s %v", ErrInvalidMediaFilter, group.Name, err)
			}
			checkboxes[index].Values = merged
			continue
		}
		checkboxIndexes[key] = len(checkboxes)
		checkboxes = append(checkboxes, MediaCheckboxFilter{Name: group.Name, Values: values})
	}
	query.Categories = categories
	query.SiteCheckboxes = checkboxes
	query.Promotions = promotions
	return nil
}

func storageCheckboxFilters(filters []MediaCheckboxFilter) []storage.TorrentCheckboxFilter {
	result := make([]storage.TorrentCheckboxFilter, 0, len(filters))
	for _, filter := range filters {
		result = append(result, storage.TorrentCheckboxFilter{Name: filter.Name, Values: filter.Values})
	}
	return result
}

func containsAnyFold(values, selected []string) bool {
	for _, value := range values {
		if containsFold(selected, value) {
			return true
		}
	}
	return false
}

func canonicalMediaFilterValues(selected []string, options []MediaFilterOption) ([]string, error) {
	allowed := make(map[string]string, len(options))
	for _, option := range options {
		value := strings.TrimSpace(option.Value)
		if value != "" {
			allowed[strings.ToLower(value)] = value
		}
	}
	result := make([]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, raw := range selected {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		value, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("%q is not configured", raw)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func recordPromotionKey(record storage.TorrentRecord) string {
	class := strings.TrimSpace(record.PromotionClass)
	if class == "" {
		return "normal"
	}
	if fields := strings.Fields(class); len(fields) > 0 {
		return fields[0]
	}
	return class
}

func (a *App) qbProgressFilterKeys(query TorrentQuery) (map[storage.TorrentKey]struct{}, error) {
	if query.QBProgress == "" {
		return nil, nil
	}
	a.qbRuntimeMu.RLock()
	defer a.qbRuntimeMu.RUnlock()
	if !a.qbRuntimeReady {
		return nil, ErrQBRuntimeNotReady
	}
	result := make(map[storage.TorrentKey]struct{})
	for key, snapshot := range a.qbRuntime {
		if !snapshot.Added {
			continue
		}
		complete := snapshot.Progress >= 1
		if (query.QBProgress == "complete" && complete) || (query.QBProgress == "incomplete" && !complete) {
			result[key] = struct{}{}
		}
	}
	return result, nil
}

func (a *App) searchSiteIDs(search string) []string {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return nil
	}
	result := []string{}
	for _, siteID := range a.siteIDs {
		site := a.sites[siteID]
		if strings.Contains(strings.ToLower(site.ID), search) || strings.Contains(strings.ToLower(site.Name), search) {
			result = append(result, site.ID)
		}
	}
	return result
}

// getTorrent 从数据库读取单条种子。
func (a *App) getTorrent(ctx context.Context, siteID, torrentID string) (Torrent, error) {
	record, ok, err := a.store.GetTorrent(ctx, siteID, torrentID)
	if err != nil {
		return Torrent{}, err
	}
	if !ok {
		return Torrent{}, fmt.Errorf("torrent %s/%s not found", siteID, torrentID)
	}
	return torrentFromRecord(record), nil
}
