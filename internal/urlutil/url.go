// Package urlutil 提供不依赖网络请求实现的 URL 规范化操作。
package urlutil

import "strings"

// NormalizeBaseURL 补全缺失的 HTTPS scheme，并移除末尾斜杠。
func NormalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return strings.TrimRight(raw, "/")
}
