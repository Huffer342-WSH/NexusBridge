// Package network 管理 NexusBridge 启动前需要写入的代理环境变量。
package network

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	// ProxyModeSystem 恢复进程启动时继承的系统代理环境变量。
	ProxyModeSystem = "system"
	// ProxyModeManual 将用户填写的代理地址写入 HTTP_PROXY 和 HTTPS_PROXY。
	ProxyModeManual = "manual"
	// ProxyModeDirect 清除 HTTP_PROXY 和 HTTPS_PROXY。
	ProxyModeDirect = "direct"
	// DefaultNoProxy 是网络设置页面可恢复的默认直连规则，一行一项。
	DefaultNoProxy = `localhost
127.*
192.168.*
10.*
172.16.*
172.17.*
172.18.*
172.19.*
172.20.*
172.21.*
172.22.*
172.23.*
172.24.*
172.25.*
172.26.*
172.27.*
172.28.*
172.29.*
172.30.*
172.31.*
dl.steam.clngaa.com
st.dl.eccdnx.com
*.bilibili.com
*.bilivideo.com
*yuanshen.com
ug.local`
)

type environmentValue struct {
	value string
	set   bool
}

var originalProxyEnvironment = map[string]environmentValue{
	"HTTP_PROXY":  readEnvironment("HTTP_PROXY"),
	"http_proxy":  readEnvironment("http_proxy"),
	"HTTPS_PROXY": readEnvironment("HTTPS_PROXY"),
	"https_proxy": readEnvironment("https_proxy"),
}

// NormalizeProxyMode 返回规范化后的代理模式，空值兼容为系统代理。
func NormalizeProxyMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ProxyModeSystem:
		return ProxyModeSystem
	case ProxyModeManual:
		return ProxyModeManual
	case ProxyModeDirect:
		return ProxyModeDirect
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

// ParseManualProxyURL 解析手动代理地址并补全省略的 http 协议。
func ParseManualProxyURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("network.proxy_url is required when network.mode is manual")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		if err != nil {
			return nil, fmt.Errorf("network.proxy_url is invalid: %w", err)
		}
		return nil, fmt.Errorf("network.proxy_url must include host")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5":
		return parsed, nil
	default:
		return nil, fmt.Errorf("network.proxy_url scheme must be http, https, or socks5")
	}
}

// ValidateProxyConfig 验证代理模式和手动代理地址。
func ValidateProxyConfig(mode, proxyURL string) error {
	switch NormalizeProxyMode(mode) {
	case ProxyModeSystem, ProxyModeDirect:
		return nil
	case ProxyModeManual:
		_, err := ParseManualProxyURL(proxyURL)
		return err
	default:
		return fmt.Errorf("network.mode must be system, manual, or direct")
	}
}

// NormalizeNoProxyLines 将分号、逗号或换行分隔的规则规范为一行一项。
func NormalizeNoProxyLines(value string) string {
	seen := map[string]struct{}{}
	lines := make([]string, 0)
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '\r' || r == '\n'
	}) {
		item = strings.TrimSpace(item)
		key := strings.ToLower(item)
		if item == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		lines = append(lines, item)
	}
	return strings.Join(lines, "\n")
}

// ApplyEnvironment 按设置写入标准 HTTP 代理环境变量，并为 qB 地址补充 NO_PROXY。
func ApplyEnvironment(mode, proxyURL, noProxy, qbURL string) error {
	mode = NormalizeProxyMode(mode)
	if err := ValidateProxyConfig(mode, proxyURL); err != nil {
		return err
	}
	switch mode {
	case ProxyModeSystem:
		for name, value := range originalProxyEnvironment {
			if err := restoreEnvironment(name, value); err != nil {
				return err
			}
		}
	case ProxyModeManual:
		if err := setProxyEnvironment(proxyURL); err != nil {
			return err
		}
	case ProxyModeDirect:
		for name := range originalProxyEnvironment {
			if err := os.Unsetenv(name); err != nil {
				return fmt.Errorf("clear %s: %w", name, err)
			}
		}
	}
	return setNoProxyEnvironment(noProxy, qbURL)
}

// EnvironmentNoProxy 将页面规则转换为 Go 标准库可识别的 NO_PROXY 值。
func EnvironmentNoProxy(noProxy, qbURL string) string {
	values := strings.Split(NormalizeNoProxyLines(noProxy), "\n")
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values)+2)
	appendValue := func(value string) {
		for _, normalized := range normalizeNoProxyValue(value) {
			key := strings.ToLower(normalized)
			if normalized == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, normalized)
		}
	}
	for _, value := range values {
		appendValue(value)
	}
	if qbURL != "" {
		if !strings.Contains(qbURL, "://") {
			qbURL = "http://" + qbURL
		}
		if parsed, err := url.Parse(qbURL); err == nil {
			appendValue(parsed.Hostname())
		}
	}
	return strings.Join(result, ",")
}

// normalizeNoProxyValue 将界面中的通配符转换为 NO_PROXY 支持的 CIDR 或域名规则。
func normalizeNoProxyValue(value string) []string {
	value = strings.TrimSpace(value)
	if cidr := wildcardIPv4CIDR(value); cidr != "" {
		return []string{cidr}
	}
	if strings.HasPrefix(value, "*.") {
		domain := strings.TrimPrefix(value, "*.")
		return []string{domain, "." + domain}
	}
	if strings.HasPrefix(value, "*") {
		domain := strings.TrimPrefix(value, "*")
		domain = strings.TrimPrefix(domain, ".")
		return []string{domain, "." + domain}
	}
	return []string{value}
}

// wildcardIPv4CIDR 将 127.* 和 192.168.* 一类规则转换为 CIDR。
func wildcardIPv4CIDR(value string) string {
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return ""
	}
	fixed := 0
	for _, part := range parts {
		if part == "*" {
			break
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return ""
			}
		}
		fixed++
	}
	if fixed == len(parts) || parts[fixed] != "*" {
		return ""
	}
	for _, part := range parts[fixed:] {
		if part != "*" {
			return ""
		}
	}
	octets := make([]string, 4)
	for index := range octets {
		if index < fixed {
			octets[index] = parts[index]
		} else {
			octets[index] = "0"
		}
	}
	return strings.Join(octets, ".") + fmt.Sprintf("/%d", fixed*8)
}

func readEnvironment(name string) environmentValue {
	value, set := os.LookupEnv(name)
	return environmentValue{value: value, set: set}
}

func restoreEnvironment(name string, value environmentValue) error {
	if !value.set {
		return os.Unsetenv(name)
	}
	if err := os.Setenv(name, value.value); err != nil {
		return fmt.Errorf("restore %s: %w", name, err)
	}
	return nil
}

func setProxyEnvironment(proxyURL string) error {
	if !strings.Contains(proxyURL, "://") {
		proxyURL = "http://" + strings.TrimSpace(proxyURL)
	}
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		if err := os.Setenv(name, proxyURL); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	return nil
}

func setNoProxyEnvironment(noProxy, qbURL string) error {
	value := EnvironmentNoProxy(noProxy, qbURL)
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	return nil
}
