// Package core 定义 NexusBridge 的领域模型和共享应用服务。
package core

import "time"

type Site struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	UserAgent string `json:"user_agent,omitempty"`
	HasCookie bool   `json:"has_cookie"`
}

type SiteCredential struct {
	SiteID    string `json:"site_id"`
	BaseURL   string `json:"base_url"`
	UserAgent string `json:"user_agent"`
	Cookie    string `json:"cookie,omitempty"`
	HasCookie bool   `json:"has_cookie"`
}

type Torrent struct {
	ID                string           `json:"id"`
	SiteID            string           `json:"site_id"`
	Category          string           `json:"category,omitempty"`
	CategoryQuery     string           `json:"category_query,omitempty"`
	Title             string           `json:"title"`
	DetailURL         string           `json:"detail_url"`
	DownloadURL       string           `json:"download_url,omitempty"`
	CoverURL          string           `json:"cover_url,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	TagIDs            []string         `json:"tag_ids,omitempty"`
	Description       string           `json:"description,omitempty"`
	DetailTitle       string           `json:"detail_title,omitempty"`
	Subtitle          string           `json:"subtitle,omitempty"`
	ProductURL        string           `json:"product_url,omitempty"`
	DetailInfoHash    string           `json:"detail_info_hash,omitempty"`
	DetailDescription string           `json:"detail_description,omitempty"`
	DetailRawText     string           `json:"detail_raw_text,omitempty"`
	DetailFetchedAt   string           `json:"detail_fetched_at,omitempty"`
	SizeBytes         int64            `json:"size_bytes,omitempty"`
	Seeders           int              `json:"seeders,omitempty"`
	Leechers          int              `json:"leechers,omitempty"`
	Snatches          int              `json:"snatches,omitempty"`
	Comments          int              `json:"comments,omitempty"`
	Promotion         string           `json:"promotion,omitempty"`
	PromotionEndsAt   string           `json:"promotion_ends_at,omitempty"`
	PublishedAt       *time.Time       `json:"published_at,omitempty"`
	PublishedText     string           `json:"published_text,omitempty"`
	FirstSeenAt       time.Time        `json:"first_seen_at"`
	LastSeenAt        time.Time        `json:"last_seen_at"`
	TorrentFileSaved  bool             `json:"torrent_file_saved"`
	InfoHashV1        string           `json:"info_hash_v1,omitempty"`
	InfoHashV2        string           `json:"info_hash_v2,omitempty"`
	TorrentFileError  string           `json:"torrent_file_error,omitempty"`
	QBStatus          *QBTorrentStatus `json:"qb_status,omitempty"`
}

type TorrentQuery struct {
	SiteID      string
	Search      string
	Limit       int
	Offset      int
	IncludeQB   bool
	QBWeakMatch bool
}

type QBTorrentStatus struct {
	Available     bool      `json:"available"`
	Added         bool      `json:"added"`
	Source        string    `json:"source"`
	Error         string    `json:"error,omitempty"`
	FetchedAt     time.Time `json:"fetched_at"`
	TaskID        string    `json:"task_id,omitempty"`
	TaskStatus    string    `json:"task_status,omitempty"`
	RuleID        string    `json:"rule_id,omitempty"`
	Hash          string    `json:"hash,omitempty"`
	Name          string    `json:"name,omitempty"`
	State         string    `json:"state,omitempty"`
	Progress      float64   `json:"progress,omitempty"`
	Category      string    `json:"category,omitempty"`
	Tags          string    `json:"tags,omitempty"`
	SavePath      string    `json:"save_path,omitempty"`
	ContentPath   string    `json:"content_path,omitempty"`
	DownloadSpeed int64     `json:"download_speed,omitempty"`
	UploadSpeed   int64     `json:"upload_speed,omitempty"`
	ETA           int64     `json:"eta,omitempty"`
	Ratio         float64   `json:"ratio,omitempty"`
	Size          int64     `json:"size,omitempty"`
	Completed     int64     `json:"completed,omitempty"`
	AmountLeft    int64     `json:"amount_left,omitempty"`
}

type FetchResult struct {
	SiteID       string `json:"site_id"`
	Status       string `json:"status"`
	Fetched      int    `json:"fetched"`
	Changed      int    `json:"changed"`
	Matched      int    `json:"matched,omitempty"`
	DownloadSent int    `json:"download_sent,omitempty"`
	FilesSaved   int    `json:"torrent_files_saved,omitempty"`
	FilesFailed  int    `json:"torrent_files_failed,omitempty"`
}

type Rule struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	SiteIDs    []string `json:"site_ids"`
	Categories []string `json:"categories"`
	Tags       []string `json:"tags"`
	Include    string   `json:"include"`
	Exclude    string   `json:"exclude"`
	Promotion  string   `json:"promotion"`
	MinSize    int64    `json:"min_size"`
	MaxSize    int64    `json:"max_size"`
	MinSeeders int      `json:"min_seeders"`
	Action     string   `json:"action"`
}

type RuleMatch struct {
	Rule    Rule    `json:"rule"`
	Torrent Torrent `json:"torrent"`
	Matched bool    `json:"matched"`
	Reason  string  `json:"reason"`
}

type DownloadTask struct {
	ID           string    `json:"id"`
	SiteID       string    `json:"site_id"`
	TorrentID    string    `json:"torrent_id"`
	RuleID       string    `json:"rule_id"`
	Status       string    `json:"status"`
	TorrentTitle string    `json:"torrent_title"`
	DownloadURL  string    `json:"download_url"`
	QBHash       string    `json:"qb_hash,omitempty"`
	Error        string    `json:"error,omitempty"`
	ContentPath  string    `json:"content_path,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DownloadPreview struct {
	SiteID         string `json:"site_id"`
	TorrentID      string `json:"torrent_id"`
	OriginalTitle  string `json:"original_title"`
	FormattedTitle string `json:"formatted_title"`
	DownloadURL    string `json:"download_url"`
	LLMResponse    string `json:"llm_response,omitempty"`
}

type ManualDownloadRequest struct {
	SiteID         string `json:"site_id"`
	TorrentID      string `json:"torrent_id"`
	FormattedTitle string `json:"formatted_title,omitempty"`
}

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

type OrganizeResult struct {
	Processed int  `json:"processed"`
	Failed    int  `json:"failed"`
	DryRun    bool `json:"dry_run"`
}
