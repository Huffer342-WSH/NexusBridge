package core

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"nexusbridge/internal/core/subtitleartifact"

	"github.com/gravity-zero/mkvgo/matroska"
)

var playbackTextSubtitleCodecs = map[string]struct{}{
	"s_text/utf8":   {},
	"s_text/ass":    {},
	"s_text/ssa":    {},
	"s_text/webvtt": {},
	"srt":           {},
	"subrip":        {},
	"ass":           {},
	"ssa":           {},
	"webvtt":        {},
}

// discoverMKVSubtitles 读取 MKV 头部并返回可导出为 WebVTT 的内嵌文本字幕轨。
func discoverMKVSubtitles(ctx context.Context, filePath string, streamURL func(uint64) string) []PlaybackSubtitle {
	if !strings.EqualFold(filepath.Ext(filePath), ".mkv") {
		return nil
	}
	container, err := matroska.OpenMeta(ctx, filePath)
	if err != nil {
		return nil
	}
	result := make([]PlaybackSubtitle, 0)
	for _, track := range container.Tracks {
		if track.Type != matroska.SubtitleTrack || !isPlaybackTextSubtitleCodec(track.Codec) {
			continue
		}
		language := strings.TrimSpace(track.ResolvedLanguage())
		label := strings.TrimSpace(track.Name)
		if label == "" {
			label = language
		}
		if label == "" {
			label = fmt.Sprintf("字幕 %d", len(result)+1)
		}
		result = append(result, PlaybackSubtitle{
			TrackID: track.ID, Label: label, Language: language, Codec: track.Codec,
			Default: track.IsDefault, Forced: track.IsForced, StreamURL: streamURL(track.ID),
			RichURL: richSubtitleURL(track.Codec, streamURL(track.ID)),
		})
	}
	return result
}

// extractMKVSubtitle 确保指定 MKV 内嵌文本字幕轨存在持久化产物并返回内容。
func (a *App) extractMKVSubtitle(
	ctx context.Context,
	filePath string,
	trackID uint64,
	format string,
) (PlaybackSubtitleArtifact, error) {
	tracks := discoverMKVSubtitles(ctx, filePath, func(uint64) string { return "" })
	codec := ""
	for _, track := range tracks {
		if track.TrackID == trackID {
			codec = track.Codec
			break
		}
	}
	if codec == "" {
		return PlaybackSubtitleArtifact{}, ErrPlaybackNotFound
	}
	result, err := a.subtitleArtifact.Ensure(ctx, filePath, trackID, codec, format)
	if err != nil {
		return PlaybackSubtitleArtifact{}, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	return PlaybackSubtitleArtifact{
		Content: result.Content,
		Format:  result.Format,
		ETag:    result.ETag,
		Cached:  result.Cached,
	}, nil
}

// isPlaybackTextSubtitleCodec 判断 MKV 字幕编码是否能安全转换为文本 WebVTT。
func isPlaybackTextSubtitleCodec(codec string) bool {
	_, ok := playbackTextSubtitleCodecs[strings.ToLower(strings.TrimSpace(codec))]
	return ok
}

func richSubtitleURL(codec, streamURL string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec != "ass" && codec != "ssa" {
		return ""
	}
	parsed, err := url.Parse(streamURL)
	if err != nil {
		return ""
	}
	query := parsed.Query()
	query.Set("format", subtitleartifact.FormatASS)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// GetTorrentSubtitle 从数据库种子关联的 MKV 文件导出内嵌文本字幕。
func (a *App) GetTorrentSubtitle(
	ctx context.Context,
	siteID, torrentID string,
	fileIndex int,
	trackID uint64,
	format string,
) (PlaybackSubtitleArtifact, error) {
	source, err := a.OpenTorrentMedia(ctx, siteID, torrentID, fileIndex)
	if err != nil {
		return PlaybackSubtitleArtifact{}, err
	}
	_ = source.File.Close()
	return a.extractMKVSubtitle(ctx, source.Path, trackID, format)
}

// GetQBSubtitle 从 qB 任务的 MKV 文件导出内嵌文本字幕。
func (a *App) GetQBSubtitle(
	ctx context.Context,
	hash string,
	fileIndex int,
	trackID uint64,
	format string,
) (PlaybackSubtitleArtifact, error) {
	source, err := a.OpenQBMedia(ctx, hash, fileIndex)
	if err != nil {
		return PlaybackSubtitleArtifact{}, err
	}
	_ = source.File.Close()
	return a.extractMKVSubtitle(ctx, source.Path, trackID, format)
}

// GetFileSubtitle 从文件管理器选择的 MKV 文件导出内嵌文本字幕。
func (a *App) GetFileSubtitle(
	ctx context.Context,
	filePath string,
	trackID uint64,
	format string,
) (PlaybackSubtitleArtifact, error) {
	source, err := a.OpenFileMedia(ctx, filePath)
	if err != nil {
		return PlaybackSubtitleArtifact{}, err
	}
	_ = source.File.Close()
	return a.extractMKVSubtitle(ctx, source.Path, trackID, format)
}

// scheduleSubtitlePrewarm 在媒体库扫描完成后低并发预生成 MKV 文本字幕产物。
func (a *App) scheduleSubtitlePrewarm(videos []SeriesVideo) {
	paths := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.Available && strings.EqualFold(filepath.Ext(video.Path), ".mkv") {
			path := filepath.Clean(video.Path)
			key := normalizedFilesystemPath(path)
			a.subtitleQueueMu.Lock()
			if _, queued := a.subtitleQueued[key]; !queued {
				a.subtitleQueued[key] = struct{}{}
				paths = append(paths, path)
			}
			a.subtitleQueueMu.Unlock()
		}
	}
	if len(paths) == 0 {
		return
	}
	a.subtitleWG.Add(1)
	go func() {
		defer a.subtitleWG.Done()
		defer func() {
			for _, path := range paths {
				a.finishSubtitleQueueItem(normalizedFilesystemPath(path))
			}
		}()
		for _, path := range paths {
			key := normalizedFilesystemPath(path)
			select {
			case <-a.ctx.Done():
				a.finishSubtitleQueueItem(key)
				return
			case a.subtitlePrewarm <- struct{}{}:
			}
			a.prewarmSubtitleFile(path)
			<-a.subtitlePrewarm
			a.finishSubtitleQueueItem(key)
		}
	}()
}

func (a *App) finishSubtitleQueueItem(key string) {
	a.subtitleQueueMu.Lock()
	delete(a.subtitleQueued, key)
	a.subtitleQueueMu.Unlock()
}

func (a *App) prewarmSubtitleFile(path string) {
	tracks := discoverMKVSubtitles(a.ctx, path, func(uint64) string { return "" })
	for _, track := range tracks {
		if a.ctx.Err() != nil {
			return
		}
		_, _ = a.subtitleArtifact.Ensure(
			a.ctx, path, track.TrackID, track.Codec, subtitleartifact.FormatWebVTT,
		)
		if track.RichURL != "" || strings.EqualFold(track.Codec, "ass") || strings.EqualFold(track.Codec, "ssa") {
			_, _ = a.subtitleArtifact.Ensure(
				a.ctx, path, track.TrackID, track.Codec, subtitleartifact.FormatASS,
			)
		}
	}
}
