package core

import (
	"context"
	"fmt"
)

// GetTorrentVideoThumbnail 返回数据库种子中指定视频文件的按需缩略图。
func (a *App) GetTorrentVideoThumbnail(
	ctx context.Context,
	siteID, torrentID string,
	fileIndex int,
) (VideoThumbnail, error) {
	source, err := a.OpenTorrentMedia(ctx, siteID, torrentID, fileIndex)
	if err != nil {
		return VideoThumbnail{}, err
	}
	return a.generateVideoThumbnail(ctx, source)
}

// GetQBVideoThumbnail 返回 qB 任务中指定视频文件的按需缩略图。
func (a *App) GetQBVideoThumbnail(ctx context.Context, hash string, fileIndex int) (VideoThumbnail, error) {
	source, err := a.OpenQBMedia(ctx, hash, fileIndex)
	if err != nil {
		return VideoThumbnail{}, err
	}
	return a.generateVideoThumbnail(ctx, source)
}

// GetFileVideoThumbnail 返回经过本机媒体边界校验的视频缩略图。
func (a *App) GetFileVideoThumbnail(ctx context.Context, path string) (VideoThumbnail, error) {
	source, err := a.OpenFileMedia(ctx, path)
	if err != nil {
		return VideoThumbnail{}, err
	}
	return a.generateVideoThumbnail(ctx, source)
}

func (a *App) generateVideoThumbnail(ctx context.Context, source PlaybackSource) (VideoThumbnail, error) {
	if source.File != nil {
		_ = source.File.Close()
	}
	mediaType, _, supported := playbackMediaType(source.Name)
	if !supported || mediaType != PlaybackMediaVideo {
		return VideoThumbnail{}, fmt.Errorf("%w: thumbnail source is not a video", ErrPlaybackNotFound)
	}
	result, err := a.videoThumbnail.Get(ctx, source.Path)
	if err != nil {
		return VideoThumbnail{}, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	return VideoThumbnail{Path: result.Path, ETag: result.ETag, ModTime: result.ModTime}, nil
}
