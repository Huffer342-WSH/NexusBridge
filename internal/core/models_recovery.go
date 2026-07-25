package core

import "time"

// RecoveryPreviewRequest 描述恢复预览的本地路径和 torrent 搜索范围。
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
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	IsDir      bool              `json:"is_dir"`
	Size       int64             `json:"size,omitempty"`
	ModifiedAt time.Time         `json:"modified_at,omitempty"`
	MediaType  PlaybackMediaType `json:"media_type,omitempty"`
	QBTasks    []FileQBTask      `json:"qb_tasks"`
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
