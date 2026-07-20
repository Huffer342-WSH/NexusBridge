package core

import "time"

// OrganizeTask 表示下载内容的媒体整理任务。
type OrganizeTask struct {
	ID             string    `json:"id"`
	DownloadTaskID string    `json:"download_task_id"`
	Status         string    `json:"status"`
	Title          string    `json:"title"`
	SourcePath     string    `json:"source_path"`
	RelativeDir    string    `json:"relative_dir,omitempty"`
	Filename       string    `json:"filename,omitempty"`
	TargetPath     string    `json:"target_path,omitempty"`
	LLMResponse    string    `json:"llm_response,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// QBSyncResult 汇总一次 qBittorrent 全量同步结果。
type QBSyncResult struct {
	Completed       int `json:"completed"`
	OrganizeCreated int `json:"organize_created"`
	Checked         int `json:"checked,omitempty"`
	Linked          int `json:"linked,omitempty"`
	Missing         int `json:"missing,omitempty"`
	TorrentMatched  int `json:"torrent_matched,omitempty"`
	TorrentUpdated  int `json:"torrent_updated,omitempty"`
	TorrentRemoved  int `json:"torrent_removed,omitempty"`
	DetailFailed    int `json:"detail_failed,omitempty"`
}

// QBTorrentUpdate 表示增量同步后需要更新的单个本地种子状态。
type QBTorrentUpdate struct {
	SiteID    string          `json:"site_id"`
	TorrentID string          `json:"torrent_id"`
	QBStatus  QBTorrentStatus `json:"qb_status"`
}

// QBPollResult 表示一次 qB sync/maindata 增量同步结果。
type QBPollResult struct {
	RID        int               `json:"rid"`
	FullUpdate bool              `json:"full_update"`
	Connected  bool              `json:"connected"`
	Updated    int               `json:"updated"`
	Removed    int               `json:"removed"`
	Updates    []QBTorrentUpdate `json:"updates"`
}

// OrganizeResult 汇总一次待整理任务处理结果。
type OrganizeResult struct {
	Processed int  `json:"processed"`
	Failed    int  `json:"failed"`
	DryRun    bool `json:"dry_run"`
}
