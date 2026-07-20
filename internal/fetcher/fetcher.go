package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"nexusbridge/internal/urlutil"
)

const defaultTimeout = 30 * time.Second

type FetchOptions struct {
	BaseURL        string
	URL            string
	CookieFile     string
	CurlFile       string
	CookieHeader   string
	Cookies        []*http.Cookie
	CookieResolver func(string) ([]*http.Cookie, error)
	Headers        http.Header
	Timeout        time.Duration
	Params         map[string]string
	Client         *http.Client
	Logger         *slog.Logger
	LogRequest     bool
	LogSensitive   bool
}

type FetchResult struct {
	URL        string
	StatusCode int
	Body       []byte
	FetchedAt  time.Time
}

type CurlRequest struct {
	URL          string
	Headers      http.Header
	CookieHeader string
}

// BuildNexusPHPHeaders 构造 NexusPHP 浏览器请求头。
func BuildNexusPHPHeaders(baseURL string) http.Header {
	baseURL = normalizeBaseURL(baseURL)
	return http.Header{
		"Accept":                      []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"Accept-Language":             []string{"zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7,pt-BR;q=0.6,pt;q=0.5"},
		"Cache-Control":               []string{"no-cache"},
		"Dnt":                         []string{"1"},
		"Pragma":                      []string{"no-cache"},
		"Priority":                    []string{"u=0, i"},
		"Referer":                     []string{joinURL(baseURL, "/torrents.php")},
		"Sec-Ch-Ua":                   []string{`"Not;A=Brand";v="8", "Chromium";v="150", "Microsoft Edge";v="150"`},
		"Sec-Ch-Ua-Arch":              []string{`"x86"`},
		"Sec-Ch-Ua-Bitness":           []string{`"64"`},
		"Sec-Ch-Ua-Full-Version":      []string{`"150.0.4078.48"`},
		"Sec-Ch-Ua-Full-Version-List": []string{`"Not;A=Brand";v="8.0.0.0", "Chromium";v="150.0.7871.47", "Microsoft Edge";v="150.0.4078.48"`},
		"Sec-Ch-Ua-Mobile":            []string{"?0"},
		"Sec-Ch-Ua-Model":             []string{`""`},
		"Sec-Ch-Ua-Platform":          []string{`"Windows"`},
		"Sec-Ch-Ua-Platform-Version":  []string{`"19.0.0"`},
		"Sec-Fetch-Dest":              []string{"document"},
		"Sec-Fetch-Mode":              []string{"navigate"},
		"Sec-Fetch-Site":              []string{"same-origin"},
		"Sec-Fetch-User":              []string{"?1"},
		"Upgrade-Insecure-Requests":   []string{"1"},
		"User-Agent":                  []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36 Edg/150.0.0.0"},
	}
}

func LoadCookiesJSON(path string) ([]*http.Cookie, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("cookie file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cookie file: %w", err)
	}

	values := map[string]string{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse cookie json: %w", err)
	}

	cookies := make([]*http.Cookie, 0, len(values))
	for name, value := range values {
		if strings.TrimSpace(name) == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	}
	return cookies, nil
}

func LoadCookiesFromHeader(header string) ([]*http.Cookie, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, errors.New("cookie header is required")
	}

	cookies := []*http.Cookie{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid cookie pair")
		}
		cookies = append(cookies, &http.Cookie{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	return cookies, nil
}

func LoadCookiesFromCurlFile(path string) ([]*http.Cookie, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("curl file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read curl file: %w", err)
	}

	header, err := ExtractCookieHeaderFromCurl(string(data))
	if err != nil {
		return nil, err
	}
	return LoadCookiesFromHeader(header)
}

func ParseCurlRequest(script string) (CurlRequest, error) {
	requestURL, err := extractFirstCurlURL(script)
	if err != nil {
		return CurlRequest{}, err
	}
	cookieHeader, err := ExtractCookieHeaderFromCurl(script)
	if err != nil {
		return CurlRequest{}, err
	}
	headers := ExtractHeadersFromCurl(script)
	return CurlRequest{
		URL:          requestURL,
		Headers:      headers,
		CookieHeader: cookieHeader,
	}, nil
}

func ExtractHeadersFromCurl(script string) http.Header {
	headers := http.Header{}
	for _, value := range extractFlagValues(script, "-H") {
		name, headerValue, ok := strings.Cut(value, ":")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		headers.Add(strings.TrimSpace(name), strings.TrimSpace(headerValue))
	}
	return headers
}

func ExtractCookieHeaderFromCurl(script string) (string, error) {
	index := strings.Index(script, "-b")
	if index < 0 {
		return "", errors.New("curl script does not contain a -b cookie argument")
	}
	rest := strings.TrimSpace(script[index+len("-b"):])
	if rest == "" {
		return "", errors.New("curl -b cookie argument is empty")
	}
	quote := rest[0]
	if quote != '\'' && quote != '"' {
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return "", errors.New("curl -b cookie argument is empty")
		}
		return fields[0], nil
	}
	end := strings.IndexByte(rest[1:], quote)
	if end < 0 {
		return "", errors.New("curl -b cookie argument is not closed")
	}
	return rest[1 : end+1], nil
}

// FetchTorrentsPage 抓取默认 torrents.php 页面。
func FetchTorrentsPage(ctx context.Context, opts FetchOptions) (FetchResult, error) {
	baseURL := normalizeBaseURL(opts.BaseURL)
	if baseURL == "" {
		return FetchResult{}, errors.New("base url is required")
	}
	opts.URL = joinURL(baseURL, "/torrents.php")
	opts.BaseURL = baseURL
	return fetch(ctx, opts)
}

// FetchTorrentsURL 抓取指定的种子列表 URL。
func FetchTorrentsURL(ctx context.Context, fullURL string, opts FetchOptions) (FetchResult, error) {
	if strings.TrimSpace(fullURL) == "" {
		return FetchResult{}, errors.New("url is required")
	}
	parsed, err := url.Parse(normalizeBaseURL(fullURL))
	if err != nil {
		return FetchResult{}, fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return FetchResult{}, fmt.Errorf("url must include host")
	}
	opts.URL = parsed.String()
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = parsed.Scheme + "://" + parsed.Host
	}
	return fetch(ctx, opts)
}

func fetch(ctx context.Context, opts FetchOptions) (FetchResult, error) {
	requestURL, err := buildRequestURL(opts.URL, opts.Params)
	if err != nil {
		return FetchResult{}, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	client := requestClient(opts.Client, timeout, opts.CookieResolver)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create request: %w", err)
	}

	for key, values := range BuildNexusPHPHeaders(opts.BaseURL) {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for key, values := range opts.Headers {
		req.Header.Del(key)
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	cookies, err := resolveCookies(opts, requestURL)
	if err != nil {
		return FetchResult{}, err
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	if opts.LogRequest {
		logFetchRequest(opts, req, cookies)
	}

	resp, err := client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch torrents page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchResult{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logFetchResponse(opts, requestURL, resp.StatusCode, len(body))
		return FetchResult{
			URL:        requestURL,
			StatusCode: resp.StatusCode,
			Body:       body,
			FetchedAt:  time.Now().UTC(),
		}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	logFetchResponse(opts, requestURL, resp.StatusCode, len(body))

	return FetchResult{
		URL:        requestURL,
		StatusCode: resp.StatusCode,
		Body:       body,
		FetchedAt:  time.Now().UTC(),
	}, nil
}

// logFetchRequest 输出抓取请求调试信息。
func logFetchRequest(opts FetchOptions, req *http.Request, cookies []*http.Cookie) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("fetch request",
		"method", req.Method,
		"url", req.URL.String(),
		"headers", headerValues(req.Header, opts.LogSensitive),
		"cookie_names", requestCookieNames(cookies),
	)
}

// logFetchResponse 输出抓取响应调试信息。
func logFetchResponse(opts FetchOptions, requestURL string, statusCode int, bodyBytes int) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("fetch response", "url", requestURL, "status", statusCode, "body_bytes", bodyBytes)
}

// headerValues 转换请求头为可记录结构。
func headerValues(headers http.Header, includeSensitive bool) map[string]string {
	values := map[string]string{}
	for key, value := range headers {
		if strings.EqualFold(key, "Cookie") {
			values[key] = "[redacted]"
			continue
		}
		if !includeSensitive && (strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Proxy-Authorization") || strings.EqualFold(key, "X-API-Key")) {
			values[key] = "[redacted]"
			continue
		}
		values[key] = strings.Join(value, ", ")
	}
	return values
}

func loadCookies(opts FetchOptions) ([]*http.Cookie, error) {
	switch {
	case len(opts.Cookies) > 0:
		return opts.Cookies, nil
	case strings.TrimSpace(opts.CookieHeader) != "":
		return LoadCookiesFromHeader(opts.CookieHeader)
	case strings.TrimSpace(opts.CookieFile) != "":
		return LoadCookiesJSON(opts.CookieFile)
	case strings.TrimSpace(opts.CurlFile) != "":
		return LoadCookiesFromCurlFile(opts.CurlFile)
	default:
		return nil, nil
	}
}

// resolveCookies 为初始请求解析 Cookie，未配置动态策略时兼容原有来源。
func resolveCookies(opts FetchOptions, requestURL string) ([]*http.Cookie, error) {
	if opts.CookieResolver != nil {
		return opts.CookieResolver(requestURL)
	}
	return loadCookies(opts)
}

// requestClient 为动态 Cookie 策略复制客户端并在每次重定向时重新选择 Cookie。
func requestClient(source *http.Client, timeout time.Duration, resolver func(string) ([]*http.Cookie, error)) *http.Client {
	client := &http.Client{Timeout: timeout}
	if source != nil {
		cloned := *source
		client = &cloned
		if client.Timeout == 0 {
			client.Timeout = timeout
		}
	}
	if resolver == nil {
		return client
	}
	previousRedirect := client.CheckRedirect
	client.Jar = nil
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		req.Header.Del("Cookie")
		cookies, err := resolver(req.URL.String())
		if err != nil {
			return err
		}
		for _, cookie := range cookies {
			if cookie != nil && strings.TrimSpace(cookie.Name) != "" {
				req.AddCookie(cookie)
			}
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		return nil
	}
	return client
}

// requestCookieNames 返回请求携带的 Cookie 名称且不暴露值。
func requestCookieNames(cookies []*http.Cookie) []string {
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && strings.TrimSpace(cookie.Name) != "" {
			names = append(names, cookie.Name)
		}
	}
	return names
}

func extractFirstCurlURL(script string) (string, error) {
	index := strings.Index(script, "curl")
	if index < 0 {
		return "", errors.New("curl script does not contain a curl command")
	}
	rest := strings.TrimSpace(script[index+len("curl"):])
	if rest == "" {
		return "", errors.New("curl url is missing")
	}
	value, _, err := readShellValue(rest)
	if err != nil {
		return "", fmt.Errorf("parse curl url: %w", err)
	}
	return value, nil
}

func extractFlagValues(script, flag string) []string {
	values := []string{}
	searchFrom := 0
	for {
		index := strings.Index(script[searchFrom:], flag)
		if index < 0 {
			return values
		}
		index += searchFrom
		beforeOK := index == 0 || isShellSpace(script[index-1])
		after := index + len(flag)
		afterOK := after >= len(script) || isShellSpace(script[after])
		if !beforeOK || !afterOK {
			searchFrom = after
			continue
		}
		rest := strings.TrimSpace(script[after:])
		value, consumed, err := readShellValue(rest)
		if err == nil {
			values = append(values, value)
			searchFrom = after + consumed
			continue
		}
		searchFrom = after
	}
}

func readShellValue(input string) (string, int, error) {
	if input == "" {
		return "", 0, errors.New("empty value")
	}
	quote := input[0]
	if quote == '\'' || quote == '"' {
		end := strings.IndexByte(input[1:], quote)
		if end < 0 {
			return "", 0, errors.New("quoted value is not closed")
		}
		return input[1 : end+1], end + 2, nil
	}
	for index, r := range input {
		if isShellSpace(byte(r)) {
			return input[:index], index, nil
		}
	}
	return input, len(input), nil
}

func isShellSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\\'
}

func buildRequestURL(rawURL string, params map[string]string) (string, error) {
	parsed, err := url.Parse(normalizeBaseURL(rawURL))
	if err != nil {
		return "", fmt.Errorf("parse request url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("request url must include host")
	}

	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func normalizeBaseURL(raw string) string {
	return urlutil.NormalizeBaseURL(raw)
}

func joinURL(baseURL, path string) string {
	baseURL = normalizeBaseURL(baseURL)
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}
