package core

import (
	"os"
	"time"
)

// PlaybackMediaType 表示浏览器媒体画布支持的源文件类型。
type PlaybackMediaType string

const (
	PlaybackMediaVideo PlaybackMediaType = "video"
	PlaybackMediaAudio PlaybackMediaType = "audio"
	PlaybackMediaImage PlaybackMediaType = "image"
)

// PlaybackSourceType 表示播放上下文最终匹配到的数据来源。
type PlaybackSourceType string

const (
	PlaybackSourceTorrent PlaybackSourceType = "torrent"
	PlaybackSourceQB      PlaybackSourceType = "qb"
	PlaybackSourceFile    PlaybackSourceType = "file"
)

// PlaybackMedia 表示播放上下文中的一个浏览器媒体文件。
type PlaybackMedia struct {
	Index        int                `json:"index"`
	Name         string             `json:"name"`
	MediaType    PlaybackMediaType  `json:"media_type"`
	MIMEType     string             `json:"mime_type"`
	Size         int64              `json:"size"`
	Progress     float64            `json:"progress"`
	Selected     bool               `json:"selected"`
	Complete     bool               `json:"complete"`
	Available    bool               `json:"available"`
	StreamURL    string             `json:"stream_url"`
	ThumbnailURL string             `json:"thumbnail_url,omitempty"`
	Subtitles    []PlaybackSubtitle `json:"subtitles,omitempty"`
}

// PlaybackSubtitle 表示可由浏览器加载的 MKV 内嵌文本字幕轨。
type PlaybackSubtitle struct {
	TrackID   uint64 `json:"track_id"`
	Label     string `json:"label"`
	Language  string `json:"language,omitempty"`
	Codec     string `json:"codec"`
	Default   bool   `json:"default"`
	Forced    bool   `json:"forced"`
	StreamURL string `json:"stream_url"`
}

// PlaybackDirectoryFile 表示当前媒体所在目录中的一个普通文件。
type PlaybackDirectoryFile struct {
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	Size       int64             `json:"size"`
	ModifiedAt time.Time         `json:"modified_at"`
	MediaType  PlaybackMediaType `json:"media_type,omitempty"`
	MIMEType   string            `json:"mime_type,omitempty"`
	ThumbnailURL string          `json:"thumbnail_url,omitempty"`
	Playable   bool              `json:"playable"`
	Current    bool              `json:"current"`
}

// PlaybackContext 返回播放页统一使用的来源、详情、选集和目录文件。
type PlaybackContext struct {
	Source           PlaybackSourceType      `json:"source"`
	Title            string                  `json:"title"`
	CurrentPath      string                  `json:"current_path"`
	CurrentDirectory string                  `json:"current_directory"`
	Torrent          *Torrent                `json:"torrent,omitempty"`
	QBStatus         *QBTorrentStatus        `json:"qb_status,omitempty"`
	QBHash           string                  `json:"qb_hash,omitempty"`
	Files            []PlaybackMedia         `json:"files"`
	DirectoryFiles   []PlaybackDirectoryFile `json:"directory_files"`
	DefaultFileIndex *int                    `json:"default_file_index,omitempty"`
	CurrentFileIndex *int                    `json:"current_file_index,omitempty"`
	Series           *SeriesSummary          `json:"series,omitempty"`
	SeriesFiles      []SeriesVideo           `json:"series_files,omitempty"`
}

// PlaybackTorrent 表示播放页右侧可跳转的其他种子。
type PlaybackTorrent struct {
	ID          string     `json:"id"`
	SiteID      string     `json:"site_id"`
	Title       string     `json:"title"`
	CoverURL    string     `json:"cover_url,omitempty"`
	Category    string     `json:"category,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

// PlaybackSource 表示已经通过 qB 文件索引解析并校验的本地源文件。
type PlaybackSource struct {
	File        *os.File
	Path        string
	Name        string
	ContentType string
	ModTime     time.Time
}

// VideoThumbnail 表示已经生成的本地 JPEG 缩略图。
type VideoThumbnail struct {
	Path    string
	ETag    string
	ModTime time.Time
}
