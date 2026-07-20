package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

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
