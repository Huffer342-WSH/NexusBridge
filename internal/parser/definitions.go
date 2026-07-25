package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nexusbridge/internal/requestpolicy"
)

// DecodeSiteDefinition 解析新格式站点定义。
func DecodeSiteDefinition(data []byte) (SiteDefinition, error) {
	var definition SiteDefinition
	if err := json.Unmarshal(data, &definition); err != nil {
		return SiteDefinition{}, fmt.Errorf("parse site definition json: %w", err)
	}
	definition = NormalizeSiteDefinition(definition)
	if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.Domain) == "" {
		return SiteDefinition{}, fmt.Errorf("site definition must contain id and domain")
	}
	if err := validateAttendancePageURL(definition.Domain, definition.AttendancePageURL); err != nil {
		return SiteDefinition{}, err
	}
	if err := requestpolicy.ValidateRules(definition.RequestRules); err != nil {
		return SiteDefinition{}, err
	}
	return definition, nil
}

// LoadSiteDefinitionsDir 加载目录中的所有站点定义 JSON。
func LoadSiteDefinitionsDir(dir string) ([]SiteDefinition, error) {
	if strings.TrimSpace(dir) == "" {
		return []SiteDefinition{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SiteDefinition{}, nil
		}
		return nil, fmt.Errorf("read site definitions dir: %w", err)
	}
	definitions := make([]SiteDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		definition, err := LoadSiteDefinition(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool {
		return strings.ToLower(definitions[i].ID) < strings.ToLower(definitions[j].ID)
	})
	return definitions, nil
}

// NormalizeSiteDefinition 补齐新格式站点定义默认值。
func NormalizeSiteDefinition(definition SiteDefinition) SiteDefinition {
	definition.ID = strings.TrimSpace(definition.ID)
	definition.Name = strings.TrimSpace(firstNonEmpty(definition.Name, definition.ID))
	definition.Domain = normalizeBaseURL(definition.Domain)
	definition.AttendancePageURL = strings.TrimSpace(definition.AttendancePageURL)
	if definition.AttendancePageURL == "" {
		definition.AttendancePageURL = "attendance.php"
	}
	definition.RequestRules = requestpolicy.NormalizeRules(definition.RequestRules)
	definition.HTML.Torrents.List.Selector = normalizeNexusMediaTorrentRowsSelector(definition.HTML.Torrents.List.Selector)
	return definition
}

// validateAttendancePageURL 限制签到页面只能使用站点同源地址。
func validateAttendancePageURL(baseURL, attendancePageURL string) error {
	base, err := url.Parse(normalizeBaseURL(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("site definition domain must be an absolute URL")
	}
	target, err := url.Parse(strings.TrimSpace(attendancePageURL))
	if err != nil {
		return fmt.Errorf("parse attendance_page_url: %w", err)
	}
	if target.Host != "" && !target.IsAbs() {
		return fmt.Errorf("attendance_page_url must be relative or same-origin")
	}
	if target.IsAbs() && (!strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host)) {
		return fmt.Errorf("attendance_page_url must use the site origin")
	}
	return nil
}

// SiteConfigFromDefinition 生成运行时站点配置。
func SiteConfigFromDefinition(definition SiteDefinition) SiteConfig {
	definition = NormalizeSiteDefinition(definition)
	baseURL := normalizeBaseURL(definition.Domain)
	searchPath := "torrents.php"
	if len(definition.HTML.Search.Paths) > 0 && strings.TrimSpace(definition.HTML.Search.Paths[0].Path) != "" {
		searchPath = strings.TrimSpace(definition.HTML.Search.Paths[0].Path)
	}
	listPath := searchPath
	if strings.TrimSpace(definition.HTML.Browse.Path) != "" {
		listPath = strings.TrimSpace(definition.HTML.Browse.Path)
	}
	queryTemplate := map[string]string{
		"category": "{{query_name}}=1",
		"tag":      "tag_id={{value}}",
		"keyword":  "search={{value}}",
	}
	for name, value := range definition.HTML.Search.Params {
		queryTemplate[name] = fmt.Sprint(value)
	}
	pagination := SitePaginationConfig{Start: definition.HTML.Browse.Start}
	if page := definition.HTML.Search.Fields.Page; page != nil && strings.EqualFold(strings.TrimSpace(page.Type), "number") {
		pagination.Parameter = strings.TrimSpace(page.Name)
		pagination.Query = strings.TrimSpace(page.Query)
		if pagination.Parameter != "" && pagination.Query == "" {
			pagination.Query = pagination.Parameter + "={{value}}"
		}
	}
	return normalizeSiteConfig(SiteConfig{
		SiteID:        definition.ID,
		Name:          firstNonEmpty(definition.Name, definition.ID),
		BaseURL:       baseURL,
		Domain:        baseURL,
		Encoding:      definition.Encoding,
		URL:           absoluteURL(baseURL, listPath),
		SearchPath:    pathFromURL(searchPath, "/torrents.php"),
		DownloadPath:  "/download.php",
		AttendanceURL: absoluteURL(baseURL, definition.AttendancePageURL),
		Pagination:    pagination,
		QueryTemplate: queryTemplate,
	})
}

// ParseSelectorsFromDefinition 生成运行时解析 selector。
func ParseSelectorsFromDefinition(definition SiteDefinition) ParseSelectors {
	definition = NormalizeSiteDefinition(definition)
	return defaultParseSelectors(ParseSelectors{
		TorrentRows: definition.HTML.Torrents.List.Selector,
	})
}

// normalizeNexusMediaTorrentRowsSelector 转换 nexus-media 的行 selector。
func normalizeNexusMediaTorrentRowsSelector(selector string) string {
	selector = strings.TrimSpace(selector)
	changed := false
	for {
		start := strings.Index(selector, ":has(")
		if start < 0 {
			if changed && strings.Contains(selector, "table.torrents") {
				return defaultParseSelectors(ParseSelectors{}).TorrentRows
			}
			return selector
		}
		changed = true
		end := strings.Index(selector[start:], ")")
		if end < 0 {
			selector = strings.TrimSpace(selector[:start])
			if strings.Contains(selector, "table.torrents") {
				return defaultParseSelectors(ParseSelectors{}).TorrentRows
			}
			return selector
		}
		selector = strings.TrimSpace(selector[:start] + selector[start+end+1:])
	}
}

// normalizeSiteConfig 补齐站点配置中的默认 URL 与模板。
func normalizeSiteConfig(cfg SiteConfig) SiteConfig {
	cfg.SiteID = strings.TrimSpace(cfg.SiteID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.BaseURL = normalizeBaseURL(firstNonEmpty(cfg.BaseURL, cfg.Domain))
	cfg.Domain = normalizeBaseURL(firstNonEmpty(cfg.Domain, cfg.BaseURL))
	cfg.URL = absoluteURL(cfg.BaseURL, firstNonEmpty(cfg.URL, cfg.SearchPath, "/torrents.php"))
	cfg.SearchPath = pathFromURL(firstNonEmpty(cfg.SearchPath, cfg.URL), "/torrents.php")
	if !strings.HasPrefix(cfg.SearchPath, "/") {
		cfg.SearchPath = "/" + cfg.SearchPath
	}
	if strings.TrimSpace(cfg.DownloadPath) == "" {
		cfg.DownloadPath = "/download.php"
	}
	if strings.TrimSpace(cfg.AttendanceURL) == "" {
		cfg.AttendanceURL = absoluteURL(cfg.BaseURL, "attendance.php")
	}
	if cfg.QueryTemplate == nil {
		cfg.QueryTemplate = map[string]string{
			"category": "{{query_name}}=1",
			"tag":      "tag_id={{value}}",
			"keyword":  "search={{value}}",
		}
	}
	return cfg
}
