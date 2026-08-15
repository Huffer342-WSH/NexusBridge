package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"nexusbridge/internal/storage"
)

const (
	defaultSubtitleScanIntervalMinutes = 6 * 60
	maxSubtitleScanIntervalMinutes     = 1440
)

// GetSubtitleScanSettings 返回所有媒体库共用的字幕扫描周期。
func (a *App) GetSubtitleScanSettings(ctx context.Context) (SubtitleScanSettings, error) {
	settings := SubtitleScanSettings{IntervalMinutes: defaultSubtitleScanIntervalMinutes}
	found, err := a.store.LoadSetting(ctx, storage.SubtitleScanSettingKey, &settings)
	if err != nil {
		return SubtitleScanSettings{}, err
	}
	if found && (settings.IntervalMinutes < 0 || settings.IntervalMinutes > maxSubtitleScanIntervalMinutes) {
		return SubtitleScanSettings{}, fmt.Errorf("stored subtitle scan interval_minutes must be between 0 and %d", maxSubtitleScanIntervalMinutes)
	}
	return settings, nil
}

// SaveSubtitleScanSettings 保存全局扫描周期并立即重排后台计时器。
func (a *App) SaveSubtitleScanSettings(ctx context.Context, settings SubtitleScanSettings) (SubtitleScanSettings, error) {
	if settings.IntervalMinutes < 0 || settings.IntervalMinutes > maxSubtitleScanIntervalMinutes {
		return SubtitleScanSettings{}, fmt.Errorf("subtitle scan interval_minutes must be between 0 and %d", maxSubtitleScanIntervalMinutes)
	}
	if err := a.store.SaveSetting(ctx, storage.SubtitleScanSettingKey, settings); err != nil {
		return SubtitleScanSettings{}, err
	}
	select {
	case a.subtitleSettingsWake <- struct{}{}:
	default:
	}
	return settings, nil
}

// startSubtitleAutomation 监控已配置媒体库，自动扫描并预生成 MKV 文本字幕。
func (a *App) startSubtitleAutomation() {
	a.maintenanceWG.Add(1)
	go func() {
		defer a.maintenanceWG.Done()
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		timerC := timer.C
		initialScan := true
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-timerC:
				a.scanConfiguredLibrariesForSubtitles(a.ctx, initialScan)
				initialScan = false
				timerC = a.resetSubtitleScanTimer(timer)
			case <-a.subtitleSettingsWake:
				if !initialScan {
					timerC = a.resetSubtitleScanTimer(timer)
				}
			}
		}
	}()
}

func (a *App) resetSubtitleScanTimer(timer *time.Timer) <-chan time.Time {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	settings, err := a.GetSubtitleScanSettings(a.ctx)
	if err != nil {
		slog.Warn("读取字幕扫描设置失败，使用默认周期", "error", err)
		settings.IntervalMinutes = defaultSubtitleScanIntervalMinutes
	}
	if settings.IntervalMinutes == 0 {
		return nil
	}
	timer.Reset(time.Duration(settings.IntervalMinutes) * time.Minute)
	return timer.C
}

func (a *App) scanConfiguredLibrariesForSubtitles(ctx context.Context, prewarmAll bool) {
	libraries, err := a.store.ListMediaLibraries(ctx)
	if err != nil {
		slog.Warn("自动字幕扫描读取媒体库失败", "error", err)
		return
	}
	for _, library := range libraries {
		if ctx.Err() != nil {
			return
		}
		var scanErr error
		if prewarmAll {
			_, scanErr = a.ScanMediaLibrary(ctx, library.Series.ID)
		} else {
			_, scanErr = a.scanMediaLibraryChanges(ctx, library.Series.ID)
		}
		if scanErr != nil && ctx.Err() == nil {
			slog.Warn("自动字幕扫描媒体库失败", "library", library.Series.Name, "error", scanErr)
		}
	}
}
