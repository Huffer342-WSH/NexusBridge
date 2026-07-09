// assets.go 负责向桌面入口暴露应用图标。
package main

import _ "embed"

//go:embed resources/appicon.png
var icon []byte

// desktopIcon 返回桌面窗口与托盘共用的图标数据副本。
func desktopIcon() []byte {
	return append([]byte(nil), icon...)
}
