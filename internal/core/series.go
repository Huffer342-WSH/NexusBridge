package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nexusbridge/internal/storage"
)

var (
	// ErrSeriesNotFound 表示剧集不存在。
	ErrSeriesNotFound = errors.New("series not found")
	// ErrSeriesNoPlayableVideo 表示剧集当前没有可读取视频。
	ErrSeriesNoPlayableVideo = errors.New("series has no playable video")
	// ErrSeriesInvalid 表示剧集名称、目录或选集输入无效。
	ErrSeriesInvalid = errors.New("invalid series request")
)

// ListSeries 返回缓存摘要，不触发目录扫描。
func (a *App) ListSeries(ctx context.Context) ([]SeriesSummary, error) {
	records, err := a.store.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SeriesSummary, 0, len(records))
	for _, record := range records {
		result = append(result, seriesDetailFromRecord(record).SeriesSummary)
	}
	return result, nil
}

// GetSeries 返回缓存详情，不触发目录扫描。
func (a *App) GetSeries(ctx context.Context, id string) (SeriesDetail, error) {
	record, ok, err := a.store.GetSeries(ctx, strings.TrimSpace(id))
	if err != nil {
		return SeriesDetail{}, err
	}
	if !ok {
		return SeriesDetail{}, ErrSeriesNotFound
	}
	return seriesDetailFromRecord(record), nil
}

// SaveSeries 校验并保存剧集，然后扫描全部目录。
func (a *App) SaveSeries(ctx context.Context, id string, request SeriesSaveRequest) (SeriesDetail, error) {
	id = strings.TrimSpace(id)
	creating := id == ""
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return SeriesDetail{}, fmt.Errorf("%w: series name is required", ErrSeriesInvalid)
	}
	directories, err := normalizeSeriesDirectories(request.Directories)
	if err != nil {
		return SeriesDetail{}, err
	}
	if creating {
		id, err = newSeriesID()
		if err != nil {
			return SeriesDetail{}, err
		}
	}
	lock := a.seriesMutex(id)
	if err := lock.Lock(ctx); err != nil {
		return SeriesDetail{}, err
	}
	defer lock.Unlock()

	if _, ok, err := a.store.GetSeries(ctx, id); err != nil {
		return SeriesDetail{}, err
	} else if !creating && !ok {
		return SeriesDetail{}, ErrSeriesNotFound
	}
	all, err := a.store.ListSeries(ctx)
	if err != nil {
		return SeriesDetail{}, err
	}
	for _, item := range all {
		if item.Series.ID != id && strings.EqualFold(strings.TrimSpace(item.Series.Name), name) {
			return SeriesDetail{}, fmt.Errorf("%w: series name %q already exists", ErrSeriesInvalid, name)
		}
	}
	records := make([]storage.SeriesDirectoryRecord, 0, len(directories))
	for index, directory := range directories {
		records = append(records, storage.SeriesDirectoryRecord{
			SeriesID: id, Path: directory, SourceOrder: index,
		})
	}
	if err := a.store.SaveSeries(ctx, storage.SeriesRecord{ID: id, Name: name}, records); err != nil {
		return SeriesDetail{}, err
	}
	return a.scanSeriesLocked(ctx, id)
}

// DeleteSeries 删除剧集配置和缓存，不操作任何源文件。
func (a *App) DeleteSeries(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrSeriesNotFound
	}
	lock := a.seriesMutex(id)
	if err := lock.Lock(ctx); err != nil {
		return false, err
	}
	defer lock.Unlock()
	return a.store.DeleteSeries(ctx, id)
}

// ScanSeries 串行重扫一个剧集的全部目录。
func (a *App) ScanSeries(ctx context.Context, id string) (SeriesDetail, error) {
	id = strings.TrimSpace(id)
	lock := a.seriesMutex(id)
	if err := lock.Lock(ctx); err != nil {
		return SeriesDetail{}, err
	}
	defer lock.Unlock()
	return a.scanSeriesLocked(ctx, id)
}

func (a *App) scanSeriesLocked(ctx context.Context, id string) (SeriesDetail, error) {
	record, ok, err := a.store.GetSeries(ctx, id)
	if err != nil {
		return SeriesDetail{}, err
	}
	if !ok {
		return SeriesDetail{}, ErrSeriesNotFound
	}
	scannedAt := time.Now()
	for _, directory := range record.Directories {
		videos, scanErr := scanSeriesDirectory(ctx, id, directory.Path)
		if scanErr != nil {
			if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
				return SeriesDetail{}, scanErr
			}
			if err := a.store.MarkSeriesDirectoryUnavailable(
				ctx, id, directory.Path, scanErr.Error(), scannedAt,
			); err != nil {
				return SeriesDetail{}, err
			}
			continue
		}
		if err := a.store.ReplaceSeriesDirectoryVideos(
			ctx, id, directory.Path, videos, scannedAt,
		); err != nil {
			return SeriesDetail{}, err
		}
	}
	if err := a.store.FinalizeSeriesScan(ctx, id, scannedAt); err != nil {
		return SeriesDetail{}, err
	}
	return a.GetSeries(ctx, id)
}

// SelectSeriesVideo 校验并记录最后选择的可用视频。
func (a *App) SelectSeriesVideo(
	ctx context.Context,
	id string,
	request SeriesSelectionRequest,
) (SeriesDetail, error) {
	id = strings.TrimSpace(id)
	path := strings.TrimSpace(request.Path)
	if path == "" {
		return SeriesDetail{}, fmt.Errorf("%w: series video path is required", ErrSeriesInvalid)
	}
	if !filepath.IsAbs(path) {
		return SeriesDetail{}, fmt.Errorf("%w: series video path must be absolute", ErrSeriesInvalid)
	}
	lock := a.seriesMutex(id)
	if err := lock.Lock(ctx); err != nil {
		return SeriesDetail{}, err
	}
	defer lock.Unlock()
	record, ok, err := a.store.GetSeries(ctx, id)
	if err != nil {
		return SeriesDetail{}, err
	}
	if !ok {
		return SeriesDetail{}, ErrSeriesNotFound
	}
	selected := ""
	for _, video := range record.Videos {
		if video.Available && sameFilesystemPath(video.Path, path) {
			selected = video.Path
			break
		}
	}
	if selected == "" {
		return SeriesDetail{}, fmt.Errorf("%w: selected video is unavailable", ErrSeriesNoPlayableVideo)
	}
	if err := a.store.SaveSeriesSelection(ctx, id, selected); err != nil {
		return SeriesDetail{}, err
	}
	return a.GetSeries(ctx, id)
}

// GetSeriesPlayback 重扫剧集，并用现有本机文件归属识别生成播放上下文。
func (a *App) GetSeriesPlayback(ctx context.Context, id, requestedPath string) (PlaybackContext, error) {
	detail, err := a.ScanSeries(ctx, id)
	if err != nil {
		return PlaybackContext{}, err
	}
	selected := selectSeriesPlaybackVideo(detail, requestedPath)
	if selected == nil {
		return PlaybackContext{}, ErrSeriesNoPlayableVideo
	}
	playback, err := a.GetFilePlayback(ctx, selected.Path)
	if err != nil {
		return PlaybackContext{}, err
	}
	playback.Title = detail.Name
	summary := detail.SeriesSummary
	playback.Series = &summary
	playback.SeriesFiles = detail.Videos
	return playback, nil
}

func selectSeriesPlaybackVideo(detail SeriesDetail, requestedPath string) *SeriesVideo {
	if strings.TrimSpace(requestedPath) != "" {
		for index := range detail.Videos {
			if detail.Videos[index].Available && sameFilesystemPath(detail.Videos[index].Path, requestedPath) {
				return &detail.Videos[index]
			}
		}
	}
	if detail.LastSelectedPath != "" {
		for index := range detail.Videos {
			if detail.Videos[index].Available &&
				sameFilesystemPath(detail.Videos[index].Path, detail.LastSelectedPath) {
				return &detail.Videos[index]
			}
		}
	}
	for index := range detail.Videos {
		if detail.Videos[index].Available {
			return &detail.Videos[index]
		}
	}
	return nil
}

func normalizeSeriesDirectories(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: series requires at least one directory", ErrSeriesInvalid)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%w: series directory is required", ErrSeriesInvalid)
		}
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("%w: series directory must be absolute: %s", ErrSeriesInvalid, value)
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("%w: resolve series directory %q: %v", ErrSeriesInvalid, value, err)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
		if err != nil {
			return nil, fmt.Errorf("%w: resolve series directory %q: %v", ErrSeriesInvalid, value, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("%w: inspect series directory %q: %v", ErrSeriesInvalid, value, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%w: series path must be a directory: %s", ErrSeriesInvalid, value)
		}
		if _, err := os.ReadDir(resolved); err != nil {
			return nil, fmt.Errorf("%w: read series directory %q: %v", ErrSeriesInvalid, value, err)
		}
		for _, existing := range result {
			if sameFilesystemPath(existing, resolved) ||
				filesystemPathWithinRoot(existing, resolved) ||
				filesystemPathWithinRoot(resolved, existing) {
				return nil, fmt.Errorf("%w: duplicate or overlapping series directory: %s", ErrSeriesInvalid, value)
			}
		}
		result = append(result, resolved)
	}
	return result, nil
}

func scanSeriesDirectory(
	ctx context.Context,
	seriesID, root string,
) ([]storage.SeriesVideoRecord, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect series directory %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("series path is not a directory: %s", root)
	}
	videos := []storage.SeriesVideoRecord{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		mediaType, _, supported := playbackMediaType(entry.Name())
		if !supported || mediaType != PlaybackMediaVideo {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		videos = append(videos, storage.SeriesVideoRecord{
			SeriesID: seriesID, DirectoryPath: root, Path: path,
			RelativePath: relative, Name: entry.Name(), ByteSize: fileInfo.Size(),
			ModifiedAt: fileInfo.ModTime(), Available: true,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan series directory %q: %w", root, err)
	}
	sort.SliceStable(videos, func(i, j int) bool {
		return naturalLess(videos[i].RelativePath, videos[j].RelativePath)
	})
	return videos, nil
}

func seriesDetailFromRecord(record storage.SeriesBundleRecord) SeriesDetail {
	directories := make([]SeriesDirectory, 0, len(record.Directories))
	order := make(map[string]int, len(record.Directories))
	scanErrors := []string{}
	for _, directory := range record.Directories {
		item := SeriesDirectory{
			Path: directory.Path, Order: directory.SourceOrder,
			Available: directory.Available, LastError: directory.LastError,
		}
		if !directory.LastScannedAt.IsZero() {
			value := directory.LastScannedAt
			item.LastScannedAt = &value
		}
		directories = append(directories, item)
		order[normalizedFilesystemPath(directory.Path)] = directory.SourceOrder
		if strings.TrimSpace(directory.LastError) != "" {
			scanErrors = append(scanErrors, fmt.Sprintf("%s: %s", directory.Path, directory.LastError))
		}
	}
	videos := make([]SeriesVideo, 0, len(record.Videos))
	available := 0
	for _, video := range record.Videos {
		videos = append(videos, SeriesVideo{
			Path: video.Path, DirectoryPath: video.DirectoryPath,
			RelativePath: video.RelativePath, Name: video.Name,
			Size: video.ByteSize, ModifiedAt: video.ModifiedAt, Available: video.Available,
		})
		if video.Available {
			available++
		}
	}
	sort.SliceStable(videos, func(i, j int) bool {
		leftOrder := order[normalizedFilesystemPath(videos[i].DirectoryPath)]
		rightOrder := order[normalizedFilesystemPath(videos[j].DirectoryPath)]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if naturalLess(videos[i].RelativePath, videos[j].RelativePath) {
			return true
		}
		if naturalLess(videos[j].RelativePath, videos[i].RelativePath) {
			return false
		}
		return naturalLess(videos[i].Path, videos[j].Path)
	})
	summary := SeriesSummary{
		ID: record.Series.ID, Name: record.Series.Name, Directories: directories,
		VideoCount: len(videos), AvailableVideoCount: available,
		LastSelectedPath: record.Series.LastSelectedPath, ScanErrors: scanErrors,
		CreatedAt: record.Series.CreatedAt, UpdatedAt: record.Series.UpdatedAt,
	}
	if !record.Series.LastScannedAt.IsZero() {
		value := record.Series.LastScannedAt
		summary.LastScannedAt = &value
	}
	return SeriesDetail{SeriesSummary: summary, Videos: videos}
}

func newSeriesID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate series id: %w", err)
	}
	return hex.EncodeToString(value), nil
}

// seriesMutex 返回同一剧集保存和扫描共用的进程内互斥锁。
func (a *App) seriesMutex(id string) *contextMutex {
	id = strings.TrimSpace(id)
	a.seriesLockMu.Lock()
	defer a.seriesLockMu.Unlock()
	if lock, ok := a.seriesLocks[id]; ok {
		return lock
	}
	lock := newContextMutex()
	a.seriesLocks[id] = lock
	return lock
}
