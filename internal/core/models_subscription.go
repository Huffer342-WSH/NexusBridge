package core

import "time"

// Rule 描述种子筛选、排序及命中后动作。
type Rule struct {
	Name                   string   `json:"name"`
	SiteIDs                []string `json:"site_ids"`
	SiteCategories         []string `json:"site_categories"`
	SiteTags               []string `json:"site_tags"`
	SubtitleTags           []string `json:"subtitle_tags"`
	TitleExpression        string   `json:"title_expression"`
	Promotions             []string `json:"promotions"`
	MinSize                int64    `json:"min_size"`
	MaxSize                int64    `json:"max_size"`
	MinSeeders             int      `json:"min_seeders"`
	MaxSeeders             int      `json:"max_seeders"`
	MinLeechers            int      `json:"min_leechers"`
	MaxLeechers            int      `json:"max_leechers"`
	MinSnatches            int      `json:"min_snatches"`
	MaxSnatches            int      `json:"max_snatches"`
	PublishedWithinMinutes int      `json:"published_within_minutes"`
	SortBy                 string   `json:"sort_by"`
	SortDirection          string   `json:"sort_direction"`
	Action                 string   `json:"action"`
}

// RuleMatch 表示规则对单个种子的匹配判断。
type RuleMatch struct {
	Rule     Rule         `json:"rule"`
	Torrent  Torrent      `json:"torrent"`
	Matched  bool         `json:"matched"`
	Reason   string       `json:"reason"`
	Reasons  []RuleReason `json:"reasons"`
	Eligible bool         `json:"eligible"`
}

// RuleReason 描述筛选或执行拒绝的稳定原因。
type RuleReason struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// DownloadOptions 描述一次 qBittorrent 新增任务的输出配置。
type DownloadOptions struct {
	QBCategory       string   `json:"qb_category"`
	SavePathTemplate string   `json:"save_path_template"`
	QBTags           []string `json:"qb_tags"`
	FilenameTemplate string   `json:"filename_template"`
	Paused           bool     `json:"paused"`
	MaxConcurrent    int      `json:"max_concurrent"`
	DailyLimit       int      `json:"daily_limit"`
}

// Subscription 将可复用筛选规则与自动 qB 下载配置关联起来。
type Subscription struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Enabled   bool            `json:"enabled"`
	RuleName  string          `json:"rule_name"`
	SiteIDs   []string        `json:"site_ids"`
	Priority  int             `json:"priority"`
	Download  DownloadOptions `json:"download"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
	UpdatedAt time.Time       `json:"updated_at,omitempty"`
}

// SiteSchedule 描述站点自动检索周期。
type SiteSchedule struct {
	SiteID          string     `json:"site_id"`
	Enabled         bool       `json:"enabled"`
	IntervalSeconds int        `json:"interval_seconds"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
}

// TorrentKey 是 API 中引用数据库种子的联合键。
type TorrentKey struct {
	SiteID    string `json:"site_id"`
	TorrentID string `json:"torrent_id"`
}

// DownloadPlan 是预览和执行共用的最终 qB 参数快照。
type DownloadPlan struct {
	Category     string       `json:"category"`
	SavePath     string       `json:"save_path"`
	Tags         []string     `json:"tags"`
	Rename       string       `json:"rename"`
	OriginalName string       `json:"original_name,omitempty"`
	Paused       bool         `json:"paused"`
	AutoTMM      *bool        `json:"auto_tmm,omitempty"`
	Reasons      []RuleReason `json:"reasons"`
}

// RulePreviewRequest 描述草稿筛选规则的分页预览请求。
type RulePreviewRequest struct {
	Rule   Rule   `json:"rule"`
	SiteID string `json:"site_id,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// RulePreviewItem 描述单个数据库种子的筛选结论。
type RulePreviewItem struct {
	Torrent Torrent      `json:"torrent"`
	Matched bool         `json:"matched"`
	Reasons []RuleReason `json:"reasons"`
	QBState string       `json:"qb_state"`
}

// RulePreviewResult 汇总草稿筛选预览结果。
type RulePreviewResult struct {
	Evaluated int               `json:"evaluated"`
	Matched   int               `json:"matched"`
	Items     []RulePreviewItem `json:"items"`
	Truncated bool              `json:"truncated"`
}

// RuleSelectOption 描述规则编辑器中的一个可选值。
type RuleSelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// RuleFilterOptions 汇总单个站点的本地筛选选项。
type RuleFilterOptions struct {
	SiteID         string             `json:"site_id"`
	SiteCategories []RuleSelectOption `json:"site_categories"`
	SiteTags       []RuleSelectOption `json:"site_tags"`
	SubtitleTags   []RuleSelectOption `json:"subtitle_tags"`
	Promotions     []RuleSelectOption `json:"promotions"`
}

// SubscriptionPreviewItem 描述订阅对单个种子的筛选和下载计划。
type SubscriptionPreviewItem struct {
	Torrent  Torrent      `json:"torrent"`
	Matched  bool         `json:"matched"`
	Eligible bool         `json:"eligible"`
	Reasons  []RuleReason `json:"reasons"`
	Plan     DownloadPlan `json:"plan"`
}

// SubscriptionPreview 汇总订阅预览。
type SubscriptionPreview struct {
	SubscriptionID string                    `json:"subscription_id"`
	Evaluated      int                       `json:"evaluated"`
	Matched        int                       `json:"matched"`
	Eligible       int                       `json:"eligible"`
	Items          []SubscriptionPreviewItem `json:"items"`
}

// SubscriptionCandidate 是首个命中订阅独占的待处理种子。
type SubscriptionCandidate struct {
	SiteID         string    `json:"site_id"`
	TorrentID      string    `json:"torrent_id"`
	SubscriptionID string    `json:"subscription_id"`
	RuleName       string    `json:"rule_name"`
	Status         string    `json:"status"`
	SourceOrder    int       `json:"source_order"`
	ReasonCode     string    `json:"reason_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SubscriptionRun 保存一次订阅执行的可审计统计。
type SubscriptionRun struct {
	ID             string     `json:"id"`
	SubscriptionID string     `json:"subscription_id,omitempty"`
	SiteID         string     `json:"site_id,omitempty"`
	Trigger        string     `json:"trigger"`
	Status         string     `json:"status"`
	Fetched        int        `json:"fetched"`
	Inserted       int        `json:"inserted"`
	Matched        int        `json:"matched"`
	Attempted      int        `json:"attempted"`
	Sent           int        `json:"sent"`
	Exists         int        `json:"exists"`
	Failed         int        `json:"failed"`
	Skipped        int        `json:"skipped"`
	Error          string     `json:"error,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// QBCategory 描述同步到本应用的 qB 分类。
