package parser

import (
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"
)

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
