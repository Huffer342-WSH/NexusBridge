package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"nexusbridge/internal/config"
	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

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

// FetchSite 抓取并保存指定站点种子。
func (a *App) FetchSite(ctx context.Context, siteID string) (FetchResult, error) {
	job, err := a.runTrackedSiteFetch(ctx, siteID, "manual", SiteFetchRequest{Mode: "incremental"})
	result := FetchResult{
		SiteID: job.SiteID, Fetched: job.Fetched, Inserted: job.Inserted, Changed: job.Changed,
		Matched: job.Matched, DownloadSent: job.DownloadSent, FilesSaved: job.FilesSaved, FilesFailed: job.FilesFailed,
	}
	if err != nil {
		result.Status = "failed"
		return result, err
	}
	result.Status = "ok"
	return result, nil
}

// RunOnce 保留内部调用兼容，实际执行与统一站点抓取入口相同的增量订阅流程。
func (a *App) RunOnce(ctx context.Context, siteID string) (FetchResult, error) {
	return a.FetchSite(ctx, siteID)
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

// findSite 按站点 ID 查找运行时站点。
func (a *App) findSite(siteID string) (runtimeSite, error) {
	if site, ok := a.sites[siteID]; ok {
		return site, nil
	}
	return runtimeSite{}, fmt.Errorf("site %q not found", siteID)
}
