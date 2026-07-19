package core

import (
	"context"

	"nexusbridge/internal/config"
	"nexusbridge/internal/network"
	"nexusbridge/internal/storage"
)

// GetNetworkConfig 读取当前生效的网络代理设置。
func (a *App) GetNetworkConfig(ctx context.Context) config.NetworkConfig {
	return a.effectiveNetworkConfig(ctx)
}

// SaveNetworkConfig 保存网络代理设置。
func (a *App) SaveNetworkConfig(ctx context.Context, cfg config.NetworkConfig) (config.NetworkConfig, error) {
	cfg.Mode = network.NormalizeProxyMode(cfg.Mode)
	cfg.NoProxy = network.NormalizeNoProxyLines(cfg.NoProxy)
	if cfg.NoProxy == "" {
		cfg.NoProxy = network.DefaultNoProxy
	}
	if err := network.ValidateProxyConfig(cfg.Mode, cfg.ProxyURL); err != nil {
		return config.NetworkConfig{}, err
	}
	if err := a.store.SaveSetting(ctx, storage.NetworkSettingKey, cfg); err != nil {
		return config.NetworkConfig{}, err
	}
	if err := a.applyNetworkConfig(ctx); err != nil {
		return config.NetworkConfig{}, err
	}
	return cfg, nil
}

// applyNetworkConfig 将数据库中已保存的网络代理设置写入进程环境变量。
func (a *App) applyNetworkConfig(ctx context.Context) error {
	cfg := a.effectiveNetworkConfig(ctx)
	qbCfg := a.effectiveQBConfig(ctx)
	return network.ApplyEnvironment(cfg.Mode, cfg.ProxyURL, cfg.NoProxy, qbCfg.URL)
}

// effectiveNetworkConfig 合并配置文件和数据库中的网络代理设置。
func (a *App) effectiveNetworkConfig(ctx context.Context) config.NetworkConfig {
	cfg := a.cfg.Network
	var stored config.NetworkConfig
	if ok, err := a.store.LoadSetting(ctx, storage.NetworkSettingKey, &stored); err == nil && ok {
		cfg = stored
	}
	cfg.Mode = network.NormalizeProxyMode(cfg.Mode)
	cfg.NoProxy = network.NormalizeNoProxyLines(cfg.NoProxy)
	if cfg.NoProxy == "" {
		cfg.NoProxy = network.DefaultNoProxy
	}
	return cfg
}
