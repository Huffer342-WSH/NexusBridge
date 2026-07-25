package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

const (
	defaultFetchMaxPages      = 3
	maxFetchMaxPages          = 100
	stopExistingTorrentCount  = 5
	retainedSiteFetchJobCount = 100
)

type activeSiteFetch struct {
	jobID string
	done  chan struct{}
}

type sitePageExecution struct {
	matched int
	sent    int
	errors  []error
}

// GetFetchSettings 返回运行时全局抓取设置。
func (a *App) GetFetchSettings(ctx context.Context) (FetchSettings, error) {
	settings := FetchSettings{MaxPages: defaultFetchMaxPages}
	var stored FetchSettings
	ok, err := a.store.LoadSetting(ctx, storage.FetchSettingKey, &stored)
	if err != nil {
		return FetchSettings{}, err
	}
	if !ok {
		return settings, nil
	}
	if stored.MaxPages < 1 || stored.MaxPages > maxFetchMaxPages {
		return FetchSettings{}, fmt.Errorf("stored fetch max_pages must be between 1 and %d", maxFetchMaxPages)
	}
	return stored, nil
}

// SaveFetchSettings 校验并保存所有站点共享的抓取设置。
func (a *App) SaveFetchSettings(ctx context.Context, settings FetchSettings) (FetchSettings, error) {
	if settings.MaxPages < 1 || settings.MaxPages > maxFetchMaxPages {
		return FetchSettings{}, fmt.Errorf("fetch max_pages must be between 1 and %d", maxFetchMaxPages)
	}
	if err := a.store.SaveSetting(ctx, storage.FetchSettingKey, settings); err != nil {
		return FetchSettings{}, err
	}
	return settings, nil
}

// StartSiteFetch 创建后台站点扫描；同一站点已有任务时直接返回该任务。
func (a *App) StartSiteFetch(ctx context.Context, siteID, trigger string, request SiteFetchRequest) (SiteFetchJob, error) {
	request, err := a.normalizeSiteFetchRequest(ctx, request)
	if err != nil {
		return SiteFetchJob{}, err
	}
	if _, err := a.findSite(siteID); err != nil {
		return SiteFetchJob{}, err
	}
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		trigger = "manual"
	}

	a.fetchMu.Lock()
	if active, ok := a.activeFetches[siteID]; ok {
		a.fetchMu.Unlock()
		record, found, getErr := a.store.GetSiteFetchJob(ctx, active.jobID)
		if getErr != nil {
			return SiteFetchJob{}, getErr
		}
		if found {
			return siteFetchJobFromRecord(record), nil
		}
		return SiteFetchJob{}, fmt.Errorf("active site fetch job %q not found", active.jobID)
	}
	now := time.Now()
	record := storage.SiteFetchJobRecord{
		ID: fmt.Sprintf("%s:%d", siteID, now.UnixNano()), SiteID: siteID, Trigger: trigger,
		Mode: request.Mode, RequestedPages: request.Pages, Status: "queued", CreatedAt: now, UpdatedAt: now,
	}
	if err := a.store.SaveSiteFetchJob(ctx, record); err != nil {
		a.fetchMu.Unlock()
		return SiteFetchJob{}, err
	}
	active := &activeSiteFetch{jobID: record.ID, done: make(chan struct{})}
	a.activeFetches[siteID] = active
	a.fetchWG.Add(1)
	go func() {
		defer a.fetchWG.Done()
		defer close(active.done)
		a.executeSiteFetchJob(a.ctx, record)
		a.fetchMu.Lock()
		delete(a.activeFetches, siteID)
		a.fetchMu.Unlock()
	}()
	a.fetchMu.Unlock()
	return siteFetchJobFromRecord(record), nil
}

func (a *App) runTrackedSiteFetch(ctx context.Context, siteID, trigger string, request SiteFetchRequest) (SiteFetchJob, error) {
	job, err := a.StartSiteFetch(ctx, siteID, trigger, request)
	if err != nil {
		return SiteFetchJob{}, err
	}
	a.fetchMu.Lock()
	active := a.activeFetches[siteID]
	a.fetchMu.Unlock()
	if active != nil && active.jobID == job.ID {
		select {
		case <-ctx.Done():
			return SiteFetchJob{}, ctx.Err()
		case <-active.done:
		}
	}
	record, ok, err := a.store.GetSiteFetchJob(ctx, job.ID)
	if err != nil {
		return SiteFetchJob{}, err
	}
	if !ok {
		return SiteFetchJob{}, fmt.Errorf("site fetch job %q not found", job.ID)
	}
	job = siteFetchJobFromRecord(record)
	if job.Status == "failed" || job.Status == "completed_with_errors" {
		return job, fmt.Errorf("site fetch %s: %s", job.Status, firstNonEmpty(job.Error, job.StopReason))
	}
	return job, nil
}

// ListSiteFetchJobs 返回最近的站点扫描任务。
func (a *App) ListSiteFetchJobs(ctx context.Context, siteID string, limit int) ([]SiteFetchJob, error) {
	records, err := a.store.ListSiteFetchJobs(ctx, strings.TrimSpace(siteID), limit)
	if err != nil {
		return nil, err
	}
	result := make([]SiteFetchJob, 0, len(records))
	for _, record := range records {
		result = append(result, siteFetchJobFromRecord(record))
	}
	return result, nil
}

func (a *App) normalizeSiteFetchRequest(ctx context.Context, request SiteFetchRequest) (SiteFetchRequest, error) {
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = "incremental"
	}
	settings, err := a.GetFetchSettings(ctx)
	if err != nil {
		return SiteFetchRequest{}, err
	}
	switch request.Mode {
	case "incremental":
		request.Pages = settings.MaxPages
	case "pages":
		if request.Pages < 1 || request.Pages > settings.MaxPages {
			return SiteFetchRequest{}, fmt.Errorf("fetch pages must be between 1 and configured max_pages %d", settings.MaxPages)
		}
	default:
		return SiteFetchRequest{}, fmt.Errorf("fetch mode must be incremental or pages")
	}
	return request, nil
}

func (a *App) executeSiteFetchJob(ctx context.Context, record storage.SiteFetchJobRecord) storage.SiteFetchJobRecord {
	record.Status = "running"
	record.StartedAt = time.Now()
	_ = a.store.SaveSiteFetchJob(context.WithoutCancel(ctx), record)
	result, runErr := a.scanSitePages(ctx, &record)
	record = result
	record.FinishedAt = time.Now()
	if runErr != nil {
		record.Status = "failed"
		record.Error = appendErrorText(record.Error, runErr.Error())
		if record.StopReason == "" {
			record.StopReason = "error"
		}
	} else if record.Error != "" || record.FilesFailed > 0 {
		record.Status = "completed_with_errors"
	} else {
		record.Status = "completed"
	}
	_ = a.store.SaveSiteFetchJob(context.WithoutCancel(ctx), record)
	_ = a.store.PruneSiteFetchJobs(context.WithoutCancel(ctx), record.SiteID, retainedSiteFetchJobCount)
	return record
}

func (a *App) scanSitePages(ctx context.Context, job *storage.SiteFetchJobRecord) (storage.SiteFetchJobRecord, error) {
	lock := a.siteMutex(job.SiteID)
	if err := lock.Lock(ctx); err != nil {
		return *job, err
	}
	defer lock.Unlock()

	site, err := a.findSite(job.SiteID)
	if err != nil {
		return *job, err
	}
	siteCfg := parser.SiteConfigFromDefinition(site.Definition)
	listURL := firstNonEmpty(siteCfg.URL, site.BaseURL+"/torrents.php")
	currentURL := listURL
	if siteCfg.Pagination.Parameter != "" {
		currentURL = sitePageURL(listURL, siteCfg.Pagination, siteCfg.Pagination.Start)
	}
	seen := map[storage.TorrentKey]struct{}{}
	sourceOrder := 0
	existingStreak := 0
	var nonFatalErrors []error

	for pageNumber := 1; pageNumber <= job.RequestedPages; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return *job, err
		}
		job.CurrentPage = pageNumber
		fetched, err := a.fetchSiteResource(ctx, site, currentURL, siteRequestOptions{
			Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
		})
		if err != nil {
			return *job, fmt.Errorf("fetch site page %d: %w", pageNumber, err)
		}
		parsed, err := parser.ParsePageWithDefinition(fetched.Body, site.Definition)
		if err != nil {
			return *job, fmt.Errorf("parse site page %d: %w", pageNumber, err)
		}

		records := make([]storage.TorrentRecord, 0, len(parsed.Torrents))
		normalEntries := make([]parser.TorrentEntry, 0, len(parsed.Torrents))
		for _, entry := range parsed.Torrents {
			key := storage.TorrentKey{SiteID: entry.SiteID, TorrentID: strconv.Itoa(entry.ID)}
			if entry.ID <= 0 {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			records = append(records, recordFromParser(entry, sourceOrder))
			sourceOrder++
			if entry.StickyLevel == 0 {
				normalEntries = append(normalEntries, entry)
			}
		}
		upsert, err := a.store.UpsertTorrentPage(ctx, records)
		if err != nil {
			return *job, fmt.Errorf("save site page %d: %w", pageNumber, err)
		}
		if pageNumber == 1 {
			a.replaceSitePinned(site.ID, records)
		}
		insertedKeys := make(map[storage.TorrentKey]struct{}, len(upsert.Inserted))
		inserted := make([]Torrent, 0, len(upsert.Inserted))
		for _, record := range upsert.Inserted {
			insertedKeys[storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = struct{}{}
			inserted = append(inserted, torrentFromRecord(record))
		}
		job.PagesFetched++
		job.FetchedCount += len(records)
		job.InsertedCount += len(upsert.Inserted)
		job.ChangedCount += len(upsert.Changed)
		filesSaved, filesFailed, fileErr := a.ensureTorrentFiles(ctx, inserted)
		job.FilesSaved += filesSaved
		job.FilesFailed += filesFailed
		if fileErr != nil {
			nonFatalErrors = append(nonFatalErrors, fmt.Errorf("page %d torrent files: %w", pageNumber, fileErr))
		}

		execution := a.executeSiteSubscriptionsForPage(ctx, site.ID, job.Trigger, len(records), len(upsert.Inserted))
		job.MatchedCount += execution.matched
		job.SentCount += execution.sent
		nonFatalErrors = append(nonFatalErrors, execution.errors...)

		for _, entry := range normalEntries {
			key := storage.TorrentKey{SiteID: entry.SiteID, TorrentID: strconv.Itoa(entry.ID)}
			if _, insertedNow := insertedKeys[key]; insertedNow {
				existingStreak = 0
			} else {
				existingStreak++
			}
		}
		job.Error = joinedErrorText(nonFatalErrors)
		_ = a.store.SaveSiteFetchJob(context.WithoutCancel(ctx), *job)

		if len(records) == 0 {
			job.StopReason = "empty_page"
			break
		}
		if len(normalEntries) == 0 {
			job.StopReason = "no_normal_torrents"
			break
		}
		if job.Mode == "incremental" && existingStreak >= stopExistingTorrentCount {
			job.StopReason = "existing_boundary"
			break
		}
		if pageNumber >= job.RequestedPages {
			job.StopReason = "max_pages"
			break
		}
		nextURL, ok := nextSitePageURL(parsed.Pagination, listURL, currentURL, pageNumber, siteCfg.Pagination)
		if !ok {
			job.StopReason = "no_next_page"
			break
		}
		currentURL = nextURL
	}
	if job.StopReason == "" {
		job.StopReason = "completed"
	}
	return *job, nil
}

func (a *App) executeSiteSubscriptionsForPage(ctx context.Context, siteID, trigger string, fetched, inserted int) sitePageExecution {
	result := sitePageExecution{}
	records, err := a.store.ListEnabledSubscriptionsBySite(ctx, siteID)
	if err != nil {
		result.errors = append(result.errors, err)
		return result
	}
	if len(records) == 0 {
		return result
	}
	assigned, err := a.assignPendingSiteTorrents(ctx, siteID, records)
	if err != nil {
		result.errors = append(result.errors, err)
		return result
	}
	for _, count := range assigned {
		result.matched += count
	}
	for _, record := range records {
		subscription := subscriptionFromRecord(record)
		ruleRecord, ok, err := a.store.GetRule(ctx, subscription.RuleName)
		if err != nil {
			result.errors = append(result.errors, err)
			continue
		}
		if !ok {
			continue
		}
		run := storage.SubscriptionRunRecord{
			ID: newSubscriptionRunID(subscription.ID, siteID, trigger), SubscriptionID: subscription.ID,
			SiteID: siteID, Trigger: trigger, Status: "running", FetchedCount: fetched,
			InsertedCount: inserted, MatchedCount: assigned[subscription.ID], StartedAt: time.Now(),
		}
		if err := a.store.SaveSubscriptionRun(ctx, run); err != nil {
			result.errors = append(result.errors, err)
			continue
		}
		counts, runErr := a.runSubscriptionCandidates(ctx, subscription, ruleFromRecord(ruleRecord), trigger, siteID)
		run.AttemptedCount = counts.attempted
		run.SentCount = counts.sent
		run.ExistsCount = counts.exists
		run.FailedCount = counts.failed
		run.SkippedCount = counts.skipped
		run.FinishedAt = time.Now()
		result.sent += counts.sent
		if runErr != nil {
			run.Status = "failed"
			run.Error = runErr.Error()
			result.errors = append(result.errors, fmt.Errorf("subscription %s: %w", subscription.ID, runErr))
		} else {
			run.Status = "completed"
		}
		if err := a.store.SaveSubscriptionRun(context.WithoutCancel(ctx), run); err != nil {
			result.errors = append(result.errors, err)
		}
	}
	return result
}

// nextSitePageURL 根据分页声明构造下一页，并用网页链接判断是否仍有后续页。
func nextSitePageURL(pagination parser.Pagination, listURL, currentURL string, pageNumber int, pageConfig parser.SitePaginationConfig) (string, bool) {
	type candidate struct {
		value int
		url   string
	}
	candidates := []candidate{}
	for _, link := range pagination.Pages {
		value, err := strconv.Atoi(strings.TrimSpace(link.Page))
		if err != nil || strings.TrimSpace(link.URL) == "" || link.URL == currentURL {
			continue
		}
		candidates = append(candidates, candidate{value: value, url: link.URL})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].value < candidates[j].value })
	nextPage := pageNumber
	if pageConfig.Parameter != "" {
		nextPage = pageConfig.Start + pageNumber
	}
	for _, item := range candidates {
		if item.value >= nextPage {
			if pageConfig.Parameter != "" {
				return sitePageURL(listURL, pageConfig, nextPage), true
			}
			return item.url, true
		}
	}
	if len(pagination.Pages) > 0 {
		return "", false
	}
	if pageConfig.Parameter != "" {
		return sitePageURL(listURL, pageConfig, nextPage), true
	}
	return "", false
}

// sitePageURL 按站点声明的查询模板构造数字分页地址。
func sitePageURL(listURL string, pageConfig parser.SitePaginationConfig, page int) string {
	parsed, err := url.Parse(listURL)
	if err != nil {
		return listURL
	}
	template := strings.TrimSpace(pageConfig.Query)
	if template == "" {
		template = pageConfig.Parameter + "={{value}}"
	}
	rendered := strings.ReplaceAll(template, "{{value}}", strconv.Itoa(page))
	pageQuery, err := url.ParseQuery(strings.TrimPrefix(rendered, "?"))
	if err != nil {
		return listURL
	}
	query := parsed.Query()
	for name, values := range pageQuery {
		query.Del(name)
		for _, value := range values {
			query.Add(name, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func siteFetchJobFromRecord(record storage.SiteFetchJobRecord) SiteFetchJob {
	result := SiteFetchJob{
		ID: record.ID, SiteID: record.SiteID, Trigger: record.Trigger, Mode: record.Mode,
		RequestedPages: record.RequestedPages, Status: record.Status, CurrentPage: record.CurrentPage,
		PagesFetched: record.PagesFetched, Fetched: record.FetchedCount, Inserted: record.InsertedCount,
		Changed: record.ChangedCount, Matched: record.MatchedCount, DownloadSent: record.SentCount,
		FilesSaved: record.FilesSaved, FilesFailed: record.FilesFailed, StopReason: record.StopReason,
		Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if !record.StartedAt.IsZero() {
		started := record.StartedAt
		result.StartedAt = &started
	}
	if !record.FinishedAt.IsZero() {
		finished := record.FinishedAt
		result.FinishedAt = &finished
	}
	return result
}

func appendErrorText(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" || strings.Contains(existing, next) {
		return existing
	}
	return existing + "; " + next
}

func joinedErrorText(items []error) string {
	if len(items) == 0 {
		return ""
	}
	return errors.Join(items...).Error()
}
