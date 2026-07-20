package parser

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	promotionTimeRE = regexp.MustCompile(`title=&quot;([^&]+)&quot;&gt;([^<]+)&lt;/span&gt;`)
	categoryIDRE    = regexp.MustCompile(`\d+`)
)

// ParsePage 使用兼容解析选项读取站点列表页。
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

// ParsePageWithDefinition 使用站点定义读取列表页。
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
