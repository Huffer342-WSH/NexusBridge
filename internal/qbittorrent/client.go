package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	AuthMode string   `json:"auth_mode"`
	URL      string   `json:"url"`
	APIKey   string   `json:"api_key"`
	Username string   `json:"username"`
	UserID   string   `json:"user_id"`
	Password string   `json:"password"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

type Client struct {
	cfg           Config
	client        *http.Client
	authMu        sync.Mutex
	authenticated bool
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("qbittorrent.url is required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg: normalizeConfig(cfg),
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}, nil
}

// Test 校验 qBittorrent WebAPI 是否可访问。
func (c *Client) Test(ctx context.Context) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodGet, "/api/v2/app/webapiVersion", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, "webapiVersion")
}

// Login 使用 qBittorrent 原生账号密码登录接口获取 SID cookie。
func (c *Client) Login(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.login(ctx)
}

// login 执行一次 qB 原生登录并记录认证状态。
func (c *Client) login(ctx context.Context) error {
	c.authenticated = false
	username := firstNonEmpty(c.cfg.Username, c.cfg.UserID)
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", c.cfg.Password)
	resp, err := c.doAuthed(ctx, http.MethodPost, "/api/v2/auth/login", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent login status %d: %s", resp.StatusCode, trimBody(body))
	}
	if strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qbittorrent login failed")
	}
	c.authenticated = true
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, values url.Values, op string, target any) error {
	if len(values) > 0 {
		path += "?" + values.Encode()
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, op); err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode qbittorrent %s response: %w", op, err)
	}
	return nil
}

func (c *Client) postForm(ctx context.Context, path string, values url.Values, op string) error {
	resp, err := c.do(ctx, http.MethodPost, path, strings.NewReader(values.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, op)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}
	resp, err := c.doAuthed(ctx, method, path, body, contentType)
	if err == nil && c.authMode() == "uid" && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		c.authMu.Lock()
		c.authenticated = false
		c.authMu.Unlock()
	}
	return resp, err
}

func (c *Client) doAuthed(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.client.Do(req)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	base := strings.TrimRight(c.cfg.URL, "/")
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", base)
	req.Header.Set("Origin", base)
	c.addAPIKeyHeaders(req)
	return req, nil
}

// ensureAuth 按配置的认证模式准备请求认证。
func (c *Client) ensureAuth(ctx context.Context) error {
	if c.authMode() == "api_key" {
		if strings.TrimSpace(c.cfg.APIKey) == "" {
			return fmt.Errorf("qbittorrent api_key is required")
		}
		return nil
	}
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.authenticated {
		return nil
	}
	return c.login(ctx)
}

// authMode 返回规范化后的 qB 认证模式。
func (c *Client) authMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.cfg.AuthMode))
	if mode == "" {
		return "uid"
	}
	return mode
}

// addAPIKeyHeaders 给 API Key 模式请求添加认证头。
func (c *Client) addAPIKeyHeaders(req *http.Request) {
	if c.authMode() != "api_key" || strings.TrimSpace(c.cfg.APIKey) == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("X-API-Key", c.cfg.APIKey)
	if userID := firstNonEmpty(c.cfg.UserID, c.cfg.Username); userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
}

// IsCompleted 判断 qBittorrent 种子任务是否已完成下载。
func IsCompleted(t TorrentInfo) bool {
	state := strings.ToLower(t.State)
	if t.Progress >= 1 {
		return true
	}
	switch state {
	case "uploading", "stalledup", "queuedup", "pausedup", "stoppedup", "forcedup", "checkingup":
		return true
	default:
		return false
	}
}

// normalizeConfig 补齐 qB 客户端配置默认值。
func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.AuthMode) == "" {
		cfg.AuthMode = "uid"
	}
	cfg.URL = normalizeURL(cfg.URL)
	if cfg.Username == "" && cfg.UserID != "" {
		cfg.Username = cfg.UserID
	}
	if cfg.UserID == "" && cfg.Username != "" {
		cfg.UserID = cfg.Username
	}
	return cfg
}

func normalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.Contains(rawURL, "://") {
		return rawURL
	}
	return "http://" + rawURL
}

func checkStatus(resp *http.Response, op string) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("qbittorrent %s status %d: %s", op, resp.StatusCode, trimBody(body))
}

func trimBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

func joinHashes(hashes []string) string {
	values := nonEmptyStrings(hashes)
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 && strings.EqualFold(values[0], "all") {
		return "all"
	}
	return strings.Join(values, "|")
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
