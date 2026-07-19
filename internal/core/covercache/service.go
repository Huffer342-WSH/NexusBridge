// Package covercache 提供种子封面的本地持久缓存。
package covercache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/storage"
)

const (
	currentAttemptVersion = 2

	// StatusPending 表示缓存正在准备或上次准备被进程中断。
	StatusPending = "pending"
	// StatusReady 表示缓存文件和元数据均已保存。
	StatusReady = "ready"
	// StatusFailed 表示当前源地址下载失败且不会自动重试。
	StatusFailed = "failed"
)

// Source 描述一次封面获取所需的种子和远程资源信息。
type Source struct {
	SiteID    string
	TorrentID string
	SourceURL string
}

// Download 表示已完成校验的远程封面响应。
type Download struct {
	Data     []byte
	MIMEType string
}

// Downloader 使用站点凭据下载并校验远程封面。
type Downloader interface {
	DownloadCover(ctx context.Context, source Source) (Download, error)
}

// Repository 提供封面缓存状态的持久化操作。
type Repository interface {
	GetCoverCache(ctx context.Context, siteID, torrentID string) (storage.CoverCacheRecord, bool, error)
	UpsertCoverCache(ctx context.Context, record storage.CoverCacheRecord) error
}

// Result 表示可由 HTTP 层直接读取的本地封面文件。
type Result struct {
	Path      string
	MIMEType  string
	SHA256    string
	FileSize  int64
	ModTime   time.Time
	FromCache bool
}

// Service 协调封面状态、远程下载和本地文件写入。
type Service struct {
	repository Repository
	downloader Downloader
	root       string
	locks      sync.Map
}

// New 创建封面缓存服务并准备缓存根目录。
func New(repository Repository, downloader Downloader, root string) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("cover cache repository is required")
	}
	if downloader == nil {
		return nil, fmt.Errorf("cover downloader is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("cover cache root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve cover cache root: %w", err)
	}
	absoluteRoot = filepath.Clean(absoluteRoot)
	if err := os.MkdirAll(absoluteRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create cover cache root: %w", err)
	}
	return &Service{repository: repository, downloader: downloader, root: absoluteRoot}, nil
}

// GetCover 返回有效的本地封面；缓存未命中时同步下载并持久化。
func (s *Service) GetCover(ctx context.Context, source Source) (Result, error) {
	source.SiteID = strings.TrimSpace(source.SiteID)
	source.TorrentID = strings.TrimSpace(source.TorrentID)
	source.SourceURL = strings.TrimSpace(source.SourceURL)
	if source.SiteID == "" || source.TorrentID == "" {
		return Result{}, fmt.Errorf("cover site_id and torrent_id are required")
	}
	if source.SourceURL == "" {
		return Result{}, fmt.Errorf("torrent cover url is empty")
	}

	lockValue, _ := s.locks.LoadOrStore(source.SiteID+"\x00"+source.TorrentID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	record, exists, err := s.repository.GetCoverCache(ctx, source.SiteID, source.TorrentID)
	if err != nil {
		return Result{}, fmt.Errorf("load cover cache: %w", err)
	}
	previous, hasPrevious := s.cachedResult(record)
	sameSource := exists && record.SourceURL == source.SourceURL
	if sameSource && record.Status == StatusReady && hasPrevious {
		return previous, nil
	}
	if sameSource && record.Status == StatusFailed && record.AttemptVersion >= currentAttemptVersion {
		if hasPrevious {
			return previous, nil
		}
		return Result{}, previousFailure(record)
	}

	pending := record
	if !exists {
		pending = storage.CoverCacheRecord{SiteID: source.SiteID, TorrentID: source.TorrentID}
	}
	if !sameSource {
		pending.FailCount = 0
	}
	pending.SiteID = source.SiteID
	pending.TorrentID = source.TorrentID
	pending.SourceURL = source.SourceURL
	pending.Status = StatusPending
	pending.LastError = ""
	pending.AttemptVersion = currentAttemptVersion
	if err := s.repository.UpsertCoverCache(ctx, pending); err != nil {
		return Result{}, fmt.Errorf("mark cover cache pending: %w", err)
	}

	download, err := s.downloader.DownloadCover(ctx, source)
	if err == nil {
		err = validateDownload(download)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return Result{}, err
		}
		failed := pending
		failed.Status = StatusFailed
		failed.LastError = err.Error()
		failed.LastFailedAt = time.Now().UTC()
		failed.FailCount++
		if saveErr := s.repository.UpsertCoverCache(ctx, failed); saveErr != nil {
			return Result{}, errors.Join(err, fmt.Errorf("save cover failure: %w", saveErr))
		}
		if hasPrevious {
			return previous, nil
		}
		return Result{}, err
	}

	relativePath := cacheRelativePath(source)
	absolutePath, err := s.resolvePath(relativePath)
	if err != nil {
		return Result{}, err
	}
	if err := writeFileAtomically(absolutePath, download.Data); err != nil {
		return Result{}, fmt.Errorf("write cover cache: %w", err)
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		_ = os.Remove(absolutePath)
		return Result{}, fmt.Errorf("inspect saved cover cache: %w", err)
	}
	contentHash := sha256.Sum256(download.Data)
	now := time.Now().UTC()
	ready := storage.CoverCacheRecord{
		SiteID:         source.SiteID,
		TorrentID:      source.TorrentID,
		SourceURL:      source.SourceURL,
		LocalPath:      filepath.ToSlash(relativePath),
		MIMEType:       download.MIMEType,
		FileSize:       int64(len(download.Data)),
		SHA256:         hex.EncodeToString(contentHash[:]),
		Status:         StatusReady,
		AttemptVersion: currentAttemptVersion,
		LastSuccessAt:  now,
		LastFailedAt:   pending.LastFailedAt,
	}
	if err := s.repository.UpsertCoverCache(ctx, ready); err != nil {
		_ = os.Remove(absolutePath)
		return Result{}, fmt.Errorf("save cover cache metadata: %w", err)
	}
	return Result{
		Path: absolutePath, MIMEType: ready.MIMEType, SHA256: ready.SHA256,
		FileSize: ready.FileSize, ModTime: info.ModTime(), FromCache: false,
	}, nil
}

func (s *Service) cachedResult(record storage.CoverCacheRecord) (Result, bool) {
	if strings.TrimSpace(record.LocalPath) == "" || record.FileSize <= 0 ||
		strings.TrimSpace(record.SHA256) == "" || !strings.HasPrefix(strings.ToLower(record.MIMEType), "image/") {
		return Result{}, false
	}
	path, err := s.resolvePath(record.LocalPath)
	if err != nil {
		return Result{}, false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != record.FileSize {
		return Result{}, false
	}
	return Result{
		Path: path, MIMEType: record.MIMEType, SHA256: record.SHA256,
		FileSize: record.FileSize, ModTime: info.ModTime(), FromCache: true,
	}, true
}

func (s *Service) resolvePath(relativePath string) (string, error) {
	relativePath = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relativePath)))
	if relativePath == "." || filepath.IsAbs(relativePath) || filepath.VolumeName(relativePath) != "" {
		return "", fmt.Errorf("invalid cover cache path")
	}
	absolutePath := filepath.Join(s.root, relativePath)
	relativeToRoot, err := filepath.Rel(s.root, absolutePath)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cover cache path escapes root")
	}
	return absolutePath, nil
}

func cacheRelativePath(source Source) string {
	keyHash := sha256.Sum256([]byte(source.SiteID + "\x00" + source.TorrentID + "\x00" + source.SourceURL))
	name := hex.EncodeToString(keyHash[:])
	return filepath.Join(name[:2], name+".img")
}

func validateDownload(download Download) error {
	if len(download.Data) == 0 {
		return fmt.Errorf("cover image is empty")
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(download.MIMEType)), "image/") {
		return fmt.Errorf("unexpected cover content type: %s", download.MIMEType)
	}
	return nil
}

func previousFailure(record storage.CoverCacheRecord) error {
	if strings.TrimSpace(record.LastError) == "" {
		return fmt.Errorf("cover download previously failed")
	}
	return fmt.Errorf("cover download previously failed: %s", record.LastError)
}

func writeFileAtomically(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".cover-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	renameErr := os.Rename(temporaryPath, path)
	if renameErr == nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return renameErr
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}
