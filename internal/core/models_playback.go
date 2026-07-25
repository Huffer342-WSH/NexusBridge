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

// PlaybackMedia 表示当前种子中的一个浏览器媒体文件。
type PlaybackMedia struct {
	Index     int               `json:"index"`
	Name      string            `json:"name"`
	MediaType PlaybackMediaType `json:"media_type"`
	MIMEType  string            `json:"mime_type"`
	Size      int64             `json:"size"`
	Progress  float64           `json:"progress"`
	Selected  bool              `json:"selected"`
	Complete  bool              `json:"complete"`
	Available bool              `json:"available"`
	StreamURL string            `json:"stream_url"`
}

// TorrentPlayback 返回播放页当前种子的元数据、qB 状态和选集。
type TorrentPlayback struct {
	Torrent          Torrent         `json:"torrent"`
	QBStatus         QBTorrentStatus `json:"qb_status"`
	Files            []PlaybackMedia `json:"files"`
	DefaultFileIndex *int            `json:"default_file_index,omitempty"`
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
	Name        string
	ContentType string
	ModTime     time.Time
}
