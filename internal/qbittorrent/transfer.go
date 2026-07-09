package qbittorrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	infohash_v2 "github.com/anacrolix/torrent/types/infohash-v2"
)

const (
	verifiedAddAttempts = 10
	verifiedAddInterval = 500 * time.Millisecond
)

// TorrentHashes 保存 torrent 元信息中可用的 v1 和 v2 info hash。
type TorrentHashes struct {
	V1 string
	V2 string
}

// TorrentMetadata 保存 torrent 的 info hash 和原始 info.name。
type TorrentMetadata struct {
	Hashes      TorrentHashes
	Name        string
	Files       []TorrentFileMetadata
	IsDirectory bool
}

// TorrentFileMetadata 保存 torrent 中用于目录匹配的相对路径和大小。
type TorrentFileMetadata struct {
	Path    string
	Size    int64
	Padding bool
}

// VerifiedAddResult 表示经 hash 验证的添加结果。
type VerifiedAddResult struct {
	Torrent TorrentInfo
	Added   bool
}

// ParseTorrentMetadata 解析 torrent 的 v1/v2 info hash 和原始 info.name。
func ParseTorrentMetadata(data []byte) (TorrentMetadata, error) {
	if len(data) == 0 {
		return TorrentMetadata{}, fmt.Errorf("torrent data is required")
	}
	meta, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return TorrentMetadata{}, fmt.Errorf("parse torrent metainfo: %w", err)
	}
	if len(meta.InfoBytes) == 0 {
		return TorrentMetadata{}, fmt.Errorf("torrent info dictionary is required")
	}
	info, err := meta.UnmarshalInfo()
	if err != nil {
		return TorrentMetadata{}, fmt.Errorf("parse torrent info dictionary: %w", err)
	}
	if strings.TrimSpace(info.BestName()) == "" || info.PieceLength <= 0 {
		return TorrentMetadata{}, fmt.Errorf("torrent info dictionary is incomplete")
	}
	hashes := TorrentHashes{}
	if info.HasV1() {
		hashes.V1 = strings.ToLower(meta.HashInfoBytes().HexString())
	}
	if info.HasV2() {
		v2 := infohash_v2.HashBytes(meta.InfoBytes)
		hashes.V2 = strings.ToLower(v2.HexString())
	}
	if hashes.V1 == "" && hashes.V2 == "" {
		return TorrentMetadata{}, fmt.Errorf("torrent info dictionary has no supported v1 or v2 metadata")
	}
	files := make([]TorrentFileMetadata, 0)
	for _, file := range info.UpvertedFiles() {
		path := strings.Join(file.BestPath(), "/")
		if !info.IsDir() {
			path = info.BestName()
		}
		files = append(files, TorrentFileMetadata{
			Path: path, Size: file.Length, Padding: strings.Contains(file.Attr, "p"),
		})
	}
	return TorrentMetadata{
		Hashes: hashes, Name: info.BestName(), Files: files, IsDirectory: info.IsDir(),
	}, nil
}

// ParseTorrentHashes 从 torrent 原始 info 字典字节计算 v1 和 v2 info hash。
func ParseTorrentHashes(data []byte) (TorrentHashes, error) {
	metadata, err := ParseTorrentMetadata(data)
	if err != nil {
		return TorrentHashes{}, err
	}
	return metadata.Hashes, nil
}

// AddTorrentFileVerified 上传 torrent 文件并按本地计算的 v1 hash 查询验证。
func (c *Client) AddTorrentFileVerified(ctx context.Context, opts AddOptions) (TorrentInfo, error) {
	result, err := c.addTorrentFileVerified(ctx, opts, false)
	if err != nil {
		return TorrentInfo{}, err
	}
	return result.Torrent, nil
}

// AddTorrentFileVerifiedResult 上传 torrent 文件，并区分本次新增与已存在任务。
func (c *Client) AddTorrentFileVerifiedResult(ctx context.Context, opts AddOptions) (VerifiedAddResult, error) {
	return c.addTorrentFileVerified(ctx, opts, true)
}

func (c *Client) addTorrentFileVerified(ctx context.Context, opts AddOptions, checkExisting bool) (VerifiedAddResult, error) {
	metadata, err := ParseTorrentMetadata(opts.Data)
	if err != nil {
		return VerifiedAddResult{}, err
	}
	if metadata.Hashes.V1 == "" {
		return VerifiedAddResult{}, fmt.Errorf("pure v2 torrent verification is not supported")
	}
	if checkExisting {
		existing, found, err := c.FindTorrentByHash(ctx, metadata.Hashes.V1)
		if err != nil {
			return VerifiedAddResult{}, fmt.Errorf("query existing torrent %s: %w", metadata.Hashes.V1, err)
		}
		if found {
			return VerifiedAddResult{Torrent: existing, Added: false}, nil
		}
	}
	result, err := c.addTorrentFileWithResult(ctx, opts)
	if err != nil && !errors.Is(err, ErrTorrentConflict) {
		return VerifiedAddResult{}, err
	}
	if err == nil && len(result.AddedTorrentIDs) > 0 {
		for _, id := range result.AddedTorrentIDs {
			if strings.EqualFold(id, metadata.Hashes.V1) {
				return VerifiedAddResult{Torrent: TorrentInfo{Hash: metadata.Hashes.V1}, Added: true}, nil
			}
		}
		return VerifiedAddResult{}, fmt.Errorf("qbittorrent added torrent ids do not contain local hash %s", metadata.Hashes.V1)
	}

	torrent, waitErr := c.waitTorrentByHash(ctx, metadata.Hashes.V1)
	if waitErr != nil {
		return VerifiedAddResult{}, waitErr
	}
	return VerifiedAddResult{Torrent: torrent, Added: err == nil}, nil
}

func (c *Client) waitTorrentByHash(ctx context.Context, hash string) (TorrentInfo, error) {
	var lastQueryErr error
	for attempt := 0; attempt < verifiedAddAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(verifiedAddInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return TorrentInfo{}, ctx.Err()
			case <-timer.C:
			}
		}
		torrent, found, err := c.FindTorrentByHash(ctx, hash)
		if err != nil {
			lastQueryErr = err
			continue
		}
		if found {
			return torrent, nil
		}
	}
	if lastQueryErr != nil {
		return TorrentInfo{}, fmt.Errorf("verify added torrent %s timed out: %w", hash, lastQueryErr)
	}
	return TorrentInfo{}, fmt.Errorf("verify added torrent %s timed out", hash)
}
