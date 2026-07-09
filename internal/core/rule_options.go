package core

import (
	"context"
	"strings"

	"nexusbridge/internal/parser"
)

// RuleFilterOptions 返回单站点规则编辑器使用的本地分类、标签和促销选项。
func (a *App) RuleFilterOptions(ctx context.Context, siteID string) (RuleFilterOptions, error) {
	site, err := a.findSite(strings.TrimSpace(siteID))
	if err != nil {
		return RuleFilterOptions{}, err
	}
	facets, err := a.store.ListRuleFacets(ctx, site.ID)
	if err != nil {
		return RuleFilterOptions{}, err
	}
	result := RuleFilterOptions{
		SiteID:         site.ID,
		SiteCategories: stringRuleOptions(facets.SiteCategories),
		SiteTags:       stringRuleOptions(facets.SiteTags),
		SubtitleTags:   stringRuleOptions(facets.SubtitleTags),
		Promotions:     promotionRuleOptions(site.Definition.HTML.Search.Fields),
	}
	return result, nil
}

func stringRuleOptions(values []string) []RuleSelectOption {
	result := make([]RuleSelectOption, 0, len(values))
	for _, value := range values {
		result = append(result, RuleSelectOption{Value: value, Label: value})
	}
	return result
}

func promotionRuleOptions(fields parser.SiteSearchFields) []RuleSelectOption {
	result := []RuleSelectOption{}
	for _, field := range fields.Selects {
		if !strings.EqualFold(strings.TrimSpace(field.Role), "promotion") {
			continue
		}
		for _, option := range field.Options {
			value := strings.TrimSpace(option.FilterValue)
			if value == "" || strings.EqualFold(value, "all") {
				continue
			}
			result = append(result, RuleSelectOption{Value: value, Label: option.Label})
		}
	}
	return result
}
