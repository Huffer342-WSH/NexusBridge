package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nexusbridge/internal/storage"
)

var (
	// ErrMediaLibraryNotFound 表示媒体库节点不存在。
	ErrMediaLibraryNotFound = errors.New("media library not found")
	// ErrMediaLibraryInvalid 表示媒体库层级、目录或设置无效。
	ErrMediaLibraryInvalid = errors.New("invalid media library request")
	// ErrMediaLibraryHasChildren 表示删除仍有子节点的媒体库会破坏层级。
	ErrMediaLibraryHasChildren = errors.New("media library has children")
)

type storedMediaLibrarySettings struct {
	Managed                bool  `json:"managed,omitempty"`
	EpisodeNumberDetection *bool `json:"episode_number_detection,omitempty"`
	AutoDetectSeries       *bool `json:"auto_detect_series,omitempty"`
}

const mediaLibraryHierarchyLockID = "media-library-hierarchy"

// ListMediaLibraries 返回扁平媒体库树；父子关系由 parent_id 表达。
func (a *App) ListMediaLibraries(ctx context.Context) ([]MediaLibrarySummary, error) {
	records, err := a.store.ListMediaLibraries(ctx)
	if err != nil {
		return nil, err
	}
	resolver := newMediaLibrarySettingsResolver(records)
	result := make([]MediaLibrarySummary, 0, len(records))
	for _, record := range records {
		effective, err := resolver.resolve(record.Series.ID)
		if err != nil {
			return nil, err
		}
		record = mediaLibraryWithoutChildVideos(record, records)
		result = append(result, mediaLibraryDetailFromRecord(record, resolver.local(record.Series), effective).MediaLibrarySummary)
	}
	return result, nil
}

// GetMediaLibrary 返回一个媒体库节点及已有缓存。
func (a *App) GetMediaLibrary(ctx context.Context, id string) (MediaLibraryDetail, error) {
	id = strings.TrimSpace(id)
	record, ok, err := a.store.GetMediaLibrary(ctx, id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	if !ok {
		return MediaLibraryDetail{}, ErrMediaLibraryNotFound
	}
	all, err := a.store.ListMediaLibraries(ctx)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	resolver := newMediaLibrarySettingsResolver(all)
	effective, err := resolver.resolve(id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	record = mediaLibraryWithoutChildVideos(record, all)
	return mediaLibraryDetailFromRecord(record, resolver.local(record.Series), effective), nil
}

// SaveMediaLibrary 创建或更新媒体库节点；节点类型创建后保持不变。
func (a *App) SaveMediaLibrary(
	ctx context.Context,
	id string,
	request MediaLibrarySaveRequest,
) (MediaLibraryDetail, error) {
	id = strings.TrimSpace(id)
	creating := id == ""
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return MediaLibraryDetail{}, fmt.Errorf("%w: name is required", ErrMediaLibraryInvalid)
	}
	if request.Kind != MediaLibraryCollection && request.Kind != MediaLibrarySeries {
		return MediaLibraryDetail{}, fmt.Errorf("%w: kind must be collection or series", ErrMediaLibraryInvalid)
	}
	directories, err := normalizeMediaLibraryDirectories(request.Directories)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	if creating {
		id, err = newSeriesID()
		if err != nil {
			return MediaLibraryDetail{}, err
		}
	}
	unlock, err := a.lockMediaLibraryMutation(ctx, id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	defer unlock()

	records, err := a.store.ListMediaLibraries(ctx)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	byID := make(map[string]storage.SeriesBundleRecord, len(records)+1)
	for _, item := range records {
		byID[item.Series.ID] = item
		if item.Series.ID != id && strings.EqualFold(strings.TrimSpace(item.Series.Name), name) {
			return MediaLibraryDetail{}, fmt.Errorf("%w: name %q already exists", ErrMediaLibraryInvalid, name)
		}
	}
	if current, exists := byID[id]; !creating {
		if !exists {
			return MediaLibraryDetail{}, ErrMediaLibraryNotFound
		}
		if MediaLibraryKind(current.Series.Kind) != request.Kind {
			return MediaLibraryDetail{}, fmt.Errorf("%w: kind cannot be changed", ErrMediaLibraryInvalid)
		}
	}
	parentID := strings.TrimSpace(request.ParentID)
	if parentID == id {
		return MediaLibraryDetail{}, fmt.Errorf("%w: a library cannot be its own parent", ErrMediaLibraryInvalid)
	}
	if parentID != "" {
		parent, exists := byID[parentID]
		if !exists {
			return MediaLibraryDetail{}, fmt.Errorf("%w: parent not found", ErrMediaLibraryInvalid)
		}
		if MediaLibraryKind(parent.Series.Kind) == MediaLibrarySeries {
			return MediaLibraryDetail{}, fmt.Errorf("%w: a series cannot contain child media libraries", ErrMediaLibraryInvalid)
		}
		if mediaLibraryParentChainContains(byID, parentID, id) {
			return MediaLibraryDetail{}, fmt.Errorf("%w: parent would create a cycle", ErrMediaLibraryInvalid)
		}
		if !mediaLibraryDirectoriesWithinParent(directories, parent.Directories) {
			return MediaLibraryDetail{}, fmt.Errorf("%w: child directories must be inside a parent directory", ErrMediaLibraryInvalid)
		}
	}
	for _, child := range byID {
		if request.Kind == MediaLibrarySeries && child.Series.ParentID == id {
			return MediaLibraryDetail{}, fmt.Errorf(
				"%w: a series cannot contain child media libraries",
				ErrMediaLibraryInvalid,
			)
		}
		if child.Series.ParentID == id && !mediaLibraryDirectoriesWithinParent(
			mediaLibraryDirectoryPaths(child.Directories), directoryRecordsFromPaths(directories),
		) {
			return MediaLibraryDetail{}, fmt.Errorf(
				"%w: updated directories would move child %q outside its parent",
				ErrMediaLibraryInvalid,
				child.Series.Name,
			)
		}
	}
	settingsJSON, err := encodeMediaLibrarySettings(request.Settings)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	directoryRecords := make([]storage.SeriesDirectoryRecord, 0, len(directories))
	for index, directory := range directories {
		directoryRecords = append(directoryRecords, storage.SeriesDirectoryRecord{
			SeriesID: id, Path: directory, SourceOrder: index,
		})
	}
	newRecord := storage.SeriesBundleRecord{Series: storage.SeriesRecord{
		ID: id, Name: name, Kind: string(request.Kind), ParentID: parentID, SettingsJSON: settingsJSON,
	}, Directories: directoryRecords}
	byID[id] = newRecord
	resolverRecords := make([]storage.SeriesBundleRecord, 0, len(byID))
	for _, item := range byID {
		resolverRecords = append(resolverRecords, item)
	}
	resolver := newMediaLibrarySettingsResolver(resolverRecords)
	effective, err := resolver.resolve(id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	newRecord.Series.EpisodeNumberDetection = effective.EpisodeNumberDetection
	if err := a.store.SaveMediaLibrary(ctx, newRecord.Series, directoryRecords); err != nil {
		return MediaLibraryDetail{}, err
	}
	detail, err := a.scanMediaLibraryLocked(ctx, id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	return detail, nil
}

// DeleteMediaLibrary 删除叶子媒体库，不删除任何媒体文件。
func (a *App) DeleteMediaLibrary(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrMediaLibraryNotFound
	}
	unlock, err := a.lockMediaLibraryMutation(ctx, id)
	if err != nil {
		return false, err
	}
	defer unlock()
	hasChildren, err := a.store.MediaLibraryHasChildren(ctx, id)
	if err != nil {
		return false, err
	}
	if hasChildren {
		return false, ErrMediaLibraryHasChildren
	}
	return a.store.DeleteMediaLibrary(ctx, id)
}

// ScanMediaLibrary 重扫媒体库目录，并排除由直接子媒体库管理的目录树。
func (a *App) ScanMediaLibrary(ctx context.Context, id string) (MediaLibraryDetail, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MediaLibraryDetail{}, ErrMediaLibraryNotFound
	}
	unlock, err := a.lockMediaLibraryMutation(ctx, id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	defer unlock()
	return a.scanMediaLibraryLocked(ctx, id)
}

func (a *App) scanMediaLibraryLocked(ctx context.Context, id string) (MediaLibraryDetail, error) {
	record, ok, err := a.store.GetMediaLibrary(ctx, id)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	if !ok {
		return MediaLibraryDetail{}, ErrMediaLibraryNotFound
	}
	all, err := a.store.ListMediaLibraries(ctx)
	if err != nil {
		return MediaLibraryDetail{}, err
	}
	excluded := []string{}
	for _, item := range all {
		if item.Series.ParentID == id {
			excluded = append(excluded, mediaLibraryDirectoryPaths(item.Directories)...)
		}
	}
	scannedAt := time.Now()
	for _, directory := range record.Directories {
		videos, scanErr := scanSeriesDirectory(ctx, id, directory.Path, excluded)
		if scanErr != nil {
			if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
				return MediaLibraryDetail{}, scanErr
			}
			if err := a.store.MarkSeriesDirectoryUnavailable(
				ctx, id, directory.Path, scanErr.Error(), scannedAt,
			); err != nil {
				return MediaLibraryDetail{}, err
			}
			continue
		}
		if err := a.store.ReplaceSeriesDirectoryVideos(
			ctx, id, directory.Path, videos, scannedAt,
		); err != nil {
			return MediaLibraryDetail{}, err
		}
	}
	if err := a.store.FinalizeSeriesScan(ctx, id, scannedAt); err != nil {
		return MediaLibraryDetail{}, err
	}
	return a.GetMediaLibrary(ctx, id)
}

type mediaLibrarySettingsResolver struct {
	records map[string]storage.SeriesBundleRecord
	cache   map[string]MediaLibrarySettings
	active  map[string]bool
}

func newMediaLibrarySettingsResolver(records []storage.SeriesBundleRecord) *mediaLibrarySettingsResolver {
	result := &mediaLibrarySettingsResolver{
		records: make(map[string]storage.SeriesBundleRecord, len(records)),
		cache:   map[string]MediaLibrarySettings{}, active: map[string]bool{},
	}
	for _, record := range records {
		result.records[record.Series.ID] = record
	}
	return result
}

func (r *mediaLibrarySettingsResolver) local(record storage.SeriesRecord) MediaLibrarySettingsOverride {
	var stored storedMediaLibrarySettings
	if err := json.Unmarshal([]byte(record.SettingsJSON), &stored); err == nil && stored.Managed {
		return MediaLibrarySettingsOverride{
			EpisodeNumberDetection: stored.EpisodeNumberDetection,
			AutoDetectSeries:       stored.AutoDetectSeries,
		}
	}
	if MediaLibraryKind(record.Kind) == MediaLibrarySeries {
		value := record.EpisodeNumberDetection
		return MediaLibrarySettingsOverride{EpisodeNumberDetection: &value}
	}
	return MediaLibrarySettingsOverride{}
}

func (r *mediaLibrarySettingsResolver) resolve(id string) (MediaLibrarySettings, error) {
	if value, ok := r.cache[id]; ok {
		return value, nil
	}
	if r.active[id] {
		return MediaLibrarySettings{}, fmt.Errorf("%w: hierarchy contains a cycle", ErrMediaLibraryInvalid)
	}
	record, ok := r.records[id]
	if !ok {
		return MediaLibrarySettings{}, ErrMediaLibraryNotFound
	}
	r.active[id] = true
	defer delete(r.active, id)
	result := MediaLibrarySettings{}
	if record.Series.ParentID != "" {
		parent, err := r.resolve(record.Series.ParentID)
		if err != nil {
			return MediaLibrarySettings{}, err
		}
		result = parent
	}
	local := r.local(record.Series)
	if local.EpisodeNumberDetection != nil {
		result.EpisodeNumberDetection = *local.EpisodeNumberDetection
	}
	if local.AutoDetectSeries != nil {
		result.AutoDetectSeries = *local.AutoDetectSeries
	}
	r.cache[id] = result
	return result, nil
}

func encodeMediaLibrarySettings(settings MediaLibrarySettingsOverride) (string, error) {
	content, err := json.Marshal(storedMediaLibrarySettings{
		Managed: true, EpisodeNumberDetection: settings.EpisodeNumberDetection,
		AutoDetectSeries: settings.AutoDetectSeries,
	})
	if err != nil {
		return "", fmt.Errorf("%w: encode settings: %v", ErrMediaLibraryInvalid, err)
	}
	return string(content), nil
}

func mediaLibraryParentChainContains(records map[string]storage.SeriesBundleRecord, parentID, targetID string) bool {
	seen := map[string]bool{}
	for parentID != "" && !seen[parentID] {
		if parentID == targetID {
			return true
		}
		seen[parentID] = true
		parent, ok := records[parentID]
		if !ok {
			return false
		}
		parentID = parent.Series.ParentID
	}
	return parentID != ""
}

func mediaLibraryDirectoriesWithinParent(children []string, parents []storage.SeriesDirectoryRecord) bool {
	for _, child := range children {
		matched := false
		for _, parent := range parents {
			if !sameFilesystemPath(child, parent.Path) && filesystemPathWithinRoot(child, parent.Path) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func mediaLibraryDirectoryPaths(records []storage.SeriesDirectoryRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Path)
	}
	return result
}

func directoryRecordsFromPaths(paths []string) []storage.SeriesDirectoryRecord {
	result := make([]storage.SeriesDirectoryRecord, 0, len(paths))
	for _, path := range paths {
		result = append(result, storage.SeriesDirectoryRecord{Path: path})
	}
	return result
}

func mediaLibraryWithoutChildVideos(
	record storage.SeriesBundleRecord,
	all []storage.SeriesBundleRecord,
) storage.SeriesBundleRecord {
	excluded := []string{}
	for _, item := range all {
		if item.Series.ParentID == record.Series.ID {
			excluded = append(excluded, mediaLibraryDirectoryPaths(item.Directories)...)
		}
	}
	if len(excluded) == 0 || len(record.Videos) == 0 {
		return record
	}
	videos := make([]storage.SeriesVideoRecord, 0, len(record.Videos))
	for _, video := range record.Videos {
		managedByChild := false
		for _, root := range excluded {
			if sameFilesystemPath(video.Path, root) || filesystemPathWithinRoot(video.Path, root) {
				managedByChild = true
				break
			}
		}
		if !managedByChild {
			videos = append(videos, video)
		}
	}
	record.Videos = videos
	return record
}

func normalizeMediaLibraryDirectories(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: at least one directory is required", ErrMediaLibraryInvalid)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%w: directory is required", ErrMediaLibraryInvalid)
		}
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("%w: directory must be absolute: %s", ErrMediaLibraryInvalid, value)
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("%w: resolve directory %q: %v", ErrMediaLibraryInvalid, value, err)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
		if err != nil {
			return nil, fmt.Errorf("%w: resolve directory %q: %v", ErrMediaLibraryInvalid, value, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("%w: inspect directory %q: %v", ErrMediaLibraryInvalid, value, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%w: path must be a directory: %s", ErrMediaLibraryInvalid, value)
		}
		if _, err := os.ReadDir(resolved); err != nil {
			return nil, fmt.Errorf("%w: read directory %q: %v", ErrMediaLibraryInvalid, value, err)
		}
		for _, existing := range result {
			if sameFilesystemPath(existing, resolved) ||
				filesystemPathWithinRoot(existing, resolved) ||
				filesystemPathWithinRoot(resolved, existing) {
				return nil, fmt.Errorf("%w: duplicate or overlapping directory: %s", ErrMediaLibraryInvalid, value)
			}
		}
		result = append(result, resolved)
	}
	return result, nil
}

func mediaLibraryDetailFromRecord(
	record storage.SeriesBundleRecord,
	local MediaLibrarySettingsOverride,
	effective MediaLibrarySettings,
) MediaLibraryDetail {
	record.Series.EpisodeNumberDetection = effective.EpisodeNumberDetection
	series := seriesDetailFromRecord(record)
	return MediaLibraryDetail{
		MediaLibrarySummary: MediaLibrarySummary{
			ID: record.Series.ID, Name: record.Series.Name, Kind: MediaLibraryKind(record.Series.Kind),
			ParentID: record.Series.ParentID, Directories: series.Directories,
			Settings: local, EffectiveSettings: effective,
			VideoCount: series.VideoCount, AvailableVideos: series.AvailableVideoCount,
			LastScannedAt: series.LastScannedAt, ScanErrors: series.ScanErrors,
			CreatedAt: series.CreatedAt, UpdatedAt: series.UpdatedAt,
		},
		Videos: series.Videos,
	}
}

func (a *App) lockMediaLibraryMutation(ctx context.Context, id string) (func(), error) {
	hierarchy := a.seriesMutex(mediaLibraryHierarchyLockID)
	if err := hierarchy.Lock(ctx); err != nil {
		return nil, err
	}
	node := a.seriesMutex(id)
	if err := node.Lock(ctx); err != nil {
		hierarchy.Unlock()
		return nil, err
	}
	return func() {
		node.Unlock()
		hierarchy.Unlock()
	}, nil
}
