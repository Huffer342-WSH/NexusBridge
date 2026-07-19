// Package core 提供 NexusBridge 的共享应用服务实现。
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core/covercache"
	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/llm"
	"nexusbridge/internal/organizer"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

type App struct {
	cfg                config.Config
	store              *storage.SQLiteStore
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
	app := &App{
		cfg: cfg, store: store, cache: map[string]Torrent{}, sites: sites, siteIDs: siteIDs,
		automationWake: make(chan struct{}, 1), siteLocks: map[string]*contextMutex{}, subscriptionLocks: map[string]*contextMutex{},
		hashLocks: map[string]*contextMutex{},
	}
	coverCache, err := covercache.New(store, coverDownloader{app: app}, filepath.Join(filepath.Dir(cfg.Storage.Path), "covers"))
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	app.coverCache = coverCache
	if err := app.applyNetworkConfig(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.RecoverInterruptedSubscriptionWork(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	existingRules, err := store.ListRules(ctx)
	if err != nil {
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
			_ = store.Close()
			return nil, err
		}
	}
	if err := app.loadCache(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return app, nil
}

// Close 关闭应用持有的资源。
func (a *App) Close() error {
	a.automationMu.Lock()
	cancel := a.automationCancel
	a.automationMu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.automationWG.Wait()
	return a.store.Close()
}

// ListSites 返回已加载的站点列表。
func (a *App) ListSites(ctx context.Context) ([]Site, error) {
	sites := make([]Site, 0, len(a.siteIDs))
	for _, siteID := range a.siteIDs {
		site := a.sites[siteID]
		credential, ok, _ := a.store.LoadSiteCredential(ctx, site.ID)
		userAgent := firstNonEmpty(credential.UserAgent, site.UserAgent)
		hasCookie := ok && credential.HasCookie
		if !hasCookie {
			hasCookie = a.store.HasCookies(ctx, site.BaseURL)
		}
		sites = append(sites, Site{
			ID:        site.ID,
			Name:      site.Name,
			BaseURL:   site.BaseURL,
			UserAgent: userAgent,
			HasCookie: hasCookie,
		})
	}
	return sites, nil
}

// GetSiteCredential 读取站点凭据状态。
func (a *App) GetSiteCredential(ctx context.Context, siteID string) (SiteCredential, error) {
	site, err := a.findSite(siteID)
	if err != nil {
		return SiteCredential{}, err
	}
	credential, ok, err := a.store.LoadSiteCredential(ctx, site.ID)
	if err != nil {
		return SiteCredential{}, err
	}
	userAgent := site.UserAgent
	if ok && strings.TrimSpace(credential.UserAgent) != "" {
		userAgent = credential.UserAgent
	}
	return SiteCredential{
		SiteID:    site.ID,
		BaseURL:   site.BaseURL,
		UserAgent: userAgent,
		HasCookie: a.store.HasCookies(ctx, site.BaseURL),
	}, nil
}

// SaveSiteCredential 保存站点 cookie 和 user-agent。
func (a *App) SaveSiteCredential(ctx context.Context, credential SiteCredential) (SiteCredential, error) {
	site, err := a.findSite(credential.SiteID)
	if err != nil {
		return SiteCredential{}, err
	}
	if strings.TrimSpace(credential.Cookie) != "" {
		cookies, err := fetcher.LoadCookiesFromHeader(credential.Cookie)
		if err != nil {
			return SiteCredential{}, err
		}
		if err := a.store.SaveCookies(ctx, site.BaseURL, cookies); err != nil {
			return SiteCredential{}, err
		}
	}
	record := storage.SiteCredentialRecord{
		SiteID:    site.ID,
		BaseURL:   site.BaseURL,
		UserAgent: strings.TrimSpace(credential.UserAgent),
	}
	if record.UserAgent == "" {
		record.UserAgent = site.UserAgent
	}
	if err := a.store.SaveSiteCredential(ctx, record); err != nil {
		return SiteCredential{}, err
	}
	return a.GetSiteCredential(ctx, site.ID)
}

// ListTorrents 查询本地种子缓存。
func (a *App) ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error) {
	records, err := a.store.ListTorrents(ctx, storage.TorrentListQuery{
		SiteID: query.SiteID, Search: query.Search, SortBy: query.SortBy, SortDirection: query.SortDirection,
		Limit: query.Limit, Offset: query.Offset,
	})
	if err != nil {
		return nil, err
	}
	torrents := make([]Torrent, 0, len(records))
	keys := make([]storage.TorrentKey, 0, len(records))
	for _, record := range records {
		torrents = append(torrents, torrentFromRecord(record))
		keys = append(keys, storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID})
	}
	files, err := a.store.ListTorrentFileMetadata(ctx, keys)
	if err != nil {
		return nil, err
	}
	snapshots, err := a.store.ListQBSnapshots(ctx, keys)
	if err != nil {
		return nil, err
	}
	for i := range torrents {
		key := storage.TorrentKey{SiteID: torrents[i].SiteID, TorrentID: torrents[i].ID}
		if file, ok := files[key]; ok {
			torrents[i].TorrentFileSaved = file.HasData
			torrents[i].InfoHashV1 = file.InfoHashV1
			torrents[i].InfoHashV2 = file.InfoHashV2
			torrents[i].TorrentFileError = file.LastError
		}
		if snapshot, ok := snapshots[key]; ok {
			status := qbStatusFromSnapshot(snapshot)
			torrents[i].QBStatus = &status
		}
	}
	if query.IncludeQB {
		statuses, err := a.qbStatusesForTorrents(ctx, torrents, query.QBWeakMatch)
		if err != nil {
			return nil, err
		}
		for i := range torrents {
			if status, ok := statuses[torrentKey(torrents[i])]; ok {
				torrents[i].QBStatus = status
			}
		}
	}
	return torrents, nil
}

// FetchSite 抓取并保存指定站点种子。
func (a *App) FetchSite(ctx context.Context, siteID string) (FetchResult, error) {
	lock := a.siteMutex(siteID)
	if err := lock.Lock(ctx); err != nil {
		return FetchResult{SiteID: siteID, Status: "failed"}, err
	}
	defer lock.Unlock()
	result, _, err := a.refreshSite(ctx, siteID)
	return result, err
}

// RunOnce 抓取指定站点并执行其全部已启用订阅。
func (a *App) RunOnce(ctx context.Context, siteID string) (FetchResult, error) {
	return a.runSiteSubscriptions(ctx, siteID, "run-once")
}

// ListRules 返回本地筛选规则。
func (a *App) ListRules(ctx context.Context) ([]Rule, error) {
	records, err := a.store.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(records))
	for _, record := range records {
		rules = append(rules, ruleFromRecord(record))
	}
	return rules, nil
}

// SaveRule 保存本地筛选规则。
func (a *App) SaveRule(ctx context.Context, rule Rule) (Rule, error) {
	rule = NormalizeRule(rule)
	if err := ValidateRule(rule, true); err != nil {
		return Rule{}, err
	}
	if err := a.store.SaveRule(ctx, ruleToRecord(rule)); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// RenameRule 更新规则并在名称变化时级联修改全部本地引用。
func (a *App) RenameRule(ctx context.Context, oldName string, rule Rule) (Rule, error) {
	oldName = strings.TrimSpace(oldName)
	rule = NormalizeRule(rule)
	if oldName == "" {
		return Rule{}, fmt.Errorf("original rule name is required")
	}
	if err := ValidateRule(rule, true); err != nil {
		return Rule{}, err
	}
	if err := a.store.RenameRule(ctx, oldName, ruleToRecord(rule)); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// PreviewRule 只读预览草稿筛选规则对数据库种子的匹配结果。
func (a *App) PreviewRule(ctx context.Context, request RulePreviewRequest) (RulePreviewResult, error) {
	rule := NormalizeRule(request.Rule)
	if err := ValidateRule(rule, false); err != nil {
		return RulePreviewResult{}, err
	}
	limit := request.Limit
	if limit <= 0 || limit > storage.MaxTorrentListLimit {
		limit = 100
	}
	queryLimit := limit
	if limit < storage.MaxTorrentListLimit {
		queryLimit++
	}
	torrents, err := a.ListTorrents(ctx, TorrentQuery{
		SiteID: request.SiteID, SortBy: rule.SortBy, SortDirection: rule.SortDirection,
		Limit: queryLimit, Offset: request.Offset,
	})
	if err != nil {
		return RulePreviewResult{}, err
	}
	truncated := len(torrents) > limit
	if truncated {
		torrents = torrents[:limit]
	}
	SortTorrentsForRule(torrents, rule)
	keys := make([]storage.TorrentKey, 0, len(torrents))
	for _, torrent := range torrents {
		keys = append(keys, storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID})
	}
	tasksByTorrent, err := a.store.ListDownloadTasksByTorrents(ctx, keys)
	if err != nil {
		return RulePreviewResult{}, err
	}
	result := RulePreviewResult{Evaluated: len(torrents), Items: make([]RulePreviewItem, 0, len(torrents)), Truncated: truncated}
	for _, torrent := range torrents {
		match := EvaluateRule(rule, torrent, time.Now())
		if match.Matched {
			result.Matched++
		}
		result.Items = append(result.Items, RulePreviewItem{Torrent: torrent, Matched: match.Matched, Reasons: match.Reasons, QBState: previewQBState(torrent, tasksByTorrent[torrent.SiteID+":"+torrent.ID])})
	}
	sort.SliceStable(result.Items, func(i, j int) bool { return result.Items[i].Matched && !result.Items[j].Matched })
	return result, nil
}

func previewQBState(torrent Torrent, tasks []storage.DownloadTaskRecord) string {
	if torrent.QBStatus == nil {
		for _, task := range tasks {
			if task.Status == "sent" || task.Status == "exists" {
				return "added"
			}
		}
		return "unknown"
	}
	if torrent.QBStatus.Added || torrent.QBStatus.TaskStatus == "sent" || torrent.QBStatus.TaskStatus == "exists" {
		return "added"
	}
	for _, task := range tasks {
		if task.Status == "sent" || task.Status == "exists" {
			return "added"
		}
	}
	if torrent.QBStatus.Available {
		return "not_added"
	}
	return "unknown"
}

// ListDownloadTasks 返回下载任务。
func (a *App) ListDownloadTasks(ctx context.Context) ([]DownloadTask, error) {
	records, err := a.store.ListDownloadTasks(ctx)
	if err != nil {
		return nil, err
	}
	tasks := make([]DownloadTask, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, downloadTaskFromRecord(record))
	}
	return tasks, nil
}

// ListOrganizeTasks 返回整理任务。
func (a *App) ListOrganizeTasks(ctx context.Context) ([]OrganizeTask, error) {
	records, err := a.store.ListOrganizeTasks(ctx, false)
	if err != nil {
		return nil, err
	}
	tasks := make([]OrganizeTask, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, organizeTaskFromRecord(record))
	}
	return tasks, nil
}

// OrganizePending 处理待整理任务。
func (a *App) OrganizePending(ctx context.Context) (OrganizeResult, error) {
	records, err := a.store.ListOrganizeTasks(ctx, true)
	if err != nil {
		return OrganizeResult{}, err
	}
	llmCfg := a.effectiveLLMConfig(ctx)
	org := organizer.New(llm.New(llm.Config{
		BaseURL: llmCfg.BaseURL,
		APIKey:  llmCfg.APIKey,
		Model:   llmCfg.Model,
	}), organizer.Options{
		LibraryRoot: a.cfg.MediaLibrary.Root,
		DryRun:      a.cfg.MediaLibrary.DryRun,
	})
	result := OrganizeResult{DryRun: a.cfg.MediaLibrary.DryRun}
	for _, record := range records {
		out, err := org.Organize(ctx, record.Title, record.SourcePath)
		if err != nil {
			result.Failed++
			_ = a.store.UpdateOrganizeTask(ctx, record.ID, "failed", "", "", "", "", err.Error(), 0)
			continue
		}
		status := "linked"
		if a.cfg.MediaLibrary.DryRun {
			status = "dry_run"
		}
		if err := a.store.UpdateOrganizeTask(ctx, record.ID, status, out.RelativeDir, out.Filename, out.TargetPath, out.LLMResponse, "", out.Confidence); err != nil {
			return result, err
		}
		result.Processed++
	}
	return result, nil
}

// GetLLMConfig 读取 LLM 配置。
func (a *App) GetLLMConfig(ctx context.Context) config.LLMConfig {
	return a.effectiveLLMConfig(ctx)
}

// SaveLLMConfig 保存 LLM 配置。
func (a *App) SaveLLMConfig(ctx context.Context, cfg config.LLMConfig) (config.LLMConfig, error) {
	if err := a.store.SaveSetting(ctx, storage.LLMSettingKey, cfg); err != nil {
		return config.LLMConfig{}, err
	}
	return cfg, nil
}

// PreviewTorrentDownload 生成手动下载前的 LLM 标题预览。
func (a *App) PreviewTorrentDownload(ctx context.Context, request ManualDownloadRequest) (DownloadPreview, error) {
	torrent, err := a.getTorrent(ctx, request.SiteID, request.TorrentID)
	if err != nil {
		return DownloadPreview{}, err
	}
	if strings.TrimSpace(torrent.DownloadURL) == "" {
		return DownloadPreview{}, fmt.Errorf("torrent download url is empty")
	}
	llmCfg := a.effectiveLLMConfig(ctx)
	formatted, raw, err := llm.New(llm.Config{
		BaseURL: llmCfg.BaseURL,
		APIKey:  llmCfg.APIKey,
		Model:   llmCfg.Model,
	}).FormatTorrentTitle(ctx, torrent.Title)
	if err != nil {
		return DownloadPreview{}, err
	}
	return DownloadPreview{
		SiteID:         torrent.SiteID,
		TorrentID:      torrent.ID,
		OriginalTitle:  torrent.Title,
		FormattedTitle: formatted,
		DownloadURL:    torrent.DownloadURL,
		LLMResponse:    raw,
	}, nil
}

// SendTorrentDownload 通过 qBittorrent 发送单个种子下载。
func (a *App) SendTorrentDownload(ctx context.Context, request ManualDownloadRequest) (DownloadTask, error) {
	torrent, err := a.getTorrent(ctx, request.SiteID, request.TorrentID)
	if err != nil {
		return DownloadTask{}, err
	}
	if strings.TrimSpace(torrent.DownloadURL) == "" {
		return DownloadTask{}, fmt.Errorf("torrent download url is empty")
	}
	title := strings.TrimSpace(request.FormattedTitle)
	if title == "" {
		title = torrent.Title
	}
	task := DownloadTask{
		ID:           downloadTaskID(torrent.SiteID, torrent.ID, "manual"),
		SiteID:       torrent.SiteID,
		TorrentID:    torrent.ID,
		RuleName:     "manual",
		Status:       "pending",
		TorrentTitle: title,
		DownloadURL:  torrent.DownloadURL,
	}
	created, err := a.store.CreateDownloadTaskIfAbsent(ctx, downloadTaskToRecord(task))
	if err != nil {
		return DownloadTask{}, err
	}
	if !created {
		task.Status = "exists"
		return task, nil
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", err.Error())
		task.Status = "failed"
		task.Error = err.Error()
		return task, nil
	}
	if err := a.addTorrentToQBAndLink(ctx, qb, torrent, task.ID); err != nil {
		_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", err.Error())
		task.Status = "failed"
		task.Error = err.Error()
		return task, nil
	}
	if err := a.store.UpdateDownloadTask(ctx, task.ID, "sent", "", "", ""); err != nil {
		return DownloadTask{}, err
	}
	task.Status = "sent"
	return task, nil
}

// FetchTorrentDetail 抓取详情页并保存到种子记录。
func (a *App) FetchTorrentDetail(ctx context.Context, siteID, torrentID string) (Torrent, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return Torrent{}, err
	}
	if strings.TrimSpace(torrent.DetailURL) == "" {
		return Torrent{}, fmt.Errorf("torrent detail url is empty")
	}
	site, err := a.findSite(torrent.SiteID)
	if err != nil {
		return Torrent{}, err
	}
	headers := http.Header{}
	headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	headers.Set("Referer", site.BaseURL+"/torrents.php")
	slog.Info("torrent detail fetch started", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "url", torrent.DetailURL)
	result, err := a.fetchSiteResource(ctx, site, torrent.DetailURL, siteRequestOptions{
		Headers: headers, Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
	})
	if err != nil {
		return Torrent{}, err
	}
	detail, err := parser.ParseTorrentDetail(result.Body, parser.TorrentDetailParseOptions{
		SiteID:    torrent.SiteID,
		TorrentID: torrent.ID,
		BaseURL:   site.BaseURL,
		URL:       result.URL,
	})
	if err != nil {
		return Torrent{}, err
	}
	if err := a.store.UpdateTorrentDetail(ctx, storage.TorrentRecord{
		SiteID:            torrent.SiteID,
		TorrentID:         torrent.ID,
		DetailTitle:       detail.DetailTitle,
		Subtitle:          detail.Subtitle,
		ProductURL:        detail.ProductURL,
		DetailInfoHash:    detail.InfoHash,
		DetailDescription: detail.DetailDescription,
		DetailRawText:     detail.DetailRawText,
		DetailFetchedAt:   result.FetchedAt.Format(time.RFC3339Nano),
	}); err != nil {
		return Torrent{}, err
	}
	slog.Info("torrent detail fetch completed", "site_id", torrent.SiteID, "torrent_id", torrent.ID, "bytes", len(result.Body))
	return a.getTorrent(ctx, torrent.SiteID, torrent.ID)
}

// loadRuntimeSites 加载目录站点并合并旧配置站点。
func loadRuntimeSites(cfg config.Config) (map[string]runtimeSite, []string, error) {
	sites := map[string]runtimeSite{}
	siteIDs := []string{}
	addSite := func(site runtimeSite) {
		if strings.TrimSpace(site.ID) == "" || strings.TrimSpace(site.BaseURL) == "" {
			return
		}
		if _, exists := sites[site.ID]; !exists {
			siteIDs = append(siteIDs, site.ID)
		}
		sites[site.ID] = site
	}

	definitions, err := parser.LoadSiteDefinitionsDir(cfg.SitesDir)
	if err != nil {
		return nil, nil, err
	}
	for _, definition := range definitions {
		siteCfg := parser.SiteConfigFromDefinition(definition)
		addSite(runtimeSite{
			ID:         siteCfg.SiteID,
			Name:       firstNonEmpty(siteCfg.Name, siteCfg.SiteID),
			BaseURL:    siteCfg.BaseURL,
			Definition: definition,
		})
	}

	sort.Slice(siteIDs, func(i, j int) bool {
		return strings.ToLower(siteIDs[i]) < strings.ToLower(siteIDs[j])
	})
	return sites, siteIDs, nil
}

// refreshSite 抓取站点页面并写入缓存。
func (a *App) refreshSite(ctx context.Context, siteID string) (FetchResult, []Torrent, error) {
	site, err := a.findSite(siteID)
	if err != nil {
		return FetchResult{SiteID: siteID, Status: "failed"}, nil, err
	}
	slog.Info("fetch site started", "site_id", site.ID, "base_url", site.BaseURL)
	siteCfg := parser.SiteConfigFromDefinition(site.Definition)
	fetchURL := firstNonEmpty(siteCfg.URL, site.BaseURL+"/torrents.php")
	result, err := a.fetchSiteResource(ctx, site, fetchURL, siteRequestOptions{
		Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
	})
	if err != nil {
		slog.Error("fetch site request failed", "site_id", site.ID, "url", fetchURL, "error", err)
		return FetchResult{SiteID: siteID, Status: "failed"}, nil, err
	}
	parsed, err := parser.ParsePageWithDefinition(result.Body, site.Definition)
	if err != nil {
		slog.Error("parse fetched html failed", "site_id", site.ID, "url", result.URL, "error", err)
		return FetchResult{SiteID: siteID, Status: "failed"}, nil, err
	}
	records := make([]storage.TorrentRecord, 0, len(parsed.Torrents))
	for index, entry := range parsed.Torrents {
		records = append(records, recordFromParser(entry, index))
	}
	upsert, err := a.store.UpsertTorrents(ctx, records)
	if err != nil {
		slog.Error("upsert fetched torrents failed", "site_id", site.ID, "error", err)
		return FetchResult{SiteID: siteID, Status: "failed"}, nil, err
	}
	allTorrents := make([]Torrent, 0, len(records))
	for _, record := range records {
		allTorrents = append(allTorrents, torrentFromRecord(record))
	}
	inserted := make([]Torrent, 0, len(upsert.Inserted))
	a.mu.Lock()
	for _, record := range records {
		a.cache[record.SiteID+":"+record.TorrentID] = torrentFromRecord(record)
	}
	for _, record := range upsert.Inserted {
		inserted = append(inserted, torrentFromRecord(record))
	}
	a.mu.Unlock()
	filesSaved, filesFailed, err := a.ensureTorrentFiles(ctx, allTorrents)
	if err != nil {
		slog.Error("persist fetched torrent files failed", "site_id", site.ID, "error", err)
		return FetchResult{SiteID: siteID, Status: "failed", Fetched: len(records), Inserted: len(inserted), Changed: len(upsert.Changed), FilesSaved: filesSaved, FilesFailed: filesFailed}, inserted, err
	}
	slog.Info("fetch site completed", "site_id", site.ID, "fetched", len(records), "inserted", len(inserted), "changed", len(upsert.Changed), "torrent_files_saved", filesSaved, "torrent_files_failed", filesFailed)
	return FetchResult{SiteID: site.ID, Status: "ok", Fetched: len(records), Inserted: len(inserted), Changed: len(upsert.Changed), FilesSaved: filesSaved, FilesFailed: filesFailed}, inserted, nil
}

// findSite 按站点 ID 查找运行时站点。
func (a *App) findSite(siteID string) (runtimeSite, error) {
	if site, ok := a.sites[siteID]; ok {
		return site, nil
	}
	return runtimeSite{}, fmt.Errorf("site %q not found", siteID)
}

// loadCache 从数据库加载最近种子缓存。
func (a *App) loadCache(ctx context.Context) error {
	records, err := a.store.ListTorrents(ctx, storage.TorrentListQuery{Limit: storage.MaxTorrentListLimit})
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, record := range records {
		a.cache[record.SiteID+":"+record.TorrentID] = torrentFromRecord(record)
	}
	return nil
}

// effectiveLLMConfig 合并配置文件和数据库中的 LLM 设置。
func (a *App) effectiveLLMConfig(ctx context.Context) config.LLMConfig {
	cfg := a.cfg.LLM
	var stored config.LLMConfig
	if ok, err := a.store.LoadSetting(ctx, storage.LLMSettingKey, &stored); err == nil && ok {
		cfg = stored
	}
	return cfg
}

// getTorrent 从数据库读取单条种子。
func (a *App) getTorrent(ctx context.Context, siteID, torrentID string) (Torrent, error) {
	record, ok, err := a.store.GetTorrent(ctx, siteID, torrentID)
	if err != nil {
		return Torrent{}, err
	}
	if !ok {
		return Torrent{}, fmt.Errorf("torrent %s/%s not found", siteID, torrentID)
	}
	return torrentFromRecord(record), nil
}

func recordFromParser(entry parser.TorrentEntry, sourceOrder int) storage.TorrentRecord {
	return storage.TorrentRecord{
		SiteID:             entry.SiteID,
		TorrentID:          strconv.Itoa(entry.ID),
		Title:              entry.Title,
		Category:           entry.Category,
		CategoryQuery:      entry.CategoryQuery,
		DetailURL:          entry.DetailURL,
		DownloadURL:        entry.DownloadURL,
		CoverURL:           entry.CoverURL,
		Tags:               entry.Tags,
		TagIDs:             entry.TagIDs,
		Promotion:          entry.Promotion,
		PromotionClass:     entry.PromotionClass,
		PromotionEndsAt:    entry.PromotionEndsAt,
		PromotionRemaining: entry.PromotionRemaining,
		Subtitle:           entry.Subtitle,
		Description:        entry.Description,
		SizeText:           entry.SizeText,
		SizeBytes:          entry.SizeBytes,
		Seeders:            entry.Seeders,
		Leechers:           entry.Leechers,
		Snatches:           entry.Snatches,
		Comments:           entry.Comments,
		PublishedAt:        entry.PublishedAt,
		PublishedText:      entry.PublishedText,
		StickyLevel:        entry.StickyLevel,
		Bookmarked:         entry.Bookmarked,
		SourceOrder:        sourceOrder,
	}
}

func torrentFromRecord(record storage.TorrentRecord) Torrent {
	var published *time.Time
	if record.PublishedAt != "" {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", record.PublishedAt, time.Local); err == nil {
			published = &parsed
		}
	}
	return Torrent{
		ID:                record.TorrentID,
		SiteID:            record.SiteID,
		Category:          record.Category,
		CategoryQuery:     record.CategoryQuery,
		Title:             record.Title,
		DetailURL:         record.DetailURL,
		DownloadURL:       record.DownloadURL,
		CoverURL:          record.CoverURL,
		Tags:              record.Tags,
		TagIDs:            record.TagIDs,
		Description:       record.Description,
		DetailTitle:       record.DetailTitle,
		Subtitle:          record.Subtitle,
		ProductURL:        record.ProductURL,
		DetailInfoHash:    record.DetailInfoHash,
		DetailDescription: record.DetailDescription,
		DetailRawText:     record.DetailRawText,
		DetailFetchedAt:   record.DetailFetchedAt,
		SizeBytes:         record.SizeBytes,
		Seeders:           record.Seeders,
		Leechers:          record.Leechers,
		Snatches:          record.Snatches,
		Comments:          record.Comments,
		Promotion:         record.Promotion,
		PromotionClass:    record.PromotionClass,
		PromotionEndsAt:   record.PromotionEndsAt,
		PublishedAt:       published,
		PublishedText:     record.PublishedText,
		FirstSeenAt:       record.FirstSeenAt,
		LastSeenAt:        record.LastSeenAt,
		SourceOrder:       record.SourceOrder,
	}
}

func ruleFromConfig(rule config.RuleConfig) Rule {
	return Rule{
		Name: rule.Name, SiteIDs: rule.SiteIDs, SiteCategories: rule.SiteCategories, SiteTags: rule.SiteTags,
		SubtitleTags: rule.SubtitleTags, TitleExpression: rule.TitleExpression, Promotions: rule.Promotions,
		MinSize: rule.MinSize, MaxSize: rule.MaxSize,
		MinSeeders: rule.MinSeeders, MaxSeeders: rule.MaxSeeders, MinLeechers: rule.MinLeechers,
		MaxLeechers: rule.MaxLeechers, MinSnatches: rule.MinSnatches, MaxSnatches: rule.MaxSnatches,
		PublishedWithinMinutes: rule.PublishedWithinMinutes, SortBy: rule.SortBy, SortDirection: rule.SortDirection,
		Action: firstNonEmpty(rule.Action, "download"),
	}
}

func ruleToRecord(rule Rule) storage.RuleRecord {
	return storage.RuleRecord{
		Name: rule.Name, SiteIDs: rule.SiteIDs, SiteCategories: rule.SiteCategories, SiteTags: rule.SiteTags,
		SubtitleTags: rule.SubtitleTags, TitleExpression: rule.TitleExpression, Promotions: rule.Promotions,
		MinSize: rule.MinSize, MaxSize: rule.MaxSize,
		MinSeeders: rule.MinSeeders, MaxSeeders: rule.MaxSeeders, MinLeechers: rule.MinLeechers,
		MaxLeechers: rule.MaxLeechers, MinSnatches: rule.MinSnatches, MaxSnatches: rule.MaxSnatches,
		PublishedWithinMinutes: rule.PublishedWithinMinutes, SortBy: rule.SortBy, SortDirection: rule.SortDirection,
		Action: firstNonEmpty(rule.Action, "download"),
	}
}

func ruleFromRecord(record storage.RuleRecord) Rule {
	return Rule{
		Name: record.Name, SiteIDs: record.SiteIDs, SiteCategories: record.SiteCategories, SiteTags: record.SiteTags,
		SubtitleTags: record.SubtitleTags, TitleExpression: record.TitleExpression, Promotions: record.Promotions,
		MinSize: record.MinSize, MaxSize: record.MaxSize,
		MinSeeders: record.MinSeeders, MaxSeeders: record.MaxSeeders, MinLeechers: record.MinLeechers,
		MaxLeechers: record.MaxLeechers, MinSnatches: record.MinSnatches, MaxSnatches: record.MaxSnatches,
		PublishedWithinMinutes: record.PublishedWithinMinutes, SortBy: record.SortBy, SortDirection: record.SortDirection,
		Action: firstNonEmpty(record.Action, "download"),
	}
}

func downloadTaskID(siteID, torrentID, ruleName string) string {
	return siteID + ":" + torrentID + ":" + ruleName
}

func downloadTaskToRecord(task DownloadTask) storage.DownloadTaskRecord {
	return storage.DownloadTaskRecord{
		ID: task.ID, SiteID: task.SiteID, TorrentID: task.TorrentID, RuleName: task.RuleName,
		SubscriptionID: task.SubscriptionID, Trigger: task.Trigger, Status: task.Status,
		TorrentTitle: task.TorrentTitle, DownloadURL: task.DownloadURL, QBHash: task.QBHash,
		Error: task.Error, ContentPath: task.ContentPath, PlanCategory: task.Category,
		PlanSavePath: task.SavePath, PlanTags: task.Tags, PlanRename: task.Rename,
		PlanPaused: task.Paused, ReasonCode: task.ReasonCode, AttemptCount: task.AttemptCount,
		RetryCount: task.RetryCount, LastAttemptAt: timeValue(task.LastAttemptAt), SentAt: timeValue(task.SentAt),
	}
}

func downloadTaskFromRecord(record storage.DownloadTaskRecord) DownloadTask {
	task := DownloadTask{
		ID: record.ID, SiteID: record.SiteID, TorrentID: record.TorrentID, RuleName: record.RuleName,
		SubscriptionID: record.SubscriptionID, Trigger: record.Trigger, Status: record.Status,
		TorrentTitle: record.TorrentTitle, DownloadURL: record.DownloadURL, QBHash: record.QBHash,
		Error: record.Error, ContentPath: record.ContentPath, Category: record.PlanCategory,
		SavePath: record.PlanSavePath, Tags: record.PlanTags, Rename: record.PlanRename,
		Paused: record.PlanPaused, ReasonCode: record.ReasonCode, AttemptCount: record.AttemptCount,
		RetryCount: record.RetryCount, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if !record.LastAttemptAt.IsZero() {
		value := record.LastAttemptAt
		task.LastAttemptAt = &value
	}
	if !record.SentAt.IsZero() {
		value := record.SentAt
		task.SentAt = &value
	}
	return task
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func organizeTaskFromRecord(record storage.OrganizeTaskRecord) OrganizeTask {
	return OrganizeTask{
		ID:             record.ID,
		DownloadTaskID: record.DownloadTaskID,
		Status:         record.Status,
		Title:          record.Title,
		SourcePath:     record.SourcePath,
		RelativeDir:    record.RelativeDir,
		Filename:       record.Filename,
		TargetPath:     record.TargetPath,
		LLMResponse:    record.LLMResponse,
		Confidence:     record.Confidence,
		Error:          record.Error,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func torrentKey(torrent Torrent) string {
	return torrent.SiteID + ":" + torrent.ID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
