package core

import "context"

type SiteService interface {
	ListSites(ctx context.Context) ([]Site, error)
	MediaFilterOptions(ctx context.Context, siteID string) (MediaFilterOptions, error)
	GetSiteCredential(ctx context.Context, siteID string) (SiteCredential, error)
	SaveSiteCredential(ctx context.Context, credential SiteCredential) (SiteCredential, error)
	GetSiteAttendance(ctx context.Context, siteID string) (SiteAttendance, error)
	SaveSiteAttendance(ctx context.Context, attendance SiteAttendance) (SiteAttendance, error)
}

type TorrentService interface {
	ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error)
	ListTorrentPage(ctx context.Context, query TorrentQuery) (TorrentPage, error)
	GetTorrentQBStatus(ctx context.Context, siteID, torrentID string, weakMatch bool) (QBTorrentStatus, error)
	ControlTorrentQB(ctx context.Context, siteID, torrentID, action string) (QBTorrentStatus, error)
	DeleteQBTorrent(ctx context.Context, hash string, deleteFiles bool) error
	FetchTorrentCover(ctx context.Context, siteID, torrentID string) (CoverImage, error)
}

// PlaybackService 提供同机 qB 下载文件的媒体清单和只读源文件访问。
type PlaybackService interface {
	GetTorrentPlayback(ctx context.Context, siteID, torrentID, fileName string) (PlaybackContext, error)
	GetQBPlayback(ctx context.Context, hash, fileName string) (PlaybackContext, error)
	GetFilePlayback(ctx context.Context, path string) (PlaybackContext, error)
	ListPlaybackTorrents(ctx context.Context, excludeSiteID, excludeTorrentID string, limit int) ([]PlaybackTorrent, error)
	OpenTorrentMedia(ctx context.Context, siteID, torrentID string, fileIndex int) (PlaybackSource, error)
	OpenQBMedia(ctx context.Context, hash string, fileIndex int) (PlaybackSource, error)
	OpenFileMedia(ctx context.Context, path string) (PlaybackSource, error)
	GetTorrentSubtitle(ctx context.Context, siteID, torrentID string, fileIndex int, trackID uint64) ([]byte, error)
	GetQBSubtitle(ctx context.Context, hash string, fileIndex int, trackID uint64) ([]byte, error)
	GetFileSubtitle(ctx context.Context, path string, trackID uint64) ([]byte, error)
}

type FetchService interface {
	StartSiteFetch(ctx context.Context, siteID, trigger string, request SiteFetchRequest) (SiteFetchJob, error)
	ListSiteFetchJobs(ctx context.Context, siteID string, limit int) ([]SiteFetchJob, error)
	GetFetchSettings(ctx context.Context) (FetchSettings, error)
	SaveFetchSettings(ctx context.Context, settings FetchSettings) (FetchSettings, error)
}

type RuleService interface {
	ListRules(ctx context.Context) ([]Rule, error)
	SaveRule(ctx context.Context, rule Rule) (Rule, error)
	RenameRule(ctx context.Context, oldName string, rule Rule) (Rule, error)
	DeleteRule(ctx context.Context, name string) (bool, error)
	PreviewRule(ctx context.Context, request RulePreviewRequest) (RulePreviewResult, error)
	RuleFilterOptions(ctx context.Context, siteID string) (RuleFilterOptions, error)
}

type SubscriptionService interface {
	ListSubscriptions(ctx context.Context) ([]Subscription, error)
	SaveSubscription(ctx context.Context, subscription Subscription) (Subscription, error)
	DeleteSubscription(ctx context.Context, id string) (bool, error)
	PreviewSubscription(ctx context.Context, id string, limit int) (SubscriptionPreview, error)
	ListSubscriptionCandidates(ctx context.Context, subscriptionID, status string) ([]SubscriptionCandidate, error)
	ListSubscriptionRuns(ctx context.Context, subscriptionID string) ([]SubscriptionRun, error)
	GetSiteSchedule(ctx context.Context, siteID string) (SiteSchedule, error)
	SaveSiteSchedule(ctx context.Context, schedule SiteSchedule) (SiteSchedule, error)
}

type QBCatalogService interface {
	GetQBCategories(ctx context.Context, refresh bool) (QBCategoriesResult, error)
	CreateQBCategory(ctx context.Context, category QBCategory) (QBCategoriesResult, error)
	GetQBTags(ctx context.Context, refresh bool) (QBTagsResult, error)
	CreateQBTags(ctx context.Context, tags []string) (QBTagsResult, error)
}

type RecoveryService interface {
	BrowseFiles(ctx context.Context, request FileBrowseRequest) (FileBrowseResult, error)
	PreviewRecovery(ctx context.Context, request RecoveryPreviewRequest) (RecoveryPreview, error)
	RecoverFolder(ctx context.Context, request RecoveryRequest) (RecoveryResult, error)
	ControlRecoveryTorrent(ctx context.Context, hash, action string) (RecoveryActionResult, error)
	GetTorrentSizeIndexStatus(ctx context.Context) (TorrentSizeIndexStatus, error)
	RebuildTorrentSizeIndex(ctx context.Context) (TorrentSizeIndexStatus, error)
	ScanRecoveryCandidates(ctx context.Context, request RecoveryScanRequest) (RecoveryScanResult, error)
	RecoverFolders(ctx context.Context, request RecoveryBatchRequest) (RecoveryBatchResult, error)
}

type BatchDownloadService interface {
	PreviewBatchDownload(ctx context.Context, request BatchDownloadRequest) (BatchDownloadPreview, error)
	ExecuteBatchDownload(ctx context.Context, request BatchDownloadRequest) (BatchDownloadResult, error)
	RetryDownloadTask(ctx context.Context, id string) (DownloadTask, error)
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
