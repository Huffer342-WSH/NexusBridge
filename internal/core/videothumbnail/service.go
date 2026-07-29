// Package videothumbnail 提供基于 FFprobe 和 FFmpeg 的按需视频缩略图缓存。
package videothumbnail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	generationVersion = "jpeg-320x180-v1"
	generationTimeout = 30 * time.Second
)

// Result 描述已经生成并可读取的 JPEG 缩略图。
type Result struct {
	Path    string
	ETag    string
	ModTime time.Time
}

// Service 串行处理同一视频，并限制全局 FFmpeg 并发。
type Service struct {
	root       string
	ffmpegPath string
	locks      sync.Map
	slots      chan struct{}
}

// New 创建缩略图服务；FFmpeg 不可用不会阻止服务初始化。
func New(root, ffmpegPath string) (*Service, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, fmt.Errorf("resolve thumbnail cache root: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create thumbnail cache root: %w", err)
	}
	return &Service{
		root:       absolute,
		ffmpegPath: strings.TrimSpace(ffmpegPath),
		slots:      make(chan struct{}, 2),
	}, nil
}

// Get 返回命中的缓存，或同步生成源视频当前版本的缩略图。
func (s *Service) Get(ctx context.Context, sourcePath string) (Result, error) {
	resolved, info, identity, err := inspectSource(sourcePath)
	if err != nil {
		return Result{}, err
	}
	sourceKey := digest(identity)
	lockValue, _ := s.locks.LoadOrStore(sourceKey, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	resolved, info, identity, err = inspectSource(resolved)
	if err != nil {
		return Result{}, err
	}
	sourceKey = digest(identity)
	fingerprint := digest(strings.Join([]string{
		identity,
		strconv.FormatInt(info.Size(), 10),
		strconv.FormatInt(info.ModTime().UnixNano(), 10),
		generationVersion,
	}, "\x00"))
	cacheDir := filepath.Join(s.root, sourceKey[:2], sourceKey)
	cachePath := filepath.Join(cacheDir, fingerprint+".jpg")
	if cached, ok := cachedResult(cachePath, fingerprint); ok {
		return cached, nil
	}

	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}

	runCtx, cancel := context.WithTimeout(ctx, generationTimeout)
	defer cancel()
	ffmpeg, ffprobe, err := s.executables()
	if err != nil {
		return Result{}, err
	}
	seek := 5.0
	if ffprobe != "" {
		if duration, probeErr := probeDuration(runCtx, ffprobe, resolved); probeErr == nil &&
			duration > 0 && !math.IsNaN(duration) && !math.IsInf(duration, 0) {
			seek = duration * 0.1
		}
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create thumbnail cache directory: %w", err)
	}
	temporary, err := os.CreateTemp(cacheDir, ".thumbnail-*.jpg")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary thumbnail: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return Result{}, fmt.Errorf("close temporary thumbnail: %w", err)
	}
	defer os.Remove(temporaryPath)

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", strconv.FormatFloat(seek, 'f', 3, 64),
		"-i", resolved,
		"-map", "0:v:0", "-frames:v", "1",
		"-vf", "scale=320:180:force_original_aspect_ratio=decrease",
		"-q:v", "4", "-f", "image2", "-y", temporaryPath,
	}
	output, err := exec.CommandContext(runCtx, ffmpeg, args...).CombinedOutput()
	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return Result{}, fmt.Errorf("generate video thumbnail: timeout after %s", generationTimeout)
		}
		return Result{}, fmt.Errorf("generate video thumbnail: %w: %s", err, compactCommandOutput(output))
	}
	generated, err := os.Stat(temporaryPath)
	if err != nil || !generated.Mode().IsRegular() || generated.Size() == 0 {
		if err == nil {
			err = errors.New("FFmpeg produced an empty file")
		}
		return Result{}, fmt.Errorf("inspect generated video thumbnail: %w", err)
	}
	_, currentInfo, currentIdentity, err := inspectSource(resolved)
	if err != nil {
		return Result{}, err
	}
	if currentIdentity != identity ||
		currentInfo.Size() != info.Size() ||
		!currentInfo.ModTime().Equal(info.ModTime()) {
		return Result{}, errors.New("video source changed while generating thumbnail")
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return Result{}, fmt.Errorf("set video thumbnail permissions: %w", err)
	}
	if err := syncFile(temporaryPath); err != nil {
		return Result{}, err
	}
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("replace video thumbnail cache: %w", err)
	}
	if err := os.Rename(temporaryPath, cachePath); err != nil {
		return Result{}, fmt.Errorf("commit video thumbnail cache: %w", err)
	}
	cleanupOldVersions(cacheDir, cachePath)
	result, ok := cachedResult(cachePath, fingerprint)
	if !ok {
		return Result{}, errors.New("video thumbnail cache is unavailable after generation")
	}
	return result, nil
}

func inspectSource(sourcePath string) (string, os.FileInfo, string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(sourcePath))
	if err != nil {
		return "", nil, "", fmt.Errorf("resolve video source: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", nil, "", fmt.Errorf("resolve video source links: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, "", fmt.Errorf("inspect video source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, "", errors.New("video source is not a regular file")
	}
	identity := filepath.Clean(resolved)
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	return resolved, info, identity, nil
}

func (s *Service) executables() (string, string, error) {
	ffmpeg := s.ffmpegPath
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil {
			return "", "", errors.New("FFmpeg executable was not found")
		}
	} else if resolved, err := exec.LookPath(ffmpeg); err == nil {
		ffmpeg = resolved
	}

	ffprobeName := "ffprobe"
	if strings.EqualFold(filepath.Ext(ffmpeg), ".exe") {
		ffprobeName += ".exe"
	}
	sibling := filepath.Join(filepath.Dir(ffmpeg), ffprobeName)
	if info, err := os.Stat(sibling); err == nil && info.Mode().IsRegular() {
		return ffmpeg, sibling, nil
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return ffmpeg, "", nil
	}
	return ffmpeg, ffprobe, nil
}

func probeDuration(ctx context.Context, ffprobe, sourcePath string) (float64, error) {
	output, err := exec.CommandContext(
		ctx,
		ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		sourcePath,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
}

func cachedResult(path, etag string) (Result, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return Result{}, false
	}
	return Result{Path: path, ETag: etag, ModTime: info.ModTime()}, true
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open generated video thumbnail: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync generated video thumbnail: %w", err)
	}
	return nil
}

func cleanupOldVersions(cacheDir, currentPath string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(cacheDir, entry.Name())
		if entry.IsDir() || samePath(path, currentPath) || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jpg") {
			continue
		}
		_ = os.Remove(path)
	}
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func compactCommandOutput(output []byte) string {
	value := strings.TrimSpace(string(output))
	if len(value) > 1024 {
		value = value[:1024] + "..."
	}
	if value == "" {
		return "no FFmpeg diagnostics"
	}
	return value
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
