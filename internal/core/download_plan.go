package core

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	defaultRuleSortBy        = "source_order"
	defaultRuleSortDirection = "asc"
	maxQBTagRunes            = 64
	maxQBRenameRunes         = 240
)

var templatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)
var subtitleTagPattern = regexp.MustCompile(`[\[【]([^\]】]+)[\]】]`)
var windowsReservedFilenamePattern = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)

var allowedTemplateVariables = map[string]struct{}{
	"site_id": {}, "site_name": {}, "torrent_id": {}, "category": {}, "category_query": {},
	"rule_name": {}, "subscription_name": {}, "title": {}, "detail_title": {}, "subtitle": {},
}

var allowedRuleSortFields = map[string]struct{}{
	"source_order": {}, "published_at": {}, "size_bytes": {}, "seeders": {}, "leechers": {}, "snatches": {},
}

// NormalizeRule 补齐规则默认值并清理集合字段。
func NormalizeRule(rule Rule) Rule {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.SiteIDs = normalizeStrings(rule.SiteIDs)
	rule.SiteCategories = normalizeStrings(rule.SiteCategories)
	rule.SiteTags = normalizeStrings(rule.SiteTags)
	rule.SubtitleTags = normalizeStrings(rule.SubtitleTags)
	rule.TitleExpression = strings.TrimSpace(rule.TitleExpression)
	rule.Promotions = normalizeStrings(rule.Promotions)
	rule.SortBy = strings.ToLower(strings.TrimSpace(rule.SortBy))
	if rule.SortBy == "" {
		rule.SortBy = defaultRuleSortBy
	}
	rule.SortDirection = strings.ToLower(strings.TrimSpace(rule.SortDirection))
	if rule.SortDirection == "" {
		rule.SortDirection = defaultRuleSortDirection
	}
	if rule.Action == "" {
		rule.Action = "download"
	}
	return rule
}

// ValidateRule 校验筛选规则；requireIdentity 控制草稿是否必须提供名称。
func ValidateRule(rule Rule, requireIdentity bool) error {
	rule = NormalizeRule(rule)
	if requireIdentity && rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if len(rule.SiteCategories)+len(rule.SiteTags)+len(rule.SubtitleTags)+len(rule.Promotions) > 0 && len(rule.SiteIDs) != 1 {
		return fmt.Errorf("site-specific filters require exactly one site_id")
	}
	if _, err := ParseTitleExpression(rule.TitleExpression); err != nil {
		return fmt.Errorf("title_expression: %w", err)
	}
	if _, ok := allowedRuleSortFields[rule.SortBy]; !ok {
		return fmt.Errorf("unsupported rule sort_by %q", rule.SortBy)
	}
	if rule.SortDirection != "asc" && rule.SortDirection != "desc" {
		return fmt.Errorf("rule sort_direction must be asc or desc")
	}
	if rule.Action != "" && rule.Action != "download" {
		return fmt.Errorf("rule action must be download")
	}
	values := []struct {
		name  string
		value int64
	}{
		{"min_size", rule.MinSize}, {"max_size", rule.MaxSize},
		{"min_seeders", int64(rule.MinSeeders)}, {"max_seeders", int64(rule.MaxSeeders)},
		{"min_leechers", int64(rule.MinLeechers)}, {"max_leechers", int64(rule.MaxLeechers)},
		{"min_snatches", int64(rule.MinSnatches)}, {"max_snatches", int64(rule.MaxSnatches)},
		{"published_within_minutes", int64(rule.PublishedWithinMinutes)},
	}
	for _, value := range values {
		if value.value < 0 {
			return fmt.Errorf("rule %s must be non-negative", value.name)
		}
	}
	if err := validateRange("size", rule.MinSize, rule.MaxSize); err != nil {
		return err
	}
	if err := validateRange("seeders", int64(rule.MinSeeders), int64(rule.MaxSeeders)); err != nil {
		return err
	}
	if err := validateRange("leechers", int64(rule.MinLeechers), int64(rule.MaxLeechers)); err != nil {
		return err
	}
	return validateRange("snatches", int64(rule.MinSnatches), int64(rule.MaxSnatches))
}

// NormalizeSubscription 补齐订阅字段默认值。
func NormalizeSubscription(subscription Subscription) Subscription {
	subscription.ID = strings.TrimSpace(subscription.ID)
	subscription.Name = strings.TrimSpace(subscription.Name)
	subscription.RuleName = strings.TrimSpace(subscription.RuleName)
	subscription.SiteIDs = normalizeStrings(subscription.SiteIDs)
	subscription.Download.QBCategory = strings.TrimSpace(subscription.Download.QBCategory)
	subscription.Download.SavePathTemplate = strings.TrimSpace(subscription.Download.SavePathTemplate)
	subscription.Download.QBTags = normalizeStrings(subscription.Download.QBTags)
	subscription.Download.FilenameTemplate = strings.TrimSpace(subscription.Download.FilenameTemplate)
	return subscription
}

// ValidateSubscription 校验订阅和下载模板。
func ValidateSubscription(subscription Subscription) error {
	subscription = NormalizeSubscription(subscription)
	if subscription.ID == "" || subscription.Name == "" || subscription.RuleName == "" {
		return fmt.Errorf("subscription id, name and rule_name are required")
	}
	if len(subscription.SiteIDs) == 0 {
		return fmt.Errorf("subscription site_ids is required")
	}
	if subscription.Enabled && subscription.Download.QBCategory == "" {
		return fmt.Errorf("enabled subscription qb_category is required")
	}
	if subscription.Download.MaxConcurrent < 0 || subscription.Download.DailyLimit < 0 {
		return fmt.Errorf("subscription limits must be non-negative")
	}
	if err := validateTemplate(subscription.Download.SavePathTemplate); err != nil {
		return fmt.Errorf("save_path_template: %w", err)
	}
	if err := validateTemplate(subscription.Download.FilenameTemplate); err != nil {
		return fmt.Errorf("filename_template: %w", err)
	}
	for index, tag := range subscription.Download.QBTags {
		if err := validateTemplate(tag); err != nil {
			return fmt.Errorf("qb_tags[%d]: %w", index, err)
		}
	}
	return nil
}

// BuildDownloadPlan 生成确定性的 qB 下载参数，不访问网络或持久化状态。
func BuildDownloadPlan(rule Rule, subscriptionName, siteName string, options DownloadOptions, torrent Torrent, originalName string, globalTags []string) (DownloadPlan, error) {
	values := map[string]string{
		"site_id": torrent.SiteID, "site_name": siteName, "torrent_id": torrent.ID,
		"category": torrent.Category, "category_query": torrent.CategoryQuery,
		"rule_name": rule.Name, "subscription_name": subscriptionName, "title": torrent.Title,
		"detail_title": torrent.DetailTitle, "subtitle": torrent.Subtitle,
	}
	savePath, err := renderTemplate(options.SavePathTemplate, values, sanitizePathValue)
	if err != nil {
		return DownloadPlan{}, err
	}
	rename, err := renderTemplate(options.FilenameTemplate, values, sanitizeFilename)
	if err != nil {
		return DownloadPlan{}, err
	}
	if rename != "" {
		rename = sanitizeFilename(rename)
	}
	renderedTags := make([]string, 0, len(options.QBTags))
	for _, tagTemplate := range options.QBTags {
		tag, err := renderTemplate(tagTemplate, values, sanitizeTag)
		if err != nil {
			return DownloadPlan{}, err
		}
		if tag != "" {
			renderedTags = append(renderedTags, tag)
		}
	}
	tags := mergeTags(globalTags, renderedTags, torrent.TagIDs, torrent.Tags, subtitleTags(torrent.Subtitle))
	plan := DownloadPlan{
		Category: strings.TrimSpace(options.QBCategory), SavePath: strings.TrimSpace(savePath), Tags: tags,
		Rename: rename, OriginalName: strings.TrimSpace(originalName), Paused: options.Paused, Reasons: []RuleReason{},
	}
	if plan.SavePath != "" {
		autoTMM := false
		plan.AutoTMM = &autoTMM
	}
	if plan.Rename == "" && plan.OriginalName == "" {
		plan.Reasons = append(plan.Reasons, RuleReason{Code: "original_name_unavailable", Field: "filename_template", Message: "原始 torrent 名称尚不可用"})
	}
	return plan, nil
}

// ResolveRenameConflict 为不同 hash 的同名任务追加稳定后缀。
func ResolveRenameConflict(rename, siteID, torrentID string) string {
	base := strings.TrimSpace(rename)
	if base == "" {
		return ""
	}
	suffix := truncateRunes(sanitizeFilename(siteID+"-"+torrentID), maxQBRenameRunes-4)
	maxBase := maxQBRenameRunes - utf8.RuneCountInString(suffix) - 3
	if maxBase < 1 {
		maxBase = 1
	}
	return truncateRunes(base, maxBase) + " [" + suffix + "]"
}

func validateRange(name string, minValue, maxValue int64) error {
	if maxValue > 0 && minValue > maxValue {
		return fmt.Errorf("rule min_%s must not exceed max_%s", name, name)
	}
	return nil
}

func validateTemplate(template string) error {
	if strings.TrimSpace(template) == "" {
		return nil
	}
	matches := templatePlaceholderPattern.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		if _, ok := allowedTemplateVariables[match[1]]; !ok {
			return fmt.Errorf("unknown placeholder %q", match[1])
		}
	}
	remaining := templatePlaceholderPattern.ReplaceAllString(template, "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return fmt.Errorf("invalid placeholder syntax")
	}
	return nil
}

func renderTemplate(template string, values map[string]string, sanitizer func(string) string) (string, error) {
	if err := validateTemplate(template); err != nil {
		return "", err
	}
	return templatePlaceholderPattern.ReplaceAllStringFunc(template, func(placeholder string) string {
		match := templatePlaceholderPattern.FindStringSubmatch(placeholder)
		return sanitizer(values[match[1]])
	}), nil
}

func sanitizePathValue(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' })
	for index := range parts {
		parts[index] = sanitizeFilename(parts[index])
	}
	return strings.Join(parts, "_")
}

func sanitizeFilename(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsControl(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(r)
	}
	cleaned := strings.TrimRight(builder.String(), ". ")
	base := cleaned
	if index := strings.IndexRune(base, '.'); index >= 0 {
		base = base[:index]
	}
	if cleaned == "." || cleaned == ".." || windowsReservedFilenamePattern.MatchString(base) {
		cleaned = "_" + cleaned
	}
	return truncateRunes(cleaned, maxQBRenameRunes)
}

func sanitizeTag(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsControl(r) || r == ',' {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(r)
	}
	return truncateRunes(strings.TrimSpace(builder.String()), maxQBTagRunes)
}

func subtitleTags(subtitle string) []string {
	result := []string{}
	for _, match := range subtitleTagPattern.FindAllStringSubmatch(subtitle, -1) {
		for _, value := range strings.FieldsFunc(match[1], func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '|'
		}) {
			if value = strings.TrimSpace(value); value != "" {
				result = append(result, value)
			}
		}
	}
	return result
}

func mergeTags(groups ...[]string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, group := range groups {
		for _, value := range group {
			tag := sanitizeTag(value)
			key := strings.ToLower(tag)
			if tag == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, tag)
		}
	}
	return result
}

// normalizeStrings 清理空值，并按不区分大小写的值去重。
func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
