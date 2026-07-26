package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/mihomo"
)

type Server struct {
	cfg           config.Config
	sites         core.SiteService
	torrents      core.TorrentService
	playback      core.PlaybackService
	fetcher       core.FetchService
	rules         core.RuleService
	subscriptions core.SubscriptionService
	qbCatalog     core.QBCatalogService
	recovery      core.RecoveryService
	batch         core.BatchDownloadService
	downloads     core.DownloadTaskService
	organizer     core.OrganizeTaskService
	settings      settingsService
	actions       torrentActionService
	static        string
	staticFS      fs.FS
}

type settingsService interface {
	GetQBittorrentConfig(ctx context.Context) config.QBittorrentConfig
	SaveQBittorrentConfig(ctx context.Context, cfg config.QBittorrentConfig) (config.QBittorrentConfig, error)
	GetLLMConfig(ctx context.Context) config.LLMConfig
	SaveLLMConfig(ctx context.Context, cfg config.LLMConfig) (config.LLMConfig, error)
	GetNetworkConfig(ctx context.Context) config.NetworkConfig
	SaveNetworkConfig(ctx context.Context, cfg config.NetworkConfig) (config.NetworkConfig, error)
	GetMihomoSettings(ctx context.Context, configDir string) (mihomo.Settings, error)
	SaveMihomoConfigDir(ctx context.Context, configDir string) (mihomo.Settings, error)
	AddMihomoProvider(ctx context.Context, request mihomo.AddProviderRequest) (mihomo.Settings, error)
}

type torrentActionService interface {
	PreviewTorrentDownload(ctx context.Context, request core.ManualDownloadRequest) (core.DownloadPreview, error)
	SendTorrentDownload(ctx context.Context, request core.ManualDownloadRequest) (core.DownloadTask, error)
}

// New 创建使用本地 WebUI 目录的 HTTP 服务。
func New(cfg config.Config, app interface {
	core.SiteService
	core.TorrentService
	core.PlaybackService
	core.FetchService
	core.RuleService
	core.SubscriptionService
	core.QBCatalogService
	core.RecoveryService
	core.BatchDownloadService
	core.DownloadTaskService
	core.OrganizeTaskService
	settingsService
	torrentActionService
}, staticDir string) *Server {
	return newServer(cfg, app, staticDir, nil)
}

// NewWithAssets 创建使用嵌入 WebUI 文件系统的 HTTP 服务。
func NewWithAssets(cfg config.Config, app interface {
	core.SiteService
	core.TorrentService
	core.PlaybackService
	core.FetchService
	core.RuleService
	core.SubscriptionService
	core.QBCatalogService
	core.RecoveryService
	core.BatchDownloadService
	core.DownloadTaskService
	core.OrganizeTaskService
	settingsService
	torrentActionService
}, assets fs.FS) *Server {
	return newServer(cfg, app, "", assets)
}

// newServer 组装共享业务服务与可选静态资源来源。
func newServer(cfg config.Config, app interface {
	core.SiteService
	core.TorrentService
	core.PlaybackService
	core.FetchService
	core.RuleService
	core.SubscriptionService
	core.QBCatalogService
	core.RecoveryService
	core.BatchDownloadService
	core.DownloadTaskService
	core.OrganizeTaskService
	settingsService
	torrentActionService
}, staticDir string, assets fs.FS) *Server {
	return &Server{
		cfg:           cfg,
		sites:         app,
		torrents:      app,
		playback:      app,
		fetcher:       app,
		rules:         app,
		subscriptions: app,
		qbCatalog:     app,
		recovery:      app,
		batch:         app,
		downloads:     app,
		organizer:     app,
		settings:      app,
		actions:       app,
		static:        staticDir,
		staticFS:      assets,
	}
}

// Handler 返回包含 API 和 WebUI 回退路由的 HTTP handler。
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", s.handleHealth)
	r.Get("/api/session", s.handleSession)
	r.Post("/api/session/login", s.handleLogin)
	r.Get("/api/sites", s.handleSites)
	r.Get("/api/sites/{site_id}/credential", s.handleGetSiteCredential)
	r.Post("/api/sites/{site_id}/credential", s.handleSaveSiteCredential)
	r.Get("/api/sites/{site_id}/attendance", s.handleGetSiteAttendance)
	r.Post("/api/sites/{site_id}/attendance", s.handleSaveSiteAttendance)
	r.Get("/api/sites/{site_id}/filter-options", s.handleRuleFilterOptions)
	r.Get("/api/torrents", s.handleTorrents)
	r.Get("/api/torrents/{site_id}/{torrent_id}/cover", s.handleTorrentCover)
	r.Get("/api/torrents/{site_id}/{torrent_id}/playback", s.handleTorrentPlayback)
	r.Get("/api/playback/torrents", s.handlePlaybackTorrents)
	r.Get("/api/playback/qb/{hash}", s.handleQBPlayback)
	r.Get("/api/playback/file", s.handleFilePlayback)
	r.Get("/api/torrents/{site_id}/{torrent_id}/media/{file_index}", s.handleTorrentMedia)
	r.Head("/api/torrents/{site_id}/{torrent_id}/media/{file_index}", s.handleTorrentMedia)
	r.Get("/api/torrents/{site_id}/{torrent_id}/media/{file_index}/subtitles/{track_id}", s.handleTorrentSubtitle)
	r.Get("/api/playback/qb/{hash}/media/{file_index}", s.handleQBMedia)
	r.Head("/api/playback/qb/{hash}/media/{file_index}", s.handleQBMedia)
	r.Get("/api/playback/qb/{hash}/media/{file_index}/subtitles/{track_id}", s.handleQBSubtitle)
	r.Get("/api/playback/file/media", s.handleFileMedia)
	r.Head("/api/playback/file/media", s.handleFileMedia)
	r.Get("/api/playback/file/subtitles/{track_id}", s.handleFileSubtitle)
	r.Get("/api/torrents/{site_id}/{torrent_id}/qb-status", s.handleTorrentQBStatus)
	r.Post("/api/torrents/{site_id}/{torrent_id}/qb-control", s.handleTorrentQBControl)
	r.Delete("/api/qb/torrents/{hash}", s.handleDeleteQBTorrent)
	r.Post("/api/sites/{site_id}/fetch", s.handleFetchSite)
	r.Get("/api/site-fetch-jobs", s.handleSiteFetchJobs)
	r.Post("/api/torrents/{site_id}/{torrent_id}/download/preview", s.handlePreviewTorrentDownload)
	r.Post("/api/torrents/{site_id}/{torrent_id}/download", s.handleSendTorrentDownload)
	r.Get("/api/rules", s.handleRules)
	r.Post("/api/rules", s.handleSaveRule)
	r.Put("/api/rules/{rule_name}", s.handleRenameRule)
	r.Delete("/api/rules/{rule_name}", s.handleDeleteRule)
	r.Post("/api/rules/preview", s.handlePreviewRule)
	r.Get("/api/subscriptions", s.handleSubscriptions)
	r.Post("/api/subscriptions", s.handleSaveSubscription)
	r.Delete("/api/subscriptions/{subscription_id}", s.handleDeleteSubscription)
	r.Post("/api/subscriptions/{subscription_id}/preview", s.handlePreviewSubscription)
	r.Get("/api/subscriptions/{subscription_id}/candidates", s.handleSubscriptionCandidates)
	r.Get("/api/subscription-runs", s.handleSubscriptionRuns)
	r.Get("/api/sites/{site_id}/schedule", s.handleGetSiteSchedule)
	r.Post("/api/sites/{site_id}/schedule", s.handleSaveSiteSchedule)
	r.Get("/api/settings/qbittorrent", s.handleGetQBittorrent)
	r.Post("/api/settings/qbittorrent", s.handleSaveQBittorrent)
	r.Get("/api/settings/llm", s.handleGetLLM)
	r.Post("/api/settings/llm", s.handleSaveLLM)
	r.Get("/api/settings/network", s.handleGetNetwork)
	r.Post("/api/settings/network", s.handleSaveNetwork)
	r.Get("/api/settings/fetch", s.handleGetFetchSettings)
	r.Post("/api/settings/fetch", s.handleSaveFetchSettings)
	r.Get("/api/settings/mihomo", s.handleGetMihomo)
	r.Post("/api/settings/mihomo/directory", s.handleSaveMihomoDirectory)
	r.Post("/api/settings/mihomo/providers", s.handleAddMihomoProvider)
	r.Post("/api/qb/sync", s.handleQBSync)
	r.Get("/api/qb/poll", s.handleQBPoll)
	r.Get("/api/qb/categories", s.handleQBCategories)
	r.Post("/api/qb/categories", s.handleCreateQBCategory)
	r.Get("/api/qb/tags", s.handleQBTags)
	r.Post("/api/qb/tags", s.handleCreateQBTags)
	r.Post("/api/files/browse", s.handleBrowseFiles)
	r.Post("/api/qb/recovery/preview", s.handlePreviewRecovery)
	r.Post("/api/qb/recovery", s.handleRecoverFolder)
	r.Post("/api/qb/recovery/{hash}/action", s.handleRecoveryAction)
	r.Post("/api/qb/recovery/scan", s.handleScanRecoveryCandidates)
	r.Post("/api/qb/recovery/batch", s.handleRecoverFolders)
	r.Get("/api/qb/recovery/index", s.handleTorrentSizeIndexStatus)
	r.Post("/api/qb/recovery/index/rebuild", s.handleRebuildTorrentSizeIndex)
	r.Get("/api/download-tasks", s.handleDownloadTasks)
	r.Post("/api/download-tasks/{task_id}/retry", s.handleRetryDownloadTask)
	r.Post("/api/downloads/batch/preview", s.handlePreviewBatchDownload)
	r.Post("/api/downloads/batch", s.handleExecuteBatchDownload)
	r.Get("/api/organize-tasks", s.handleOrganizeTasks)
	r.Post("/api/organize/pending", s.handleOrganizePending)
	r.Get("/rss/{feed_id}", s.handleRSSPlaceholder)

	if s.staticFS != nil {
		r.Handle("/*", spaFSFileServer(s.staticFS))
	} else if s.static != "" && dirExists(s.static) {
		r.Handle("/*", spaFileServer(s.static))
	}

	return r
}

// handleBrowseFiles 返回文件管理器目录内容和 qB 归属。
func (s *Server) handleBrowseFiles(w http.ResponseWriter, r *http.Request) {
	var request core.FileBrowseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.BrowseFiles(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ListenAndServe 运行 HTTP 服务并在上下文取消时优雅关闭。
func (s *Server) ListenAndServe(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.cfg.Address(),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
