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

type TorrentQuery struct {
	SiteID        string
	Search        string
	SortBy        string
	SortDirection string
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

// FilterRule 是可复用于订阅、预览和手动批量操作的筛选规则领域名称；Rule 保留 API 兼容。
type FilterRule = Rule

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

// RecoveryPreviewRequest 描述一次保留目录匹配预览。
type RecoveryPreviewRequest struct {
	Path       string   `json:"path"`
	SiteIDs    []string `json:"site_ids,omitempty"`
	SearchMode string   `json:"search_mode,omitempty"`
	TorrentURL string   `json:"torrent_url,omitempty"`
	Category   string   `json:"category,omitempty"`
}

// RecoveryRequest 按目录、可选站点和检索方式执行 qB 恢复。
type RecoveryRequest struct {
	Path       string `json:"path"`
	SiteID     string `json:"site_id,omitempty"`
	TorrentID  string `json:"torrent_id,omitempty"`
	SearchMode string `json:"search_mode,omitempty"`
	TorrentURL string `json:"torrent_url,omitempty"`
	Category   string `json:"category,omitempty"`
}

// RecoveryMatch 表示一个通过完整文件大小集合匹配的 torrent。
type RecoveryMatch struct {
	Torrent         Torrent `json:"torrent"`
	Source          string  `json:"source"`
	OriginalName    string  `json:"original_name"`
	SavePath        string  `json:"save_path"`
	RootFolder      bool    `json:"root_folder"`
	InfoHashV1      string  `json:"info_hash_v1,omitempty"`
	InfoHashV2      string  `json:"info_hash_v2,omitempty"`
	FileCount       int     `json:"file_count"`
	TotalSize       int64   `json:"total_size"`
	MatchMethod     string  `json:"match_method"`
	MappingComplete bool    `json:"mapping_complete"`
}

// RecoverySearchAttempt 记录单个站点当前页检索的可见结果。
type RecoverySearchAttempt struct {
	SiteID      string `json:"site_id"`
	Keyword     string `json:"keyword"`
	Status      string `json:"status"`
	Candidates  int    `json:"candidates"`
	FilesSaved  int    `json:"files_saved"`
	FilesFailed int    `json:"files_failed"`
	Error       string `json:"error,omitempty"`
}

// RecoveryPreview 返回只读目录匹配结果和站点检索诊断。
type RecoveryPreview struct {
	Path           string                  `json:"path"`
	FolderName     string                  `json:"folder_name"`
	SavePath       string                  `json:"save_path"`
	Source         string                  `json:"source"`
	SearchMode     string                  `json:"search_mode"`
	Category       string                  `json:"category,omitempty"`
	Evaluated      int                     `json:"evaluated"`
	Matches        []RecoveryMatch         `json:"matches"`
	SearchAttempts []RecoverySearchAttempt `json:"search_attempts,omitempty"`
}

// RecoveryResult 表示目录重验证后的 qB 重建结果。
type RecoveryResult struct {
	Path               string          `json:"path"`
	SavePath           string          `json:"save_path"`
	Category           string          `json:"category,omitempty"`
	Match              RecoveryMatch   `json:"match"`
	QBStatus           QBTorrentStatus `json:"qb_status"`
	VerificationStatus string          `json:"verification_status"`
	VerificationError  string          `json:"verification_error,omitempty"`
	RecheckAttempts    int             `json:"recheck_attempts"`
	Started            bool            `json:"started"`
	CanStart           bool            `json:"can_start"`
	CanDelete          bool            `json:"can_delete"`
}

// RecoveryActionRequest 描述校验失败后对保留 qB 任务的显式操作。
type RecoveryActionRequest struct {
	Action string `json:"action"`
}

// RecoveryActionResult 返回校验失败任务的启动或删除结果。
type RecoveryActionResult struct {
	Hash     string           `json:"hash"`
	Action   string           `json:"action"`
	Deleted  bool             `json:"deleted"`
	QBStatus *QBTorrentStatus `json:"qb_status,omitempty"`
}

// TorrentSizeIndexStatus 返回恢复大小索引覆盖和最近重建结果。
type TorrentSizeIndexStatus struct {
	Version   int    `json:"version"`
	Total     int    `json:"total"`
	Indexed   int    `json:"indexed"`
	Pending   int    `json:"pending"`
	Failed    int    `json:"failed"`
	Processed int    `json:"processed,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// FileBrowseRequest 描述文件管理器目录浏览请求。
type FileBrowseRequest struct {
	Path string `json:"path,omitempty"`
}

// FileQBTask 描述拥有当前文件系统条目的 qB 任务。
type FileQBTask struct {
	Hash        string `json:"hash"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Category    string `json:"category,omitempty"`
	SavePath    string `json:"save_path,omitempty"`
	ContentPath string `json:"content_path"`
}

// FileEntry 描述一个可浏览的文件或目录。
type FileEntry struct {
	Name       string       `json:"name"`
	Path       string       `json:"path"`
	IsDir      bool         `json:"is_dir"`
	Size       int64        `json:"size,omitempty"`
	ModifiedAt time.Time    `json:"modified_at,omitempty"`
	QBTasks    []FileQBTask `json:"qb_tasks"`
}

// FileBrowseResult 返回目录内容和 qB 连接诊断。
type FileBrowseResult struct {
	Path        string      `json:"path"`
	Parent      string      `json:"parent,omitempty"`
	IsRoot      bool        `json:"is_root"`
	QBConnected bool        `json:"qb_connected"`
	QBError     string      `json:"qb_error,omitempty"`
	Entries     []FileEntry `json:"entries"`
}

// RecoveryScanRequest 描述只读丢失任务扫描范围。
type RecoveryScanRequest struct {
	Path       string   `json:"path"`
	SiteIDs    []string `json:"site_ids,omitempty"`
	SearchMode string   `json:"search_mode,omitempty"`
	MaxDepth   int      `json:"max_depth,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// RecoveryScanItem 描述一个扫描条目的恢复候选状态。
type RecoveryScanItem struct {
	Path    string          `json:"path"`
	Name    string          `json:"name"`
	IsDir   bool            `json:"is_dir"`
	Status  string          `json:"status"`
	Reason  string          `json:"reason,omitempty"`
	Preview RecoveryPreview `json:"preview"`
}

// RecoveryScanResult 汇总一次只读目录扫描。
type RecoveryScanResult struct {
	Path      string             `json:"path"`
	Evaluated int                `json:"evaluated"`
	Matched   int                `json:"matched"`
	Truncated bool               `json:"truncated"`
	Items     []RecoveryScanItem `json:"items"`
}

// RecoveryBatchItemRequest 描述扫描阶段已经唯一选定的恢复候选。
type RecoveryBatchItemRequest struct {
	Path      string `json:"path"`
	SiteID    string `json:"site_id"`
	TorrentID string `json:"torrent_id"`
	Category  string `json:"category,omitempty"`
}

// RecoveryBatchRequest 描述一次自动批量恢复请求。
type RecoveryBatchRequest struct {
	Items     []RecoveryBatchItemRequest `json:"items"`
	WebSearch bool                       `json:"web_search,omitempty"`
}

// RecoveryBatchItemResult 返回单个批量候选的执行状态。
type RecoveryBatchItemResult struct {
	Path      string          `json:"path"`
	SiteID    string          `json:"site_id"`
	TorrentID string          `json:"torrent_id"`
	Status    string          `json:"status"`
	Error     string          `json:"error,omitempty"`
	Result    *RecoveryResult `json:"result,omitempty"`
}

// RecoveryBatchResult 汇总一次串行自动恢复。
type RecoveryBatchResult struct {
	Attempted      int                       `json:"attempted"`
	Recovered      int                       `json:"recovered"`
	NeedsAttention int                       `json:"needs_attention"`
	Failed         int                       `json:"failed"`
	Skipped        int                       `json:"skipped"`
	Items          []RecoveryBatchItemResult `json:"items"`
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
