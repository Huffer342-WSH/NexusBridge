// Package subtitleartifact 提供 MKV 文本字幕的按需生成、进程内防重和磁盘持久化。
package subtitleartifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gravity-zero/mkvgo/matroska"
	"github.com/gravity-zero/mkvgo/mkv/subtitle"
)

const (
	maxArtifactBytes   = 32 << 20
	defaultCueDuration = int64(3000)
	FormatWebVTT       = "vtt"
	FormatASS          = "ass"
)

// Result 是一次可直接返回给浏览器的字幕产物。
type Result struct {
	Content []byte
	Format  string
	ETag    string
	Cached  bool
}

// EnsureExternal 返回外挂字幕的原始 ASS 或兼容 WebVTT 产物。
func (s *Service) EnsureExternal(ctx context.Context, videoPath, subtitlePath, format string) (Result, error) {
	format = normalizeFormat(format)
	ext := strings.ToLower(filepath.Ext(subtitlePath))
	if format == FormatASS && ext != ".ass" && ext != ".ssa" {
		return Result{}, fmt.Errorf("subtitle %q has no ASS artifact", subtitlePath)
	}
	key, err := externalArtifactKey(videoPath, subtitlePath)
	if err != nil {
		return Result{}, err
	}
	path := s.artifactPath(key, format)
	if content, err := readArtifact(path); err == nil {
		_ = persistSourceMetadata(path, videoPath)
		return Result{Content: content, Format: format, ETag: key, Cached: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	call, owner := s.begin(key + ":" + format)
	if !owner {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-call.done:
			return call.result, call.err
		}
	}
	var output bytes.Buffer
	writer := &limitedWriter{buffer: &output, remaining: maxArtifactBytes}
	if format == FormatASS {
		file, openErr := os.Open(subtitlePath)
		if openErr == nil {
			_, openErr = io.Copy(writer, file)
			_ = file.Close()
		}
		err = openErr
	} else {
		err = subtitle.FileToWebVTT(subtitlePath, writer)
	}
	result := Result{Content: output.Bytes(), Format: format, ETag: key}
	if err == nil {
		if persistArtifact(path, result.Content) == nil {
			_ = persistSourceMetadata(path, videoPath)
			result.Cached = true
		}
	}
	s.finish(key+":"+format, call, result, err)
	return result, err
}

func externalArtifactKey(videoPath, subtitlePath string) (string, error) {
	video, err := filepath.Abs(strings.TrimSpace(videoPath))
	if err != nil {
		return "", err
	}
	subtitlePath, err = filepath.Abs(strings.TrimSpace(subtitlePath))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(subtitlePath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("subtitle source is not a regular file")
	}
	value := fmt.Sprintf("%s\x00%s\x00%d\x00%d", strings.ToLower(filepath.Clean(video)), strings.ToLower(filepath.Clean(subtitlePath)), info.Size(), info.ModTime().UnixNano())
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), nil
}

type pendingCall struct {
	done   chan struct{}
	result Result
	err    error
}

// Service 管理字幕缓存目录和同一产物的进程内防重。
type Service struct {
	root    string
	mu      sync.Mutex
	pending map[string]*pendingCall
}

// New 创建字幕产物服务。
func New(root string) (*Service, error) {
	resolved, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, fmt.Errorf("resolve subtitle cache root: %w", err)
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return nil, fmt.Errorf("create subtitle cache root: %w", err)
	}
	return &Service{root: resolved, pending: map[string]*pendingCall{}}, nil
}

// Ensure 返回指定轨道的字幕产物；缓存缺失时在当前请求内生成并尽力持久化。
func (s *Service) Ensure(ctx context.Context, sourcePath string, trackID uint64, codec, format string) (Result, error) {
	format = normalizeFormat(format)
	codec = strings.ToLower(strings.TrimSpace(codec))
	if format == FormatASS && codec != "ass" && codec != "ssa" {
		return Result{}, fmt.Errorf("track %d codec %q has no ASS artifact", trackID, codec)
	}
	key, err := artifactKey(sourcePath, trackID)
	if err != nil {
		return Result{}, err
	}
	path := s.artifactPath(key, format)
	if content, err := readArtifact(path); err == nil {
		_ = persistSourceMetadata(path, sourcePath)
		return Result{Content: content, Format: format, ETag: key, Cached: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	call, owner := s.begin(key + ":" + format)
	if !owner {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-call.done:
			return call.result, call.err
		}
	}
	result, ensureErr := s.generate(ctx, sourcePath, trackID, codec, format, key, path)
	s.finish(key+":"+format, call, result, ensureErr)
	return result, ensureErr
}

func (s *Service) begin(key string) (*pendingCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if call := s.pending[key]; call != nil {
		return call, false
	}
	call := &pendingCall{done: make(chan struct{})}
	s.pending[key] = call
	return call, true
}

func (s *Service) finish(key string, call *pendingCall, result Result, err error) {
	s.mu.Lock()
	call.result, call.err = result, err
	delete(s.pending, key)
	close(call.done)
	s.mu.Unlock()
}

func (s *Service) generate(
	ctx context.Context,
	sourcePath string,
	trackID uint64,
	codec, format, key, path string,
) (Result, error) {
	var content []byte
	var err error
	if format == FormatASS {
		content, err = extractASS(ctx, sourcePath, trackID, codec)
	} else {
		content, err = extractWebVTT(ctx, sourcePath, trackID)
	}
	if err != nil {
		return Result{}, err
	}
	result := Result{Content: content, Format: format, ETag: key}
	// 缓存写入失败不阻断本次播放；调用方仍可返回当前内存产物。
	if err := persistArtifact(path, content); err == nil {
		_ = persistSourceMetadata(path, sourcePath)
		result.Cached = true
	}
	return result, nil
}

// CleanupVideos 只删除成功扫描确认消失且当前明确不存在的视频字幕缓存。
func (s *Service) CleanupVideos(videoPaths []string) (int, error) {
	missing := make(map[string]struct{}, len(videoPaths))
	for _, videoPath := range videoPaths {
		if _, err := os.Stat(videoPath); errors.Is(err, os.ErrNotExist) {
			missing[strings.ToLower(filepath.Clean(videoPath))] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}
	removed := 0
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || entry.Name() != "source.video" {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		videoPath := strings.ToLower(filepath.Clean(strings.TrimSpace(string(content))))
		if _, confirmed := missing[videoPath]; !confirmed {
			return nil
		}
		if err := os.RemoveAll(filepath.Dir(path)); err != nil {
			return err
		}
		removed++
		return filepath.SkipDir
	})
	return removed, err
}

func persistSourceMetadata(artifactPath, sourcePath string) error {
	return os.WriteFile(filepath.Join(filepath.Dir(artifactPath), "source.video"), []byte(filepath.Clean(sourcePath)), 0o644)
}

func (s *Service) artifactPath(key, format string) string {
	return filepath.Join(s.root, key[:2], key, "subtitle."+format)
}

func artifactKey(sourcePath string, trackID uint64) (string, error) {
	resolved, err := filepath.Abs(strings.TrimSpace(sourcePath))
	if err != nil {
		return "", fmt.Errorf("resolve subtitle source: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect subtitle source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("subtitle source is not a regular file")
	}
	value := fmt.Sprintf("%s\x00%d\x00%d\x00%d", strings.ToLower(filepath.Clean(resolved)), info.Size(), info.ModTime().UnixNano(), trackID)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), nil
}

func normalizeFormat(format string) string {
	if strings.EqualFold(strings.TrimSpace(format), FormatASS) {
		return FormatASS
	}
	return FormatWebVTT
}

func readArtifact(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > maxArtifactBytes {
		return nil, fmt.Errorf("subtitle artifact exceeds %d bytes", maxArtifactBytes)
	}
	return os.ReadFile(path)
}

func persistArtifact(path string, content []byte) error {
	if len(content) > maxArtifactBytes {
		return fmt.Errorf("subtitle artifact exceeds %d bytes", maxArtifactBytes)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".subtitle-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func extractWebVTT(ctx context.Context, sourcePath string, trackID uint64) ([]byte, error) {
	var output bytes.Buffer
	writer := &limitedWriter{buffer: &output, remaining: maxArtifactBytes}
	if err := matroska.ExtractSubtitleWebVTT(ctx, sourcePath, trackID, writer); err != nil {
		return nil, fmt.Errorf("extract MKV WebVTT subtitle: %w", err)
	}
	return output.Bytes(), nil
}

func extractASS(ctx context.Context, sourcePath string, trackID uint64, codec string) ([]byte, error) {
	container, err := matroska.OpenMeta(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open MKV metadata: %w", err)
	}
	var header []byte
	found := false
	for _, track := range container.Tracks {
		if track.ID == trackID && track.Type == matroska.SubtitleTrack {
			header = track.CodecPrivate
			found = true
			break
		}
	}
	if !found || (codec != "ass" && codec != "ssa") {
		return nil, fmt.Errorf("ASS subtitle track %d not found", trackID)
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := matroska.NewBlockReader(file, container.Info.TimecodeScale)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := &limitedWriter{buffer: &output, remaining: maxArtifactBytes}
	if len(header) > 0 {
		if _, err := writer.Write(append(bytes.TrimRight(header, "\r\n"), '\n')); err != nil {
			return nil, err
		}
	}
	var pending *assCue
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		block, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if block.TrackNumber != trackID {
			continue
		}
		text := strings.TrimRight(string(block.Data), "\x00")
		parts := strings.SplitN(text, ",", 3)
		if len(parts) < 3 {
			continue
		}
		if pending != nil {
			if pending.end <= pending.start {
				pending.end = block.Timecode
			}
			if err := writeASSCue(writer, *pending); err != nil {
				return nil, err
			}
		}
		end := block.Timecode
		if block.Duration > 0 {
			end += block.Duration
		}
		pending = &assCue{layer: parts[1], fields: parts[2], start: block.Timecode, end: end}
	}
	if pending != nil {
		if pending.end <= pending.start {
			pending.end = pending.start + defaultCueDuration
		}
		if err := writeASSCue(writer, *pending); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

type assCue struct {
	layer  string
	fields string
	start  int64
	end    int64
}

func writeASSCue(writer io.Writer, cue assCue) error {
	_, err := fmt.Fprintf(
		writer,
		"Dialogue: %s,%s,%s,%s\n",
		cue.layer,
		matroska.FormatASSTimestamp(cue.start),
		matroska.FormatASSTimestamp(cue.end),
		cue.fields,
	)
	return err
}

type limitedWriter struct {
	buffer    *bytes.Buffer
	remaining int
}

func (w *limitedWriter) Write(value []byte) (int, error) {
	if len(value) > w.remaining {
		return 0, fmt.Errorf("subtitle exceeds %d bytes", maxArtifactBytes)
	}
	n, err := w.buffer.Write(value)
	w.remaining -= n
	return n, err
}
