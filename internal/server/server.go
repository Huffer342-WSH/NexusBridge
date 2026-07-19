package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func New(cfg config.Config, app interface {
	core.SiteService
	core.TorrentService
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

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", s.handleHealth)
	r.Get("/api/session", s.handleSession)
	r.Post("/api/session/login", s.handleLogin)
	r.Get("/api/sites", s.handleSites)
	r.Get("/api/sites/{site_id}/credential", s.handleGetSiteCredential)
	r.Post("/api/sites/{site_id}/credential", s.handleSaveSiteCredential)
	r.Get("/api/sites/{site_id}/filter-options", s.handleRuleFilterOptions)
	r.Get("/api/torrents", s.handleTorrents)
	r.Get("/api/torrents/{site_id}/{torrent_id}/cover", s.handleTorrentCover)
	r.Get("/api/torrents/{site_id}/{torrent_id}/qb-status", s.handleTorrentQBStatus)
	r.Post("/api/torrents/{site_id}/{torrent_id}/qb-control", s.handleTorrentQBControl)
	r.Post("/api/sites/{site_id}/fetch", s.handleFetchSite)
	r.Post("/api/sites/{site_id}/run-once", s.handleRunOnce)
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
	r.Post("/api/subscriptions/{subscription_id}/run-once", s.handleRunSubscriptionOnce)
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"addr":   s.cfg.Address(),
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":           "local",
		"requires_login": s.cfg.Auth.Enabled,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.Enabled {
		writeJSON(w, http.StatusOK, map[string]string{"token": "local"})
		return
	}

	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Username != s.cfg.Auth.Username || request.Password != s.cfg.Auth.Password {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": "local"})
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	sites, err := s.sites.ListSites(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sites)
}

// handleGetSiteCredential 返回站点凭据状态。
func (s *Server) handleGetSiteCredential(w http.ResponseWriter, r *http.Request) {
	credential, err := s.sites.GetSiteCredential(r.Context(), chi.URLParam(r, "site_id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	credential.Cookie = ""
	writeJSON(w, http.StatusOK, credential)
}

// handleSaveSiteCredential 保存站点 cookie 和 user-agent。
func (s *Server) handleSaveSiteCredential(w http.ResponseWriter, r *http.Request) {
	var credential core.SiteCredential
	if err := json.NewDecoder(r.Body).Decode(&credential); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	credential.SiteID = chi.URLParam(r, "site_id")
	saved, err := s.sites.SaveSiteCredential(r.Context(), credential)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved.Cookie = ""
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleTorrents(w http.ResponseWriter, r *http.Request) {
	torrents, err := s.torrents.ListTorrents(r.Context(), core.TorrentQuery{
		SiteID:      r.URL.Query().Get("site_id"),
		Search:      r.URL.Query().Get("q"),
		Limit:       50,
		IncludeQB:   queryBool(r, "include_qb"),
		QBWeakMatch: queryBool(r, "qb_weak_match"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, torrents)
}

// handleTorrentCover 返回持久缓存中的封面图片并支持浏览器条件请求。
func (s *Server) handleTorrentCover(w http.ResponseWriter, r *http.Request) {
	cover, err := s.torrents.FetchTorrentCover(r.Context(), chi.URLParam(r, "site_id"), chi.URLParam(r, "torrent_id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	file, err := os.Open(cover.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", cover.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", `"`+cover.SHA256+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(cover.Path), cover.ModTime, file)
}

// handleTorrentQBStatus 返回单个本地种子的 qBittorrent 实时状态。
func (s *Server) handleTorrentQBStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.torrents.GetTorrentQBStatus(
		r.Context(),
		chi.URLParam(r, "site_id"),
		chi.URLParam(r, "torrent_id"),
		queryBool(r, "weak_match"),
	)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleTorrentQBControl 暂停或恢复单个本地种子对应的 qB 任务。
func (s *Server) handleTorrentQBControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, err := s.torrents.ControlTorrentQB(r.Context(), chi.URLParam(r, "site_id"), chi.URLParam(r, "torrent_id"), body.Action)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleFetchSite(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "site_id")
	slog.Info("api fetch requested", "site_id", siteID)
	result, err := s.fetcher.FetchSite(r.Context(), siteID)
	if err != nil {
		slog.Error("api fetch failed", "site_id", siteID, "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slog.Info("api fetch accepted", "site_id", siteID, "fetched", result.Fetched, "changed", result.Changed)
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleRunOnce(w http.ResponseWriter, r *http.Request) {
	result, err := s.fetcher.RunOnce(r.Context(), chi.URLParam(r, "site_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.rules.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) handleSaveRule(w http.ResponseWriter, r *http.Request) {
	var rule core.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.rules.SaveRule(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleRenameRule(w http.ResponseWriter, r *http.Request) {
	var rule core.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.rules.RenameRule(r.Context(), chi.URLParam(r, "rule_name"), rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.rules.DeleteRule(r.Context(), chi.URLParam(r, "rule_name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Errorf("rule not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleRuleFilterOptions(w http.ResponseWriter, r *http.Request) {
	result, err := s.rules.RuleFilterOptions(r.Context(), chi.URLParam(r, "site_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePreviewRule(w http.ResponseWriter, r *http.Request) {
	var request core.RulePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.rules.PreviewRule(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	subscriptions, err := s.subscriptions.ListSubscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, subscriptions)
}

func (s *Server) handleSaveSubscription(w http.ResponseWriter, r *http.Request) {
	var subscription core.Subscription
	if err := json.NewDecoder(r.Body).Decode(&subscription); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.subscriptions.SaveSubscription(r.Context(), subscription)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.subscriptions.DeleteSubscription(r.Context(), chi.URLParam(r, "subscription_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Errorf("subscription not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handlePreviewSubscription(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, err := s.subscriptions.PreviewSubscription(r.Context(), chi.URLParam(r, "subscription_id"), limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRunSubscriptionOnce(w http.ResponseWriter, r *http.Request) {
	result, err := s.subscriptions.RunSubscriptionOnce(r.Context(), chi.URLParam(r, "subscription_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleSubscriptionCandidates(w http.ResponseWriter, r *http.Request) {
	items, err := s.subscriptions.ListSubscriptionCandidates(r.Context(), chi.URLParam(r, "subscription_id"), r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleSubscriptionRuns(w http.ResponseWriter, r *http.Request) {
	items, err := s.subscriptions.ListSubscriptionRuns(r.Context(), r.URL.Query().Get("subscription_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetSiteSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, err := s.subscriptions.GetSiteSchedule(r.Context(), chi.URLParam(r, "site_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (s *Server) handleSaveSiteSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule core.SiteSchedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	schedule.SiteID = chi.URLParam(r, "site_id")
	saved, err := s.subscriptions.SaveSiteSchedule(r.Context(), schedule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleQBCategories(w http.ResponseWriter, r *http.Request) {
	result, err := s.qbCatalog.GetQBCategories(r.Context(), queryBool(r, "refresh"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateQBCategory(w http.ResponseWriter, r *http.Request) {
	var category core.QBCategory
	if err := json.NewDecoder(r.Body).Decode(&category); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.qbCatalog.CreateQBCategory(r.Context(), category)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleQBTags(w http.ResponseWriter, r *http.Request) {
	result, err := s.qbCatalog.GetQBTags(r.Context(), queryBool(r, "refresh"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateQBTags(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.qbCatalog.CreateQBTags(r.Context(), body.Tags)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// handlePreviewRecovery 按本地目录结构返回精确 torrent 候选。
func (s *Server) handlePreviewRecovery(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.PreviewRecovery(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRecoverFolder 重新校验指定候选并恢复 qB 任务。
func (s *Server) handleRecoverFolder(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.RecoverFolder(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

// handleRecoveryAction 启动或删除校验失败后保留的 qB 任务。
func (s *Server) handleRecoveryAction(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryActionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.ControlRecoveryTorrent(r.Context(), chi.URLParam(r, "hash"), request.Action)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleTorrentSizeIndexStatus 返回恢复文件大小索引状态。
func (s *Server) handleTorrentSizeIndexStatus(w http.ResponseWriter, r *http.Request) {
	result, err := s.recovery.GetTorrentSizeIndexStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRebuildTorrentSizeIndex 手动重建全部已保存 torrent 的大小索引。
func (s *Server) handleRebuildTorrentSizeIndex(w http.ResponseWriter, r *http.Request) {
	result, err := s.recovery.RebuildTorrentSizeIndex(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleScanRecoveryCandidates 递归预览可能丢失的 qB 任务。
func (s *Server) handleScanRecoveryCandidates(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryScanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.ScanRecoveryCandidates(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRecoverFolders 串行执行扫描阶段已经唯一选定的恢复候选。
func (s *Server) handleRecoverFolders(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.RecoverFolders(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handlePreviewBatchDownload(w http.ResponseWriter, r *http.Request) {
	var request core.BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.batch.PreviewBatchDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExecuteBatchDownload(w http.ResponseWriter, r *http.Request) {
	var request core.BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.batch.ExecuteBatchDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleRetryDownloadTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.batch.RetryDownloadTask(r.Context(), chi.URLParam(r, "task_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

// handlePreviewTorrentDownload 返回手动下载前的 LLM 标题预览。
func (s *Server) handlePreviewTorrentDownload(w http.ResponseWriter, r *http.Request) {
	request := core.ManualDownloadRequest{
		SiteID:    chi.URLParam(r, "site_id"),
		TorrentID: chi.URLParam(r, "torrent_id"),
	}
	preview, err := s.actions.PreviewTorrentDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// handleSendTorrentDownload 发送单个种子到 qBittorrent。
func (s *Server) handleSendTorrentDownload(w http.ResponseWriter, r *http.Request) {
	request := core.ManualDownloadRequest{
		SiteID:    chi.URLParam(r, "site_id"),
		TorrentID: chi.URLParam(r, "torrent_id"),
	}
	var body struct {
		FormattedTitle string `json:"formatted_title"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
		request.FormattedTitle = body.FormattedTitle
	}
	task, err := s.actions.SendTorrentDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

// handleGetQBittorrent 返回 qBittorrent 设置。
func (s *Server) handleGetQBittorrent(w http.ResponseWriter, r *http.Request) {
	cfg := s.settings.GetQBittorrentConfig(r.Context())
	cfg.Password = ""
	cfg.APIKey = ""
	writeJSON(w, http.StatusOK, cfg)
}

// handleSaveQBittorrent 保存 qBittorrent 设置。
func (s *Server) handleSaveQBittorrent(w http.ResponseWriter, r *http.Request) {
	var cfg config.QBittorrentConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.Password == "" {
		current := s.settings.GetQBittorrentConfig(r.Context())
		cfg.Password = current.Password
	}
	if cfg.APIKey == "" {
		current := s.settings.GetQBittorrentConfig(r.Context())
		cfg.APIKey = current.APIKey
	}
	saved, err := s.settings.SaveQBittorrentConfig(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved.Password = ""
	saved.APIKey = ""
	writeJSON(w, http.StatusOK, saved)
}

// handleGetLLM 返回 LLM 设置。
func (s *Server) handleGetLLM(w http.ResponseWriter, r *http.Request) {
	cfg := s.settings.GetLLMConfig(r.Context())
	cfg.APIKey = ""
	writeJSON(w, http.StatusOK, cfg)
}

// handleSaveLLM 保存 LLM 设置。
func (s *Server) handleSaveLLM(w http.ResponseWriter, r *http.Request) {
	var cfg config.LLMConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.APIKey == "" {
		current := s.settings.GetLLMConfig(r.Context())
		cfg.APIKey = current.APIKey
	}
	saved, err := s.settings.SaveLLMConfig(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved.APIKey = ""
	writeJSON(w, http.StatusOK, saved)
}

// handleGetNetwork 返回网络代理设置。
func (s *Server) handleGetNetwork(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settings.GetNetworkConfig(r.Context()))
}

// handleSaveNetwork 保存网络代理设置。
func (s *Server) handleSaveNetwork(w http.ResponseWriter, r *http.Request) {
	var cfg config.NetworkConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.settings.SaveNetworkConfig(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleGetMihomo 返回 Mihomo 配置目录与 Provider 列表。
func (s *Server) handleGetMihomo(w http.ResponseWriter, r *http.Request) {
	settings, err := s.settings.GetMihomoSettings(r.Context(), r.URL.Query().Get("config_dir"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleSaveMihomoDirectory 验证并保存 Mihomo 配置目录。
func (s *Server) handleSaveMihomoDirectory(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ConfigDir string `json:"config_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := s.settings.SaveMihomoConfigDir(r.Context(), request.ConfigDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleAddMihomoProvider 追加一个 Mihomo HTTP Provider。
func (s *Server) handleAddMihomoProvider(w http.ResponseWriter, r *http.Request) {
	var request mihomo.AddProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := s.settings.AddMihomoProvider(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, settings)
}

func (s *Server) handleQBSync(w http.ResponseWriter, r *http.Request) {
	result, err := s.downloads.SyncQB(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleQBPoll 使用 qB 增量同步接口刷新媒体状态。
func (s *Server) handleQBPoll(w http.ResponseWriter, r *http.Request) {
	rid, _ := strconv.Atoi(r.URL.Query().Get("rid"))
	result, err := s.downloads.PollQB(r.Context(), rid)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDownloadTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.downloads.ListDownloadTasks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleOrganizeTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.organizer.ListOrganizeTasks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleOrganizePending(w http.ResponseWriter, r *http.Request) {
	result, err := s.organizer.OrganizePending(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRSSPlaceholder(w http.ResponseWriter, r *http.Request) {
	http.Error(w, fmt.Sprintf("RSS feed %q is not implemented yet", chi.URLParam(r, "feed_id")), http.StatusNotImplemented)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func queryBool(r *http.Request, name string) bool {
	switch r.URL.Query().Get(name) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func spaFileServer(root string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

// spaFSFileServer 从嵌入文件系统返回静态文件并为前端路由回退 index.html。
func spaFSFileServer(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	index, indexErr := fs.ReadFile(root, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(r.URL.Path)), "/")
		if info, err := fs.Stat(root, name); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		if indexErr != nil {
			http.Error(w, indexErr.Error(), http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}
