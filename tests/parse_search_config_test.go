package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"nexusbridge/internal/parser"
)

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
	if len(reloaded.HTML.Category) == 0 {
		t.Fatalf("reloaded category definition is empty")
	}
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
}
