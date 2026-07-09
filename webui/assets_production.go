// assets_production.go 为正式构建嵌入 pnpm 生成的 WebUI。
//go:build production

package webuiassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// Dist 返回以正式 WebUI 构建目录为根的只读文件系统。
func Dist() fs.FS {
	result, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return result
}
