package core

import "context"

type SiteService interface {
	ListSites(ctx context.Context) ([]Site, error)
	GetSiteCredential(ctx context.Context, siteID string) (SiteCredential, error)
	SaveSiteCredential(ctx context.Context, credential SiteCredential) (SiteCredential, error)
}

type TorrentService interface {
	ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error)
	GetTorrentQBStatus(ctx context.Context, siteID, torrentID string, weakMatch bool) (QBTorrentStatus, error)
	ControlTorrentQB(ctx context.Context, siteID, torrentID, action string) (QBTorrentStatus, error)
	FetchTorrentCover(ctx context.Context, siteID, torrentID string) (CoverImage, error)
}

type FetchService interface {
	FetchSite(ctx context.Context, siteID string) (FetchResult, error)
	RunOnce(ctx context.Context, siteID string) (FetchResult, error)
}

type RuleService interface {
	ListRules(ctx context.Context) ([]Rule, error)
	SaveRule(ctx context.Context, rule Rule) (Rule, error)
}

type DownloadTaskService interface {
	ListDownloadTasks(ctx context.Context) ([]DownloadTask, error)
	SyncQB(ctx context.Context) (QBSyncResult, error)
	PollQB(ctx context.Context, rid int) (QBPollResult, error)
}

type OrganizeTaskService interface {
	ListOrganizeTasks(ctx context.Context) ([]OrganizeTask, error)
	OrganizePending(ctx context.Context) (OrganizeResult, error)
}
