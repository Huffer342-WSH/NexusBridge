// helpers_test.go 提供真实集成测试共用的环境与数据目录辅助函数。
package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/runtimeconfig"
	"nexusbridge/internal/storage"
)

type testSettings struct {
	BaseURL                   string
	ConfigPath                string
	CurlFile                  string
	DBPath                    string
	SaveHTMLPath              string
	SiteID                    string
	UpdatedSiteDefinitionPath string
}

func loadTestSettings(t *testing.T) testSettings {
	t.Helper()

	baseURL := requiredEnv(t, "NEXUSBRIDGE_TEST_BASE_URL")
	configPath := resolveReadableTestPath(t, "NEXUSBRIDGE_TEST_CONFIG", requiredEnv(t, "NEXUSBRIDGE_TEST_CONFIG"))
	siteID := requiredEnv(t, "NEXUSBRIDGE_TEST_SITE_ID")
	curlFile := resolveReadableTestPath(t, "NEXUSBRIDGE_TEST_CURL_FILE", requiredEnv(t, "NEXUSBRIDGE_TEST_CURL_FILE"))

	dbPath := strings.TrimSpace(os.Getenv("NEXUSBRIDGE_TEST_DB_FILE"))
	if dbPath == "" {
		dbPath = testDataPath(t, "cookies.db")
	}
	saveHTMLPath := strings.TrimSpace(os.Getenv("NEXUSBRIDGE_TEST_SAVE_HTML"))
	if saveHTMLPath == "" {
		saveHTMLPath = testDataPath(t, "torrents_page.html")
	}
	updatedSiteDefinitionPath := strings.TrimSpace(os.Getenv("NEXUSBRIDGE_TEST_UPDATED_SITE_CONFIG"))
	if updatedSiteDefinitionPath == "" {
		updatedSiteDefinitionPath = testDataPath(t, siteID+".updated.json")
	}

	return testSettings{
		BaseURL:                   strings.TrimRight(baseURL, "/"),
		ConfigPath:                configPath,
		CurlFile:                  curlFile,
		DBPath:                    dbPath,
		SaveHTMLPath:              saveHTMLPath,
		SiteID:                    siteID,
		UpdatedSiteDefinitionPath: updatedSiteDefinitionPath,
	}
}

// testDataPath 返回项目 data/tests 下的默认测试产物路径。
func testDataPath(t *testing.T, names ...string) string {
	t.Helper()
	dataDir, err := runtimeconfig.DevelopmentDataDir()
	if err != nil {
		t.Fatalf("resolve development data directory: %v", err)
	}
	parts := append([]string{dataDir, "tests"}, names...)
	return filepath.Join(parts...)
}

// requiredEnv 读取真实集成测试变量，缺失时跳过当前测试。
func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Skipf("%s is required for real integration tests", name)
	}
	return value
}

// resolveReadableTestPath 从当前包或仓库根目录解析测试输入文件。
func resolveReadableTestPath(t *testing.T, name, value string) string {
	t.Helper()
	if _, err := os.Stat(value); err == nil {
		return value
	}
	parentPath := filepath.Join("..", value)
	if _, err := os.Stat(parentPath); err == nil {
		return parentPath
	}
	t.Fatalf("%s must point to a readable file: %s", name, value)
	return value
}

func readCurlRequest(t *testing.T, path string) fetcher.CurlRequest {
	t.Helper()

	curlScript, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read curl file: %v", err)
	}
	curlRequest, err := fetcher.ParseCurlRequest(string(curlScript))
	if err != nil {
		t.Fatalf("parse curl request: %v", err)
	}
	return curlRequest
}

func loadSiteDefinition(t *testing.T, settings testSettings) parser.SiteDefinition {
	t.Helper()

	prepared, err := runtimeconfig.Load(runtimeconfig.Options{ExplicitConfig: settings.ConfigPath})
	if err != nil {
		t.Fatalf("load test config: %v", err)
	}
	sitesDir := prepared.Config.SitesDir
	definitions, err := parser.LoadSiteDefinitionsDir(sitesDir)
	if err != nil {
		t.Fatalf("load site definitions from %s: %v", sitesDir, err)
	}
	for _, definition := range definitions {
		if definition.ID != settings.SiteID {
			continue
		}
		definition.Domain = settings.BaseURL
		return definition
	}
	t.Fatalf("site %q not found in %s", settings.SiteID, sitesDir)
	return parser.SiteDefinition{}
}

func loadFixtureSiteDefinition(t *testing.T) parser.SiteDefinition {
	t.Helper()

	definition, err := parser.LoadSiteDefinition(filepath.Join("fixtures", "site_parse_config.json"))
	if err != nil {
		t.Fatalf("load fixture site definition: %v", err)
	}
	return definition
}

func readFixtureHTML(t *testing.T) []byte {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("fixtures", "torrents_page.html"))
	if err != nil {
		t.Fatalf("read fixture html: %v", err)
	}
	return body
}

func fetchConfiguredHTMLFromDB(t *testing.T, ctx context.Context, settings testSettings, definition parser.SiteDefinition) []byte {
	t.Helper()

	store, err := storage.OpenSQLite(ctx, settings.DBPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	cookies, err := store.LoadCookies(ctx, settings.BaseURL)
	if err != nil {
		t.Fatalf("load cookies from sqlite: %v", err)
	}
	if len(cookies) == 0 {
		t.Fatalf("expected cookies in %s; run TestFetchTorrentsPageHTML first", settings.DBPath)
	}

	curlRequest := readCurlRequest(t, settings.CurlFile)
	siteConfig := parser.SiteConfigFromDefinition(definition)
	result, err := fetcher.FetchTorrentsURL(ctx, siteConfig.URL, fetcher.FetchOptions{
		BaseURL: siteConfig.BaseURL,
		Cookies: cookies,
		Headers: curlRequest.Headers,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("fetch configured page with database cookies: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if len(result.Body) == 0 {
		t.Fatal("expected non-empty HTML body")
	}
	return result.Body
}
