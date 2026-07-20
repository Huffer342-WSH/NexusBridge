package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/core"
)

func (s *Server) handleTorrents(w http.ResponseWriter, r *http.Request) {
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if offset < 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("offset must not be negative"))
		return
	}
	if limit < 1 || limit > 100 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("limit must be between 1 and 100"))
		return
	}
	includePinned := true
	if raw := strings.TrimSpace(r.URL.Query().Get("include_pinned")); raw != "" {
		includePinned, err = strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid include_pinned: %w", err))
			return
		}
	}
	page, err := s.torrents.ListTorrentPage(r.Context(), core.TorrentQuery{
		SiteID:        r.URL.Query().Get("site_id"),
		Search:        r.URL.Query().Get("q"),
		SortBy:        r.URL.Query().Get("sort_by"),
		SortDirection: r.URL.Query().Get("sort_direction"),
		Limit:         limit,
		Offset:        offset,
		IncludeQB:     queryBool(r, "include_qb"),
		QBWeakMatch:   queryBool(r, "qb_weak_match"),
		ExcludePinned: !includePinned,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
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

// handleFetchSite 创建后台站点扫描并立即返回持久化任务。
func (s *Server) handleFetchSite(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "site_id")
	slog.Info("api fetch requested", "site_id", siteID)
	var request core.SiteFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	trigger := "manual"
	if r.URL.Query().Get("trigger") == "homepage" {
		trigger = "homepage"
	}
	result, err := s.fetcher.StartSiteFetch(r.Context(), siteID, trigger, request)
	if err != nil {
		slog.Error("api fetch failed", "site_id", siteID, "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slog.Info("api fetch accepted", "site_id", siteID, "job_id", result.ID)
	writeJSON(w, http.StatusAccepted, result)
}

// handleSiteFetchJobs 返回最近的持久化站点扫描任务。
func (s *Server) handleSiteFetchJobs(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := s.fetcher.ListSiteFetchJobs(r.Context(), r.URL.Query().Get("site_id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
