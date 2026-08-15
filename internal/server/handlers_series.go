package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/core"
)

// handleSeries 返回全部剧集缓存摘要，不触发扫描。
func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	items, err := s.series.ListSeries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleCreateSeries 创建并扫描一个本地剧集。
func (s *Server) handleCreateSeries(w http.ResponseWriter, r *http.Request) {
	var request core.SeriesSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.series.SaveSeries(r.Context(), "", request)
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// handleGetSeries 返回一个剧集的缓存详情，不触发扫描。
func (s *Server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	result, err := s.series.GetSeries(r.Context(), chi.URLParam(r, "series_id"))
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleUpdateSeries 更新剧集名称、目录并立即扫描。
func (s *Server) handleUpdateSeries(w http.ResponseWriter, r *http.Request) {
	var request core.SeriesSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.series.SaveSeries(r.Context(), chi.URLParam(r, "series_id"), request)
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleDeleteSeries 删除剧集配置和缓存，不删除任何源文件。
func (s *Server) handleDeleteSeries(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.series.DeleteSeries(r.Context(), chi.URLParam(r, "series_id"))
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, core.ErrSeriesNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// handleScanSeries 手动重扫一个剧集。
func (s *Server) handleScanSeries(w http.ResponseWriter, r *http.Request) {
	result, err := s.series.ScanSeries(r.Context(), chi.URLParam(r, "series_id"))
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleSelectSeriesVideo 校验并记录剧集最后选择的视频。
func (s *Server) handleSelectSeriesVideo(w http.ResponseWriter, r *http.Request) {
	var request core.SeriesSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.series.SelectSeriesVideo(
		r.Context(), chi.URLParam(r, "series_id"), request,
	)
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleSeriesPlayback 重扫剧集并返回带剧集清单的统一播放上下文。
func (s *Server) handleSeriesPlayback(w http.ResponseWriter, r *http.Request) {
	result, err := s.series.GetSeriesPlayback(
		r.Context(), chi.URLParam(r, "series_id"), r.URL.Query().Get("path"),
	)
	if err != nil {
		writeSeriesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeSeriesError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrSeriesNotFound), errors.Is(err, core.ErrPlaybackNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, core.ErrSeriesNoPlayableVideo),
		errors.Is(err, core.ErrMediaLibraryHasChildren),
		errors.Is(err, core.ErrPlaybackNotAdded),
		errors.Is(err, core.ErrPlaybackNoData):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, core.ErrSeriesInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, core.ErrPlaybackUnavailable):
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		if errors.Is(err, context.Canceled) {
			writeError(w, http.StatusRequestTimeout, err)
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Errorf("series request failed: %w", err))
	}
}
