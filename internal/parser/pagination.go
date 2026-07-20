package parser

import (
	"html"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"nexusbridge/internal/stringutil"
	"nexusbridge/internal/urlutil"
)

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
	return stringutil.FirstNonEmpty(values...)
}

func normalizeBaseURL(raw string) string {
	return urlutil.NormalizeBaseURL(raw)
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}
