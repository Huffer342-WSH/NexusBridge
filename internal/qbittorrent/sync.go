// sync.go 封装 qBittorrent 增量同步原生 API。
package qbittorrent

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// MainData 表示 sync/maindata 返回的增量任务数据。
type MainData struct {
	RID             int                        `json:"rid"`
	FullUpdate      bool                       `json:"full_update"`
	Torrents        map[string]json.RawMessage `json:"torrents"`
	TorrentsRemoved []string                   `json:"torrents_removed"`
}

// GetMainData 按响应 ID 获取 qBittorrent 主数据增量。
func (c *Client) GetMainData(ctx context.Context, rid int) (MainData, error) {
	values := url.Values{}
	if rid > 0 {
		values.Set("rid", strconv.Itoa(rid))
	}
	var result MainData
	if err := c.getJSON(ctx, "/api/v2/sync/maindata", values, "sync/maindata", &result); err != nil {
		return MainData{}, err
	}
	if result.Torrents == nil {
		result.Torrents = map[string]json.RawMessage{}
	}
	return result, nil
}
