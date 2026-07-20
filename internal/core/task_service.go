package core

import (
	"context"
	"fmt"
	"strings"

	"nexusbridge/internal/config"
	"nexusbridge/internal/llm"
	"nexusbridge/internal/organizer"
	"nexusbridge/internal/storage"
)

// ListDownloadTasks 返回下载任务。
func (a *App) ListDownloadTasks(ctx context.Context) ([]DownloadTask, error) {
	records, err := a.store.ListDownloadTasks(ctx)
	if err != nil {
		return nil, err
	}
	tasks := make([]DownloadTask, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, downloadTaskFromRecord(record))
	}
	return tasks, nil
}

// ListOrganizeTasks 返回整理任务。
func (a *App) ListOrganizeTasks(ctx context.Context) ([]OrganizeTask, error) {
	records, err := a.store.ListOrganizeTasks(ctx, false)
	if err != nil {
		return nil, err
	}
	tasks := make([]OrganizeTask, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, organizeTaskFromRecord(record))
	}
	return tasks, nil
}

// OrganizePending 处理待整理任务。
func (a *App) OrganizePending(ctx context.Context) (OrganizeResult, error) {
	records, err := a.store.ListOrganizeTasks(ctx, true)
	if err != nil {
		return OrganizeResult{}, err
	}
	llmCfg := a.effectiveLLMConfig(ctx)
	org := organizer.New(llm.New(llm.Config{
		BaseURL: llmCfg.BaseURL,
		APIKey:  llmCfg.APIKey,
		Model:   llmCfg.Model,
	}), organizer.Options{
		LibraryRoot: a.cfg.MediaLibrary.Root,
		DryRun:      a.cfg.MediaLibrary.DryRun,
	})
	result := OrganizeResult{DryRun: a.cfg.MediaLibrary.DryRun}
	for _, record := range records {
		out, err := org.Organize(ctx, record.Title, record.SourcePath)
		if err != nil {
			result.Failed++
			_ = a.store.UpdateOrganizeTask(ctx, record.ID, "failed", "", "", "", "", err.Error(), 0)
			continue
		}
		status := "linked"
		if a.cfg.MediaLibrary.DryRun {
			status = "dry_run"
		}
		if err := a.store.UpdateOrganizeTask(ctx, record.ID, status, out.RelativeDir, out.Filename, out.TargetPath, out.LLMResponse, "", out.Confidence); err != nil {
			return result, err
		}
		result.Processed++
	}
	return result, nil
}

// GetLLMConfig 读取 LLM 配置。
func (a *App) GetLLMConfig(ctx context.Context) config.LLMConfig {
	return a.effectiveLLMConfig(ctx)
}

// SaveLLMConfig 保存 LLM 配置。
func (a *App) SaveLLMConfig(ctx context.Context, cfg config.LLMConfig) (config.LLMConfig, error) {
	if err := a.store.SaveSetting(ctx, storage.LLMSettingKey, cfg); err != nil {
		return config.LLMConfig{}, err
	}
	return cfg, nil
}

// PreviewTorrentDownload 生成手动下载前的 LLM 标题预览。
func (a *App) PreviewTorrentDownload(ctx context.Context, request ManualDownloadRequest) (DownloadPreview, error) {
	torrent, err := a.getTorrent(ctx, request.SiteID, request.TorrentID)
	if err != nil {
		return DownloadPreview{}, err
	}
	if strings.TrimSpace(torrent.DownloadURL) == "" {
		return DownloadPreview{}, fmt.Errorf("torrent download url is empty")
	}
	llmCfg := a.effectiveLLMConfig(ctx)
	formatted, raw, err := llm.New(llm.Config{
		BaseURL: llmCfg.BaseURL,
		APIKey:  llmCfg.APIKey,
		Model:   llmCfg.Model,
	}).FormatTorrentTitle(ctx, torrent.Title)
	if err != nil {
		return DownloadPreview{}, err
	}
	return DownloadPreview{
		SiteID:         torrent.SiteID,
		TorrentID:      torrent.ID,
		OriginalTitle:  torrent.Title,
		FormattedTitle: formatted,
		DownloadURL:    torrent.DownloadURL,
		LLMResponse:    raw,
	}, nil
}

// SendTorrentDownload 通过 qBittorrent 发送单个种子下载。
func (a *App) SendTorrentDownload(ctx context.Context, request ManualDownloadRequest) (DownloadTask, error) {
	torrent, err := a.getTorrent(ctx, request.SiteID, request.TorrentID)
	if err != nil {
		return DownloadTask{}, err
	}
	if strings.TrimSpace(torrent.DownloadURL) == "" {
		return DownloadTask{}, fmt.Errorf("torrent download url is empty")
	}
	title := strings.TrimSpace(request.FormattedTitle)
	if title == "" {
		title = torrent.Title
	}
	task := DownloadTask{
		ID:           downloadTaskID(torrent.SiteID, torrent.ID, "manual"),
		SiteID:       torrent.SiteID,
		TorrentID:    torrent.ID,
		RuleName:     "manual",
		Status:       "pending",
		TorrentTitle: title,
		DownloadURL:  torrent.DownloadURL,
	}
	created, err := a.store.CreateDownloadTaskIfAbsent(ctx, downloadTaskToRecord(task))
	if err != nil {
		return DownloadTask{}, err
	}
	if !created {
		task.Status = "exists"
		return task, nil
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", err.Error())
		task.Status = "failed"
		task.Error = err.Error()
		return task, nil
	}
	if err := a.addTorrentToQBAndLink(ctx, qb, torrent, task.ID); err != nil {
		_ = a.store.UpdateDownloadTask(ctx, task.ID, "failed", "", "", err.Error())
		task.Status = "failed"
		task.Error = err.Error()
		return task, nil
	}
	if err := a.store.UpdateDownloadTask(ctx, task.ID, "sent", "", "", ""); err != nil {
		return DownloadTask{}, err
	}
	task.Status = "sent"
	return task, nil
}

// effectiveLLMConfig 合并配置文件和数据库中的 LLM 设置。
func (a *App) effectiveLLMConfig(ctx context.Context) config.LLMConfig {
	cfg := a.cfg.LLM
	var stored config.LLMConfig
	if ok, err := a.store.LoadSetting(ctx, storage.LLMSettingKey, &stored); err == nil && ok {
		cfg = stored
	}
	return cfg
}
