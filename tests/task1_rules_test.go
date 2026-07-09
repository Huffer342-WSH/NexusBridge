// task1_rules_test.go 验证任务 1 的筛选规则和种子增量入库语义。
package tests

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"nexusbridge/internal/core"
	"nexusbridge/internal/storage"
)

// TestRuleMatchAndTorrentUpsert 覆盖完整筛选、稳定原因、默认排序及种子增量入库。
func TestRuleMatchAndTorrentUpsert(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	publishedAt := now.Add(-30 * time.Minute)
	rule := core.Rule{
		Name:                   "完整规则",
		SiteIDs:                []string{"Demo"},
		SiteCategories:         []string{"movie", "Anime"},
		SubtitleTags:           []string{"4K", "HDR"},
		SiteTags:               []string{"tag-10", "TAG-20"},
		TitleExpression:        "wanted&!blocked",
		Promotions:             []string{"pro_free", "pro_2up"},
		MinSize:                100,
		MaxSize:                200,
		MinSeeders:             2,
		MaxSeeders:             20,
		MinLeechers:            1,
		MaxLeechers:            10,
		MinSnatches:            3,
		MaxSnatches:            30,
		PublishedWithinMinutes: 60,
	}
	torrent := core.Torrent{
		ID:             "100",
		SiteID:         "demo",
		Category:       "anime",
		CategoryQuery:  "anime-hd",
		Title:          "A WANTED release",
		Tags:           []string{"hdr", "4k", "extra"},
		TagIDs:         []string{"TAG-20", "tag-10", "tag-30"},
		Promotion:      "FREE",
		PromotionClass: "pro_free",
		SizeBytes:      150,
		Seeders:        8,
		Leechers:       4,
		Snatches:       12,
		PublishedAt:    &publishedAt,
	}

	matched := core.EvaluateRule(rule, torrent, now)
	if !matched.Matched || !matched.Eligible || matched.Reason != "matched" || len(matched.Reasons) != 0 {
		t.Fatalf("expected all rule fields to match, got %#v", matched)
	}

	tooOld := now.Add(-2 * time.Hour)
	rejectedTorrent := torrent
	rejectedTorrent.SiteID = "outside"
	rejectedTorrent.Category = "music"
	rejectedTorrent.Tags = []string{"4k"}
	rejectedTorrent.TagIDs = []string{"tag-10"}
	rejectedTorrent.Title = "blocked release"
	rejectedTorrent.Promotion = "normal"
	rejectedTorrent.PromotionClass = ""
	rejectedTorrent.SizeBytes = 99
	rejectedTorrent.Seeders = 1
	rejectedTorrent.Leechers = 0
	rejectedTorrent.Snatches = 2
	rejectedTorrent.PublishedAt = &tooOld
	rejected := core.EvaluateRule(rule, rejectedTorrent, now)
	wantReasons := []string{
		"site_not_included",
		"site_category_not_included",
		"site_tag_missing",
		"subtitle_tag_missing",
		"title_expression_not_matched",
		"promotion_not_matched",
		"size_below_minimum",
		"seeders_below_minimum",
		"leechers_below_minimum",
		"snatches_below_minimum",
		"published_too_old",
	}
	if rejected.Matched || rejected.Eligible || rejected.Reason != wantReasons[0] {
		t.Fatalf("expected rejected rule with first stable reason, got %#v", rejected)
	}
	if got := ruleReasonCodes(rejected.Reasons); !reflect.DeepEqual(got, wantReasons) {
		t.Fatalf("unexpected stable rejection reasons:\nwant %v\n got %v", wantReasons, got)
	}

	aboveMaximum := torrent
	aboveMaximum.SizeBytes = 201
	aboveMaximum.Seeders = 21
	aboveMaximum.Leechers = 11
	aboveMaximum.Snatches = 31
	upperRejected := core.EvaluateRule(rule, aboveMaximum, now)
	if got, want := ruleReasonCodes(upperRejected.Reasons), []string{
		"size_above_maximum", "seeders_above_maximum", "leechers_above_maximum", "snatches_above_maximum",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected upper-bound reasons: want %v, got %v", want, got)
	}

	missingPublishedAt := torrent
	missingPublishedAt.PublishedAt = nil
	if got := ruleReasonCodes(core.EvaluateRule(rule, missingPublishedAt, now).Reasons); !reflect.DeepEqual(got, []string{"published_at_missing"}) {
		t.Fatalf("unexpected missing published_at reason: %v", got)
	}
	expression, err := core.ParseTitleExpression(`A&B&C|D|E&!F&(!H|!I)|"A&B"`)
	if err != nil || !expression.Match("release A&B") || expression.Match("E F H I") {
		t.Fatalf("unexpected title expression behavior: match=%t rejected=%t err=%v", expression.Match("release A&B"), expression.Match("E F H I"), err)
	}
	if err := core.ValidateRule(core.Rule{Name: "invalid-site-scope", SiteIDs: []string{"one", "two"}, SiteCategories: []string{"anime"}}, true); err == nil {
		t.Fatal("site-derived filters must require exactly one site")
	}
	if err := core.ValidateRule(core.Rule{Name: "invalid-expression", TitleExpression: "A&("}, true); err == nil {
		t.Fatal("invalid title expression must be rejected")
	}

	ordered := []core.Torrent{
		{ID: "3", SiteID: "demo", SourceOrder: 2},
		{ID: "2", SiteID: "demo", SourceOrder: 1},
		{ID: "1", SiteID: "demo", SourceOrder: 1},
	}
	core.SortTorrentsForRule(ordered, core.Rule{})
	if got, want := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID}, []string{"1", "2", "3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default sort must preserve source_order with stable key fallback: want %v, got %v", want, got)
	}
	longRename := strings.Repeat("a", 240)
	resolvedRename := core.ResolveRenameConflict(longRename, strings.Repeat("s", 240), "100")
	if utf8.RuneCountInString(resolvedRename) > 240 || !strings.HasSuffix(resolvedRename, "]") {
		t.Fatalf("conflict suffix must remain intact within the 240-rune limit: %q", resolvedRename)
	}

	store, err := storage.OpenSQLite(t.Context(), filepath.Join(t.TempDir(), "task1.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	record := storage.TorrentRecord{
		SiteID: "demo", TorrentID: "100", SourceOrder: 7, Title: "initial", Category: "anime",
		DownloadURL: "https://example.invalid/download/100", Tags: []string{"4k"}, TagIDs: []string{"tag-10"},
		Promotion: "free", Subtitle: "subtitle", SizeBytes: 150, Seeders: 8, Leechers: 4, Snatches: 12,
	}
	inserted, err := store.UpsertTorrents(t.Context(), []storage.TorrentRecord{record})
	if err != nil {
		t.Fatalf("insert torrent: %v", err)
	}
	if inserted.Total != 1 || len(inserted.Inserted) != 1 || len(inserted.Changed) != 0 {
		t.Fatalf("expected one inserted torrent, got %#v", inserted)
	}
	pendingIngest, err := store.ListPendingSubscriptionIngest(t.Context(), record.SiteID, 10)
	if err != nil || len(pendingIngest) != 1 || pendingIngest[0].TorrentID != record.TorrentID || pendingIngest[0].SourceOrder != 7 {
		t.Fatalf("new torrent must enter durable subscription ingest queue: %#v err=%v", pendingIngest, err)
	}

	unchanged, err := store.UpsertTorrents(t.Context(), []storage.TorrentRecord{record})
	if err != nil {
		t.Fatalf("upsert unchanged torrent: %v", err)
	}
	if unchanged.Total != 1 || len(unchanged.Inserted) != 0 || len(unchanged.Changed) != 0 {
		t.Fatalf("unchanged torrent must not be classified as inserted or changed, got %#v", unchanged)
	}

	record.Title = "updated"
	record.SourceOrder = 3
	changed, err := store.UpsertTorrents(t.Context(), []storage.TorrentRecord{record})
	if err != nil {
		t.Fatalf("update torrent: %v", err)
	}
	if changed.Total != 1 || len(changed.Inserted) != 0 || len(changed.Changed) != 1 {
		t.Fatalf("expected one changed torrent, got %#v", changed)
	}
	stored, ok, err := store.GetTorrent(t.Context(), record.SiteID, record.TorrentID)
	if err != nil || !ok {
		t.Fatalf("get updated torrent: ok=%t err=%v", ok, err)
	}
	if stored.Title != "updated" || stored.SourceOrder != 3 {
		t.Fatalf("unexpected persisted update: %#v", stored)
	}
	pendingIngest, err = store.ListPendingSubscriptionIngest(t.Context(), record.SiteID, 10)
	if err != nil || len(pendingIngest) != 1 || pendingIngest[0].SourceOrder != 7 {
		t.Fatalf("later refresh must preserve the initial list order in ingest queue: %#v err=%v", pendingIngest, err)
	}
	if err := store.DeletePendingSubscriptionIngest(t.Context(), pendingIngest); err != nil {
		t.Fatalf("acknowledge subscription ingest queue: %v", err)
	}
	pendingIngest, err = store.ListPendingSubscriptionIngest(t.Context(), record.SiteID, 10)
	if err != nil || len(pendingIngest) != 0 {
		t.Fatalf("acknowledged subscription ingest must not replay: %#v err=%v", pendingIngest, err)
	}
}

func ruleReasonCodes(reasons []core.RuleReason) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, reason.Code)
	}
	return codes
}
