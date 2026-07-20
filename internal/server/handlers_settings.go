package server

import (
	"encoding/json"
	"net/http"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/mihomo"
)

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

// handleGetFetchSettings 返回全局站点抓取设置。
func (s *Server) handleGetFetchSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.fetcher.GetFetchSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleSaveFetchSettings 保存全局站点抓取设置。
func (s *Server) handleSaveFetchSettings(w http.ResponseWriter, r *http.Request) {
	var settings core.FetchSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.fetcher.SaveFetchSettings(r.Context(), settings)
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
