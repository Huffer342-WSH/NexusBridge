package core

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/mkvgo/matroska"
)

const maxPlaybackSubtitleBytes = 32 << 20

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
		})
	}
	return result
}

// extractMKVSubtitle 将指定 MKV 内嵌文本字幕轨导出为浏览器可加载的 WebVTT。
func extractMKVSubtitle(ctx context.Context, filePath string, trackID uint64) ([]byte, error) {
	tracks := discoverMKVSubtitles(ctx, filePath, func(uint64) string { return "" })
	found := false
	for _, track := range tracks {
		if track.TrackID == trackID {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrPlaybackNotFound
	}
	var output bytes.Buffer
	writer := &playbackSubtitleWriter{buffer: &output, remaining: maxPlaybackSubtitleBytes}
	if err := matroska.ExtractSubtitleWebVTT(ctx, filePath, trackID, writer); err != nil {
		return nil, fmt.Errorf("%w: extract MKV subtitle: %v", ErrPlaybackUnavailable, err)
	}
	return output.Bytes(), nil
}

// isPlaybackTextSubtitleCodec 判断 MKV 字幕编码是否能安全转换为文本 WebVTT。
func isPlaybackTextSubtitleCodec(codec string) bool {
	_, ok := playbackTextSubtitleCodecs[strings.ToLower(strings.TrimSpace(codec))]
	return ok
}

type playbackSubtitleWriter struct {
	buffer    *bytes.Buffer
	remaining int
}

// Write 限制单条字幕响应的最大内存占用。
func (w *playbackSubtitleWriter) Write(value []byte) (int, error) {
	if len(value) > w.remaining {
		return 0, fmt.Errorf("subtitle exceeds %d bytes", maxPlaybackSubtitleBytes)
	}
	n, err := w.buffer.Write(value)
	w.remaining -= n
	return n, err
}

// GetTorrentSubtitle 从数据库种子关联的 MKV 文件导出内嵌文本字幕。
func (a *App) GetTorrentSubtitle(
	ctx context.Context,
	siteID, torrentID string,
	fileIndex int,
	trackID uint64,
) ([]byte, error) {
	source, err := a.OpenTorrentMedia(ctx, siteID, torrentID, fileIndex)
	if err != nil {
		return nil, err
	}
	_ = source.File.Close()
	return extractMKVSubtitle(ctx, source.Path, trackID)
}

// GetQBSubtitle 从 qB 任务的 MKV 文件导出内嵌文本字幕。
func (a *App) GetQBSubtitle(ctx context.Context, hash string, fileIndex int, trackID uint64) ([]byte, error) {
	source, err := a.OpenQBMedia(ctx, hash, fileIndex)
	if err != nil {
		return nil, err
	}
	_ = source.File.Close()
	return extractMKVSubtitle(ctx, source.Path, trackID)
}

// GetFileSubtitle 从文件管理器选择的 MKV 文件导出内嵌文本字幕。
func (a *App) GetFileSubtitle(ctx context.Context, filePath string, trackID uint64) ([]byte, error) {
	source, err := a.OpenFileMedia(ctx, filePath)
	if err != nil {
		return nil, err
	}
	_ = source.File.Close()
	return extractMKVSubtitle(ctx, source.Path, trackID)
}
