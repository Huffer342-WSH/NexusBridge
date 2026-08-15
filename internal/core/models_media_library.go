package core

import "time"

// MediaLibraryKind 表示媒体库节点的业务形态。
type MediaLibraryKind string

const (
	MediaLibraryCollection MediaLibraryKind = "collection"
	MediaLibrarySeries     MediaLibraryKind = "series"
)

// MediaLibrarySettingsOverride 保存当前节点显式覆盖的可继承设置。
type MediaLibrarySettingsOverride struct {
	EpisodeNumberDetection *bool `json:"episode_number_detection,omitempty"`
	AutoDetectSeries       *bool `json:"auto_detect_series,omitempty"`
}

// MediaLibrarySettings 表示沿父链合并后的有效设置。
type MediaLibrarySettings struct {
	EpisodeNumberDetection bool `json:"episode_number_detection"`
	AutoDetectSeries       bool `json:"auto_detect_series"`
}

// MediaLibrarySummary 返回媒体库树和管理页所需的稳定摘要。
type MediaLibrarySummary struct {
	ID                string                       `json:"id"`
	Name              string                       `json:"name"`
	Kind              MediaLibraryKind             `json:"kind"`
	ParentID          string                       `json:"parent_id,omitempty"`
	Directories       []SeriesDirectory            `json:"directories"`
	Settings          MediaLibrarySettingsOverride `json:"settings"`
	EffectiveSettings MediaLibrarySettings         `json:"effective_settings"`
	VideoCount        int                          `json:"video_count"`
	AvailableVideos   int                          `json:"available_video_count"`
	LastScannedAt     *time.Time                   `json:"last_scanned_at,omitempty"`
	ScanErrors        []string                     `json:"scan_errors"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
}

// MediaLibraryDetail 返回媒体库摘要和剧集型节点的已有视频缓存。
type MediaLibraryDetail struct {
	MediaLibrarySummary
	Videos []SeriesVideo `json:"videos"`
}

// MediaLibrarySaveRequest 描述一个媒体库节点及其本地设置覆盖。
type MediaLibrarySaveRequest struct {
	Name        string                       `json:"name"`
	Kind        MediaLibraryKind             `json:"kind"`
	ParentID    string                       `json:"parent_id"`
	Directories []string                     `json:"directories"`
	Settings    MediaLibrarySettingsOverride `json:"settings"`
}
