// Package core 提供 NexusBridge 的共享应用服务实现。
package core

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core/covercache"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

// App 组合持久化、站点抓取、订阅执行和外部服务适配。
type App struct {
	cfg                config.Config
	store              *storage.SQLiteStore
	ctx                context.Context
	cancel             context.CancelFunc
	mu                 sync.RWMutex
	qbMu               sync.Mutex
	mihomoMu           sync.Mutex
	qbCached           *qbittorrent.Client
	qbCacheKey         string
	cache              map[string]Torrent
	coverCache         *covercache.Service
	sites              map[string]runtimeSite
	siteIDs            []string
	automationOnce     sync.Once
	automationWake     chan struct{}
	automationMu       sync.Mutex
	automationCancel   context.CancelFunc
	automationWG       sync.WaitGroup
	siteLockMu         sync.Mutex
	siteLocks          map[string]*contextMutex
	subscriptionLockMu sync.Mutex
	subscriptionLocks  map[string]*contextMutex
	hashLockMu         sync.Mutex
	hashLocks          map[string]*contextMutex
	fetchMu            sync.Mutex
	activeFetches      map[string]*activeSiteFetch
	fetchWG            sync.WaitGroup
}

type runtimeSite struct {
	ID         string
	Name       string
	BaseURL    string
	UserAgent  string
	Definition parser.SiteDefinition
}

// NewApp 创建核心应用服务。
func NewApp(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := storage.OpenSQLite(ctx, cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	sites, siteIDs, err := loadRuntimeSites(cfg)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	appCtx, cancel := context.WithCancel(ctx)
	app := &App{
		cfg: cfg, store: store, cache: map[string]Torrent{}, sites: sites, siteIDs: siteIDs,
		ctx: appCtx, cancel: cancel,
		automationWake: make(chan struct{}, 1), siteLocks: map[string]*contextMutex{}, subscriptionLocks: map[string]*contextMutex{},
		hashLocks: map[string]*contextMutex{}, activeFetches: map[string]*activeSiteFetch{},
	}
	coverCache, err := covercache.New(store, coverDownloader{app: app}, filepath.Join(filepath.Dir(cfg.Storage.Path), "covers"))
	if err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	app.coverCache = coverCache
	if err := app.applyNetworkConfig(ctx); err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	if err := store.RecoverInterruptedSubscriptionWork(ctx); err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	if err := store.RecoverInterruptedSiteFetchJobs(ctx); err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	existingRules, err := store.ListRules(ctx)
	if err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	existingRuleNames := make(map[string]struct{}, len(existingRules))
	for _, rule := range existingRules {
		existingRuleNames[strings.ToLower(strings.TrimSpace(rule.Name))] = struct{}{}
	}
	for _, rule := range cfg.Rules {
		if _, exists := existingRuleNames[strings.ToLower(strings.TrimSpace(rule.Name))]; exists {
			continue
		}
		if err := store.SaveRule(ctx, ruleToRecord(ruleFromConfig(rule))); err != nil {
			cancel()
			_ = store.Close()
			return nil, err
		}
	}
	if err := app.loadCache(ctx); err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	return app, nil
}

// Close 关闭应用持有的资源。
func (a *App) Close() error {
	a.cancel()
	a.automationMu.Lock()
	cancel := a.automationCancel
	a.automationMu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.automationWG.Wait()
	a.fetchWG.Wait()
	return a.store.Close()
}
