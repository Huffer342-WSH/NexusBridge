package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadSiteDefinition 从文件读取站点定义。
func LoadSiteDefinition(path string) (SiteDefinition, error) {
	if strings.TrimSpace(path) == "" {
		return SiteDefinition{}, fmt.Errorf("site definition path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SiteDefinition{}, fmt.Errorf("read site definition: %w", err)
	}
	return DecodeSiteDefinition(data)
}

// ExportSiteDefinition 将新格式站点定义写入 JSON 文件。
func ExportSiteDefinition(path string, definition SiteDefinition) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// UpdateSiteDefinitionSearchOptions 用真实页面解析出的搜索选项更新新格式站点定义。
func UpdateSiteDefinitionSearchOptions(definition SiteDefinition, page ParsedPage) SiteDefinition {
	definition = NormalizeSiteDefinition(definition)
	if len(definition.HTML.Search.Paths) == 0 && strings.TrimSpace(page.SiteConfig.SearchPath) != "" {
		definition.HTML.Search.Paths = []SiteSearchPath{{
			Path:   strings.TrimPrefix(page.SiteConfig.SearchPath, "/"),
			Method: "get",
		}}
	}
	if definition.HTML.Search.Params == nil {
		definition.HTML.Search.Params = map[string]interface{}{}
	}
	if keywordName := strings.TrimSpace(page.SearchConfig.Keyword.Name); keywordName != "" {
		if _, exists := definition.HTML.Search.Params[keywordName]; !exists {
			definition.HTML.Search.Params[keywordName] = "{keyword}"
		}
	}
	if !page.SearchConfig.Fields.Empty() {
		fields := page.SearchConfig.Fields
		if fields.Page == nil {
			fields.Page = definition.HTML.Search.Fields.Page
		}
		definition.HTML.Search.Fields = fields
	}
	if len(definition.HTML.Category) == 0 || len(page.SearchConfig.Categories) == 0 {
		return definition
	}
	parsedCategories := map[string]SearchOption{}
	for _, option := range page.SearchConfig.Categories {
		id := categoryIDFromSearchOption(option)
		if id != "" {
			parsedCategories[id] = option
		}
	}
	for group, categories := range definition.HTML.Category {
		for i := range categories {
			id := strconv.Itoa(categories[i].ID)
			option, ok := parsedCategories[id]
			if !ok {
				continue
			}
			if label := strings.TrimSpace(option.Label); label != "" {
				categories[i].Desc = label
			}
			if searchField := strings.TrimSpace(option.QueryName); searchField != "" {
				categories[i].SearchField = searchField
			}
		}
		definition.HTML.Category[group] = categories
	}
	return definition
}

func categoryIDFromSearchOption(option SearchOption) string {
	for _, value := range []string{option.QueryName, option.Name, option.Value, option.Query} {
		matches := categoryIDRE.FindString(value)
		if matches != "" {
			return matches
		}
	}
	return ""
}
