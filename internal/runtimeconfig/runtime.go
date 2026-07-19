// runtime.go 统一解析开发、测试和正式发布时的数据与配置目录。
package runtimeconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"nexusbridge/internal/builtin"
	"nexusbridge/internal/config"
)

const (
	// BootstrapFileName 是正式版在可执行文件旁读取的数据目录引导文件。
	BootstrapFileName = "nexusbridge.bootstrap.json"
	// DataDirEnv 是开发、测试和正式版共用的数据目录覆盖变量。
	DataDirEnv              = "NEXUSBRIDGE_DATA_DIR"
	legacyDesktopDataDirEnv = "NEXUSBRIDGE_DESKTOP_DATA_DIR"
)

// BootstrapConfig 描述正式版数据目录的旁置引导设置。
type BootstrapConfig struct {
	DataDir string `json:"data_dir"`
}

// Options 描述运行配置解析时允许的显式覆盖项。
type Options struct {
	ExplicitConfig string
	DataDir        string
	ExecutablePath string
}

// Result 表示已经准备完成的运行目录与应用配置。
type Result struct {
	DataDir       string
	ConfigPath    string
	BootstrapPath string
	Config        config.Config
}

// Load 准备运行目录、内置站点和配置文件并解析相对路径。
func Load(opts Options) (Result, error) {
	configPath := strings.TrimSpace(opts.ExplicitConfig)
	dataDir := strings.TrimSpace(opts.DataDir)
	bootstrapPath := ""
	var err error

	if configPath != "" {
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			return Result{}, err
		}
		if dataDir == "" {
			dataDir = filepath.Dir(configPath)
		}
	}
	if dataDir == "" {
		dataDir, bootstrapPath, err = defaultRuntimeDataDir(opts.ExecutablePath)
	} else {
		dataDir, err = filepath.Abs(dataDir)
	}
	if err != nil {
		return Result{}, err
	}
	if configPath == "" {
		configPath = filepath.Join(dataDir, "config.json")
	}

	if err := prepareDirectories(dataDir); err != nil {
		return Result{}, err
	}
	if err := installBuiltinSites(builtin.Sites(), filepath.Join(dataDir, "sites", "html")); err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if strings.TrimSpace(opts.ExplicitConfig) != "" {
			return Result{}, fmt.Errorf("config file does not exist: %s", configPath)
		}
		if _, err := writeConfigIfMissing(configPath, defaultConfig()); err != nil {
			return Result{}, err
		}
	} else if err != nil {
		return Result{}, fmt.Errorf("inspect runtime config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return Result{}, err
	}
	cfg.Storage.Path = resolveDataPath(dataDir, cfg.Storage.Path)
	cfg.SitesDir = resolveDataPath(dataDir, cfg.SitesDir)
	if strings.TrimSpace(cfg.Logging.File) != "" {
		cfg.Logging.File = resolveDataPath(dataDir, cfg.Logging.File)
	}
	return Result{DataDir: dataDir, ConfigPath: configPath, BootstrapPath: bootstrapPath, Config: cfg}, nil
}

// DevelopmentDataDir 返回当前仓库开发和测试共用的 data 目录。
func DevelopmentDataDir() (string, error) {
	root, err := findProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "data"), nil
}

// IsProduction 返回当前二进制是否使用 production 构建标签。
func IsProduction() bool {
	return productionBuild
}

// defaultRuntimeDataDir 根据构建模式解析默认数据目录。
func defaultRuntimeDataDir(executablePath string) (string, string, error) {
	override := strings.TrimSpace(os.Getenv(DataDirEnv))
	if override == "" {
		override = strings.TrimSpace(os.Getenv(legacyDesktopDataDirEnv))
	}
	if override != "" {
		path, err := filepath.Abs(override)
		return path, "", err
	}
	if !productionBuild {
		path, err := DevelopmentDataDir()
		return path, "", err
	}
	executable := strings.TrimSpace(executablePath)
	var err error
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return "", "", fmt.Errorf("resolve executable path: %w", err)
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", "", err
	}
	bootstrapPath := filepath.Join(filepath.Dir(executable), BootstrapFileName)
	if data, err := os.ReadFile(bootstrapPath); err == nil {
		var bootstrap BootstrapConfig
		if err := json.Unmarshal(data, &bootstrap); err != nil {
			return "", bootstrapPath, fmt.Errorf("parse bootstrap config: %w", err)
		}
		if strings.TrimSpace(bootstrap.DataDir) == "" {
			return "", bootstrapPath, fmt.Errorf("bootstrap data_dir is required")
		}
		if filepath.IsAbs(bootstrap.DataDir) {
			return filepath.Clean(bootstrap.DataDir), bootstrapPath, nil
		}
		return filepath.Join(filepath.Dir(executable), filepath.FromSlash(bootstrap.DataDir)), bootstrapPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", bootstrapPath, fmt.Errorf("read bootstrap config: %w", err)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", bootstrapPath, fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "NexusBridge"), bootstrapPath, nil
}

// findProjectRoot 从当前目录向上查找 go.mod 所在仓库根目录。
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot locate project root from current directory")
		}
		dir = parent
	}
}

// prepareDirectories 创建运行时固定使用的数据子目录。
func prepareDirectories(dataDir string) error {
	for _, dir := range []string{dataDir, filepath.Join(dataDir, "logs"), filepath.Join(dataDir, "sites", "html")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", dir, err)
		}
	}
	return nil
}

// installBuiltinSites 仅安装数据目录中尚不存在的内置站点定义。
func installBuiltinSites(source fs.FS, targetDir string) error {
	return fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(targetDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if _, err := writeFileIfMissing(target, data, 0o600); err != nil {
			return fmt.Errorf("install builtin site %s: %w", path, err)
		}
		return nil
	})
}

// defaultConfig 创建相对于数据目录的默认应用配置。
func defaultConfig() config.Config {
	cfg := config.Default()
	cfg.Storage.Path = "nexusbridge.db"
	cfg.SitesDir = filepath.ToSlash(filepath.Join("sites", "html"))
	cfg.Logging.File = filepath.ToSlash(filepath.Join("logs", "nexusbridge.log"))
	return cfg
}

// writeConfigIfMissing 仅在目标不存在时写入首次启动使用的默认配置文件。
func writeConfigIfMissing(path string, cfg config.Config) (bool, error) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode runtime config: %w", err)
	}
	created, err := writeFileIfMissing(path, append(data, '\n'), 0o600)
	if err != nil {
		return false, fmt.Errorf("write runtime config: %w", err)
	}
	return created, nil
}

// writeFileIfMissing 以排他创建方式写入文件，目标已存在时保持原内容不变。
func writeFileIfMissing(path string, data []byte, perm fs.FileMode) (bool, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	return true, nil
}

// resolveDataPath 将应用配置中的相对路径固定到数据目录。
func resolveDataPath(dataDir, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(dataDir, filepath.FromSlash(value))
}
