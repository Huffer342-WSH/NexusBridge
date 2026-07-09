// assets_development.go 为开发和测试提供可选的本地 WebUI 文件系统。
//go:build !production

package webuiassets

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing/fstest"
)

// Dist 返回本地构建结果，不存在时返回说明页面以避免开发测试依赖构建产物。
func Dist() fs.FS {
	if path, ok := localDistPath(); ok {
		return os.DirFS(path)
	}
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><meta charset=\"utf-8\"><title>NexusBridge</title><p>WebUI 尚未构建，请运行 pnpm run dev 或 pnpm run build。</p>")},
	}
}

// localDistPath 从当前目录向上查找可用的 WebUI 构建目录。
func localDistPath() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		dist := filepath.Join(dir, "webui", "dist")
		if info, err := os.Stat(filepath.Join(dist, "index.html")); err == nil && !info.IsDir() {
			return dist, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
