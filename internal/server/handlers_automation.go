package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/core"
)

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
