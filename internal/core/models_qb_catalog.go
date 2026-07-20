package core

import "time"

// QBCategory 表示 qBittorrent 分类及其有效保存路径。
type QBCategory struct {
	Name         string    `json:"name"`
	SavePath     string    `json:"save_path"`
	PathSegments []string  `json:"path_segments"`
	SyncedAt     time.Time `json:"synced_at,omitempty"`
}

// QBCategoriesResult 返回 qB 分类快照和连接状态。
type QBCategoriesResult struct {
	Items     []QBCategory `json:"items"`
	Stale     bool         `json:"stale"`
	Connected bool         `json:"connected"`
	Error     string       `json:"error,omitempty"`
	SyncedAt  *time.Time   `json:"synced_at,omitempty"`
}

// QBTagsResult 返回 qB 标签快照和连接状态。
type QBTagsResult struct {
	Items     []string   `json:"items"`
	Stale     bool       `json:"stale"`
	Connected bool       `json:"connected"`
	Error     string     `json:"error,omitempty"`
	SyncedAt  *time.Time `json:"synced_at,omitempty"`
}

// BatchDownloadRequest 描述手动批量下载的配置来源。
