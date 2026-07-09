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
)

type Server struct {
	cfg       config.Config
	sites     core.SiteService
	torrents  core.TorrentService
	fetcher   core.FetchService
	rules     core.RuleService
	downloads core.DownloadTaskService
	organizer core.OrganizeTaskService
	settings  settingsService
	actions   torrentActionService
	static    string
	staticFS  fs.FS
}

type settingsService interface {
	GetQBittorrentConfig(ctx context.Context) config.QBittorrentConfig
	SaveQBittorrentConfig(ctx context.Context, cfg config.QBittorrentConfig) (config.QBittorrentConfig, error)
	GetLLMConfig(ctx context.Context) config.LLMConfig
	SaveLLMConfig(ctx context.Context, cfg config.LLMConfig) (config.LLMConfig, error)
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
	core.DownloadTaskService
	core.OrganizeTaskService
	settingsService
	torrentActionService
}, staticDir string, assets fs.FS) *Server {
	return &Server{
		cfg:       cfg,
		sites:     app,
		torrents:  app,
		fetcher:   app,
		rules:     app,
		downloads: app,
		organizer: app,
		settings:  app,
		actions:   app,
		static:    staticDir,
		staticFS:  assets,
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
	r.Get("/api/settings/qbittorrent", s.handleGetQBittorrent)
	r.Post("/api/settings/qbittorrent", s.handleSaveQBittorrent)
	r.Get("/api/settings/llm", s.handleGetLLM)
	r.Post("/api/settings/llm", s.handleSaveLLM)
	r.Post("/api/qb/sync", s.handleQBSync)
	r.Get("/api/qb/poll", s.handleQBPoll)
	r.Get("/api/download-tasks", s.handleDownloadTasks)
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

// handleTorrentCover 代理返回需要站点凭据或 Referer 的封面图片。
func (s *Server) handleTorrentCover(w http.ResponseWriter, r *http.Request) {
	cover, err := s.torrents.FetchTorrentCover(r.Context(), chi.URLParam(r, "site_id"), chi.URLParam(r, "torrent_id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", cover.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cover.Data)
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
