package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"nexusbridge/internal/network"
)

const (
	DefaultHost                              = "0.0.0.0"
	DefaultPort                              = 8090
	DefaultStoragePath                       = "nexusbridge.db"
	defaultQBSyncIntervalSeconds             = 3
	minQBSyncIntervalSeconds                 = 2
	maxQBSyncIntervalSeconds                 = 60
	defaultQBInactiveSyncIntervalSeconds     = 30
	minQBInactiveSyncIntervalSeconds         = 10
	maxQBInactiveSyncIntervalSeconds         = 300
	defaultQBDisconnectedSyncIntervalSeconds = 60
	minQBDisconnectedSyncIntervalSeconds     = 15
	maxQBDisconnectedSyncIntervalSeconds     = 600
)

type Config struct {
	Server       ServerConfig       `json:"server"`
	Storage      StorageConfig      `json:"storage"`
	SitesDir     string             `json:"sites_dir"`
	Logging      LoggingConfig      `json:"logging"`
	Auth         AuthConfig         `json:"auth"`
	QBittorrent  QBittorrentConfig  `json:"qbittorrent"`
	LLM          LLMConfig          `json:"llm"`
	Network      NetworkConfig      `json:"network"`
	MediaLibrary MediaLibraryConfig `json:"media_library"`
	Rules        []RuleConfig       `json:"rules"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type StorageConfig struct {
	Path string `json:"path"`
}

type LoggingConfig struct {
	Level             string `json:"level"`
	File              string `json:"file"`
	LogSensitiveFetch bool   `json:"log_sensitive_fetch"`
}

type AuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type QBittorrentConfig struct {
	AuthMode                        string   `json:"auth_mode"`
	URL                             string   `json:"url"`
	APIKey                          string   `json:"api_key"`
	Username                        string   `json:"username"`
	UserID                          string   `json:"user_id"`
	Password                        string   `json:"password"`
	Category                        string   `json:"category"`
	Tags                            []string `json:"tags"`
	AutoSync                        bool     `json:"auto_sync"`
	SyncIntervalSeconds             int      `json:"sync_interval_seconds"`
	InactiveSyncIntervalSeconds     int      `json:"inactive_sync_interval_seconds"`
	DisconnectedSyncIntervalSeconds int      `json:"disconnected_sync_interval_seconds"`
}

type LLMConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// NetworkConfig 描述全部出站 HTTP 请求使用的代理方式。
type NetworkConfig struct {
	Mode     string `json:"mode"`
	ProxyURL string `json:"proxy_url"`
	NoProxy  string `json:"no_proxy"`
}

type MediaLibraryConfig struct {
	Root   string `json:"root"`
	DryRun bool   `json:"dry_run"`
}

type RuleConfig struct {
	Name                   string   `json:"name"`
	SiteIDs                []string `json:"site_ids"`
	SiteCategories         []string `json:"site_categories"`
	SiteTags               []string `json:"site_tags"`
	SubtitleTags           []string `json:"subtitle_tags"`
	TitleExpression        string   `json:"title_expression"`
	Promotions             []string `json:"promotions"`
	MinSize                int64    `json:"min_size"`
	MaxSize                int64    `json:"max_size"`
	MinSeeders             int      `json:"min_seeders"`
	MaxSeeders             int      `json:"max_seeders"`
	MinLeechers            int      `json:"min_leechers"`
	MaxLeechers            int      `json:"max_leechers"`
	MinSnatches            int      `json:"min_snatches"`
	MaxSnatches            int      `json:"max_snatches"`
	PublishedWithinMinutes int      `json:"published_within_minutes"`
	SortBy                 string   `json:"sort_by"`
	SortDirection          string   `json:"sort_direction"`
	Action                 string   `json:"action"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host: DefaultHost,
			Port: DefaultPort,
		},
		Storage: StorageConfig{
			Path: DefaultStoragePath,
		},
		SitesDir: filepath.ToSlash(filepath.Join("sites", "html")),
		Logging: LoggingConfig{
			Level: "info",
		},
		QBittorrent: QBittorrentConfig{
			AuthMode:                        "uid",
			Tags:                            []string{},
			AutoSync:                        true,
			SyncIntervalSeconds:             defaultQBSyncIntervalSeconds,
			InactiveSyncIntervalSeconds:     defaultQBInactiveSyncIntervalSeconds,
			DisconnectedSyncIntervalSeconds: defaultQBDisconnectedSyncIntervalSeconds,
		},
		Network: NetworkConfig{Mode: network.ProxyModeSystem, NoProxy: network.DefaultNoProxy},
		Rules:   []RuleConfig{},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) == "" {
		return cfg, cfg.Validate()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = DefaultHost
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = DefaultPort
	}
	if cfg.Storage.Path == "" {
		cfg.Storage.Path = DefaultStoragePath
	}
	if strings.TrimSpace(cfg.SitesDir) == "" {
		cfg.SitesDir = filepath.ToSlash(filepath.Join("sites", "html"))
	}
	if strings.TrimSpace(cfg.Logging.Level) == "" {
		cfg.Logging.Level = "info"
	}
	cfg.Network.Mode = network.NormalizeProxyMode(cfg.Network.Mode)
	if strings.TrimSpace(cfg.Network.NoProxy) == "" {
		cfg.Network.NoProxy = network.DefaultNoProxy
	} else {
		cfg.Network.NoProxy = network.NormalizeNoProxyLines(cfg.Network.NoProxy)
	}
	if cfg.QBittorrent.Tags == nil {
		cfg.QBittorrent.Tags = []string{}
	}
	if strings.TrimSpace(cfg.QBittorrent.AuthMode) == "" {
		cfg.QBittorrent.AuthMode = "uid"
	}
	if cfg.QBittorrent.Username == "" && cfg.QBittorrent.UserID != "" {
		cfg.QBittorrent.Username = cfg.QBittorrent.UserID
	}
	if cfg.QBittorrent.UserID == "" && cfg.QBittorrent.Username != "" {
		cfg.QBittorrent.UserID = cfg.QBittorrent.Username
	}
	cfg.QBittorrent = NormalizeQBittorrentConfig(cfg.QBittorrent)
	if cfg.Rules == nil {
		cfg.Rules = []RuleConfig{}
	}
	for i := range cfg.Rules {
		if cfg.Rules[i].Action == "" {
			cfg.Rules[i].Action = "download"
		}
		if cfg.Rules[i].SortBy == "" {
			cfg.Rules[i].SortBy = "source_order"
		}
		if cfg.Rules[i].SortDirection == "" {
			cfg.Rules[i].SortDirection = "asc"
		}
	}
}

// NormalizeQBittorrentConfig 补齐并约束 qB 前端增量同步设置。
func NormalizeQBittorrentConfig(cfg QBittorrentConfig) QBittorrentConfig {
	legacy := cfg.SyncIntervalSeconds <= 0 && cfg.InactiveSyncIntervalSeconds <= 0 && cfg.DisconnectedSyncIntervalSeconds <= 0
	if legacy {
		cfg.AutoSync = true
	}
	if cfg.SyncIntervalSeconds <= 0 {
		cfg.SyncIntervalSeconds = defaultQBSyncIntervalSeconds
	}
	if cfg.InactiveSyncIntervalSeconds <= 0 {
		cfg.InactiveSyncIntervalSeconds = defaultQBInactiveSyncIntervalSeconds
	}
	if cfg.DisconnectedSyncIntervalSeconds <= 0 {
		cfg.DisconnectedSyncIntervalSeconds = defaultQBDisconnectedSyncIntervalSeconds
	}
	cfg.SyncIntervalSeconds = min(max(cfg.SyncIntervalSeconds, minQBSyncIntervalSeconds), maxQBSyncIntervalSeconds)
	cfg.InactiveSyncIntervalSeconds = min(max(cfg.InactiveSyncIntervalSeconds, minQBInactiveSyncIntervalSeconds), maxQBInactiveSyncIntervalSeconds)
	cfg.DisconnectedSyncIntervalSeconds = min(max(cfg.DisconnectedSyncIntervalSeconds, minQBDisconnectedSyncIntervalSeconds), maxQBDisconnectedSyncIntervalSeconds)
	return cfg
}

func (cfg Config) Validate() error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Server.Host) == "" {
		return errors.New("server.host is required")
	}
	if strings.TrimSpace(cfg.Storage.Path) == "" {
		return errors.New("storage.path is required")
	}
	if cfg.Auth.Enabled {
		if strings.TrimSpace(cfg.Auth.Username) == "" {
			return errors.New("auth.username is required when auth.enabled is true")
		}
		if strings.TrimSpace(cfg.Auth.Password) == "" {
			return errors.New("auth.password is required when auth.enabled is true")
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Logging.Level)) {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("logging.level must be debug, info, warn, or error")
	}

	if cfg.QBittorrent.URL != "" {
		if _, err := url.ParseRequestURI(cfg.QBittorrent.URL); err != nil {
			return fmt.Errorf("qbittorrent.url is invalid: %w", err)
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.QBittorrent.AuthMode)) {
	case "", "uid", "api_key":
	default:
		return fmt.Errorf("qbittorrent.auth_mode must be uid or api_key")
	}
	if cfg.LLM.BaseURL != "" {
		if _, err := url.ParseRequestURI(cfg.LLM.BaseURL); err != nil {
			return fmt.Errorf("llm.base_url is invalid: %w", err)
		}
	}
	if err := network.ValidateProxyConfig(cfg.Network.Mode, cfg.Network.ProxyURL); err != nil {
		return err
	}
	seenRules := map[string]struct{}{}
	for _, rule := range cfg.Rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return errors.New("rule name is required")
		}
		key := strings.ToLower(name)
		if _, ok := seenRules[key]; ok {
			return fmt.Errorf("duplicate rule name %q", name)
		}
		seenRules[key] = struct{}{}
		if len(rule.SiteCategories)+len(rule.SiteTags)+len(rule.SubtitleTags)+len(rule.Promotions) > 0 && len(rule.SiteIDs) != 1 {
			return fmt.Errorf("rule %q site-specific filters require exactly one site_id", name)
		}
		if rule.Action != "" && rule.Action != "download" {
			return fmt.Errorf("rule %q action must be download", name)
		}
		switch rule.SortBy {
		case "", "source_order", "published_at", "size_bytes", "seeders", "leechers", "snatches":
		default:
			return fmt.Errorf("rule %q sort_by is unsupported", name)
		}
		if rule.SortDirection != "" && rule.SortDirection != "asc" && rule.SortDirection != "desc" {
			return fmt.Errorf("rule %q sort_direction must be asc or desc", name)
		}
		if rule.MinSize < 0 || rule.MaxSize < 0 || rule.MinSeeders < 0 || rule.MaxSeeders < 0 ||
			rule.MinLeechers < 0 || rule.MaxLeechers < 0 || rule.MinSnatches < 0 || rule.MaxSnatches < 0 ||
			rule.PublishedWithinMinutes < 0 {
			return fmt.Errorf("rule %q numeric filters must be non-negative", name)
		}
		if rule.MaxSize > 0 && rule.MinSize > rule.MaxSize {
			return fmt.Errorf("rule %q min_size must not exceed max_size", name)
		}
		if rule.MaxSeeders > 0 && rule.MinSeeders > rule.MaxSeeders {
			return fmt.Errorf("rule %q min_seeders must not exceed max_seeders", name)
		}
		if rule.MaxLeechers > 0 && rule.MinLeechers > rule.MaxLeechers {
			return fmt.Errorf("rule %q min_leechers must not exceed max_leechers", name)
		}
		if rule.MaxSnatches > 0 && rule.MinSnatches > rule.MaxSnatches {
			return fmt.Errorf("rule %q min_snatches must not exceed max_snatches", name)
		}
	}

	return nil
}

func (cfg Config) Address() string {
	return fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
}
