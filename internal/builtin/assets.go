// assets.go 负责暴露随程序发布的内置站点定义。
package builtin

import (
	"embed"
	"io/fs"
)

//go:embed sites/*.json
var siteAssets embed.FS

// Sites 返回以内置站点文件为根的只读文件系统。
func Sites() fs.FS {
	result, err := fs.Sub(siteAssets, "sites")
	if err != nil {
		panic(err)
	}
	return result
}
