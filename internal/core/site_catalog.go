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

// MediaFilterOptions 返回站点定义中的分类 checkbox 与促销 select 选项。
func (a *App) MediaFilterOptions(_ context.Context, siteID string) (MediaFilterOptions, error) {
	site, err := a.findSite(strings.TrimSpace(siteID))
	if err != nil {
		return MediaFilterOptions{}, err
	}
	result := MediaFilterOptions{
		SiteID:         site.ID,
		CategoryLabel:  "分类",
		Categories:     []MediaFilterOption{},
		Checkboxes:     []MediaFilterGroup{},
		PromotionLabel: "促销",
		Promotions:     []MediaFilterOption{},
	}
	fields := site.Definition.HTML.Search.Fields
	seenCategories := map[string]struct{}{}
	for _, group := range fields.Checkboxes {
		if !isCategoryCheckboxGroup(group) {
			name := strings.TrimSpace(group.Name)
			if name == "" {
				continue
			}
			filterGroup := MediaFilterGroup{
				Name:    name,
				Label:   strings.TrimSpace(group.Label),
				Options: []MediaFilterOption{},
			}
			seen := map[string]struct{}{}
			for _, option := range group.Options {
				label := strings.TrimSpace(option.Label)
				if label == "" {
					continue
				}
				key := strings.ToLower(label)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				filterGroup.Options = append(filterGroup.Options, MediaFilterOption{Value: label, Label: label})
			}
			if len(filterGroup.Options) > 0 {
				if filterGroup.Label == "" {
					filterGroup.Label = filterGroup.Name
				}
				result.Checkboxes = append(result.Checkboxes, filterGroup)
			}
			continue
		}
		if label := strings.TrimSpace(group.Label); label != "" {
			result.CategoryLabel = label
		}
		for _, option := range group.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				continue
			}
			key := strings.ToLower(label)
			if _, exists := seenCategories[key]; exists {
				continue
			}
			seenCategories[key] = struct{}{}
			result.Categories = append(result.Categories, MediaFilterOption{Value: label, Label: label})
		}
	}
	seenPromotions := map[string]struct{}{}
	for _, field := range fields.Selects {
		if !strings.EqualFold(strings.TrimSpace(field.Role), "promotion") {
			continue
		}
		if label := strings.TrimSpace(field.Label); label != "" {
			result.PromotionLabel = label
		}
		for _, option := range field.Options {
			value := strings.TrimSpace(option.FilterValue)
			label := strings.TrimSpace(option.Label)
			if value == "" || label == "" || strings.EqualFold(value, "all") {
				continue
			}
			key := strings.ToLower(value)
			if _, exists := seenPromotions[key]; exists {
				continue
			}
			seenPromotions[key] = struct{}{}
			result.Promotions = append(result.Promotions, MediaFilterOption{Value: value, Label: label})
		}
	}
	return result, nil
}

func isCategoryCheckboxGroup(group parser.SiteSearchCheckboxGroup) bool {
	if strings.EqualFold(strings.TrimSpace(group.Name), "cat") {
		return true
	}
	for _, option := range group.Options {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(option.Name)), "cat") {
			return true
		}
	}
	return false
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
			ID:            siteCfg.SiteID,
			Name:          firstNonEmpty(siteCfg.Name, siteCfg.SiteID),
			BaseURL:       siteCfg.BaseURL,
			AttendanceURL: siteCfg.AttendanceURL,
			Definition:    definition,
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
