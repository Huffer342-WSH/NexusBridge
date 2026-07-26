package core

import "time"

// Torrent 表示一个站点种子的持久化媒体视图。
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
	PromotionClass    string           `json:"promotion_class,omitempty"`
	PromotionEndsAt   string           `json:"promotion_ends_at,omitempty"`
	PublishedAt       *time.Time       `json:"published_at,omitempty"`
	PublishedText     string           `json:"published_text,omitempty"`
	FirstSeenAt       time.Time        `json:"first_seen_at"`
	LastSeenAt        time.Time        `json:"last_seen_at"`
	SourceOrder       int              `json:"source_order"`
	StickyLevel       int              `json:"sticky_level"`
	TorrentFileSaved  bool             `json:"torrent_file_saved"`
	InfoHashV1        string           `json:"info_hash_v1,omitempty"`
	InfoHashV2        string           `json:"info_hash_v2,omitempty"`
	TorrentFileError  string           `json:"torrent_file_error,omitempty"`
	QBStatus          *QBTorrentStatus `json:"qb_status,omitempty"`
}

// TorrentQuery 描述本地种子列表的查询、排序和 qB 状态选项。
type TorrentQuery struct {
	SiteID        string
	Search        string
	SortBy        string
	SortDirection string
	QBTask        string
	QBProgress    string
	Limit         int
	Offset        int
	IncludeQB     bool
	QBWeakMatch   bool
	ExcludePinned bool
}

// TorrentPage 表示按范围查询的媒体种子页。
type TorrentPage struct {
	Items  []Torrent `json:"items"`
	Offset int       `json:"offset"`
	Limit  int       `json:"limit"`
	Total  int       `json:"total"`
}

// QBTorrentStatus 表示种子关联的 qBittorrent 实时或快照状态。
type QBTorrentStatus struct {
	Available     bool      `json:"available"`
	Added         bool      `json:"added"`
	Source        string    `json:"source"`
	Error         string    `json:"error,omitempty"`
	FetchedAt     time.Time `json:"fetched_at"`
	TaskID        string    `json:"task_id,omitempty"`
	TaskStatus    string    `json:"task_status,omitempty"`
	RuleName      string    `json:"rule_name,omitempty"`
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

// FetchResult 保留同步内部入口使用的站点扫描汇总。
