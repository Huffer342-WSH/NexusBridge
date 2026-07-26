package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
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
	qbTask := strings.TrimSpace(r.URL.Query().Get("qb_task"))
	if qbTask != "" && qbTask != "present" && qbTask != "absent" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("qb_task must be present or absent"))
		return
	}
	qbProgress := strings.TrimSpace(r.URL.Query().Get("qb_progress"))
	if qbProgress != "" && qbProgress != "complete" && qbProgress != "incomplete" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("qb_progress must be complete or incomplete"))
		return
	}
	if qbProgress != "" && qbTask != "present" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("qb_progress requires qb_task=present"))
		return
	}
	categories, err := mediaFilterQueryValues(r, "category")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	promotions, err := mediaFilterQueryValues(r, "promotion")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	siteCheckboxes, err := mediaCheckboxQueryValues(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	siteID := strings.TrimSpace(r.URL.Query().Get("site_id"))
	if siteID == "" && (len(categories) > 0 || len(siteCheckboxes) > 0 || len(promotions) > 0) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("site checkbox and promotion filters require site_id"))
		return
	}
	page, err := s.torrents.ListTorrentPage(r.Context(), core.TorrentQuery{
		SiteID:         siteID,
		Search:         r.URL.Query().Get("q"),
		SortBy:         r.URL.Query().Get("sort_by"),
		SortDirection:  r.URL.Query().Get("sort_direction"),
		QBTask:         qbTask,
		QBProgress:     qbProgress,
		Categories:     categories,
		SiteCheckboxes: siteCheckboxes,
		Promotions:     promotions,
		Limit:          limit,
		Offset:         offset,
		IncludeQB:      queryBool(r, "include_qb"),
		QBWeakMatch:    queryBool(r, "qb_weak_match"),
		ExcludePinned:  !includePinned,
	})
	if err != nil {
		if errors.Is(err, core.ErrInvalidMediaFilter) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if errors.Is(err, core.ErrQBRuntimeNotReady) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func mediaCheckboxQueryValues(r *http.Request) ([]core.MediaCheckboxFilter, error) {
	rawValues := r.URL.Query()["site_checkbox"]
	if len(rawValues) > 100 {
		return nil, fmt.Errorf("site_checkbox must contain at most 100 values")
	}
	result := make([]core.MediaCheckboxFilter, 0)
	indexes := map[string]int{}
	for _, raw := range rawValues {
		parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("site_checkbox values must use group:value")
		}
		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if name == "" || value == "" {
			return nil, fmt.Errorf("site_checkbox group and value must not be empty")
		}
		if len([]rune(name)) > 64 || len([]rune(value)) > 128 {
			return nil, fmt.Errorf("site_checkbox group or value is too long")
		}
		key := strings.ToLower(name)
		if index, exists := indexes[key]; exists {
			result[index].Values = append(result[index].Values, value)
			continue
		}
		indexes[key] = len(result)
		result = append(result, core.MediaCheckboxFilter{Name: name, Values: []string{value}})
	}
	return result, nil
}

func mediaFilterQueryValues(r *http.Request, name string) ([]string, error) {
	rawValues := r.URL.Query()[name]
	if len(rawValues) > 100 {
		return nil, fmt.Errorf("%s must contain at most 100 values", name)
	}
	result := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if len([]rune(value)) > 128 {
			return nil, fmt.Errorf("%s values must not exceed 128 characters", name)
		}
		result = append(result, value)
	}
	return result, nil
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

// handleTorrentPlayback 返回当前种子的实时媒体清单。
func (s *Server) handleTorrentPlayback(w http.ResponseWriter, r *http.Request) {
	result, err := s.playback.GetTorrentPlayback(
		r.Context(),
		chi.URLParam(r, "site_id"),
		chi.URLParam(r, "torrent_id"),
		r.URL.Query().Get("file"),
	)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleQBPlayback 返回可能没有数据库种子详情的 qB 播放上下文。
func (s *Server) handleQBPlayback(w http.ResponseWriter, r *http.Request) {
	result, err := s.playback.GetQBPlayback(r.Context(), chi.URLParam(r, "hash"), r.URL.Query().Get("file"))
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleFilePlayback 自动识别本机媒体文件的 qB 和数据库种子归属。
func (s *Server) handleFilePlayback(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("path")) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("media file path is required"))
		return
	}
	result, err := s.playback.GetFilePlayback(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handlePlaybackTorrents 返回按发布时间倒序的其他可播放种子。
func (s *Server) handlePlaybackTorrents(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if limit < 1 || limit > 50 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("limit must be between 1 and 50"))
		return
	}
	items, err := s.playback.ListPlaybackTorrents(
		r.Context(),
		r.URL.Query().Get("exclude_site_id"),
		r.URL.Query().Get("exclude_torrent_id"),
		limit,
	)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleTorrentMedia 使用 Range 友好的方式返回同机 qB 源文件。
func (s *Server) handleTorrentMedia(w http.ResponseWriter, r *http.Request) {
	fileIndex, ok := playbackFileIndex(w, r)
	if !ok {
		return
	}
	source, err := s.playback.OpenTorrentMedia(r.Context(), chi.URLParam(r, "site_id"), chi.URLParam(r, "torrent_id"), fileIndex)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	servePlaybackSource(w, r, source)
}

// handleQBMedia 按 qB hash 和文件索引传输源媒体。
func (s *Server) handleQBMedia(w http.ResponseWriter, r *http.Request) {
	fileIndex, ok := playbackFileIndex(w, r)
	if !ok {
		return
	}
	source, err := s.playback.OpenQBMedia(r.Context(), chi.URLParam(r, "hash"), fileIndex)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	servePlaybackSource(w, r, source)
}

// handleFileMedia 传输文件管理器选择的本机源媒体。
func (s *Server) handleFileMedia(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("path")) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("media file path is required"))
		return
	}
	source, err := s.playback.OpenFileMedia(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	servePlaybackSource(w, r, source)
}

// handleTorrentSubtitle 导出数据库种子 MKV 文件的内嵌文本字幕。
func (s *Server) handleTorrentSubtitle(w http.ResponseWriter, r *http.Request) {
	fileIndex, ok := playbackFileIndex(w, r)
	if !ok {
		return
	}
	trackID, ok := playbackSubtitleTrackID(w, r)
	if !ok {
		return
	}
	content, err := s.playback.GetTorrentSubtitle(
		r.Context(),
		chi.URLParam(r, "site_id"),
		chi.URLParam(r, "torrent_id"),
		fileIndex,
		trackID,
	)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	servePlaybackSubtitle(w, content)
}

// handleQBSubtitle 导出 qB 任务 MKV 文件的内嵌文本字幕。
func (s *Server) handleQBSubtitle(w http.ResponseWriter, r *http.Request) {
	fileIndex, ok := playbackFileIndex(w, r)
	if !ok {
		return
	}
	trackID, ok := playbackSubtitleTrackID(w, r)
	if !ok {
		return
	}
	content, err := s.playback.GetQBSubtitle(r.Context(), chi.URLParam(r, "hash"), fileIndex, trackID)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	servePlaybackSubtitle(w, content)
}

// handleFileSubtitle 导出文件管理器 MKV 文件的内嵌文本字幕。
func (s *Server) handleFileSubtitle(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("path")) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("media file path is required"))
		return
	}
	trackID, ok := playbackSubtitleTrackID(w, r)
	if !ok {
		return
	}
	content, err := s.playback.GetFileSubtitle(r.Context(), r.URL.Query().Get("path"), trackID)
	if err != nil {
		writePlaybackError(w, err)
		return
	}
	servePlaybackSubtitle(w, content)
}

// playbackFileIndex 解析播放源接口共用的 qB 文件索引。
func playbackFileIndex(w http.ResponseWriter, r *http.Request) (int, bool) {
	fileIndex, err := strconv.Atoi(chi.URLParam(r, "file_index"))
	if err != nil || fileIndex < 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid media file index"))
		return 0, false
	}
	return fileIndex, true
}

// playbackSubtitleTrackID 解析 MKV 字幕接口共用的轨道 ID。
func playbackSubtitleTrackID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	trackID, err := strconv.ParseUint(chi.URLParam(r, "track_id"), 10, 64)
	if err != nil || trackID == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid subtitle track id"))
		return 0, false
	}
	return trackID, true
}

// servePlaybackSubtitle 返回 Artplayer 可直接加载的 WebVTT 文本字幕。
func servePlaybackSubtitle(w http.ResponseWriter, content []byte) {
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="subtitle.vtt"`)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// servePlaybackSource 使用统一响应头和 Range 语义传输已经打开的媒体源。
func servePlaybackSource(w http.ResponseWriter, r *http.Request, source core.PlaybackSource) {
	defer source.File.Close()
	w.Header().Set("Content-Type", source.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": source.Name}))
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, source.Name, source.ModTime, source.File)
}

// writePlaybackError 将播放领域错误映射为稳定的 HTTP 状态码。
func writePlaybackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrPlaybackNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, core.ErrPlaybackNotAdded), errors.Is(err, core.ErrPlaybackNoData):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, core.ErrPlaybackUnavailable):
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
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

// handleDeleteQBTorrent 删除单个 qB 任务，并显式传递是否同时删除下载文件。
func (s *Server) handleDeleteQBTorrent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeleteFiles bool `json:"delete_files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.torrents.DeleteQBTorrent(r.Context(), chi.URLParam(r, "hash"), body.DeleteFiles)
	switch {
	case errors.Is(err, core.ErrInvalidQBTorrentHash):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, core.ErrQBTorrentNotFound):
		writeError(w, http.StatusNotFound, err)
	case err != nil:
		writeError(w, http.StatusBadGateway, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
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
