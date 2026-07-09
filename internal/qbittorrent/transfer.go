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

// ParseTorrentHashes 从 torrent 原始 info 字典字节计算 v1 和 v2 info hash。
func ParseTorrentHashes(data []byte) (TorrentHashes, error) {
	if len(data) == 0 {
		return TorrentHashes{}, fmt.Errorf("torrent data is required")
	}
	meta, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return TorrentHashes{}, fmt.Errorf("parse torrent metainfo: %w", err)
	}
	if len(meta.InfoBytes) == 0 {
		return TorrentHashes{}, fmt.Errorf("torrent info dictionary is required")
	}
	info, err := meta.UnmarshalInfo()
	if err != nil {
		return TorrentHashes{}, fmt.Errorf("parse torrent info dictionary: %w", err)
	}
	if strings.TrimSpace(info.BestName()) == "" || info.PieceLength <= 0 {
		return TorrentHashes{}, fmt.Errorf("torrent info dictionary is incomplete")
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
		return TorrentHashes{}, fmt.Errorf("torrent info dictionary has no supported v1 or v2 metadata")
	}
	return hashes, nil
}

// AddTorrentFileVerified 上传 torrent 文件并按本地计算的 v1 hash 查询验证。
func (c *Client) AddTorrentFileVerified(ctx context.Context, opts AddOptions) (TorrentInfo, error) {
	hashes, err := ParseTorrentHashes(opts.Data)
	if err != nil {
		return TorrentInfo{}, err
	}
	if hashes.V1 == "" {
		return TorrentInfo{}, fmt.Errorf("pure v2 torrent verification is not supported")
	}
	result, err := c.addTorrentFileWithResult(ctx, opts)
	if err != nil && !errors.Is(err, ErrTorrentConflict) {
		return TorrentInfo{}, err
	}
	if err == nil && len(result.AddedTorrentIDs) > 0 {
		for _, id := range result.AddedTorrentIDs {
			if strings.EqualFold(id, hashes.V1) {
				return TorrentInfo{Hash: hashes.V1}, nil
			}
		}
		return TorrentInfo{}, fmt.Errorf("qbittorrent added torrent ids do not contain local hash %s", hashes.V1)
	}

	return c.waitTorrentByHash(ctx, hashes.V1)
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
		torrents, err := c.ListTorrentsWithOptions(ctx, TorrentListOptions{Hashes: []string{hash}})
		if err != nil {
			lastQueryErr = err
			continue
		}
		for _, torrent := range torrents {
			if strings.EqualFold(torrent.Hash, hash) {
				return torrent, nil
			}
		}
	}
	if lastQueryErr != nil {
		return TorrentInfo{}, fmt.Errorf("verify added torrent %s timed out: %w", hash, lastQueryErr)
	}
	return TorrentInfo{}, fmt.Errorf("verify added torrent %s timed out", hash)
}
