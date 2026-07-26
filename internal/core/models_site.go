// Package core 定义 NexusBridge 的领域模型和共享应用服务。
package core

import "time"

// Site 表示已加载站点及其凭据可用状态。
type Site struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	UserAgent string `json:"user_agent,omitempty"`
	HasCookie bool   `json:"has_cookie"`
}

// MediaFilterOption 表示媒体页可选择的一个站点筛选值。
type MediaFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// MediaFilterGroup 表示站点 checkbox 定义中的一个筛选分组。
type MediaFilterGroup struct {
	Name    string              `json:"name"`
	Label   string              `json:"label"`
	Options []MediaFilterOption `json:"options"`
}

// MediaFilterOptions 描述单个站点提供的 checkbox 与促销筛选项。
type MediaFilterOptions struct {
	SiteID         string              `json:"site_id"`
	CategoryLabel  string              `json:"category_label"`
	Categories     []MediaFilterOption `json:"categories"`
	Checkboxes     []MediaFilterGroup  `json:"checkboxes"`
	PromotionLabel string              `json:"promotion_label"`
	Promotions     []MediaFilterOption `json:"promotions"`
}

// SiteCredential 表示站点作用域的请求凭据。
type SiteCredential struct {
	SiteID    string `json:"site_id"`
	BaseURL   string `json:"base_url"`
	UserAgent string `json:"user_agent"`
	Cookie    string `json:"cookie,omitempty"`
	HasCookie bool   `json:"has_cookie"`
}

// SiteAttendance 描述站点每日自动签到配置和最近执行状态。
type SiteAttendance struct {
	SiteID     string `json:"site_id"`
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
	TimeOfDay  string `json:"time_of_day"`
	Timezone   string `json:"timezone"`

	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	LastError string     `json:"last_error,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Torrent 表示一个站点种子的持久化媒体视图。

// FetchResult 汇总一次同步站点抓取的处理结果。
type FetchResult struct {
	SiteID       string `json:"site_id"`
	Status       string `json:"status"`
	Fetched      int    `json:"fetched"`
	Inserted     int    `json:"inserted"`
	Changed      int    `json:"changed"`
	Matched      int    `json:"matched,omitempty"`
	DownloadSent int    `json:"download_sent,omitempty"`
	FilesSaved   int    `json:"torrent_files_saved,omitempty"`
	FilesFailed  int    `json:"torrent_files_failed,omitempty"`
}

// FetchSettings 描述所有站点共享的分页扫描限制。
type FetchSettings struct {
	MaxPages int `json:"max_pages"`
}

// SiteFetchRequest 描述一次站点列表扫描方式。
type SiteFetchRequest struct {
	Mode  string `json:"mode"`
	Pages int    `json:"pages,omitempty"`
}

// SiteFetchJob 表示站点列表扫描及其订阅执行进度。
type SiteFetchJob struct {
	ID             string     `json:"id"`
	SiteID         string     `json:"site_id"`
	Trigger        string     `json:"trigger"`
	Mode           string     `json:"mode"`
	RequestedPages int        `json:"requested_pages"`
	Status         string     `json:"status"`
	CurrentPage    int        `json:"current_page"`
	PagesFetched   int        `json:"pages_fetched"`
	Fetched        int        `json:"fetched"`
	Inserted       int        `json:"inserted"`
	Changed        int        `json:"changed"`
	Matched        int        `json:"matched"`
	DownloadSent   int        `json:"download_sent"`
	FilesSaved     int        `json:"torrent_files_saved"`
	FilesFailed    int        `json:"torrent_files_failed"`
	StopReason     string     `json:"stop_reason,omitempty"`
	Error          string     `json:"error,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
