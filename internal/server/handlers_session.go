package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/core"
)

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

// handleGetSiteAttendance 返回站点自动签到配置和最近状态。
func (s *Server) handleGetSiteAttendance(w http.ResponseWriter, r *http.Request) {
	attendance, err := s.sites.GetSiteAttendance(r.Context(), chi.URLParam(r, "site_id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, attendance)
}

// handleSaveSiteAttendance 保存站点自动签到配置。
func (s *Server) handleSaveSiteAttendance(w http.ResponseWriter, r *http.Request) {
	var attendance core.SiteAttendance
	if err := json.NewDecoder(r.Body).Decode(&attendance); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	attendance.SiteID = chi.URLParam(r, "site_id")
	saved, err := s.sites.SaveSiteAttendance(r.Context(), attendance)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
