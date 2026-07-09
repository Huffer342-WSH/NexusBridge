package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nexusbridge/internal/parser"
)

func TestCompleteSiteDefinitionSearchConfigFromFixtureHTML(t *testing.T) {
	definition := loadFixtureSiteDefinition(t)
	body := readFixtureHTML(t)

	parsed, err := parser.ParsePageWithDefinition(body, definition)
	if err != nil {
		t.Fatalf("parse fixture page: %v", err)
	}
	assertSearchConfig(t, parsed)

	updated := parser.UpdateSiteDefinitionSearchOptions(definition, parsed)
	outputPath := filepath.Join(t.TempDir(), "site.completed.json")
	if err := parser.ExportSiteDefinition(outputPath, updated); err != nil {
		t.Fatalf("export completed site definition: %v", err)
	}

	reloaded, err := parser.LoadSiteDefinition(outputPath)
	if err != nil {
		t.Fatalf("reload completed site definition: %v", err)
	}
	assertCompletedSearchDefinition(t, reloaded)
}

func TestParseSearchConfigFromFetchedHTML(t *testing.T) {
	settings := loadTestSettings(t)
	definition := loadSiteDefinition(t, settings)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	body := fetchConfiguredHTMLFromDB(t, ctx, settings, definition)
	parsed, err := parser.ParsePageWithDefinition(body, definition)
	if err != nil {
		t.Fatalf("parse configured page: %v", err)
	}
	assertSearchConfig(t, parsed)

	updated := parser.UpdateSiteDefinitionSearchOptions(definition, parsed)
	if err := parser.ExportSiteDefinition(settings.UpdatedSiteDefinitionPath, updated); err != nil {
		t.Fatalf("export updated site definition: %v", err)
	}

	reloaded, err := parser.LoadSiteDefinition(settings.UpdatedSiteDefinitionPath)
	if err != nil {
		t.Fatalf("reload updated site definition: %v", err)
	}
	if reloaded.ID != definition.ID {
		t.Fatalf("reloaded site id mismatch: %s != %s", reloaded.ID, definition.ID)
	}
	if len(reloaded.HTML.Search.Paths) == 0 || len(reloaded.HTML.Search.Params) == 0 {
		t.Fatalf("reloaded search definition is incomplete: %#v", reloaded.HTML.Search)
	}
	assertCompletedSearchDefinition(t, reloaded)
	if _, err := os.Stat(settings.UpdatedSiteDefinitionPath); err != nil {
		t.Fatalf("expected updated site definition file: %v", err)
	}
}

func assertSearchConfig(t *testing.T, parsed parser.ParsedPage) {
	t.Helper()
	if parsed.SiteConfig.SiteID == "" || parsed.SiteConfig.BaseURL == "" || parsed.SiteConfig.URL == "" {
		t.Fatalf("expected site config fields: %#v", parsed.SiteConfig)
	}
	if parsed.Selectors.SearchBox == "" {
		t.Fatalf("expected selectors: %#v", parsed.Selectors)
	}
	if len(parsed.SearchConfig.Categories) == 0 {
		t.Fatal("expected category search options")
	}
	if len(parsed.SearchConfig.Tags) == 0 {
		t.Fatal("expected tag search options")
	}
	if len(parsed.SearchConfig.Selects) == 0 {
		t.Fatal("expected select search options")
	}
	if len(parsed.SearchConfig.Ranges) == 0 {
		t.Fatal("expected range search options")
	}
	if parsed.SearchConfig.Keyword.Name != "search" {
		t.Fatalf("expected keyword search field, got %#v", parsed.SearchConfig.Keyword)
	}
	if parsed.SearchConfig.Fields.Empty() {
		t.Fatal("expected normalized search fields")
	}
}

func assertCompletedSearchDefinition(t *testing.T, definition parser.SiteDefinition) {
	t.Helper()
	fields := definition.HTML.Search.Fields
	if fields.Empty() {
		t.Fatalf("expected completed html.search.fields: %#v", definition.HTML.Search)
	}
	if !hasCheckboxOption(fields, "cat", "cat410") {
		t.Fatalf("expected cat410 in cat checkbox group: %#v", fields.Checkboxes)
	}
	if !hasCheckboxOption(fields, "source", "source1") {
		t.Fatalf("expected source1 in source checkbox group: %#v", fields.Checkboxes)
	}
	if !hasCheckboxOption(fields, "team", "team1") {
		t.Fatalf("expected team1 in team checkbox group: %#v", fields.Checkboxes)
	}
	if !hasSearchField(fields.Selects, "incldead", "select", false) {
		t.Fatalf("expected incldead select field: %#v", fields)
	}
	if !hasSearchField(fields.Ranges, "size", "number_range", false) {
		t.Fatalf("expected size number range field: %#v", fields)
	}
	if !hasSearchField(fields.Ranges, "added", "date_range", false) {
		t.Fatalf("expected added date range field: %#v", fields)
	}
	if fields.Keyword == nil || fields.Keyword.Name != "search" || fields.Keyword.Type != "string" {
		t.Fatalf("expected search string field: %#v", fields)
	}
	if fields.Tags == nil || fields.Tags.Name != "tag_id" || fields.Tags.Type != "tag" || !fields.Tags.Exclusive {
		t.Fatalf("expected exclusive tag_id field: %#v", fields)
	}
}

func hasCheckboxOption(fields parser.SiteSearchFields, groupName, optionName string) bool {
	for _, group := range fields.Checkboxes {
		if group.Name != groupName {
			continue
		}
		for _, option := range group.Options {
			if option.Name == optionName {
				return true
			}
		}
	}
	return false
}

func hasSearchField(fields []parser.SiteSearchField, name, kind string, exclusive bool) bool {
	for _, field := range fields {
		if field.Name == name && field.Type == kind && field.Exclusive == exclusive {
			return true
		}
	}
	return false
}
