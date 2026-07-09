// request_policy_test.go 验证站点跨域 Cookie 选择与重定向重新解析。
package tests

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"nexusbridge/internal/fetcher"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/requestpolicy"
)

// TestRequestPolicyCookieSelection 验证同主机默认策略和 KamePT 图片域白名单。
func TestRequestPolicyCookieSelection(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "session", Value: "private"},
		{Name: "cf_clearance", Value: "clearance"},
	}
	rules := []requestpolicy.Rule{
		{DomainSuffix: "kamept.com", CookieNames: []string{"session"}},
		{DomainSuffix: "p.kamept.com", CookieNames: []string{"cf_clearance", "missing"}},
	}

	sameSite, err := requestpolicy.ResolveCookies("https://kamept.com", "https://kamept.com/torrents.php", rules, cookies)
	if err != nil {
		t.Fatal(err)
	}
	if !sameSite.SameSite || !slices.Equal(sameSite.SelectedNames, []string{"cf_clearance", "session"}) {
		t.Fatalf("expected all same-site cookies, got %#v", sameSite)
	}

	image, err := requestpolicy.ResolveCookies("https://kamept.com", "https://A.P.KAMEPT.COM./cover.webp", rules, cookies)
	if err != nil {
		t.Fatal(err)
	}
	if image.MatchedSuffix != "p.kamept.com" || !slices.Equal(image.SelectedNames, []string{"cf_clearance"}) || !slices.Equal(image.MissingNames, []string{"missing"}) {
		t.Fatalf("expected longest KamePT image rule, got %#v", image)
	}
	if len(image.Cookies) != 1 || image.Cookies[0].Name != "cf_clearance" {
		t.Fatalf("unexpected image cookies: %#v", image.Cookies)
	}

	unsafe, err := requestpolicy.ResolveCookies("https://kamept.com", "https://fakep.kamept.com.evil/cover.webp", rules, cookies)
	if err != nil {
		t.Fatal(err)
	}
	if len(unsafe.Cookies) != 0 || unsafe.MatchedSuffix != "" {
		t.Fatalf("expected unsafe suffix to remain isolated, got %#v", unsafe)
	}
}

// TestRequestPolicyMissingCookieStillRequests 验证缺少白名单 Cookie 时返回空选择而不是错误。
func TestRequestPolicyMissingCookieStillRequests(t *testing.T) {
	resolution, err := requestpolicy.ResolveCookies(
		"https://kamept.com",
		"https://p.kamept.com/image.webp",
		[]requestpolicy.Rule{{DomainSuffix: "p.kamept.com", CookieNames: []string{"cf_clearance"}}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolution.Cookies) != 0 || !slices.Equal(resolution.MissingNames, []string{"cf_clearance"}) {
		t.Fatalf("unexpected missing-cookie resolution: %#v", resolution)
	}
}

// TestRequestPolicyValidation 验证空白名单和重复域名后缀会被拒绝。
func TestRequestPolicyValidation(t *testing.T) {
	cases := [][]requestpolicy.Rule{
		{{DomainSuffix: "p.kamept.com"}},
		{{DomainSuffix: "p.kamept.com", CookieNames: []string{"a"}}, {DomainSuffix: "P.KAMEPT.COM.", CookieNames: []string{"b"}}},
		{{DomainSuffix: "https://p.kamept.com/path", CookieNames: []string{"a"}}},
	}
	for _, rules := range cases {
		if err := requestpolicy.ValidateRules(requestpolicy.NormalizeRules(rules)); err == nil {
			t.Fatalf("expected invalid rules: %#v", rules)
		}
	}
}

// TestSiteDefinitionRequestRules 验证站点定义会加载、规范化并校验请求规则。
func TestSiteDefinitionRequestRules(t *testing.T) {
	definition, err := parser.DecodeSiteDefinition([]byte(`{
		"id":"kamept","name":"KamePT","domain":"https://kamept.com",
		"request_rules":[{"domain_suffix":"P.KAMEPT.COM.","cookie_names":["cf_clearance"]}],
		"html":{}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(definition.RequestRules) != 1 || definition.RequestRules[0].DomainSuffix != "p.kamept.com" {
		t.Fatalf("unexpected normalized request rules: %#v", definition.RequestRules)
	}
}

// TestFetcherReevaluatesCookiesOnRedirect 验证重定向不会继承上一跳 Cookie。
func TestFetcherReevaluatesCookiesOnRedirect(t *testing.T) {
	var secondCookie string
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("ok"))
	}))
	defer second.Close()

	var firstCookie string
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCookie = r.Header.Get("Cookie")
		http.Redirect(w, r, second.URL+"/image.webp", http.StatusFound)
	}))
	defer first.Close()

	resolver := func(requestURL string) ([]*http.Cookie, error) {
		if requestURL == first.URL {
			return []*http.Cookie{{Name: "cf_clearance", Value: "secret"}}, nil
		}
		return nil, nil
	}
	if _, err := fetcher.FetchTorrentsURL(context.Background(), first.URL, fetcher.FetchOptions{CookieResolver: resolver}); err != nil {
		t.Fatal(err)
	}
	if firstCookie != "cf_clearance=secret" {
		t.Fatalf("expected first request cookie, got %q", firstCookie)
	}
	if secondCookie != "" {
		t.Fatalf("redirect leaked cookie: %q", secondCookie)
	}
}

// TestFetcherNeverLogsCookieValues 验证敏感抓取日志也只输出 Cookie 名称。
func TestFetcherNeverLogsCookieValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	_, err := fetcher.FetchTorrentsURL(context.Background(), server.URL, fetcher.FetchOptions{
		Cookies: []*http.Cookie{{Name: "cf_clearance", Value: "must-not-appear"}},
		Logger:  logger, LogRequest: true, LogSensitive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	logged := output.String()
	if strings.Contains(logged, "must-not-appear") || !strings.Contains(logged, "cf_clearance") {
		t.Fatalf("unexpected cookie log: %s", logged)
	}
}
