package qbittorrent

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) DeleteTorrents(ctx context.Context, hashes []string, deleteFiles bool) error {
	values := url.Values{"hashes": {joinHashes(hashes)}, "deleteFiles": {strconv.FormatBool(deleteFiles)}}
	return c.postForm(ctx, "/api/v2/torrents/delete", values, "torrents/delete")
}

// StopTorrents 暂停一个或多个 qBittorrent 种子任务。
func (c *Client) StopTorrents(ctx context.Context, hashes []string) error {
	return c.postForm(ctx, "/api/v2/torrents/stop", url.Values{"hashes": {joinHashes(hashes)}}, "torrents/stop")
}

// StartTorrents 恢复一个或多个 qBittorrent 种子任务。
func (c *Client) StartTorrents(ctx context.Context, hashes []string) error {
	return c.postForm(ctx, "/api/v2/torrents/start", url.Values{"hashes": {joinHashes(hashes)}}, "torrents/start")
}

// RecheckTorrents 强制重新校验一个或多个 qBittorrent 种子任务。
func (c *Client) RecheckTorrents(ctx context.Context, hashes []string) error {
	return c.postForm(ctx, "/api/v2/torrents/recheck", url.Values{"hashes": {joinHashes(hashes)}}, "torrents/recheck")
}

func torrentTagsForm(hashes, tags []string) url.Values {
	return url.Values{"hashes": {joinHashes(hashes)}, "tags": {strings.Join(nonEmptyStrings(tags), ",")}}
}
