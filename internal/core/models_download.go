package core

import "time"

// BatchDownloadRequest 描述手动批量下载的筛选范围和下载配置。
type BatchDownloadRequest struct {
	Torrents       []TorrentKey     `json:"torrents"`
	SubscriptionID string           `json:"subscription_id,omitempty"`
	Options        *DownloadOptions `json:"options,omitempty"`
}

// BatchDownloadPreview 是手动批量下载的只读计划。
type BatchDownloadPreview struct {
	Items []SubscriptionPreviewItem `json:"items"`
}

// BatchDownloadResult 汇总一次手动批量执行。
type BatchDownloadResult struct {
	Attempted int            `json:"attempted"`
	Sent      int            `json:"sent"`
	Exists    int            `json:"exists"`
	Failed    int            `json:"failed"`
	Skipped   int            `json:"skipped"`
	Tasks     []DownloadTask `json:"tasks"`
}

// DownloadTask 表示一次种子发送到 qBittorrent 的持久化任务。
type DownloadTask struct {
	ID             string     `json:"id"`
	SiteID         string     `json:"site_id"`
	TorrentID      string     `json:"torrent_id"`
	RuleName       string     `json:"rule_name"`
	SubscriptionID string     `json:"subscription_id,omitempty"`
	Trigger        string     `json:"trigger,omitempty"`
	Status         string     `json:"status"`
	TorrentTitle   string     `json:"torrent_title"`
	DownloadURL    string     `json:"download_url"`
	QBHash         string     `json:"qb_hash,omitempty"`
	Error          string     `json:"error,omitempty"`
	ContentPath    string     `json:"content_path,omitempty"`
	Category       string     `json:"category,omitempty"`
	SavePath       string     `json:"save_path,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	Rename         string     `json:"rename,omitempty"`
	Paused         bool       `json:"paused"`
	ReasonCode     string     `json:"reason_code,omitempty"`
	AttemptCount   int        `json:"attempt_count"`
	RetryCount     int        `json:"retry_count"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// DownloadPreview 表示单个种子发送前的标题格式化结果。
type DownloadPreview struct {
	SiteID         string `json:"site_id"`
	TorrentID      string `json:"torrent_id"`
	OriginalTitle  string `json:"original_title"`
	FormattedTitle string `json:"formatted_title"`
	DownloadURL    string `json:"download_url"`
	LLMResponse    string `json:"llm_response,omitempty"`
}

// ManualDownloadRequest 描述单个种子的手动下载请求。
type ManualDownloadRequest struct {
	SiteID         string `json:"site_id"`
	TorrentID      string `json:"torrent_id"`
	FormattedTitle string `json:"formatted_title,omitempty"`
}

// RecoveryPreviewRequest 描述一次保留目录匹配预览。
