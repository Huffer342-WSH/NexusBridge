// Package core 提供 NexusBridge 的共享应用服务实现。
package core

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core/covercache"
	"nexusbridge/internal/core/videothumbnail"
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
	qbMu               sync.Mutex
	qbConfigMu         sync.RWMutex
	qbPollMu           sync.Mutex
	qbPollRID          int
	qbPollRevision     int
	qbPollLastAt       time.Time
	qbRuntimeMu        sync.RWMutex
	qbRuntime          map[storage.TorrentKey]storage.QBSnapshotRecord
	qbRuntimeReady     bool
	pinnedMu           sync.RWMutex
	pinnedBySite       map[string]map[storage.TorrentKey]int
	mihomoMu           sync.Mutex
	qbCached           *qbittorrent.Client
	qbCacheKey         string
	coverCache         *covercache.Service
	videoThumbnail     *videothumbnail.Service
	sites              map[string]runtimeSite
	siteIDs            []string
	automationOnce     sync.Once
	automationWake     chan struct{}
	automationMu       sync.Mutex
	automationCancel   context.CancelFunc
	automationWG       sync.WaitGroup
	maintenanceWG      sync.WaitGroup
	siteLockMu         sync.Mutex
	siteLocks          map[string]*contextMutex
	subscriptionLockMu sync.Mutex
	subscriptionLocks  map[string]*contextMutex
	hashLockMu         sync.Mutex
	hashLocks          map[string]*contextMutex
	seriesLockMu       sync.Mutex
	seriesLocks        map[string]*contextMutex
	fetchMu            sync.Mutex
	activeFetches      map[string]*activeSiteFetch
	fetchWG            sync.WaitGroup
}

type runtimeSite struct {
	ID            string
	Name          string
	BaseURL       string
	UserAgent     string
	AttendanceURL string
	Definition    parser.SiteDefinition
}

// NewApp 创建核心应用服务。
func NewApp(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := storage.OpenSQLiteWithOptions(ctx, cfg.Storage.Path, storage.SQLiteOptions{
		BusyTimeoutMillis: cfg.Storage.SQLite.BusyTimeoutMillis,
		MaxOpenConns:      cfg.Storage.SQLite.MaxOpenConns,
		CacheKiB:          cfg.Storage.SQLite.CacheKiB,
		MmapBytes:         cfg.Storage.SQLite.MmapBytes,
		Synchronous:       cfg.Storage.SQLite.Synchronous,
	})
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
		cfg: cfg, store: store, sites: sites, siteIDs: siteIDs,
		ctx: appCtx, cancel: cancel, qbRuntime: map[storage.TorrentKey]storage.QBSnapshotRecord{},
		pinnedBySite:   map[string]map[storage.TorrentKey]int{},
		automationWake: make(chan struct{}, 1), siteLocks: map[string]*contextMutex{}, subscriptionLocks: map[string]*contextMutex{},
		hashLocks: map[string]*contextMutex{}, seriesLocks: map[string]*contextMutex{},
		activeFetches: map[string]*activeSiteFetch{},
	}
	coverCache, err := covercache.New(store, coverDownloader{app: app}, filepath.Join(filepath.Dir(cfg.Storage.Path), "covers"))
	if err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	app.coverCache = coverCache
	videoThumbnail, err := videothumbnail.New(
		filepath.Join(filepath.Dir(cfg.Storage.Path), "thumbnails"),
		cfg.VideoThumbnail.FFmpegPath,
	)
	if err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	app.videoThumbnail = videoThumbnail
	if err := app.initializeQBConfig(ctx); err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
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
	app.startStorageMaintenance()
	return app, nil
}

func (a *App) startStorageMaintenance() {
	a.maintenanceWG.Add(1)
	go func() {
		defer a.maintenanceWG.Done()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				_ = a.store.Optimize(a.ctx)
			}
		}
	}()
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
	a.maintenanceWG.Wait()
	return a.store.Close()
}
