package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

var (
	// ErrPlaybackNotFound 表示播放种子或文件索引不存在。
	ErrPlaybackNotFound = errors.New("playback resource not found")
	// ErrPlaybackNotAdded 表示种子尚未关联到 qB 任务。
	ErrPlaybackNotAdded = errors.New("torrent is not added to qBittorrent")
	// ErrPlaybackNoData 表示媒体文件尚无可读取的下载数据。
	ErrPlaybackNoData = errors.New("media file has no downloaded data")
	// ErrPlaybackUnavailable 表示 qB 或同机源文件当前不可访问。
	ErrPlaybackUnavailable = errors.New("playback source is unavailable")
)

var playbackMediaTypes = map[string]struct {
	kind PlaybackMediaType
	mime string
}{
	".mp4":  {PlaybackMediaVideo, "video/mp4"},
	".m4v":  {PlaybackMediaVideo, "video/x-m4v"},
	".webm": {PlaybackMediaVideo, "video/webm"},
	".ogv":  {PlaybackMediaVideo, "video/ogg"},
	".mov":  {PlaybackMediaVideo, "video/quicktime"},
	".mkv":  {PlaybackMediaVideo, "video/x-matroska"},
	".avi":  {PlaybackMediaVideo, "video/x-msvideo"},
	".ts":   {PlaybackMediaVideo, "video/mp2t"},
	".m2ts": {PlaybackMediaVideo, "video/mp2t"},
	".mts":  {PlaybackMediaVideo, "video/mp2t"},
	".mp3":  {PlaybackMediaAudio, "audio/mpeg"},
	".m4a":  {PlaybackMediaAudio, "audio/mp4"},
	".aac":  {PlaybackMediaAudio, "audio/aac"},
	".flac": {PlaybackMediaAudio, "audio/flac"},
	".wav":  {PlaybackMediaAudio, "audio/wav"},
	".ogg":  {PlaybackMediaAudio, "audio/ogg"},
	".oga":  {PlaybackMediaAudio, "audio/ogg"},
	".opus": {PlaybackMediaAudio, "audio/ogg"},
	".weba": {PlaybackMediaAudio, "audio/webm"},
	".jpg":  {PlaybackMediaImage, "image/jpeg"},
	".jpeg": {PlaybackMediaImage, "image/jpeg"},
	".png":  {PlaybackMediaImage, "image/png"},
	".webp": {PlaybackMediaImage, "image/webp"},
	".gif":  {PlaybackMediaImage, "image/gif"},
	".avif": {PlaybackMediaImage, "image/avif"},
	".bmp":  {PlaybackMediaImage, "image/bmp"},
}

// GetTorrentPlayback 实时读取当前种子的 qB 文件清单。
func (a *App) GetTorrentPlayback(ctx context.Context, siteID, torrentID string) (TorrentPlayback, error) {
	torrent, qbTorrent, contents, err := a.playbackTorrentContents(ctx, siteID, torrentID)
	if err != nil {
		return TorrentPlayback{}, err
	}
	files := playbackMediaList(siteID, torrentID, qbTorrent, contents)
	status := statusFromQBTorrent(qbTorrent, storage.DownloadTaskRecord{}, "hash", time.Now())
	torrent.QBStatus = &status
	return TorrentPlayback{
		Torrent: torrent, QBStatus: status, Files: files,
		DefaultFileIndex: defaultPlaybackFileIndex(files),
	}, nil
}

// OpenTorrentMedia 按 qB 文件索引打开经过路径边界校验的源文件。
func (a *App) OpenTorrentMedia(ctx context.Context, siteID, torrentID string, fileIndex int) (PlaybackSource, error) {
	_, qbTorrent, contents, err := a.playbackTorrentContents(ctx, siteID, torrentID)
	if err != nil {
		return PlaybackSource{}, err
	}
	var selected *qbittorrent.TorrentContent
	for i := range contents {
		if contents[i].Index == fileIndex {
			selected = &contents[i]
			break
		}
	}
	if selected == nil {
		return PlaybackSource{}, fmt.Errorf("%w: media index %d", ErrPlaybackNotFound, fileIndex)
	}
	_, mimeType, ok := playbackMediaType(selected.Name)
	if !ok {
		return PlaybackSource{}, fmt.Errorf("%w: unsupported media index %d", ErrPlaybackNotFound, fileIndex)
	}
	if selected.Progress <= 0 {
		return PlaybackSource{}, ErrPlaybackNoData
	}
	resolved, info, err := resolvePlaybackFile(qbTorrent.SavePath, selected.Name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PlaybackSource{}, fmt.Errorf("%w: media index %d", ErrPlaybackNotFound, fileIndex)
		}
		return PlaybackSource{}, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	file, err := os.Open(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PlaybackSource{}, fmt.Errorf("%w: media index %d", ErrPlaybackNotFound, fileIndex)
		}
		return PlaybackSource{}, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	return PlaybackSource{File: file, Name: path.Base(selected.Name), ContentType: mimeType, ModTime: info.ModTime()}, nil
}

// ListPlaybackTorrents 返回至少包含一个完整音频或视频的其他 qB 种子。
func (a *App) ListPlaybackTorrents(ctx context.Context, excludeSiteID, excludeTorrentID string, limit int) ([]PlaybackTorrent, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		return nil, fmt.Errorf("playback torrent limit must not exceed 50")
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	qbTorrents, err := qb.ListTorrents(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	qbByHash := make(map[string]qbittorrent.TorrentInfo, len(qbTorrents))
	for _, item := range qbTorrents {
		qbByHash[strings.ToLower(strings.TrimSpace(item.Hash))] = item
	}

	const pageSize = 50
	result := make([]PlaybackTorrent, 0, limit)
	for offset := 0; len(result) < limit; offset += pageSize {
		records, err := a.store.ListTorrents(ctx, storage.TorrentListQuery{
			SortBy: "published_at", SortDirection: "desc", Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			break
		}
		torrents, err := a.torrentsFromRecords(ctx, records, TorrentQuery{})
		if err != nil {
			return nil, err
		}
		keys := make([]storage.TorrentKey, 0, len(torrents))
		for _, torrent := range torrents {
			keys = append(keys, storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID})
		}
		tasks, err := a.store.ListDownloadTasksByTorrents(ctx, keys)
		if err != nil {
			return nil, err
		}
		eligible := inspectPlaybackCandidates(ctx, qb, qbByHash, torrents, tasks, excludeSiteID, excludeTorrentID)
		for i, ok := range eligible {
			if !ok || len(result) >= limit {
				continue
			}
			item := torrents[i]
			result = append(result, PlaybackTorrent{
				ID: item.ID, SiteID: item.SiteID, Title: item.Title, CoverURL: item.CoverURL,
				Category: item.Category, PublishedAt: item.PublishedAt,
			})
		}
		if len(records) < pageSize {
			break
		}
	}
	return result, nil
}

// playbackTorrentContents 按本地种子标识解析 qB hash，并读取实时任务与文件清单。
func (a *App) playbackTorrentContents(ctx context.Context, siteID, torrentID string) (Torrent, qbittorrent.TorrentInfo, []qbittorrent.TorrentContent, error) {
	torrent, err := a.getTorrent(ctx, siteID, torrentID)
	if err != nil {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, fmt.Errorf("%w: %v", ErrPlaybackNotFound, err)
	}
	hash, err := a.torrentQBHash(ctx, torrent)
	if err != nil {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, err
	}
	if strings.TrimSpace(hash) == "" {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, ErrPlaybackNotAdded
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	matches, err := qb.ListTorrentsWithOptions(ctx, qbittorrent.TorrentListOptions{Hashes: []string{hash}})
	if err != nil {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	if len(matches) == 0 {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, ErrPlaybackNotAdded
	}
	contents, err := qb.GetTorrentContents(ctx, matches[0].Hash, nil)
	if err != nil {
		return Torrent{}, qbittorrent.TorrentInfo{}, nil, fmt.Errorf("%w: %v", ErrPlaybackUnavailable, err)
	}
	return torrent, matches[0], contents, nil
}

// playbackMediaList 将 qB 文件清单转换为自然排序的可播放媒体，并在流地址中保留文件名格式提示。
// playbackMediaList 将 qB 文件清单转换为经过类型识别和路径校验的播放选集。
func playbackMediaList(siteID, torrentID string, qbTorrent qbittorrent.TorrentInfo, contents []qbittorrent.TorrentContent) []PlaybackMedia {
	files := make([]PlaybackMedia, 0, len(contents))
	for _, content := range contents {
		mediaType, mimeType, ok := playbackMediaType(content.Name)
		if !ok {
			continue
		}
		_, _, pathErr := resolvePlaybackFile(qbTorrent.SavePath, content.Name)
		available := content.Progress > 0 && pathErr == nil
		files = append(files, PlaybackMedia{
			Index: content.Index, Name: content.Name, MediaType: mediaType, MIMEType: mimeType,
			Size: content.Size, Progress: content.Progress, Selected: content.Priority > 0,
			Complete: content.Progress >= 1, Available: available,
			StreamURL: fmt.Sprintf(
				"/api/torrents/%s/%s/media/%d?filename=%s",
				url.PathEscape(siteID),
				url.PathEscape(torrentID),
				content.Index,
				url.QueryEscape(path.Base(content.Name)),
			),
		})
	}
	sort.SliceStable(files, func(i, j int) bool { return naturalLess(files[i].Name, files[j].Name) })
	return files
}

// defaultPlaybackFileIndex 按完整视频、完整音频、完整图片和最高进度的顺序选择默认文件。
func defaultPlaybackFileIndex(files []PlaybackMedia) *int {
	for _, mediaType := range []PlaybackMediaType{PlaybackMediaVideo, PlaybackMediaAudio, PlaybackMediaImage} {
		for _, file := range files {
			if file.MediaType == mediaType && file.Complete && file.Available {
				value := file.Index
				return &value
			}
		}
	}
	best := -1
	for i := range files {
		if !files[i].Available {
			continue
		}
		if best < 0 || files[i].Progress > files[best].Progress ||
			(files[i].Progress == files[best].Progress && playbackTypeRank(files[i].MediaType) < playbackTypeRank(files[best].MediaType)) {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	value := files[best].Index
	return &value
}

// playbackTypeRank 返回媒体类型在默认文件选择中的优先级。
func playbackTypeRank(mediaType PlaybackMediaType) int {
	switch mediaType {
	case PlaybackMediaVideo:
		return 0
	case PlaybackMediaAudio:
		return 1
	default:
		return 2
	}
}

// playbackMediaType 按稳定扩展名白名单识别媒体类型和 MIME。
func playbackMediaType(name string) (PlaybackMediaType, string, bool) {
	value, ok := playbackMediaTypes[strings.ToLower(filepath.Ext(name))]
	return value.kind, value.mime, ok
}

// resolvePlaybackFile 解析 qB 相对文件名，确保最终普通文件位于任务保存目录内。
func resolvePlaybackFile(savePath, name string) (string, os.FileInfo, error) {
	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		return "", nil, errors.New("qB save path is empty")
	}
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	cleanName := path.Clean(normalized)
	if cleanName == "." || path.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, "../") {
		return "", nil, errors.New("qB media path leaves the save directory")
	}
	root, err := filepath.Abs(savePath)
	if err != nil {
		return "", nil, err
	}
	candidate := filepath.Join(root, filepath.FromSlash(cleanName))
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", nil, errors.New("qB media path leaves the save directory")
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", nil, err
	}
	candidateResolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", nil, err
	}
	resolvedRelative, err := filepath.Rel(rootResolved, candidateResolved)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRelative) {
		return "", nil, errors.New("resolved media path leaves the save directory")
	}
	info, err := os.Stat(candidateResolved)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("media path is not a regular file")
	}
	return candidateResolved, info, nil
}

// inspectPlaybackCandidates 并发检查种子是否包含完整且可读取的音频或视频文件。
func inspectPlaybackCandidates(
	ctx context.Context,
	qb *qbittorrent.Client,
	qbByHash map[string]qbittorrent.TorrentInfo,
	torrents []Torrent,
	tasks map[string][]storage.DownloadTaskRecord,
	excludeSiteID, excludeTorrentID string,
) []bool {
	result := make([]bool, len(torrents))
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range torrents {
		torrent := torrents[i]
		if torrent.SiteID == excludeSiteID && torrent.ID == excludeTorrentID {
			continue
		}
		hash := strings.TrimSpace(torrent.InfoHashV1)
		if hash == "" && torrent.QBStatus != nil {
			hash = strings.TrimSpace(torrent.QBStatus.Hash)
		}
		if hash == "" {
			for _, task := range tasks[torrentKey(torrent)] {
				if strings.TrimSpace(task.QBHash) != "" {
					hash = task.QBHash
					break
				}
			}
		}
		qbTorrent, ok := qbByHash[strings.ToLower(hash)]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(index int, item qbittorrent.TorrentInfo) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()
			contents, err := qb.GetTorrentContents(ctx, item.Hash, nil)
			if err != nil {
				return
			}
			for _, content := range contents {
				mediaType, _, supported := playbackMediaType(content.Name)
				if !supported || (mediaType != PlaybackMediaVideo && mediaType != PlaybackMediaAudio) || content.Progress < 1 {
					continue
				}
				if _, _, err := resolvePlaybackFile(item.SavePath, content.Name); err == nil {
					result[index] = true
					return
				}
			}
		}(i, qbTorrent)
	}
	wg.Wait()
	return result
}

// naturalLess 按文件名中的文本和数字片段执行不区分大小写的自然顺序比较。
func naturalLess(left, right string) bool {
	left = strings.ToLower(left)
	right = strings.ToLower(right)
	for len(left) > 0 && len(right) > 0 {
		leftDigit := left[0] >= '0' && left[0] <= '9'
		rightDigit := right[0] >= '0' && right[0] <= '9'
		if leftDigit && rightDigit {
			leftEnd, rightEnd := 0, 0
			for leftEnd < len(left) && left[leftEnd] >= '0' && left[leftEnd] <= '9' {
				leftEnd++
			}
			for rightEnd < len(right) && right[rightEnd] >= '0' && right[rightEnd] <= '9' {
				rightEnd++
			}
			leftNumber, _ := strconv.ParseUint(left[:leftEnd], 10, 64)
			rightNumber, _ := strconv.ParseUint(right[:rightEnd], 10, 64)
			if leftNumber != rightNumber {
				return leftNumber < rightNumber
			}
			left, right = left[leftEnd:], right[rightEnd:]
			continue
		}
		if left[0] != right[0] {
			return left[0] < right[0]
		}
		left, right = left[1:], right[1:]
	}
	return len(left) < len(right)
}
