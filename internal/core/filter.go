package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// MatchRule 使用当前时间判断一个种子是否命中规则。
func MatchRule(rule Rule, torrent Torrent) RuleMatch {
	return EvaluateRule(rule, torrent, time.Now())
}

// EvaluateRule 返回规则的全部稳定拒绝原因。
func EvaluateRule(rule Rule, torrent Torrent, now time.Time) RuleMatch {
	reasons := make([]RuleReason, 0)
	add := func(code, field, message string) {
		reasons = append(reasons, RuleReason{Code: code, Field: field, Message: message})
	}
	if len(rule.SiteIDs) > 0 && !containsFold(rule.SiteIDs, torrent.SiteID) {
		add("site_not_included", "site_ids", "种子站点不在规则范围内")
	}
	if len(rule.SiteCategories) > 0 && !containsFold(rule.SiteCategories, torrent.Category) {
		add("site_category_not_included", "site_categories", "种子站点分类不在规则范围内")
	}
	for _, tag := range rule.SiteTags {
		if !containsFold(torrent.TagIDs, tag) {
			add("site_tag_missing", "site_tags", fmt.Sprintf("缺少站点标签 %q", tag))
		}
	}
	for _, tag := range rule.SubtitleTags {
		if !containsFold(torrent.Tags, tag) {
			add("subtitle_tag_missing", "subtitle_tags", fmt.Sprintf("缺少副标题标签 %q", tag))
		}
	}
	if expression, err := ParseTitleExpression(rule.TitleExpression); err != nil {
		add("title_expression_invalid", "title_expression", err.Error())
	} else if !expression.Match(torrent.Title) {
		add("title_expression_not_matched", "title_expression", "主标题不满足逻辑表达式")
	}
	if len(rule.Promotions) > 0 && !containsFold(rule.Promotions, torrentPromotionKey(torrent)) {
		add("promotion_not_matched", "promotions", "促销状态不匹配")
	}
	appendRangeReasons(&reasons, "size", torrent.SizeBytes, rule.MinSize, rule.MaxSize)
	appendRangeReasons(&reasons, "seeders", int64(torrent.Seeders), int64(rule.MinSeeders), int64(rule.MaxSeeders))
	appendRangeReasons(&reasons, "leechers", int64(torrent.Leechers), int64(rule.MinLeechers), int64(rule.MaxLeechers))
	appendRangeReasons(&reasons, "snatches", int64(torrent.Snatches), int64(rule.MinSnatches), int64(rule.MaxSnatches))
	if rule.PublishedWithinMinutes > 0 {
		if torrent.PublishedAt == nil || torrent.PublishedAt.IsZero() {
			add("published_at_missing", "published_within_minutes", "种子缺少发布时间")
		} else if now.Sub(*torrent.PublishedAt) > time.Duration(rule.PublishedWithinMinutes)*time.Minute {
			add("published_too_old", "published_within_minutes", "种子发布时间超过规则窗口")
		}
	}
	matched := len(reasons) == 0
	reason := "matched"
	if !matched {
		reason = reasons[0].Code
	}
	return RuleMatch{Rule: rule, Torrent: torrent, Matched: matched, Eligible: matched, Reason: reason, Reasons: reasons}
}

func torrentPromotionKey(torrent Torrent) string {
	class := strings.TrimSpace(torrent.PromotionClass)
	if class == "" {
		return "normal"
	}
	if fields := strings.Fields(class); len(fields) > 0 {
		return fields[0]
	}
	return class
}

func appendRangeReasons(reasons *[]RuleReason, field string, value, minValue, maxValue int64) {
	if minValue > 0 && value < minValue {
		*reasons = append(*reasons, RuleReason{Code: field + "_below_minimum", Field: "min_" + field, Message: field + " 低于最小值"})
	}
	if maxValue > 0 && value > maxValue {
		*reasons = append(*reasons, RuleReason{Code: field + "_above_maximum", Field: "max_" + field, Message: field + " 高于最大值"})
	}
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

// SortTorrentsForRule 按规则排序并用站点和种子 ID 保持结果稳定。
func SortTorrentsForRule(torrents []Torrent, rule Rule) {
	direction := strings.ToLower(strings.TrimSpace(rule.SortDirection))
	desc := direction == "desc"
	sortBy := strings.ToLower(strings.TrimSpace(rule.SortBy))
	if sortBy == "" {
		sortBy = "source_order"
	}
	sort.SliceStable(torrents, func(i, j int) bool {
		left, right := torrents[i], torrents[j]
		cmp := compareTorrentField(left, right, sortBy)
		if cmp == 0 {
			cmp = strings.Compare(strings.ToLower(left.SiteID+"\x00"+left.ID), strings.ToLower(right.SiteID+"\x00"+right.ID))
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareTorrentField(left, right Torrent, field string) int {
	switch field {
	case "published_at":
		return compareTimePtr(left.PublishedAt, right.PublishedAt)
	case "size_bytes":
		return compareInt64(left.SizeBytes, right.SizeBytes)
	case "seeders":
		return compareInt64(int64(left.Seeders), int64(right.Seeders))
	case "leechers":
		return compareInt64(int64(left.Leechers), int64(right.Leechers))
	case "snatches":
		return compareInt64(int64(left.Snatches), int64(right.Snatches))
	default:
		return compareInt64(int64(left.SourceOrder), int64(right.SourceOrder))
	}
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareTimePtr(left, right *time.Time) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if left.Before(*right) {
		return -1
	}
	if left.After(*right) {
		return 1
	}
	return 0
}
