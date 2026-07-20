package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"nexusbridge/internal/storage"
)

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
