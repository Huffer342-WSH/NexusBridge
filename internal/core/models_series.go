package core

import "time"

// SeriesDirectory 表示剧集中的一个有序本机扫描根目录。
type SeriesDirectory struct {
	Path          string     `json:"path"`
	Order         int        `json:"order"`
	Available     bool       `json:"available"`
	LastError     string     `json:"last_error,omitempty"`
	LastScannedAt *time.Time `json:"last_scanned_at,omitempty"`
}

// SeriesVideo 表示剧集扫描缓存中的一个视频。
type SeriesVideo struct {
	Path           string    `json:"path"`
	DirectoryPath  string    `json:"directory_path"`
	RelativePath   string    `json:"relative_path"`
	Name           string    `json:"name"`
	Size           int64     `json:"size"`
	ModifiedAt     time.Time `json:"modified_at"`
	Available      bool      `json:"available"`
	EpisodeNumber  *int      `json:"episode_number,omitempty"`
	EpisodeVersion int       `json:"episode_version,omitempty"`
	EpisodeLabel   string    `json:"episode_label,omitempty"`
	ThumbnailURL   string    `json:"thumbnail_url,omitempty"`
}

// SeriesSummary 返回剧集管理页所需的缓存摘要。
type SeriesSummary struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	EpisodeNumberDetection bool              `json:"episode_number_detection"`
	Directories            []SeriesDirectory `json:"directories"`
	VideoCount             int               `json:"video_count"`
	AvailableVideoCount    int               `json:"available_video_count"`
	LastSelectedPath       string            `json:"last_selected_path,omitempty"`
	LastScannedAt          *time.Time        `json:"last_scanned_at,omitempty"`
	ScanErrors             []string          `json:"scan_errors"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

// SeriesDetail 返回剧集摘要及其有序视频清单。
type SeriesDetail struct {
	SeriesSummary
	Videos []SeriesVideo `json:"videos"`
}

// SeriesSaveRequest 描述剧集名称和有序本机目录。
type SeriesSaveRequest struct {
	Name                   string   `json:"name"`
	Directories            []string `json:"directories"`
	EpisodeNumberDetection bool     `json:"episode_number_detection"`
}

// SeriesSelectionRequest 描述剧集最后选择的视频绝对路径。
type SeriesSelectionRequest struct {
	Path string `json:"path"`
}
