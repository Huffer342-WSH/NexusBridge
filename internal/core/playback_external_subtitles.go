package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var externalSubtitleExtensions = map[string]bool{".ass": true, ".ssa": true, ".srt": true, ".vtt": true}

// SavePlaybackExternalSubtitle 校验源文件后保存视频与外挂字幕的一对一关联。
func (a *App) SavePlaybackExternalSubtitle(ctx context.Context, videoPath, subtitlePath string) (PlaybackExternalSubtitle, error) {
	video, _, mediaType, _, err := resolveLocalPlaybackFile(videoPath)
	if err != nil || mediaType != PlaybackMediaVideo {
		return PlaybackExternalSubtitle{}, fmt.Errorf("%w: invalid video file", ErrPlaybackNotFound)
	}
	subtitlePath, err = resolveExternalSubtitleFile(subtitlePath)
	if err != nil {
		return PlaybackExternalSubtitle{}, err
	}
	if _, err := a.subtitleArtifact.EnsureExternal(ctx, video, subtitlePath, "vtt"); err != nil {
		return PlaybackExternalSubtitle{}, fmt.Errorf("prepare external subtitle: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(subtitlePath))
	if ext == ".ass" || ext == ".ssa" {
		if _, err := a.subtitleArtifact.EnsureExternal(ctx, video, subtitlePath, "ass"); err != nil {
			return PlaybackExternalSubtitle{}, fmt.Errorf("prepare rich external subtitle: %w", err)
		}
	}
	if err := a.store.SavePlaybackExternalSubtitle(ctx, video, subtitlePath); err != nil {
		return PlaybackExternalSubtitle{}, err
	}
	return PlaybackExternalSubtitle{VideoPath: video, SubtitlePath: subtitlePath, Available: true}, nil
}

// DeletePlaybackExternalSubtitle 删除路径关联，不触碰视频和字幕源文件。
func (a *App) DeletePlaybackExternalSubtitle(ctx context.Context, videoPath string) error {
	video, _, mediaType, _, err := resolveLocalPlaybackFile(videoPath)
	if err != nil || mediaType != PlaybackMediaVideo {
		return fmt.Errorf("%w: invalid video file", ErrPlaybackNotFound)
	}
	return a.store.DeletePlaybackExternalSubtitle(ctx, video)
}

// GetPlaybackExternalSubtitleArtifact 返回已关联外挂字幕的缓存产物。
func (a *App) GetPlaybackExternalSubtitleArtifact(ctx context.Context, videoPath, format string) (PlaybackSubtitleArtifact, error) {
	video, _, mediaType, _, err := resolveLocalPlaybackFile(videoPath)
	if err != nil || mediaType != PlaybackMediaVideo {
		return PlaybackSubtitleArtifact{}, fmt.Errorf("%w: invalid video file", ErrPlaybackNotFound)
	}
	record, found, err := a.store.GetPlaybackExternalSubtitle(ctx, video)
	if err != nil {
		return PlaybackSubtitleArtifact{}, err
	}
	if !found {
		return PlaybackSubtitleArtifact{}, ErrPlaybackNotFound
	}
	result, err := a.subtitleArtifact.EnsureExternal(ctx, video, record.SubtitlePath, format)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PlaybackSubtitleArtifact{}, ErrPlaybackNotFound
		}
		return PlaybackSubtitleArtifact{}, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	return PlaybackSubtitleArtifact{Content: result.Content, Format: result.Format, ETag: result.ETag, Cached: result.Cached}, nil
}

func (a *App) attachExternalSubtitle(ctx context.Context, result *PlaybackContext) {
	if result == nil || strings.TrimSpace(result.CurrentPath) == "" || result.CurrentFileIndex == nil {
		return
	}
	record, found, err := a.store.GetPlaybackExternalSubtitle(ctx, result.CurrentPath)
	if err != nil || !found {
		return
	}
	association := &PlaybackExternalSubtitle{VideoPath: record.VideoPath, SubtitlePath: record.SubtitlePath}
	_, statErr := os.Stat(record.SubtitlePath)
	association.Available = statErr == nil
	result.ExternalSubtitle = association
	if !association.Available {
		return
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(record.SubtitlePath)), ".")
	track := PlaybackSubtitle{
		TrackID: 0, Label: filepath.Base(record.SubtitlePath), Codec: ext, External: true,
		StreamURL: "/api/playback/external-subtitle?path=" + url.QueryEscape(result.CurrentPath),
	}
	if ext == "ass" || ext == "ssa" {
		track.RichURL = track.StreamURL + "&format=ass"
	}
	for i := range result.Files {
		if result.Files[i].Index == *result.CurrentFileIndex {
			result.Files[i].Subtitles = append(result.Files[i].Subtitles, track)
			break
		}
	}
}

func resolveExternalSubtitleFile(value string) (string, error) {
	resolved, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("subtitle file path is required")
	}
	resolved = filepath.Clean(resolved)
	if !externalSubtitleExtensions[strings.ToLower(filepath.Ext(resolved))] {
		return "", fmt.Errorf("unsupported subtitle format; use ASS, SSA, SRT or VTT")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrPlaybackNotFound
		}
		return "", fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("subtitle path is not a regular file")
	}
	return resolved, nil
}
