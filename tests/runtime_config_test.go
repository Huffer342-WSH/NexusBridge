// runtime_config_test.go 验证开发目录和正式版旁置引导配置。
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexusbridge/internal/runtimeconfig"
)

// TestRuntimeConfigDevelopmentDataDir 验证开发默认数据目录固定在仓库 data 下。
func TestRuntimeConfigDevelopmentDataDir(t *testing.T) {
	dataDir, err := runtimeconfig.DevelopmentDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dataDir) != "data" {
		t.Fatalf("unexpected development data directory: %s", dataDir)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), "go.mod")); err != nil {
		t.Fatalf("data directory is not under project root: %v", err)
	}
}

// TestRuntimeConfigDataDirOverride 验证显式数据目录中的配置和资源均被正确准备。
func TestRuntimeConfigDataDirOverride(t *testing.T) {
	dataDir := t.TempDir()
	result, err := runtimeconfig.Load(runtimeconfig.Options{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{result.ConfigPath, result.Config.Storage.Path, result.Config.SitesDir, result.Config.Logging.File} {
		if !filepath.IsAbs(path) || !isWithin(path, dataDir) {
			t.Fatalf("runtime path must stay in data directory: %s", path)
		}
	}
}

// TestRuntimeConfigProductionBootstrap 验证正式版从可执行文件旁读取相对数据目录。
func TestRuntimeConfigProductionBootstrap(t *testing.T) {
	if !runtimeconfig.IsProduction() {
		t.Skip("requires -tags production")
	}
	t.Setenv(runtimeconfig.DataDirEnv, "")
	t.Setenv("NEXUSBRIDGE_DESKTOP_DATA_DIR", "")
	executableDir := t.TempDir()
	bootstrap := runtimeconfig.BootstrapConfig{DataDir: "portable-data"}
	data, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapPath := filepath.Join(executableDir, runtimeconfig.BootstrapFileName)
	if err := os.WriteFile(bootstrapPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := runtimeconfig.Load(runtimeconfig.Options{ExecutablePath: filepath.Join(executableDir, "nexusbridge")})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(executableDir, "portable-data")
	if result.DataDir != want || result.BootstrapPath != bootstrapPath {
		t.Fatalf("unexpected production paths: %#v", result)
	}
}

// isWithin 判断路径是否位于指定根目录内。
func isWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
