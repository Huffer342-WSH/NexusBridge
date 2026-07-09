package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultHost                              = "127.0.0.1"
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

type MediaLibraryConfig struct {
	Root   string `json:"root"`
	DryRun bool   `json:"dry_run"`
}

type RuleConfig struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	SiteIDs    []string `json:"site_ids"`
	Categories []string `json:"categories"`
	Tags       []string `json:"tags"`
	Include    string   `json:"include"`
	Exclude    string   `json:"exclude"`
	Promotion  string   `json:"promotion"`
	MinSize    int64    `json:"min_size"`
	MaxSize    int64    `json:"max_size"`
	MinSeeders int      `json:"min_seeders"`
	Action     string   `json:"action"`
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
		Rules: []RuleConfig{},
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
	seenRules := map[string]struct{}{}
	for _, rule := range cfg.Rules {
		if strings.TrimSpace(rule.ID) == "" {
			return errors.New("rule id is required")
		}
		if _, ok := seenRules[rule.ID]; ok {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		seenRules[rule.ID] = struct{}{}
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("rule %q name is required", rule.ID)
		}
		if rule.Action != "" && rule.Action != "download" {
			return fmt.Errorf("rule %q action must be download", rule.ID)
		}
		if rule.MinSize < 0 || rule.MaxSize < 0 || rule.MinSeeders < 0 {
			return fmt.Errorf("rule %q numeric filters must be non-negative", rule.ID)
		}
		if rule.MaxSize > 0 && rule.MinSize > rule.MaxSize {
			return fmt.Errorf("rule %q min_size must not exceed max_size", rule.ID)
		}
	}

	return nil
}

func (cfg Config) Address() string {
	return fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
}
