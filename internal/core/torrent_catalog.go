package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"nexusbridge/internal/storage"
)

// ListTorrents 查询本地种子缓存。
func (a *App) ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error) {
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
	searchSiteIDs := a.searchSiteIDs(query.Search)
	pinned := a.pinnedSnapshot(query.SiteID)
	pinnedRecords, err := a.matchingPinnedRecords(ctx, pinned, query, searchSiteIDs)
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

func (a *App) matchingPinnedRecords(ctx context.Context, pinned []pinnedTorrent, query TorrentQuery, searchSiteIDs []string) ([]storage.TorrentRecord, error) {
	records, err := a.store.ListTorrentsByKeys(ctx, pinnedKeys(pinned))
	if err != nil {
		return nil, err
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
		record.StickyLevel = levels[storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}]
		result = append(result, record)
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
