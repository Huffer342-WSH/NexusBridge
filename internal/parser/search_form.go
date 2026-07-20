package parser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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
