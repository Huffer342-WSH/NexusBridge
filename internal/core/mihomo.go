package core

import (
	"context"
	"strings"

	"nexusbridge/internal/mihomo"
	"nexusbridge/internal/storage"
)

type mihomoDirectorySetting struct {
	ConfigDir string `json:"config_dir"`
}

// GetMihomoSettings 读取选定目录中的 Mihomo Provider 设置。
func (a *App) GetMihomoSettings(ctx context.Context, configDir string) (mihomo.Settings, error) {
	if strings.TrimSpace(configDir) == "" {
		var stored mihomoDirectorySetting
		if ok, err := a.store.LoadSetting(ctx, storage.MihomoSettingKey, &stored); err != nil {
			return mihomo.Settings{}, err
		} else if ok {
			configDir = stored.ConfigDir
		}
	}
	return mihomo.Load(configDir)
}

// SaveMihomoConfigDir 验证并保存 Mihomo 配置目录选择。
func (a *App) SaveMihomoConfigDir(ctx context.Context, configDir string) (mihomo.Settings, error) {
	settings, err := mihomo.Load(configDir)
	if err != nil {
		return mihomo.Settings{}, err
	}
	if err := a.store.SaveSetting(ctx, storage.MihomoSettingKey, mihomoDirectorySetting{ConfigDir: settings.ConfigDir}); err != nil {
		return mihomo.Settings{}, err
	}
	return settings, nil
}

// AddMihomoProvider 将新 Provider 追加到选定的 Mihomo 配置。
func (a *App) AddMihomoProvider(ctx context.Context, request mihomo.AddProviderRequest) (mihomo.Settings, error) {
	a.mihomoMu.Lock()
	defer a.mihomoMu.Unlock()
	settings, err := mihomo.AddProvider(request)
	if err != nil {
		return mihomo.Settings{}, err
	}
	if err := a.store.SaveSetting(ctx, storage.MihomoSettingKey, mihomoDirectorySetting{ConfigDir: settings.ConfigDir}); err != nil {
		return mihomo.Settings{}, err
	}
	return settings, nil
}
