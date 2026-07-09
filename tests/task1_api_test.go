// task1_api_test.go 验证任务 1 新增 HTTP API 和 fake qB 下载闭环。
package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/qbittorrent"
	httpserver "nexusbridge/internal/server"
	"nexusbridge/internal/storage"
)

// TestTask1RuleSubscriptionAndQBCatalogAPI 验证草稿预览零写入及配置类 API 的闭环。
func TestTask1RuleSubscriptionAndQBCatalogAPI(t *testing.T) {
	fake := newTask1FakeQB(t)
	handler := newTask1API(t, fake, nil, func(store *storage.SQLiteStore) {
		mustUpsertTask1Torrents(t, store, storage.TorrentRecord{
			SiteID: "test-site", TorrentID: "1", SourceOrder: 1, Title: "Wanted release", Category: "anime",
			DownloadURL: "https://example.invalid/download.php?id=1", Seeders: 10, Tags: []string{"voice"}, TagIDs: []string{"official"},
		}, storage.TorrentRecord{
			SiteID: "test-site", TorrentID: "2", SourceOrder: 0, Title: "Other release", Category: "anime",
			DownloadURL: "https://example.invalid/download.php?id=2", Tags: []string{"voice"}, TagIDs: []string{"official"},
		})
	})

	draft := core.Rule{TitleExpression: "wanted"}
	var preview core.RulePreviewResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/rules/preview", core.RulePreviewRequest{Rule: draft, Limit: 10}, http.StatusOK, &preview)
	if preview.Evaluated != 2 || preview.Matched != 1 || len(preview.Items) != 2 || !preview.Items[0].Matched || preview.Items[0].Torrent.ID != "1" || preview.Items[0].QBState != "unknown" {
		t.Fatalf("unsaved draft must still be previewable: %#v", preview)
	}
	if requests := fake.requestCount(); requests != 0 {
		t.Fatalf("draft rule preview must not call qB, got %d requests", requests)
	}
	var rules []core.Rule
	task1JSONRequest(t, handler, http.MethodGet, "/api/rules", nil, http.StatusOK, &rules)
	if len(rules) != 0 {
		t.Fatalf("draft preview must not save a rule: %#v", rules)
	}
	var tasks []core.DownloadTask
	task1JSONRequest(t, handler, http.MethodGet, "/api/download-tasks", nil, http.StatusOK, &tasks)
	if len(tasks) != 0 {
		t.Fatalf("draft preview must not create download tasks: %#v", tasks)
	}

	var filterOptions core.RuleFilterOptions
	task1JSONRequest(t, handler, http.MethodGet, "/api/sites/test-site/filter-options", nil, http.StatusOK, &filterOptions)
	if len(filterOptions.SiteCategories) != 1 || filterOptions.SiteCategories[0].Value != "anime" ||
		len(filterOptions.SiteTags) != 1 || filterOptions.SiteTags[0].Value != "official" ||
		len(filterOptions.SubtitleTags) != 1 || filterOptions.SubtitleTags[0].Value != "voice" ||
		len(filterOptions.Promotions) != 2 || filterOptions.Promotions[0].Value != "normal" || filterOptions.Promotions[1].Value != "pro_free" {
		t.Fatalf("unexpected rule filter options: %#v", filterOptions)
	}

	rule := core.Rule{Name: "Anime", SiteIDs: []string{"test-site"}, TitleExpression: "wanted"}
	var savedRule core.Rule
	task1JSONRequest(t, handler, http.MethodPost, "/api/rules", rule, http.StatusOK, &savedRule)
	if savedRule.SortBy != "source_order" || savedRule.SortDirection != "asc" || savedRule.Action != "download" {
		t.Fatalf("rule defaults were not persisted in the API result: %#v", savedRule)
	}

	invalidSubscription := core.Subscription{
		ID: "invalid", Name: "Invalid", RuleName: rule.Name, SiteIDs: []string{"test-site"},
		Download: core.DownloadOptions{QBCategory: "PT/ASMR", FilenameTemplate: "{{unknown}}"},
	}
	task1JSONRequest(t, handler, http.MethodPost, "/api/subscriptions", invalidSubscription, http.StatusBadRequest, nil)

	subscription := core.Subscription{
		ID: "anime-sub", Name: "Anime subscription", RuleName: rule.Name, Enabled: false,
		SiteIDs: []string{"test-site"}, Priority: 20,
		Download: core.DownloadOptions{
			QBCategory: "PT/ASMR", SavePathTemplate: "D:/Downloads/{{site_id}}",
			QBTags: []string{"anime"}, FilenameTemplate: "{{title}}", Paused: true, MaxConcurrent: 2, DailyLimit: 5,
		},
	}
	var savedSubscription core.Subscription
	task1JSONRequest(t, handler, http.MethodPost, "/api/subscriptions", subscription, http.StatusOK, &savedSubscription)
	if savedSubscription.ID != subscription.ID || savedSubscription.Priority != 20 || savedSubscription.Download.DailyLimit != 5 {
		t.Fatalf("unexpected saved subscription: %#v", savedSubscription)
	}
	var subscriptions []core.Subscription
	task1JSONRequest(t, handler, http.MethodGet, "/api/subscriptions", nil, http.StatusOK, &subscriptions)
	if len(subscriptions) != 1 || subscriptions[0].ID != subscription.ID {
		t.Fatalf("unexpected subscription list: %#v", subscriptions)
	}
	renamedRule := savedRule
	renamedRule.Name = "Anime Renamed"
	task1JSONRequest(t, handler, http.MethodPut, "/api/rules/"+rule.Name, renamedRule, http.StatusOK, &savedRule)
	task1JSONRequest(t, handler, http.MethodGet, "/api/subscriptions", nil, http.StatusOK, &subscriptions)
	if len(subscriptions) != 1 || subscriptions[0].RuleName != renamedRule.Name {
		t.Fatalf("rule rename must cascade to subscriptions: %#v", subscriptions)
	}
	task1JSONRequest(t, handler, http.MethodDelete, "/api/rules/Anime%20Renamed", nil, http.StatusBadRequest, nil)

	var defaultSchedule core.SiteSchedule
	task1JSONRequest(t, handler, http.MethodGet, "/api/sites/test-site/schedule", nil, http.StatusOK, &defaultSchedule)
	if defaultSchedule.Enabled || defaultSchedule.IntervalSeconds != 15*60 {
		t.Fatalf("unexpected default site schedule: %#v", defaultSchedule)
	}
	var savedSchedule core.SiteSchedule
	task1JSONRequest(t, handler, http.MethodPost, "/api/sites/test-site/schedule", core.SiteSchedule{Enabled: true, IntervalSeconds: 60}, http.StatusOK, &savedSchedule)
	if !savedSchedule.Enabled || savedSchedule.IntervalSeconds != 60 || savedSchedule.NextRunAt == nil {
		t.Fatalf("unexpected saved site schedule: %#v", savedSchedule)
	}
	task1JSONRequest(t, handler, http.MethodPost, "/api/sites/test-site/schedule", core.SiteSchedule{Enabled: true, IntervalSeconds: 59}, http.StatusBadRequest, nil)

	var categories core.QBCategoriesResult
	task1JSONRequest(t, handler, http.MethodGet, "/api/qb/categories?refresh=true", nil, http.StatusOK, &categories)
	if len(categories.Items) != 1 || categories.Items[0].Name != "PT/ASMR" || !reflect.DeepEqual(categories.Items[0].PathSegments, []string{"PT", "ASMR"}) {
		t.Fatalf("unexpected category tree data: %#v", categories)
	}
	var createdCategories core.QBCategoriesResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/categories", core.QBCategory{Name: "PT/New/Leaf", SavePath: "D:/Leaf"}, http.StatusCreated, &createdCategories)
	if got := fake.createdCategoryNames(); !reflect.DeepEqual(got, []string{"PT/New/Leaf"}) {
		t.Fatalf("only the submitted full category may be created, got %v", got)
	}
	if !task1CategoryExists(createdCategories.Items, "PT/New/Leaf", []string{"PT", "New", "Leaf"}) {
		t.Fatalf("created category missing from refreshed result: %#v", createdCategories)
	}

	var tags core.QBTagsResult
	task1JSONRequest(t, handler, http.MethodGet, "/api/qb/tags?refresh=true", nil, http.StatusOK, &tags)
	if !reflect.DeepEqual(tags.Items, []string{"existing-tag"}) {
		t.Fatalf("unexpected initial qB tags: %#v", tags)
	}
	var createdTags core.QBTagsResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/tags", map[string]any{"tags": []string{"new-tag", "EXISTING-TAG"}}, http.StatusCreated, &createdTags)
	if !reflect.DeepEqual(createdTags.Items, []string{"existing-tag", "new-tag"}) {
		t.Fatalf("unexpected created tag result: %#v", createdTags)
	}
	fake.server.Close()
	var staleCategories core.QBCategoriesResult
	task1JSONRequest(t, handler, http.MethodGet, "/api/qb/categories?refresh=true", nil, http.StatusOK, &staleCategories)
	if !staleCategories.Stale || staleCategories.Connected || staleCategories.Error == "" || len(staleCategories.Items) != 2 {
		t.Fatalf("offline qB must return the latest category snapshot with stale/error: %#v", staleCategories)
	}
	var staleTags core.QBTagsResult
	task1JSONRequest(t, handler, http.MethodGet, "/api/qb/tags?refresh=true", nil, http.StatusOK, &staleTags)
	if !staleTags.Stale || staleTags.Connected || staleTags.Error == "" || !reflect.DeepEqual(staleTags.Items, []string{"existing-tag", "new-tag"}) {
		t.Fatalf("offline qB must return the latest tag snapshot with stale/error: %#v", staleTags)
	}

	task1JSONRequest(t, handler, http.MethodDelete, "/api/subscriptions/"+subscription.ID, nil, http.StatusOK, nil)
	task1JSONRequest(t, handler, http.MethodDelete, "/api/rules/Anime%20Renamed", nil, http.StatusOK, nil)
	task1JSONRequest(t, handler, http.MethodGet, "/api/subscriptions", nil, http.StatusOK, &subscriptions)
	if len(subscriptions) != 0 {
		t.Fatalf("subscription should be deleted: %#v", subscriptions)
	}
	task1JSONRequest(t, handler, http.MethodGet, "/api/rules", nil, http.StatusOK, &rules)
	if len(rules) != 0 {
		t.Fatalf("rule should be deleted: %#v", rules)
	}
	fake.assertNoContractErrors(t)
}

// TestTask1BatchDownloadAndRetryAPI 精确验证批量下载、exists 和失败重试的 qB 契约。
func TestTask1BatchDownloadAndRetryAPI(t *testing.T) {
	const (
		firstData  = "d4:infod6:lengthi0e4:name5:first12:piece lengthi16384e6:pieces0:ee"
		retryData  = "d4:infod6:lengthi0e4:name5:retry12:piece lengthi16384e6:pieces0:ee"
		existsData = "d4:infod6:lengthi0e4:name6:exists12:piece lengthi16384e6:pieces0:ee"
		pureV2Data = "d4:infod9:file treede12:meta versioni2e4:name4:test12:piece lengthi16384eee"
	)
	metadata := map[string]qbittorrent.TorrentMetadata{}
	for id, data := range map[string]string{"1": firstData, "2": retryData, "3": existsData, "4": pureV2Data} {
		parsed, err := qbittorrent.ParseTorrentMetadata([]byte(data))
		if err != nil {
			t.Fatalf("parse fixture torrent %s: %v", id, err)
		}
		metadata[id] = parsed
	}

	fake := newTask1FakeQB(t)
	fake.tags = append(fake.tags, "global-tag")
	fake.failNextAdd(metadata["2"].Hashes.V1)
	fake.addExisting(qbittorrent.TorrentInfo{
		Hash: metadata["3"].Hashes.V1, Name: "Already present", Category: "PT/ASMR",
		Tags: "global-tag", SavePath: "D:/Existing", ContentPath: "D:/Existing/exists", Progress: 0.5, AmountLeft: 1,
	})
	handler := newTask1API(t, fake, []string{"global-tag"}, func(store *storage.SQLiteStore) {
		for _, item := range []struct {
			id, title, data string
		}{
			{id: "1", title: "First Title", data: firstData},
			{id: "2", title: "Retry Title", data: retryData},
			{id: "3", title: "Exists Title", data: existsData},
			{id: "4", title: "Pure V2 Title", data: pureV2Data},
		} {
			mustUpsertTask1Torrents(t, store, storage.TorrentRecord{
				SiteID: "test-site", TorrentID: item.id, SourceOrder: 0, Title: item.title, Category: "anime",
				DownloadURL: "https://example.invalid/download.php?id=" + item.id,
			})
			parsed := metadata[item.id]
			if err := store.SaveTorrentFile(t.Context(), storage.TorrentFileRecord{
				SiteID: "test-site", TorrentID: item.id, Data: []byte(item.data),
				InfoHashV1: parsed.Hashes.V1, InfoHashV2: parsed.Hashes.V2, OriginalName: parsed.Name, FetchedAt: time.Now(),
			}); err != nil {
				t.Fatalf("save torrent file %s: %v", item.id, err)
			}
		}
	})

	options := core.DownloadOptions{
		QBCategory: "PT/ASMR", SavePathTemplate: "D:/Downloads/{{site_id}}",
		QBTags: []string{"planned-tag"}, FilenameTemplate: "{{title}}", Paused: true,
	}
	collisionOptions := options
	collisionOptions.FilenameTemplate = "Same Name"
	var collisionPreview core.BatchDownloadPreview
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch/preview", core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{{SiteID: "test-site", TorrentID: "1"}, {SiteID: "test-site", TorrentID: "2"}},
		Options:  &collisionOptions,
	}, http.StatusOK, &collisionPreview)
	if len(collisionPreview.Items) != 2 || collisionPreview.Items[0].Plan.Rename != "Same Name" ||
		collisionPreview.Items[1].Plan.Rename != "Same Name [test-site-2]" ||
		!containsTask1Reason(collisionPreview.Items[1].Plan.Reasons, "rename_conflict_adjusted") {
		t.Fatalf("same-batch rename conflict must receive a stable suffix: %#v", collisionPreview)
	}
	previewRequest := core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{
			{SiteID: "test-site", TorrentID: "1"}, {SiteID: "test-site", TorrentID: "2"},
			{SiteID: "test-site", TorrentID: "3"}, {SiteID: "test-site", TorrentID: "4"},
		},
		Options: &options,
	}
	var preview core.BatchDownloadPreview
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch/preview", previewRequest, http.StatusOK, &preview)
	if len(preview.Items) != 4 {
		t.Fatalf("unexpected batch preview: %#v", preview)
	}
	for index := 0; index < 3; index++ {
		if !preview.Items[index].Eligible {
			t.Fatalf("preview item %d should be eligible: %#v", index, preview.Items[index])
		}
	}
	if preview.Items[3].Eligible || !containsTask1Reason(preview.Items[3].Reasons, "pure_v2_unsupported") {
		t.Fatalf("pure v2 preview must be explicitly blocked: %#v", preview.Items[3])
	}
	if plan := preview.Items[0].Plan; plan.Category != "PT/ASMR" || plan.SavePath != "D:/Downloads/test-site" ||
		!reflect.DeepEqual(plan.Tags, []string{"global-tag", "planned-tag"}) || plan.Rename != "First Title" || !plan.Paused || plan.AutoTMM == nil || *plan.AutoTMM {
		t.Fatalf("unexpected first download plan: %#v", plan)
	}
	var tasksBefore []core.DownloadTask
	task1JSONRequest(t, handler, http.MethodGet, "/api/download-tasks", nil, http.StatusOK, &tasksBefore)
	if len(tasksBefore) != 0 {
		t.Fatalf("batch preview must not create tasks: %#v", tasksBefore)
	}
	missingCategoryOptions := options
	missingCategoryOptions.QBCategory = "PT/Missing"
	var missingCategoryResult core.BatchDownloadResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch", core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{{SiteID: "test-site", TorrentID: "3"}}, Options: &missingCategoryOptions,
	}, http.StatusAccepted, &missingCategoryResult)
	if missingCategoryResult.Skipped != 1 || len(missingCategoryResult.Tasks) != 1 ||
		missingCategoryResult.Tasks[0].Status != "skipped" || missingCategoryResult.Tasks[0].ReasonCode != "qb_category_missing" {
		t.Fatalf("missing qB category must remain a retryable skipped plan: %#v", missingCategoryResult)
	}

	request := core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{{SiteID: "test-site", TorrentID: "1"}, {SiteID: "test-site", TorrentID: "2"}},
		Options:  &options,
	}
	var executed core.BatchDownloadResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch", request, http.StatusAccepted, &executed)
	if executed.Attempted != 2 || executed.Sent != 1 || executed.Failed != 1 || len(executed.Tasks) != 2 {
		t.Fatalf("unexpected batch execution result: %#v", executed)
	}
	failed := task1TaskByTorrentID(t, executed.Tasks, "2")
	if failed.Status != "failed" || failed.ReasonCode != "send_failed" || !strings.Contains(failed.Error, "intentional fake add failure") {
		t.Fatalf("qB response body must be persisted on real add failure: %#v", failed)
	}
	forms := fake.addFormsSnapshot()
	if len(forms) != 2 {
		t.Fatalf("expected two initial qB add requests, got %d", len(forms))
	}
	assertTask1AddForm(t, forms[0], "First Title")
	assertTask1AddForm(t, forms[1], "Retry Title")
	if got := fake.createdTagsSnapshot(); !reflect.DeepEqual(got, []string{"planned-tag"}) {
		t.Fatalf("missing planned tag should be created once, got %v", got)
	}

	var retried core.DownloadTask
	task1JSONRequest(t, handler, http.MethodPost, "/api/download-tasks/test-site:2:manual/retry", nil, http.StatusAccepted, &retried)
	if retried.Status != "sent" || retried.RetryCount != 1 || retried.QBHash != metadata["2"].Hashes.V1 {
		t.Fatalf("unexpected retried task: %#v", retried)
	}
	forms = fake.addFormsSnapshot()
	if len(forms) != 3 {
		t.Fatalf("expected retry to issue one more qB add request, got %d", len(forms))
	}
	assertTask1AddForm(t, forms[2], "Retry Title")

	formsBeforeExists := len(forms)
	var existsResult core.BatchDownloadResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch", core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{{SiteID: "test-site", TorrentID: "3"}}, Options: &options,
	}, http.StatusAccepted, &existsResult)
	if existsResult.Attempted != 1 || existsResult.Exists != 1 || len(existsResult.Tasks) != 1 || existsResult.Tasks[0].Status != "exists" {
		t.Fatalf("pre-existing qB hash must persist exists without modification: %#v", existsResult)
	}
	if got := len(fake.addFormsSnapshot()); got != formsBeforeExists {
		t.Fatalf("existing hash must not upload a torrent; add requests before=%d after=%d", formsBeforeExists, got)
	}

	var pureV2Result core.BatchDownloadResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch", core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{{SiteID: "test-site", TorrentID: "4"}}, Options: &options,
	}, http.StatusAccepted, &pureV2Result)
	if pureV2Result.Failed != 1 || len(pureV2Result.Tasks) != 1 || pureV2Result.Tasks[0].Status != "failed" ||
		pureV2Result.Tasks[0].ReasonCode != "pure_v2_unsupported" {
		t.Fatalf("pure v2 execution must fail explicitly: %#v", pureV2Result)
	}
	if got := len(fake.addFormsSnapshot()); got != formsBeforeExists {
		t.Fatalf("pure v2 failure must happen before qB upload; before=%d after=%d", formsBeforeExists, got)
	}

	var repeated core.BatchDownloadResult
	task1JSONRequest(t, handler, http.MethodPost, "/api/downloads/batch", core.BatchDownloadRequest{
		Torrents: []core.TorrentKey{{SiteID: "test-site", TorrentID: "1"}}, Options: &options,
	}, http.StatusAccepted, &repeated)
	if repeated.Sent != 1 || repeated.Attempted != 0 || len(repeated.Tasks) != 1 || repeated.Tasks[0].Status != "sent" {
		t.Fatalf("idempotent replay should return the saved sent task: %#v", repeated)
	}
	if got := len(fake.addFormsSnapshot()); got != formsBeforeExists {
		t.Fatalf("idempotent replay must not add again; before=%d after=%d", formsBeforeExists, got)
	}
	fake.assertNoContractErrors(t)
}

type task1FakeQB struct {
	mu                sync.Mutex
	server            *httptest.Server
	categories        map[string]string
	tags              []string
	torrents          map[string]qbittorrent.TorrentInfo
	failAdds          map[string]int
	addForms          []map[string]string
	createdCategories []string
	createdTags       []string
	contractErrors    []string
	requests          int
}

func newTask1FakeQB(t *testing.T) *task1FakeQB {
	t.Helper()
	fake := &task1FakeQB{
		categories: map[string]string{"PT/ASMR": "D:/ASMR"},
		tags:       []string{"existing-tag"},
		torrents:   map[string]qbittorrent.TorrentInfo{},
		failAdds:   map[string]int{},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *task1FakeQB) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer test-key" {
		f.recordContractError("missing Authorization bearer header for " + r.URL.Path)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/api/v2/torrents/categories":
		f.mu.Lock()
		response := make(map[string]map[string]string, len(f.categories))
		for name, savePath := range f.categories {
			response[name] = map[string]string{"name": name, "savePath": savePath}
		}
		f.mu.Unlock()
		writeTask1FakeJSON(w, http.StatusOK, response)
	case "/api/v2/torrents/createCategory":
		_ = r.ParseForm()
		name := r.FormValue("category")
		f.mu.Lock()
		f.categories[name] = r.FormValue("savePath")
		f.createdCategories = append(f.createdCategories, name)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case "/api/v2/torrents/tags":
		f.mu.Lock()
		tags := append([]string(nil), f.tags...)
		f.mu.Unlock()
		writeTask1FakeJSON(w, http.StatusOK, tags)
	case "/api/v2/torrents/createTags":
		_ = r.ParseForm()
		values := splitTask1CSV(r.FormValue("tags"))
		f.mu.Lock()
		for _, tag := range values {
			if !containsTask1Fold(f.tags, tag) {
				f.tags = append(f.tags, tag)
				f.createdTags = append(f.createdTags, tag)
			}
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case "/api/v2/torrents/info":
		f.mu.Lock()
		items := make([]qbittorrent.TorrentInfo, 0, len(f.torrents))
		requested := splitTask1Hashes(r.URL.Query().Get("hashes"))
		if len(requested) == 0 {
			for _, torrent := range f.torrents {
				items = append(items, torrent)
			}
		} else {
			for _, hash := range requested {
				if torrent, ok := f.torrents[strings.ToLower(hash)]; ok {
					items = append(items, torrent)
				}
			}
		}
		f.mu.Unlock()
		sort.Slice(items, func(i, j int) bool { return items[i].Hash < items[j].Hash })
		writeTask1FakeJSON(w, http.StatusOK, items)
	case "/api/v2/torrents/properties":
		writeTask1FakeJSON(w, http.StatusOK, map[string]any{})
	case "/api/v2/torrents/add":
		f.handleAdd(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *task1FakeQB) handleAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		http.Error(w, "invalid multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	form := map[string]string{}
	for _, key := range []string{"savepath", "category", "tags", "paused", "rename", "autoTMM"} {
		form[key] = r.FormValue(key)
	}
	file, _, err := r.FormFile("torrents")
	if err != nil {
		http.Error(w, "torrent file missing", http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		http.Error(w, "read torrent file", http.StatusBadRequest)
		return
	}
	metadata, err := qbittorrent.ParseTorrentMetadata(data)
	if err != nil || metadata.Hashes.V1 == "" {
		http.Error(w, "invalid v1 torrent", http.StatusBadRequest)
		return
	}
	hash := strings.ToLower(metadata.Hashes.V1)
	f.mu.Lock()
	f.addForms = append(f.addForms, form)
	if f.failAdds[hash] > 0 {
		f.failAdds[hash]--
		f.mu.Unlock()
		http.Error(w, "intentional fake add failure", http.StatusInternalServerError)
		return
	}
	f.torrents[hash] = qbittorrent.TorrentInfo{
		Hash: hash, Name: form["rename"], Category: form["category"], Tags: form["tags"],
		SavePath: form["savepath"], ContentPath: form["savepath"] + "/" + form["rename"], State: "stoppedDL", Progress: 0, AmountLeft: 1,
	}
	f.mu.Unlock()
	writeTask1FakeJSON(w, http.StatusAccepted, map[string]any{
		"added_torrent_ids": []string{hash}, "failure_count": 0, "pending_count": 0, "success_count": 1,
	})
}

func (f *task1FakeQB) failNextAdd(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAdds[strings.ToLower(hash)]++
}

func (f *task1FakeQB) addExisting(torrent qbittorrent.TorrentInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.torrents[strings.ToLower(torrent.Hash)] = torrent
}

func (f *task1FakeQB) addFormsSnapshot() []map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]map[string]string, 0, len(f.addForms))
	for _, form := range f.addForms {
		clone := map[string]string{}
		for key, value := range form {
			clone[key] = value
		}
		result = append(result, clone)
	}
	return result
}

func (f *task1FakeQB) createdCategoryNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.createdCategories...)
}

func (f *task1FakeQB) createdTagsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.createdTags...)
}

func (f *task1FakeQB) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func (f *task1FakeQB) recordContractError(message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contractErrors = append(f.contractErrors, message)
}

func (f *task1FakeQB) assertNoContractErrors(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.contractErrors) != 0 {
		t.Fatalf("fake qB contract errors: %v", f.contractErrors)
	}
}

func newTask1API(t *testing.T, fake *task1FakeQB, globalTags []string, seed func(*storage.SQLiteStore)) http.Handler {
	t.Helper()
	tempDir := t.TempDir()
	sitesDir := filepath.Join(tempDir, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatalf("create sites directory: %v", err)
	}
	definition, err := os.ReadFile(filepath.Join("fixtures", "site_parse_config.json"))
	if err != nil {
		t.Fatalf("read site fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sitesDir, "test-site.json"), definition, 0o600); err != nil {
		t.Fatalf("write site fixture: %v", err)
	}
	dbPath := filepath.Join(tempDir, "nexusbridge.db")
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open seed sqlite: %v", err)
	}
	if seed != nil {
		seed(store)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed sqlite: %v", err)
	}
	cfg := config.Default()
	cfg.Storage.Path = dbPath
	cfg.SitesDir = sitesDir
	cfg.QBittorrent = config.QBittorrentConfig{
		AuthMode: "api_key", URL: fake.server.URL, APIKey: "test-key", Tags: append([]string(nil), globalTags...), AutoSync: false,
	}
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatalf("new task1 app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return httpserver.New(cfg, app, "").Handler()
}

func task1JSONRequest(t *testing.T, handler http.Handler, method, path string, input any, wantStatus int, output any) {
	t.Helper()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("marshal %s %s request: %v", method, path, err)
		}
		body = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, body)
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status: want %d, got %d: %s", method, path, wantStatus, response.Code, response.Body.String())
	}
	if output != nil {
		if err := json.Unmarshal(response.Body.Bytes(), output); err != nil {
			t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, response.Body.String())
		}
	}
}

func mustUpsertTask1Torrents(t *testing.T, store *storage.SQLiteStore, records ...storage.TorrentRecord) {
	t.Helper()
	if _, err := store.UpsertTorrents(t.Context(), records); err != nil {
		t.Fatalf("upsert task1 torrents: %v", err)
	}
}

func writeTask1FakeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func assertTask1AddForm(t *testing.T, form map[string]string, rename string) {
	t.Helper()
	want := map[string]string{
		"savepath": "D:/Downloads/test-site", "category": "PT/ASMR", "tags": "global-tag,planned-tag",
		"paused": "true", "rename": rename, "autoTMM": "false",
	}
	if !reflect.DeepEqual(form, want) {
		t.Fatalf("unexpected qB multipart fields:\nwant %#v\n got %#v", want, form)
	}
}

func task1TaskByTorrentID(t *testing.T, tasks []core.DownloadTask, torrentID string) core.DownloadTask {
	t.Helper()
	for _, task := range tasks {
		if task.TorrentID == torrentID {
			return task
		}
	}
	t.Fatalf("task for torrent %s not found in %#v", torrentID, tasks)
	return core.DownloadTask{}
}

func task1CategoryExists(categories []core.QBCategory, name string, segments []string) bool {
	for _, category := range categories {
		if category.Name == name && reflect.DeepEqual(category.PathSegments, segments) {
			return true
		}
	}
	return false
}

func containsTask1Reason(reasons []core.RuleReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func splitTask1CSV(raw string) []string {
	result := []string{}
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func splitTask1Hashes(raw string) []string {
	result := []string{}
	for _, value := range strings.Split(raw, "|") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func containsTask1Fold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func (f *task1FakeQB) String() string {
	return fmt.Sprintf("task1FakeQB(%s)", f.server.URL)
}
