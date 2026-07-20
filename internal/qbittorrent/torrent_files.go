package qbittorrent

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) GetTorrentContents(ctx context.Context, hash string, indexes []int) ([]TorrentContent, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, fmt.Errorf("torrent hash is required")
	}
	values := url.Values{"hash": {hash}}
	if len(indexes) > 0 {
		parts := make([]string, 0, len(indexes))
		for _, index := range indexes {
			parts = append(parts, strconv.Itoa(index))
		}
		values.Set("indexes", strings.Join(parts, "|"))
	}
	var contents []TorrentContent
	if err := c.getJSON(ctx, "/api/v2/torrents/files", values, "torrents/files", &contents); err != nil {
		return nil, err
	}
	return contents, nil
}

// RenameTorrentFile 修改指定种子中单个文件的相对路径或文件名。
func (c *Client) RenameTorrentFile(ctx context.Context, hash, oldPath, newPath string) error {
	hash = strings.TrimSpace(hash)
	oldPath = strings.TrimSpace(oldPath)
	newPath = strings.TrimSpace(newPath)
	if hash == "" {
		return fmt.Errorf("torrent hash is required")
	}
	if oldPath == "" {
		return fmt.Errorf("old torrent file path is required")
	}
	if newPath == "" {
		return fmt.Errorf("new torrent file path is required")
	}
	values := url.Values{"hash": {hash}, "oldPath": {oldPath}, "newPath": {newPath}}
	return c.postForm(ctx, "/api/v2/torrents/renameFile", values, "torrents/renameFile")
}

// RenameTorrentFolder 修改指定种子中目录的相对路径或目录名。
func (c *Client) RenameTorrentFolder(ctx context.Context, hash, oldPath, newPath string) error {
	hash = strings.TrimSpace(hash)
	oldPath = strings.TrimSpace(oldPath)
	newPath = strings.TrimSpace(newPath)
	if hash == "" {
		return fmt.Errorf("torrent hash is required")
	}
	if oldPath == "" {
		return fmt.Errorf("old torrent folder path is required")
	}
	if newPath == "" {
		return fmt.Errorf("new torrent folder path is required")
	}
	values := url.Values{"hash": {hash}, "oldPath": {oldPath}, "newPath": {newPath}}
	return c.postForm(ctx, "/api/v2/torrents/renameFolder", values, "torrents/renameFolder")
}

// SetTorrentCategory 设置一个或多个种子的分类。
