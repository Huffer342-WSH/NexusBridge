// Package stringutil 提供不依赖业务领域的字符串选择与清理操作。
package stringutil

import "strings"

// FirstNonEmpty 返回第一个去除首尾空白后不为空的原始字符串。
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
