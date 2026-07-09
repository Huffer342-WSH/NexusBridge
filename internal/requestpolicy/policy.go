// Package requestpolicy 提供站点请求的跨域 Cookie 选择策略。
package requestpolicy

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Rule 描述允许向目标域名转发的 Cookie 名称白名单。
type Rule struct {
	DomainSuffix string   `json:"domain_suffix"`
	CookieNames  []string `json:"cookie_names"`
}

// Resolution 描述一次目标地址匹配后的 Cookie 选择结果。
type Resolution struct {
	Cookies       []*http.Cookie
	TargetHost    string
	MatchedSuffix string
	SelectedNames []string
	MissingNames  []string
	SameSite      bool
}

// NormalizeRules 规范化域名后缀和 Cookie 名称但不隐藏配置错误。
func NormalizeRules(rules []Rule) []Rule {
	result := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		normalized := Rule{DomainSuffix: normalizeHost(rule.DomainSuffix)}
		seen := map[string]struct{}{}
		for _, name := range rule.CookieNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			normalized.CookieNames = append(normalized.CookieNames, name)
		}
		result = append(result, normalized)
	}
	return result
}

// ValidateRules 校验规则不存在空值、非法主机或重复后缀。
func ValidateRules(rules []Rule) error {
	seen := map[string]struct{}{}
	for index, rule := range rules {
		suffix := normalizeHost(rule.DomainSuffix)
		if suffix == "" || strings.ContainsAny(suffix, "/:*?@#") {
			return fmt.Errorf("request_rules[%d].domain_suffix is invalid", index)
		}
		parsed, err := url.Parse("https://" + suffix)
		if err != nil || normalizeHost(parsed.Hostname()) != suffix {
			return fmt.Errorf("request_rules[%d].domain_suffix is invalid", index)
		}
		if len(rule.CookieNames) == 0 {
			return fmt.Errorf("request_rules[%d].cookie_names is required", index)
		}
		if _, exists := seen[suffix]; exists {
			return fmt.Errorf("request_rules contains duplicate domain_suffix %q", suffix)
		}
		seen[suffix] = struct{}{}
	}
	return nil
}

// ResolveCookies 按同站点默认规则和最长域名后缀规则选择 Cookie。
func ResolveCookies(siteBaseURL, targetURL string, rules []Rule, cookies []*http.Cookie) (Resolution, error) {
	siteHost, err := hostFromURL(siteBaseURL)
	if err != nil {
		return Resolution{}, fmt.Errorf("parse site base url: %w", err)
	}
	targetHost, err := hostFromURL(targetURL)
	if err != nil {
		return Resolution{}, fmt.Errorf("parse target url: %w", err)
	}
	if siteHost == targetHost {
		selected := cloneCookies(cookies)
		return Resolution{Cookies: selected, TargetHost: targetHost, SelectedNames: cookieNames(selected), SameSite: true}, nil
	}

	var matched *Rule
	for index := range rules {
		rule := &rules[index]
		if !hostMatchesSuffix(targetHost, rule.DomainSuffix) {
			continue
		}
		if matched == nil || len(rule.DomainSuffix) > len(matched.DomainSuffix) {
			matched = rule
		}
	}
	if matched == nil {
		return Resolution{TargetHost: targetHost}, nil
	}

	byName := map[string]*http.Cookie{}
	for _, cookie := range cookies {
		if cookie != nil && strings.TrimSpace(cookie.Name) != "" {
			byName[cookie.Name] = cookie
		}
	}
	resolution := Resolution{TargetHost: targetHost, MatchedSuffix: matched.DomainSuffix}
	for _, name := range matched.CookieNames {
		cookie, exists := byName[name]
		if !exists {
			resolution.MissingNames = append(resolution.MissingNames, name)
			continue
		}
		cloned := *cookie
		resolution.Cookies = append(resolution.Cookies, &cloned)
		resolution.SelectedNames = append(resolution.SelectedNames, name)
	}
	return resolution, nil
}

// hostFromURL 提取并规范化 URL 主机名。
func hostFromURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("url must include scheme and host")
	}
	host := normalizeHost(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("url host is empty")
	}
	return host, nil
}

// hostMatchesSuffix 判断主机是否等于后缀或属于其点分隔子域。
func hostMatchesSuffix(host, suffix string) bool {
	host = normalizeHost(host)
	suffix = normalizeHost(suffix)
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// normalizeHost 规范化用于安全匹配的主机名。
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// cloneCookies 复制 Cookie，避免请求层修改数据库读取结果。
func cloneCookies(cookies []*http.Cookie) []*http.Cookie {
	result := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		cloned := *cookie
		result = append(result, &cloned)
	}
	return result
}

// cookieNames 返回稳定排序的 Cookie 名称用于脱敏日志。
func cookieNames(cookies []*http.Cookie) []string {
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && strings.TrimSpace(cookie.Name) != "" {
			names = append(names, cookie.Name)
		}
	}
	sort.Strings(names)
	return names
}
