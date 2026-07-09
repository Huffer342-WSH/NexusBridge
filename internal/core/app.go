// Package core 提供 NexusBridge 的共享应用服务实现。
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/llm"
	"nexusbridge/internal/organizer"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

type App struct {
	cfg        config.Config
	store      *storage.SQLiteStore
	mu         sync.RWMutex
	qbMu       sync.Mutex
	qbCached   *qbittorrent.Client
	qbCacheKey string
	cache      map[string]Torrent
	sites      map[string]runtimeSite
	siteIDs    []string
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
	app := &App{cfg: cfg, store: store, cache: map[string]Torrent{}, sites: sites, siteIDs: siteIDs}
	for _, rule := range cfg.Rules {
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
		SiteID: query.SiteID,
		Search: query.Search,
		Limit:  query.Limit,
		Offset: query.Offset,
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
	result, _, err := a.refreshSite(ctx, siteID)
	return result, err
}

// RunOnce 执行一次抓取、筛选和下载发送。
func (a *App) RunOnce(ctx context.Context, siteID string) (FetchResult, error) {
	result, changed, err := a.refreshSite(ctx, siteID)
	if err != nil {
		return result, err
	}
	rules, err := a.ListRules(ctx)
	if err != nil {
		return result, err
	}
	qb, qbErr := a.qbClient(ctx)
	for _, torrent := range changed {
		for _, rule := range rules {
			match := MatchRule(rule, torrent)
			if !match.Matched || firstNonEmpty(rule.Action, "download") != "download" {
				continue
			}
			result.Matched++
			task := DownloadTask{
				ID:           downloadTaskID(torrent.SiteID, torrent.ID, rule.ID),
				SiteID:       torrent.SiteID,
				TorrentID:    torrent.ID,
				RuleID:       rule.ID,
				Status:       "pending",
				TorrentTitle: torrent.Title,
				DownloadURL:  torrent.DownloadURL,
			}
			created, err := a.store.CreateDownloadTaskIfAbsent(ctx, downloadTaskToRecord(task))
			if err != nil {
				return result, err
			}
			if !created {
				continue
			}
			if qbErr != nil {
				_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", qbErr.Error())
				continue
			}
			if strings.TrimSpace(torrent.DownloadURL) == "" {
				_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", "torrent download url is empty")
				continue
			}
			if err := a.addTorrentToQBAndLink(ctx, qb, torrent, task.ID); err != nil {
				_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", err.Error())
				continue
			}
			if err := a.store.UpdateDownloadTask(ctx, task.ID, "sent", "", "", ""); err != nil {
				return result, err
			}
			result.DownloadSent++
		}
	}
	result.Status = "ok"
	return result, nil
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
	if rule.Action == "" {
		rule.Action = "download"
	}
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Name) == "" {
		return Rule{}, fmt.Errorf("rule id and name are required")
	}
	if err := a.store.SaveRule(ctx, ruleToRecord(rule)); err != nil {
		return Rule{}, err
	}
	return rule, nil
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
		RuleID:       "manual",
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
	for _, entry := range parsed.Torrents {
		records = append(records, recordFromParser(entry))
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
	filesSaved, filesFailed, err := a.ensureTorrentFiles(ctx, allTorrents)
	if err != nil {
		slog.Error("persist fetched torrent files failed", "site_id", site.ID, "error", err)
		return FetchResult{SiteID: siteID, Status: "failed", Fetched: len(records), Changed: len(upsert.Changed), FilesSaved: filesSaved, FilesFailed: filesFailed}, nil, err
	}
	changed := make([]Torrent, 0, len(upsert.Changed))
	a.mu.Lock()
	for _, record := range records {
		a.cache[record.SiteID+":"+record.TorrentID] = torrentFromRecord(record)
	}
	for _, record := range upsert.Changed {
		changed = append(changed, torrentFromRecord(record))
	}
	a.mu.Unlock()
	slog.Info("fetch site completed", "site_id", site.ID, "fetched", len(records), "changed", len(changed), "torrent_files_saved", filesSaved, "torrent_files_failed", filesFailed)
	return FetchResult{SiteID: site.ID, Status: "ok", Fetched: len(records), Changed: len(changed), FilesSaved: filesSaved, FilesFailed: filesFailed}, changed, nil
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

func recordFromParser(entry parser.TorrentEntry) storage.TorrentRecord {
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
		Promotion:          entry.Promotion,
		PromotionClass:     entry.PromotionClass,
		PromotionEndsAt:    entry.PromotionEndsAt,
		PromotionRemaining: entry.PromotionRemaining,
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
	}
}

func torrentFromRecord(record storage.TorrentRecord) Torrent {
	var published *time.Time
	if record.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02 15:04:05", record.PublishedAt); err == nil {
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
		Description:       record.Description,
		DetailTitle:       record.DetailTitle,
		Subtitle:          record.Subtitle,
		DetailDescription: record.DetailDescription,
		DetailRawText:     record.DetailRawText,
		DetailFetchedAt:   record.DetailFetchedAt,
		SizeBytes:         record.SizeBytes,
		Seeders:           record.Seeders,
		Leechers:          record.Leechers,
		Snatches:          record.Snatches,
		Comments:          record.Comments,
		Promotion:         record.Promotion,
		PromotionEndsAt:   record.PromotionEndsAt,
		PublishedAt:       published,
		PublishedText:     record.PublishedText,
		FirstSeenAt:       record.FirstSeenAt,
		LastSeenAt:        record.LastSeenAt,
	}
}

func ruleFromConfig(rule config.RuleConfig) Rule {
	return Rule{
		ID:         rule.ID,
		Name:       rule.Name,
		Enabled:    rule.Enabled,
		SiteIDs:    rule.SiteIDs,
		Categories: rule.Categories,
		Tags:       rule.Tags,
		Include:    rule.Include,
		Exclude:    rule.Exclude,
		Promotion:  rule.Promotion,
		MinSize:    rule.MinSize,
		MaxSize:    rule.MaxSize,
		MinSeeders: rule.MinSeeders,
		Action:     firstNonEmpty(rule.Action, "download"),
	}
}

func ruleToRecord(rule Rule) storage.RuleRecord {
	return storage.RuleRecord{
		ID:         rule.ID,
		Name:       rule.Name,
		Enabled:    rule.Enabled,
		SiteIDs:    rule.SiteIDs,
		Categories: rule.Categories,
		Tags:       rule.Tags,
		Include:    rule.Include,
		Exclude:    rule.Exclude,
		Promotion:  rule.Promotion,
		MinSize:    rule.MinSize,
		MaxSize:    rule.MaxSize,
		MinSeeders: rule.MinSeeders,
		Action:     firstNonEmpty(rule.Action, "download"),
	}
}

func ruleFromRecord(record storage.RuleRecord) Rule {
	return Rule{
		ID:         record.ID,
		Name:       record.Name,
		Enabled:    record.Enabled,
		SiteIDs:    record.SiteIDs,
		Categories: record.Categories,
		Tags:       record.Tags,
		Include:    record.Include,
		Exclude:    record.Exclude,
		Promotion:  record.Promotion,
		MinSize:    record.MinSize,
		MaxSize:    record.MaxSize,
		MinSeeders: record.MinSeeders,
		Action:     firstNonEmpty(record.Action, "download"),
	}
}

func downloadTaskID(siteID, torrentID, ruleID string) string {
	return siteID + ":" + torrentID + ":" + ruleID
}

func downloadTaskToRecord(task DownloadTask) storage.DownloadTaskRecord {
	return storage.DownloadTaskRecord{
		ID:           task.ID,
		SiteID:       task.SiteID,
		TorrentID:    task.TorrentID,
		RuleID:       task.RuleID,
		Status:       task.Status,
		TorrentTitle: task.TorrentTitle,
		DownloadURL:  task.DownloadURL,
		QBHash:       task.QBHash,
		Error:        task.Error,
		ContentPath:  task.ContentPath,
	}
}

func downloadTaskFromRecord(record storage.DownloadTaskRecord) DownloadTask {
	return DownloadTask{
		ID:           record.ID,
		SiteID:       record.SiteID,
		TorrentID:    record.TorrentID,
		RuleID:       record.RuleID,
		Status:       record.Status,
		TorrentTitle: record.TorrentTitle,
		DownloadURL:  record.DownloadURL,
		QBHash:       record.QBHash,
		Error:        record.Error,
		ContentPath:  record.ContentPath,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
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
