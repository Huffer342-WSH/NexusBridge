package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/core"
)

// handleMediaLibraries 返回全部媒体库节点，不触发目录扫描。
func (s *Server) handleMediaLibraries(w http.ResponseWriter, r *http.Request) {
	items, err := s.mediaLibraries.ListMediaLibraries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleCreateMediaLibrary 创建媒体库；剧集型节点保存后立即扫描。
func (s *Server) handleCreateMediaLibrary(w http.ResponseWriter, r *http.Request) {
	var request core.MediaLibrarySaveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.mediaLibraries.SaveMediaLibrary(r.Context(), "", request)
	if err != nil {
		writeMediaLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// handleGetMediaLibrary 返回一个媒体库节点的缓存详情。
func (s *Server) handleGetMediaLibrary(w http.ResponseWriter, r *http.Request) {
	result, err := s.mediaLibraries.GetMediaLibrary(r.Context(), chi.URLParam(r, "library_id"))
	if err != nil {
		writeMediaLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleUpdateMediaLibrary 更新媒体库层级、目录和本地设置覆盖。
func (s *Server) handleUpdateMediaLibrary(w http.ResponseWriter, r *http.Request) {
	var request core.MediaLibrarySaveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.mediaLibraries.SaveMediaLibrary(r.Context(), chi.URLParam(r, "library_id"), request)
	if err != nil {
		writeMediaLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleDeleteMediaLibrary 只删除叶子媒体库的配置和扫描缓存。
func (s *Server) handleDeleteMediaLibrary(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.mediaLibraries.DeleteMediaLibrary(r.Context(), chi.URLParam(r, "library_id"))
	if err != nil {
		writeMediaLibraryError(w, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, core.ErrMediaLibraryNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// handleScanMediaLibrary 重扫剧集型节点，集合节点只返回当前状态。
func (s *Server) handleScanMediaLibrary(w http.ResponseWriter, r *http.Request) {
	result, err := s.mediaLibraries.ScanMediaLibrary(r.Context(), chi.URLParam(r, "library_id"))
	if err != nil {
		writeMediaLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeMediaLibraryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrMediaLibraryNotFound), errors.Is(err, core.ErrSeriesNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, core.ErrMediaLibraryInvalid), errors.Is(err, core.ErrSeriesInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, core.ErrMediaLibraryHasChildren):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, core.ErrSeriesNoPlayableVideo):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, fmt.Errorf("media library request failed: %w", err))
	}
}
