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
				"page":     "page={{value}}",
			},
		},
		Selectors: selectors,
	}

	form := doc.Find(selectors.SearchBox).First()
	if form.Length() > 0 {
		page.SearchConfig = parseSearchConfig(form, baseURL)
	}
	page.Torrents = parseTorrents(doc, opts.SiteID, baseURL, selectors)
	page.Pagination = parsePagination(doc, baseURL, selectors)
	return page, nil
}

func ParsePageWithDefinition(data []byte, definition SiteDefinition) (ParsedPage, error) {
	cfg := SiteConfigFromDefinition(definition)
	page, err := ParsePage(data, SiteParseOptions{
		SiteID:    cfg.SiteID,
		BaseURL:   cfg.BaseURL,
		URL:       cfg.URL,
		Selectors: ParseSelectorsFromDefinition(definition),
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
	cfg.Categories = parseLinkedCheckboxes(form, "cat")
	cfg.Tags = parseTagLinks(form)
	cfg.Checkboxes = appendIfOptions(cfg.Checkboxes, "source", "马赛克", parseLinkedCheckboxes(form, "source"))
	cfg.Checkboxes = appendIfOptions(cfg.Checkboxes, "team", "中文字幕", parseLinkedCheckboxes(form, "team"))
	cfg.Selects = parseSelectFields(form)
	cfg.Ranges = parseRangeFields(form)
	cfg.Keyword = parseKeywordField(form)
	_ = baseURL
	return cfg
}

func parseLinkedCheckboxes(root *goquery.Selection, prefix string) []SearchOption {
	options := []SearchOption{}
	root.Find(fmt.Sprintf(`input[type="checkbox"][name^="%s"]`, prefix)).Each(func(_ int, input *goquery.Selection) {
		name, _ := input.Attr("name")
		value, _ := input.Attr("value")
		label := ""
		queryName := name
		cell := input.Parent()
		if cell.Is("label") {
			label = cleanText(cell.Text())
			cell = cell.Parent()
		}
		if label == "" {
			label = cleanText(input.PrevAllFiltered("a").First().Text())
		}
		if label == "" {
			img := cell.Find("img[alt], img[title]").First()
			label = attrFirst(img, "alt", "title")
		}
		if value == "" {
			value = "1"
		}
		if label != "" {
			options = append(options, SearchOption{
				Name:      name,
				Value:     value,
				Label:     label,
				QueryName: queryName,
				Query:     queryString(queryName, value),
			})
		}
	})
	return options
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
	pairs := []struct {
		label string
		begin string
		end   string
		kind  string
	}{
		{"体积范围(GB)", "size_begin", "size_end", "number"},
		{"做种人数范围", "seeders_begin", "seeders_end", "number"},
		{"下载人数范围", "leechers_begin", "leechers_end", "number"},
		{"完成次数范围", "times_completed_begin", "times_completed_end", "number"},
		{"发布时间范围", "added_begin", "added_end", "date"},
	}
	fields := []RangeField{}
	for _, pair := range pairs {
		if root.Find(fmt.Sprintf(`input[name="%s"]`, pair.begin)).Length() == 0 {
			continue
		}
		if root.Find(fmt.Sprintf(`input[name="%s"]`, pair.end)).Length() == 0 {
			continue
		}
		fields = append(fields, RangeField{
			Name:  strings.TrimSuffix(pair.begin, "_begin"),
			Label: pair.label,
			Begin: pair.begin,
			End:   pair.end,
			Kind:  pair.kind,
		})
	}
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

func parseTorrents(doc *goquery.Document, siteID, baseURL string, selectors ParseSelectors) []TorrentEntry {
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
		torrent := parseTorrentRow(row, nameTable, siteID, baseURL)
		if torrent.ID != 0 {
			torrents = append(torrents, torrent)
		}
	})
	slog.Info("parsed torrent rows", "site_id", siteID, "matched_rows", rows.Length(), "torrents", len(torrents))
	return torrents
}

func parseTorrentRow(row, nameTable *goquery.Selection, siteID, baseURL string) TorrentEntry {
	cells := row.ChildrenFiltered("td")
	titleLink := nameTable.Find(`a[href*="details.php"]`).First()
	detailHref, _ := titleLink.Attr("href")
	downloadHref, _ := nameTable.Find(`a[href*="download.php"]`).First().Attr("href")
	cover := attrFirst(nameTable.Find("img.nexus-lazy-load").First(), "data-src", "src")

	torrent := TorrentEntry{
		SiteID:        siteID,
		ID:            intFromQuery(detailHref, "id"),
		Category:      attrFirst(cells.Eq(0).Find("img").First(), "alt", "title"),
		CategoryQuery: hrefQuery(cells.Eq(0).Find("a").First()),
		Title:         attrFirst(titleLink, "title"),
		DetailHref:    html.UnescapeString(detailHref),
		DetailURL:     absoluteURL(baseURL, detailHref),
		DownloadHref:  html.UnescapeString(downloadHref),
		DownloadURL:   absoluteURL(baseURL, downloadHref),
		CoverURL:      absoluteURL(baseURL, cover),
		Comments:      parseInt(cleanText(cells.Eq(2).Text())),
		PublishedAt:   attrFirst(cells.Eq(3).Find("span").First(), "title"),
		PublishedText: cleanText(cells.Eq(3).Text()),
		SizeText:      cleanText(cells.Eq(4).Text()),
		Seeders:       parseInt(cleanText(cells.Eq(5).Text())),
		Leechers:      parseInt(cleanText(cells.Eq(6).Text())),
		Snatches:      parseInt(cleanText(cells.Eq(7).Text())),
		StickyLevel:   nameTable.Find("img.sticky").Length(),
		Bookmarked:    nameTable.Find("img.bookmark, img.delbookmark").Length() > 0,
	}
	if torrent.Title == "" {
		torrent.Title = cleanText(titleLink.Text())
	}
	torrent.SizeBytes = parseSizeBytes(torrent.SizeText)

	nameTable.Find(`span[style*="background-color"]`).Each(func(_ int, span *goquery.Selection) {
		tag := cleanText(span.Text())
		if tag != "" {
			torrent.Tags = append(torrent.Tags, tag)
		}
	})

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

	descriptionSource := nameTable.Find("td.embedded").Eq(1).Clone()
	descriptionSource.Find("a, img, font, span, div").Remove()
	torrent.Description = cleanText(descriptionSource.Text())
	return torrent
}

func parsePagination(doc *goquery.Document, baseURL string, selectors ParseSelectors) Pagination {
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
			URL:   absoluteURL(baseURL, href),
		})
	})
	return pagination
}

func appendIfOptions(groups []SearchGroup, name, label string, options []SearchOption) []SearchGroup {
	if len(options) == 0 {
		return groups
	}
	return append(groups, SearchGroup{Name: name, Label: label, Options: options})
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
