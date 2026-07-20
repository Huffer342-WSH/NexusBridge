package core

import (
	"context"
	"fmt"
	"strings"

	"nexusbridge/internal/storage"
)

// ListTorrents 查询本地种子缓存。
func (a *App) ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error) {
	searchSiteIDs := a.searchSiteIDs(query.Search)
	records, err := a.store.ListTorrents(ctx, storage.TorrentListQuery{
		SiteID: query.SiteID, Search: query.Search, SortBy: query.SortBy, SortDirection: query.SortDirection,
		Limit: query.Limit, Offset: query.Offset, SearchSiteIDs: searchSiteIDs, ExcludePinned: query.ExcludePinned,
	})
	if err != nil {
		return nil, err
	}
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
	records, total, err := a.store.ListTorrentPage(ctx, storage.TorrentListQuery{
		SiteID: query.SiteID, Search: query.Search, SortBy: query.SortBy, SortDirection: query.SortDirection,
		Limit: query.Limit, Offset: query.Offset, SearchSiteIDs: searchSiteIDs, ExcludePinned: query.ExcludePinned,
	})
	if err != nil {
		return TorrentPage{}, err
	}
	items, err := a.torrentsFromRecords(ctx, records, query)
	if err != nil {
		return TorrentPage{}, err
	}
	return TorrentPage{Items: items, Offset: query.Offset, Limit: query.Limit, Total: total}, nil
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

// loadCache 从数据库加载最近种子缓存。
func (a *App) loadCache(ctx context.Context) error {
	records, err := a.store.ListTorrents(ctx, storage.TorrentListQuery{Limit: storage.MaxTorrentListLimit})
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, record := range records {
		a.cache[record.SiteID+":"+record.TorrentID] = torrentFromRecord(record)
	}
	return nil
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
