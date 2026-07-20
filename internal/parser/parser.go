package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"
)

var (
	promotionTimeRE = regexp.MustCompile(`title=&quot;([^&]+)&quot;&gt;([^<]+)&lt;/span&gt;`)
	categoryIDRE    = regexp.MustCompile(`\d+`)
)

func ParsePage(data []byte, opts SiteParseOptions) (ParsedPage, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ParsedPage{}, fmt.Errorf("parse html: %w", err)
	}
	baseURL := normalizeBaseURL(opts.BaseURL)
	if baseURL == "" {
		baseURL = baseURLFromURL(opts.URL)
	}
	selectors := defaultParseSelectors(opts.Selectors)
	pageURL := absoluteURL(baseURL, firstNonEmpty(opts.URL, "/torrents.php"))
	searchPath := pathFromURL(pageURL, "/torrents.php")
	page := ParsedPage{
		SiteConfig: SiteConfig{
			SiteID:       opts.SiteID,
			BaseURL:      baseURL,
			URL:          pageURL,
			SearchPath:   searchPath,
			DownloadPath: "/download.php",
			QueryTemplate: map[string]string{
				"category": "{{query_name}}=1",
				"tag":      "tag_id={{value}}",
				"keyword":  "search={{value}}",
			},
		},
		Selectors: selectors,
	}

	form := doc.Find(selectors.SearchBox).First()
	if form.Length() > 0 {
		page.SearchConfig = parseSearchConfig(form, baseURL)
	}
	page.Torrents = parseTorrents(doc, opts.SiteID, baseURL, selectors, opts.TorrentFields)
	page.Pagination = parsePagination(doc, pageURL, selectors)
	return page, nil
}

func ParsePageWithDefinition(data []byte, definition SiteDefinition) (ParsedPage, error) {
	cfg := SiteConfigFromDefinition(definition)
	page, err := ParsePage(data, SiteParseOptions{
		SiteID:        cfg.SiteID,
		BaseURL:       cfg.BaseURL,
		URL:           cfg.URL,
		Selectors:     ParseSelectorsFromDefinition(definition),
		TorrentFields: definition.HTML.Torrents.Fields,
	})
	if err != nil {
		return ParsedPage{}, err
	}
	page.SiteConfig = cfg
	return page, nil
}

// ParseTorrentDetail 解析种子详情页的标题、副标题和描述。
func ParseTorrentDetail(data []byte, opts TorrentDetailParseOptions) (TorrentDetail, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return TorrentDetail{}, fmt.Errorf("parse detail html: %w", err)
	}
	baseURL := normalizeBaseURL(opts.BaseURL)
	if baseURL == "" {
		baseURL = baseURLFromURL(opts.URL)
	}
	detailURL := absoluteURL(baseURL, opts.URL)
	detail := TorrentDetail{
		SiteID:        opts.SiteID,
		TorrentID:     opts.TorrentID,
		DetailURL:     detailURL,
		DetailTitle:   firstNonEmpty(cleanText(doc.Find("h1#top").First().Text()), cleanText(doc.Find("h1").First().Text()), cleanPageTitle(doc.Find("title").First().Text())),
		Subtitle:      findDetailValueByLabels(doc, []string{"副标题", "小标题", "subtitle", "sub title"}),
		ProductURL:    absoluteURL(baseURL, findDetailProductURL(doc)),
		InfoHash:      findDetailInfoHash(doc),
		DetailRawText: cleanText(doc.Find("body").Text()),
	}
	detail.DetailDescription = firstNonEmpty(
		findDetailValueByLabels(doc, []string{"简介", "描述", "介绍", "资源简介", "种子描述", "description"}),
		cleanText(doc.Find("#kdescr").First().Text()),
		cleanText(doc.Find("div#descr").First().Text()),
		cleanText(doc.Find("td#descr").First().Text()),
		cleanText(doc.Find(".descr").First().Text()),
	)
	if detail.DetailTitle == "" {
		return TorrentDetail{}, fmt.Errorf("detail title not found")
	}
	return detail, nil
}

// findDetailProductURL 从详情页读取商品链接。
func findDetailProductURL(doc *goquery.Document) string {
	if value := firstNonEmpty(
		findDetailHrefByLabels(doc, []string{"商品链接", "商店链接", "商品地址", "product url", "product link"}),
		attrFirst(doc.Find(`a[href^="http"]:contains("dl.getchu"), a[href^="http"]:contains("dlsite"), a[href^="http"]:contains("www.dlsite")`).First(), "href"),
	); value != "" {
		return value
	}
	return ""
}

// findDetailInfoHash 从详情页读取站点展示的 torrent hash。
func findDetailInfoHash(doc *goquery.Document) string {
	text := firstNonEmpty(
		findDetailValueByLabels(doc, []string{"种子文件", "Hash码", "Hash", "info hash"}),
		cleanText(doc.Find("body").Text()),
	)
	match := regexp.MustCompile(`(?i)(?:Hash码|Hash|info hash)\s*[:：]?\s*([a-f0-9]{40})`).FindStringSubmatch(text)
	if len(match) == 2 {
		return strings.ToLower(match[1])
	}
	match = regexp.MustCompile(`(?i)\b([a-f0-9]{40})\b`).FindStringSubmatch(text)
	if len(match) == 2 {
		return strings.ToLower(match[1])
	}
	return ""
}

// findDetailValueByLabels 从详情页表格中按标签读取字段。
func findDetailValueByLabels(doc *goquery.Document, labels []string) string {
	var value string
	doc.Find("tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		cells := row.ChildrenFiltered("td, th")
		if cells.Length() < 2 {
			return true
		}
		label := normalizeDetailLabel(cleanText(cells.First().Text()))
		for _, candidate := range labels {
			if labelMatches(label, candidate) {
				value = cleanText(cells.Eq(1).Text())
				return false
			}
		}
		return true
	})
	return value
}

// findDetailHrefByLabels 从详情页表格中按标签读取首个链接。
func findDetailHrefByLabels(doc *goquery.Document, labels []string) string {
	var value string
	doc.Find("tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		cells := row.ChildrenFiltered("td, th")
		if cells.Length() < 2 {
			return true
		}
		label := normalizeDetailLabel(cleanText(cells.First().Text()))
		for _, candidate := range labels {
			if labelMatches(label, candidate) {
				value = attrFirst(cells.Eq(1).Find("a[href]").First(), "href")
				return false
			}
		}
		return true
	})
	return value
}

// labelMatches 判断详情页字段名是否匹配候选标签。
func labelMatches(label, candidate string) bool {
	label = strings.ToLower(normalizeDetailLabel(label))
	candidate = strings.ToLower(normalizeDetailLabel(candidate))
	return label == candidate || strings.Contains(label, candidate)
}

// normalizeDetailLabel 清理详情页字段名。
func normalizeDetailLabel(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, ":")
	text = strings.TrimSuffix(text, "：")
	return strings.Join(strings.Fields(text), " ")
}

// cleanPageTitle 清理 HTML title 标签内容。
func cleanPageTitle(text string) string {
	text = cleanText(text)
	for _, sep := range []string{"::", " - ", " | "} {
		if before, _, ok := strings.Cut(text, sep); ok {
			return cleanText(before)
		}
	}
	return text
}

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

// ExportSiteConfig 导出页面解析得到的站点配置。
func ExportSiteConfig(path string, page ParsedPage) error {
	return ExportSearchConfig(path, page)
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

// ExportSearchConfig 导出解析得到的搜索项 JSON。
func ExportSearchConfig(path string, page ParsedPage) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		SiteID        string            `json:"site_id"`
		BaseURL       string            `json:"base_url"`
		URL           string            `json:"url"`
		QueryTemplate map[string]string `json:"query_template"`
		SearchConfig  SearchConfig      `json:"search_config"`
	}{
		SiteID:        page.SiteConfig.SiteID,
		BaseURL:       page.SiteConfig.BaseURL,
		URL:           page.SiteConfig.URL,
		QueryTemplate: page.SiteConfig.QueryTemplate,
		SearchConfig:  page.SearchConfig,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
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

func parseSearchConfig(form *goquery.Selection, baseURL string) SearchConfig {
	cfg := SearchConfig{}
	checkboxes := parseCheckboxGroups(form)
	for _, group := range checkboxes {
		if group.Name == "cat" {
			cfg.Categories = group.Options
			continue
		}
		cfg.Checkboxes = append(cfg.Checkboxes, group)
	}
	cfg.Tags = parseTagLinks(form)
	cfg.Selects = parseSelectFields(form)
	cfg.Ranges = parseRangeFields(form)
	cfg.Keyword = parseKeywordField(form)
	cfg.Fields = buildSiteSearchFields(cfg)
	_ = baseURL
	return cfg
}

func parseCheckboxGroups(root *goquery.Selection) []SearchGroup {
	groups := []SearchGroup{}
	groupByName := map[string]int{}
	root.Find(`input[type="checkbox"][name]`).Each(func(_ int, input *goquery.Selection) {
		name, _ := input.Attr("name")
		value, _ := input.Attr("value")
		if value == "" {
			value = "1"
		}
		label := checkboxLabel(input)
		if label != "" {
			prefix := checkboxPrefix(name)
			groupLabel := firstNonEmpty(checkboxGroupLabel(input), prefix)
			option := SearchOption{
				Name:      name,
				Value:     value,
				Label:     label,
				QueryName: name,
				Query:     queryString(name, value),
			}
			if index, exists := groupByName[prefix]; exists {
				groups[index].Options = append(groups[index].Options, option)
				if groups[index].Label == groups[index].Name && groupLabel != "" {
					groups[index].Label = groupLabel
				}
				return
			}
			groupByName[prefix] = len(groups)
			groups = append(groups, SearchGroup{Name: prefix, Label: groupLabel, Options: []SearchOption{option}})
		}
	})
	return groups
}

func parseTagLinks(root *goquery.Selection) []SearchOption {
	options := []SearchOption{}
	root.Find(`a[href*="tag_id="]`).Each(func(_ int, link *goquery.Selection) {
		href, _ := link.Attr("href")
		parsed, err := url.Parse(href)
		if err != nil {
			return
		}
		value := parsed.Query().Get("tag_id")
		label := cleanText(link.Text())
		if value == "" || label == "" {
			return
		}
		options = append(options, SearchOption{
			Name:      "tag_id",
			Value:     value,
			Label:     label,
			QueryName: "tag_id",
			Query:     queryString("tag_id", value),
		})
	})
	return options
}

func parseSelectFields(root *goquery.Selection) []SelectField {
	fields := []SelectField{}
	root.Find("select[name]").Each(func(_ int, sel *goquery.Selection) {
		name, _ := sel.Attr("name")
		field := SelectField{Name: name, Label: labelBefore(sel)}
		sel.Find("option").Each(func(_ int, opt *goquery.Selection) {
			value, _ := opt.Attr("value")
			label := cleanText(opt.Text())
			field.Options = append(field.Options, SearchOption{
				Name:      name,
				Value:     value,
				Label:     label,
				QueryName: name,
				Query:     queryString(name, value),
			})
		})
		fields = append(fields, field)
	})
	return fields
}

func parseRangeFields(root *goquery.Selection) []RangeField {
	fields := []RangeField{}
	seen := map[string]struct{}{}
	root.Find(`input[name$="_begin"]`).Each(func(_ int, beginInput *goquery.Selection) {
		begin, _ := beginInput.Attr("name")
		name := strings.TrimSuffix(begin, "_begin")
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		end := name + "_end"
		endInput := root.Find(fmt.Sprintf(`input[name="%s"]`, end)).First()
		if endInput.Length() == 0 {
			return
		}
		seen[name] = struct{}{}
		kind := "number"
		beginType := strings.ToLower(attrFirst(beginInput, "type"))
		endType := strings.ToLower(attrFirst(endInput, "type"))
		if beginType == "date" || endType == "date" {
			kind = "date"
		}
		fields = append(fields, RangeField{
			Name:  name,
			Label: firstNonEmpty(rangeLabel(beginInput), name),
			Begin: begin,
			End:   end,
			Kind:  kind,
		})
	})
	return fields
}

func parseKeywordField(root *goquery.Selection) KeywordField {
	field := KeywordField{Name: "search", AreaSelectName: "search_area", ModeSelectName: "search_mode"}
	field.AreaOptions = optionsForSelect(root.Find(`select[name="search_area"]`).First())
	field.ModeOptions = optionsForSelect(root.Find(`select[name="search_mode"]`).First())
	root.Find(`a[href*="search="]`).Each(func(_ int, link *goquery.Selection) {
		href, _ := link.Attr("href")
		parsed, err := url.Parse(href)
		if err != nil {
			return
		}
		value := parsed.Query().Get("search")
		label := cleanText(link.Text())
		if value == "" || label == "" {
			return
		}
		field.SuggestionQueries = append(field.SuggestionQueries, SearchOption{
			Name:      "search",
			Value:     value,
			Label:     label,
			QueryName: "search",
			Query:     queryString("search", value),
		})
	})
	return field
}

func optionsForSelect(sel *goquery.Selection) []SearchOption {
	options := []SearchOption{}
	name, _ := sel.Attr("name")
	sel.Find("option").Each(func(_ int, opt *goquery.Selection) {
		value, _ := opt.Attr("value")
		options = append(options, SearchOption{
			Name:      name,
			Value:     value,
			Label:     cleanText(opt.Text()),
			QueryName: name,
			Query:     queryString(name, value),
		})
	})
	return options
}

func buildSiteSearchFields(cfg SearchConfig) SiteSearchFields {
	fields := SiteSearchFields{}
	if len(cfg.Categories) > 0 {
		fields.Checkboxes = append(fields.Checkboxes, checkboxGroupField(SearchGroup{
			Name:    "cat",
			Label:   "分类",
			Options: cfg.Categories,
		}))
	}
	for _, group := range cfg.Checkboxes {
		fields.Checkboxes = append(fields.Checkboxes, checkboxGroupField(group))
	}
	if len(cfg.Tags) > 0 {
		field := SiteSearchField{
			Name:      "tag_id",
			Type:      "tag",
			Label:     "标签",
			Query:     "tag_id={{value}}",
			Exclusive: true,
		}
		for _, option := range cfg.Tags {
			field.Options = append(field.Options, SiteSearchFieldOption{
				Value: option.Value,
				Label: option.Label,
				Query: option.Query,
			})
		}
		fields.Tags = &field
	}
	for _, selectField := range cfg.Selects {
		field := SiteSearchField{
			Name:  selectField.Name,
			Type:  "select",
			Label: firstNonEmpty(selectField.Label, selectField.Name),
			Query: queryTemplateForName(selectField.Name),
		}
		for _, option := range selectField.Options {
			field.Options = append(field.Options, SiteSearchFieldOption{
				Value: option.Value,
				Label: option.Label,
				Query: option.Query,
			})
		}
		fields.Selects = append(fields.Selects, field)
	}
	for _, rangeField := range cfg.Ranges {
		fieldType := "number_range"
		if rangeField.Kind == "date" {
			fieldType = "date_range"
		}
		fields.Ranges = append(fields.Ranges, SiteSearchField{
			Name:  rangeField.Name,
			Type:  fieldType,
			Label: firstNonEmpty(rangeField.Label, rangeField.Name),
			Begin: rangeField.Begin,
			End:   rangeField.End,
			Query: rangeField.Begin + "={{begin}}&" + rangeField.End + "={{end}}",
		})
	}
	if cfg.Keyword.Name != "" {
		fields.Keyword = &SiteSearchField{
			Name:  cfg.Keyword.Name,
			Type:  "string",
			Label: "搜索关键字",
			Query: queryTemplateForName(cfg.Keyword.Name),
		}
	}
	return fields
}

func checkboxGroupField(group SearchGroup) SiteSearchCheckboxGroup {
	field := SiteSearchCheckboxGroup{
		Name:  group.Name,
		Type:  "bool",
		Label: firstNonEmpty(group.Label, group.Name),
	}
	for _, option := range group.Options {
		field.Options = append(field.Options, SiteSearchFieldOption{
			Name:  option.Name,
			Value: option.Value,
			Label: option.Label,
			Query: option.Query,
		})
	}
	return field
}

func queryTemplateForName(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return name + "={{value}}"
}

func checkboxLabel(input *goquery.Selection) string {
	cell := input.Parent()
	if cell.Is("label") {
		if label := cleanText(cell.Text()); label != "" {
			return label
		}
		cell = cell.Parent()
	}
	if label := cleanText(input.NextFiltered("a").First().Text()); label != "" {
		return label
	}
	if label := cleanText(input.PrevAllFiltered("a").First().Text()); label != "" {
		return label
	}
	if label := cleanText(input.Parent().Find("a").First().Text()); label != "" {
		return label
	}
	return attrFirst(cell.Find("img[alt], img[title]").First(), "alt", "title")
}

func checkboxGroupLabel(input *goquery.Selection) string {
	cell := input.Closest("td")
	row := cell.Parent()
	if label := previousSearchHeading(row); label != "" {
		return label
	}
	name, _ := input.Attr("name")
	switch checkboxPrefix(name) {
	case "cat":
		return "分类"
	default:
		return ""
	}
}

func previousSearchHeading(row *goquery.Selection) string {
	for prev := row.Prev(); prev.Length() > 0; prev = prev.Prev() {
		if prev.Find("input, select, textarea").Length() > 0 {
			continue
		}
		label := cleanText(prev.Text())
		if label != "" && len([]rune(label)) <= 32 {
			return strings.TrimSuffix(strings.TrimSuffix(label, "？"), ":")
		}
	}
	return ""
}

func checkboxPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	prefix := regexp.MustCompile(`^[A-Za-z_]+`).FindString(name)
	if prefix == "" {
		return name
	}
	return strings.TrimSuffix(prefix, "_")
}

func rangeLabel(input *goquery.Selection) string {
	row := input.Closest("tr")
	if label := previousSearchHeading(row); label != "" {
		return label
	}
	text := cleanText(input.Parent().Text())
	text = strings.TrimSpace(strings.Trim(text, "~"))
	if text != "" && !strings.Contains(text, "{{") {
		return text
	}
	return ""
}

func parseTorrents(doc *goquery.Document, siteID, baseURL string, selectors ParseSelectors, fields map[string]SiteFieldDefinition) []TorrentEntry {
	torrents := []TorrentEntry{}
	rows := doc.Find(selectors.TorrentRows)
	if rows.Length() == 0 {
		fallback := defaultParseSelectors(ParseSelectors{}).TorrentRows
		if selectors.TorrentRows != fallback {
			slog.Warn("torrent row selector matched no rows, using fallback", "site_id", siteID, "selector", selectors.TorrentRows, "fallback", fallback)
			rows = doc.Find(fallback)
		}
	}
	rows.Each(func(_ int, row *goquery.Selection) {
		nameTable := row.Find(selectors.TorrentName).First()
		if nameTable.Length() == 0 {
			return
		}
		torrent := parseTorrentRow(row, nameTable, siteID, baseURL, fields)
		if torrent.ID != 0 {
			torrents = append(torrents, torrent)
		}
	})
	slog.Info("parsed torrent rows", "site_id", siteID, "matched_rows", rows.Length(), "torrents", len(torrents))
	return torrents
}

func parseTorrentRow(row, nameTable *goquery.Selection, siteID, baseURL string, fields map[string]SiteFieldDefinition) TorrentEntry {
	cells := row.ChildrenFiltered("td")
	titleLink := nameTable.Find(`a[href*="details.php"]`).First()
	detailHref, _ := titleLink.Attr("href")
	downloadHref, _ := nameTable.Find(`a[href*="download.php"]`).First().Attr("href")
	cover := attrFirst(nameTable.Find("img.nexus-lazy-load").First(), "data-src", "src")
	fieldValues := extractTorrentFieldValues(row, fields)
	id := parseInt(firstNonEmpty(fieldValues["id"], strconv.Itoa(intFromQuery(detailHref, "id"))))
	category := firstNonEmpty(fieldValues["category_name"], attrFirst(cells.Eq(0).Find("img").First(), "alt", "title"))
	categoryQuery := firstNonEmpty(fieldValues["category_query"], hrefQuery(cells.Eq(0).Find("a").First()))
	if categoryQuery == "" && fieldValues["category"] != "" {
		categoryQuery = queryString("cat", fieldValues["category"])
	}
	title := firstNonEmpty(fieldValues["title"], fieldValues["title_optional"], fieldValues["title_default"], attrFirst(titleLink, "title"))
	detailHref = firstNonEmpty(fieldValues["details"], fieldValues["detail"], detailHref)
	downloadHref = firstNonEmpty(fieldValues["download"], downloadHref)
	cover = firstNonEmpty(fieldValues["cover"], cover)
	publishedAt := firstNonEmpty(fieldValues["published_at"], fieldValues["date_added"], attrFirst(cells.Eq(3).Find("span").First(), "title"))
	publishedText := firstNonEmpty(fieldValues["published_text"], fieldValues["date_elapsed"], cleanText(cells.Eq(3).Text()))
	sizeText := firstNonEmpty(fieldValues["size"], cleanText(cells.Eq(4).Text()))
	seeders := parseInt(firstNonEmpty(fieldValues["seeders"], cleanText(cells.Eq(5).Text())))
	leechers := parseInt(firstNonEmpty(fieldValues["leechers"], cleanText(cells.Eq(6).Text())))
	snatches := parseInt(firstNonEmpty(fieldValues["snatches"], fieldValues["grabs"], cleanText(cells.Eq(7).Text())))
	comments := parseInt(firstNonEmpty(fieldValues["comments"], cleanText(cells.Eq(2).Text())))

	torrent := TorrentEntry{
		SiteID:        siteID,
		ID:            id,
		Category:      category,
		CategoryQuery: categoryQuery,
		Title:         title,
		DetailHref:    html.UnescapeString(detailHref),
		DetailURL:     absoluteURL(baseURL, detailHref),
		DownloadHref:  html.UnescapeString(downloadHref),
		DownloadURL:   absoluteURL(baseURL, downloadHref),
		CoverURL:      absoluteURL(baseURL, cover),
		Comments:      comments,
		PublishedAt:   publishedAt,
		PublishedText: publishedText,
		Subtitle:      fieldValues["subtitle"],
		Description:   fieldValues["description"],
		SizeText:      sizeText,
		Seeders:       seeders,
		Leechers:      leechers,
		Snatches:      snatches,
		StickyLevel:   nameTable.Find("img.sticky").Length(),
		Bookmarked:    nameTable.Find("img.bookmark, img.delbookmark").Length() > 0,
	}
	if torrent.Title == "" {
		torrent.Title = cleanText(titleLink.Text())
	}
	torrent.SizeBytes = parseSizeBytes(torrent.SizeText)

	torrent.Tags = extractTorrentFieldList(row, fields, "tags")
	torrent.TagIDs = firstNonEmptyStrings(
		extractTorrentFieldList(row, fields, "tag_ids"),
		extractTorrentFieldList(row, fields, "labels"),
		coloredSpanTags(nameTable),
	)

	pro := nameTable.Find(`img[class*="pro_"]`).First()
	torrent.PromotionClass, _ = pro.Attr("class")
	torrent.Promotion = attrFirst(pro, "alt")
	onmouseover, _ := pro.Attr("onmouseover")
	if onmouseover != "" {
		if match := promotionTimeRE.FindStringSubmatch(onmouseover); len(match) == 3 {
			torrent.PromotionEndsAt = html.UnescapeString(match[1])
			torrent.PromotionRemaining = html.UnescapeString(match[2])
		}
	}
	if torrent.PromotionRemaining == "" {
		timeSpan := nameTable.Find("font span[title]").First()
		torrent.PromotionEndsAt = attrFirst(timeSpan, "title")
		torrent.PromotionRemaining = cleanText(timeSpan.Text())
	}
	return torrent
}

// extractTorrentFieldValues 按站点字段规则提取种子行的标量字段。
func extractTorrentFieldValues(row *goquery.Selection, fields map[string]SiteFieldDefinition) map[string]string {
	values := map[string]string{}
	for name := range fields {
		value := extractTorrentFieldString(row, fields, name)
		if value != "" {
			values[name] = value
		}
	}
	return values
}

// extractTorrentFieldString 按字段名读取种子行中的一个字符串值。
func extractTorrentFieldString(row *goquery.Selection, fields map[string]SiteFieldDefinition, name string) string {
	if len(fields) == 0 {
		return ""
	}
	rule, ok := fields[name]
	if !ok {
		return ""
	}
	value := fieldValue(row, rule)
	return applyFieldFilters(value, rule)
}

// extractTorrentFieldList 按字段名读取种子行中的字符串数组。
func extractTorrentFieldList(row *goquery.Selection, fields map[string]SiteFieldDefinition, name string) []string {
	if len(fields) == 0 {
		return nil
	}
	rule, ok := fields[name]
	if !ok {
		return nil
	}
	split := strings.ToLower(fieldString(rule, "split"))
	if split != "" {
		return filterFieldList(splitFieldList(extractTorrentFieldString(row, fields, name), split), rule)
	}
	values := []string{}
	fieldSelections(row, rule).Each(func(_ int, sel *goquery.Selection) {
		value := applyFieldFilters(fieldText(sel, rule), rule)
		if value != "" {
			values = append(values, value)
		}
	})
	return filterFieldList(values, rule)
}

// fieldValue 提取字段规则命中的第一个值。
func fieldValue(row *goquery.Selection, rule SiteFieldDefinition) string {
	selection := fieldSelections(row, rule).First()
	if selection.Length() == 0 {
		return fieldString(rule, "default_value")
	}
	value := fieldText(selection, rule)
	if value == "" {
		value = fieldString(rule, "default_value")
	}
	return value
}

// fieldSelections 返回字段规则匹配的节点集合。
func fieldSelections(row *goquery.Selection, rule SiteFieldDefinition) *goquery.Selection {
	selector := fieldString(rule, "selector")
	if strings.TrimSpace(selector) == "" {
		return &goquery.Selection{}
	}
	if row.Is(selector) {
		return row.Filter(selector)
	}
	return row.Find(selector)
}

// fieldText 从节点中读取属性或文本。
func fieldText(sel *goquery.Selection, rule SiteFieldDefinition) string {
	attributes := fieldStringSlice(rule, "attributes")
	if attribute := fieldString(rule, "attribute"); attribute != "" {
		attributes = append([]string{attribute}, attributes...)
	}
	if len(attributes) > 0 {
		return attrFirst(sel, attributes...)
	}
	source := sel.Clone()
	removeSelectors(source, fieldStringSlice(rule, "remove"))
	if after := firstNonEmpty(fieldString(rule, "after"), fieldString(rule, "after_selector")); after != "" {
		return cleanText(textAfterFirstSelector(source, after))
	}
	return cleanText(source.Text())
}

// removeSelectors 从字段节点副本中移除不参与文本提取的子节点。
func removeSelectors(sel *goquery.Selection, selectors []string) {
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if selector != "" {
			sel.Find(selector).Remove()
		}
	}
}

// applyFieldFilters 执行站点字段规则中声明的简单文本过滤器。
func applyFieldFilters(value string, rule SiteFieldDefinition) string {
	value = strings.TrimSpace(html.UnescapeString(value))
	for _, filter := range fieldFilters(rule) {
		name := strings.ToLower(fieldString(filter, "name"))
		switch name {
		case "re_search":
			value = applyRegexSearch(value, filter["args"])
		case "replace":
			value = applyReplaceFilter(value, filter["args"])
		case "querystring":
			value = applyQueryStringFilter(value, filter["args"])
		case "dateparse":
			value = cleanText(value)
		default:
			value = cleanText(value)
		}
	}
	return cleanText(value)
}

// fieldFilters 读取字段规则中的 filters 数组。
func fieldFilters(rule SiteFieldDefinition) []SiteFieldDefinition {
	raw, ok := rule["filters"]
	if !ok {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	filters := make([]SiteFieldDefinition, 0, len(items))
	for _, item := range items {
		if filter, ok := item.(map[string]interface{}); ok {
			filters = append(filters, SiteFieldDefinition(filter))
		}
	}
	return filters
}

// applyRegexSearch 执行 re_search 过滤器。
func applyRegexSearch(value string, args interface{}) string {
	items, ok := args.([]interface{})
	if !ok || len(items) == 0 {
		return value
	}
	pattern := fmt.Sprint(items[0])
	index := 0
	if len(items) > 1 {
		index = intFromAny(items[1])
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return value
	}
	matches := re.FindStringSubmatch(value)
	if len(matches) == 0 || index < 0 || index >= len(matches) {
		return ""
	}
	return matches[index]
}

// applyReplaceFilter 执行 replace 过滤器。
func applyReplaceFilter(value string, args interface{}) string {
	items, ok := args.([]interface{})
	if !ok || len(items) < 2 {
		return value
	}
	return strings.ReplaceAll(value, fmt.Sprint(items[0]), fmt.Sprint(items[1]))
}

// applyQueryStringFilter 执行 querystring 过滤器。
func applyQueryStringFilter(value string, args interface{}) string {
	key := strings.TrimSpace(fmt.Sprint(args))
	if items, ok := args.([]interface{}); ok && len(items) > 0 {
		key = strings.TrimSpace(fmt.Sprint(items[0]))
	}
	if key == "" {
		return value
	}
	parsed, err := url.Parse(html.UnescapeString(value))
	if err == nil {
		if queryValue := parsed.Query().Get(key); queryValue != "" {
			return queryValue
		}
	}
	query, err := url.ParseQuery(strings.TrimPrefix(value, "?"))
	if err != nil {
		return ""
	}
	return query.Get(key)
}

// splitFieldList 将字段字符串拆分为标签数组。
func splitFieldList(value, mode string) []string {
	switch mode {
	case "space", "spaces", "field", "fields", "whitespace":
		return uniqueNonEmpty(strings.Fields(value))
	case "comma", ",":
		return uniqueNonEmpty(strings.Split(value, ","))
	default:
		return uniqueNonEmpty([]string{value})
	}
}

// filterFieldList 按字段规则过滤列表值。
func filterFieldList(values []string, rule SiteFieldDefinition) []string {
	maxLength := firstPositiveInt(fieldInt(rule, "max_length"), fieldInt(rule, "max_length_runes"), fieldInt(rule, "max_tag_length"))
	if maxLength <= 0 {
		return uniqueNonEmpty(values)
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanText(value)
		if value == "" || len([]rune(value)) > maxLength {
			continue
		}
		filtered = append(filtered, value)
	}
	return uniqueNonEmpty(filtered)
}

// textAfterFirstSelector 读取第一个匹配节点之后的文本。
func textAfterFirstSelector(sel *goquery.Selection, selector string) string {
	var builder strings.Builder
	found := false
	for _, node := range sel.Nodes {
		collectTextAfterSelector(node, selector, &found, &builder)
	}
	return builder.String()
}

// collectTextAfterSelector 按文档顺序收集匹配节点之后的文本节点。
func collectTextAfterSelector(node *nethtml.Node, selector string, found *bool, builder *strings.Builder) {
	if node == nil {
		return
	}
	if node.Type == nethtml.ElementNode && nodeMatchesSelector(node, selector) {
		*found = true
		return
	}
	if *found && node.Type == nethtml.TextNode {
		builder.WriteString(node.Data)
		builder.WriteString(" ")
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectTextAfterSelector(child, selector, found, builder)
	}
}

// nodeMatchesSelector 判断 HTML 节点是否匹配 CSS selector。
func nodeMatchesSelector(node *nethtml.Node, selector string) bool {
	doc := goquery.NewDocumentFromNode(node)
	return doc.Selection.Is(selector)
}

// fieldString 读取字段规则中的字符串值。
func fieldString(rule SiteFieldDefinition, key string) string {
	value, ok := rule[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// fieldInt 读取字段规则中的整数值。
func fieldInt(rule SiteFieldDefinition, key string) int {
	value, ok := rule[key]
	if !ok || value == nil {
		return 0
	}
	return intFromAny(value)
}

// fieldStringSlice 读取字段规则中的字符串数组。
func fieldStringSlice(rule SiteFieldDefinition, key string) []string {
	value, ok := rule[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				items = append(items, text)
			}
		}
		return items
	case string:
		parts := strings.Split(typed, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := strings.TrimSpace(part); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

// intFromAny 将 JSON 数字转换为 int。
func intFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return parseInt(fmt.Sprint(value))
	}
}

// firstPositiveInt 返回第一个正整数。
func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// coloredSpanTags 读取旧版彩色 span 标签。
func coloredSpanTags(nameTable *goquery.Selection) []string {
	tags := []string{}
	nameTable.Find(`span[style*="background-color"]`).Each(func(_ int, span *goquery.Selection) {
		if tag := cleanText(span.Text()); tag != "" {
			tags = append(tags, tag)
		}
	})
	return uniqueNonEmpty(tags)
}

// firstNonEmptyStrings 返回第一个非空字符串数组。
func firstNonEmptyStrings(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

// uniqueNonEmpty 去重并保留原始顺序。
func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		value = cleanText(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parsePagination(doc *goquery.Document, pageURL string, selectors ParseSelectors) Pagination {
	pagination := Pagination{}
	pager := doc.Find(selectors.Pagination).First()
	pagination.CurrentLabel = cleanText(pager.Find(".current").First().Text())
	pager.Find("a[data-page]").Each(func(_ int, link *goquery.Selection) {
		href, _ := link.Attr("href")
		page, _ := link.Attr("data-page")
		pagination.Pages = append(pagination.Pages, PageLink{
			Page:  page,
			Label: cleanText(link.Text()),
			Href:  html.UnescapeString(href),
			URL:   resolvePageReference(pageURL, href),
		})
	})
	return pagination
}

func labelBefore(sel *goquery.Selection) string {
	label := cleanText(sel.Parent().Parent().Prev().Text())
	return strings.TrimSuffix(label, "？")
}

func attrFirst(sel *goquery.Selection, names ...string) string {
	for _, name := range names {
		if value, ok := sel.Attr(name); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(html.UnescapeString(value))
		}
	}
	return ""
}

func hrefQuery(sel *goquery.Selection) string {
	href, _ := sel.Attr("href")
	parsed, err := url.Parse(html.UnescapeString(href))
	if err != nil {
		return ""
	}
	return parsed.RawQuery
}

func queryString(name, value string) string {
	values := url.Values{}
	values.Set(name, value)
	return values.Encode()
}

func intFromQuery(href, key string) int {
	parsed, err := url.Parse(html.UnescapeString(href))
	if err != nil {
		return 0
	}
	return parseInt(parsed.Query().Get(key))
}

func parseInt(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	digits := regexp.MustCompile(`\d+`).FindString(text)
	if digits == "" {
		return 0
	}
	value, _ := strconv.Atoi(digits)
	return value
}

func parseSizeBytes(text string) int64 {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.TrimSpace(text)
	match := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([KMGT]I?B)`).FindStringSubmatch(text)
	if len(match) != 3 {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(match[2])
	multiplier := float64(1)
	switch unit {
	case "KB", "KIB":
		multiplier = 1024
	case "MB", "MIB":
		multiplier = 1024 * 1024
	case "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}
	return int64(math.Round(value * multiplier))
}

func absoluteURL(baseURL, href string) string {
	href = strings.TrimSpace(html.UnescapeString(href))
	if href == "" {
		return ""
	}
	baseURL = normalizeBaseURL(baseURL)
	parsed, err := url.Parse(href)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL + "/")
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// resolvePageReference 以当前列表页为基准解析分页链接，保留 torrents.php 路径。
func resolvePageReference(pageURL, href string) string {
	href = strings.TrimSpace(html.UnescapeString(href))
	if href == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	if ref.IsAbs() {
		return ref.String()
	}
	base, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func defaultParseSelectors(selectors ParseSelectors) ParseSelectors {
	if strings.TrimSpace(selectors.SearchBox) == "" {
		selectors.SearchBox = `form[name="searchbox"]`
	}
	if strings.TrimSpace(selectors.TorrentRows) == "" {
		selectors.TorrentRows = "table.torrents > tbody > tr, table.torrents > tr"
	}
	if strings.TrimSpace(selectors.TorrentName) == "" {
		selectors.TorrentName = "table.torrentname"
	}
	if strings.TrimSpace(selectors.Pagination) == "" {
		selectors.Pagination = ".nexus-pagination"
	}
	return selectors
}

func baseURLFromURL(raw string) string {
	parsed, err := url.Parse(normalizeBaseURL(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func pathFromURL(raw, fallback string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Path == "" {
		return fallback
	}
	return parsed.Path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}
