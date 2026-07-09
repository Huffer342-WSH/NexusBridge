package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrTorrentConflict 表示 qBittorrent 拒绝重复添加已存在的种子。
var ErrTorrentConflict = errors.New("qbittorrent torrent conflict")

type torrentAddResult struct {
	SuccessCount    int      `json:"success_count"`
	PendingCount    int      `json:"pending_count"`
	FailureCount    int      `json:"failure_count"`
	AddedTorrentIDs []string `json:"added_torrent_ids"`
}

// AddOptions 描述 qBittorrent 新增种子的原生参数。
type AddOptions struct {
	URL                string
	URLs               []string
	Name               string
	Data               []byte
	SavePath           string
	Category           string
	Tags               []string
	SkipChecking       bool
	Paused             bool
	RootFolder         *bool
	Rename             string
	UploadLimit        int
	DownloadLimit      int
	RatioLimit         *float64
	SeedingTimeLimit   *int
	AutoTMM            *bool
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// TorrentListOptions 描述 qBittorrent 种子列表筛选参数。
type TorrentListOptions struct {
	Filter   string
	Category *string
	Tag      *string
	Sort     string
	Reverse  bool
	Limit    int
	Offset   *int
	Hashes   []string
}

// TorrentInfo 表示 qBittorrent WebAPI 返回的种子任务。
type TorrentInfo struct {
	AddedOn           int64   `json:"added_on"`
	AmountLeft        int64   `json:"amount_left"`
	AutoTMM           bool    `json:"auto_tmm"`
	Availability      float64 `json:"availability"`
	Category          string  `json:"category"`
	Completed         int64   `json:"completed"`
	CompletionOn      int64   `json:"completion_on"`
	ContentPath       string  `json:"content_path"`
	DownloadLimit     int64   `json:"dl_limit"`
	DownloadSpeed     int64   `json:"dlspeed"`
	Downloaded        int64   `json:"downloaded"`
	DownloadedSession int64   `json:"downloaded_session"`
	ETA               int64   `json:"eta"`
	FirstLastPrio     bool    `json:"f_l_piece_prio"`
	ForceStart        bool    `json:"force_start"`
	Hash              string  `json:"hash"`
	IsPrivate         bool    `json:"isPrivate"`
	LastActivity      int64   `json:"last_activity"`
	MagnetURI         string  `json:"magnet_uri"`
	MaxRatio          float64 `json:"max_ratio"`
	MaxSeedingTime    int64   `json:"max_seeding_time"`
	Name              string  `json:"name"`
	NumComplete       int64   `json:"num_complete"`
	NumIncomplete     int64   `json:"num_incomplete"`
	NumLeechs         int64   `json:"num_leechs"`
	NumSeeds          int64   `json:"num_seeds"`
	Priority          int64   `json:"priority"`
	Progress          float64 `json:"progress"`
	Ratio             float64 `json:"ratio"`
	RatioLimit        float64 `json:"ratio_limit"`
	Reannounce        int64   `json:"reannounce"`
	SavePath          string  `json:"save_path"`
	SeedingTime       int64   `json:"seeding_time"`
	SeedingTimeLimit  int64   `json:"seeding_time_limit"`
	SeenComplete      int64   `json:"seen_complete"`
	SequentialDL      bool    `json:"seq_dl"`
	Size              int64   `json:"size"`
	State             string  `json:"state"`
	SuperSeeding      bool    `json:"super_seeding"`
	Tags              string  `json:"tags"`
	TimeActive        int64   `json:"time_active"`
	TotalSize         int64   `json:"total_size"`
	Tracker           string  `json:"tracker"`
	UploadLimit       int64   `json:"up_limit"`
	Uploaded          int64   `json:"uploaded"`
	UploadedSession   int64   `json:"uploaded_session"`
	UploadSpeed       int64   `json:"upspeed"`
}

// TorrentProperties 表示 qBittorrent 种子的通用属性。
type TorrentProperties struct {
	SavePath               string  `json:"save_path"`
	CreationDate           int64   `json:"creation_date"`
	PieceSize              int64   `json:"piece_size"`
	Comment                string  `json:"comment"`
	TotalWasted            int64   `json:"total_wasted"`
	TotalUploaded          int64   `json:"total_uploaded"`
	TotalUploadedSession   int64   `json:"total_uploaded_session"`
	TotalDownloaded        int64   `json:"total_downloaded"`
	TotalDownloadedSession int64   `json:"total_downloaded_session"`
	UploadLimit            int64   `json:"up_limit"`
	DownloadLimit          int64   `json:"dl_limit"`
	TimeElapsed            int64   `json:"time_elapsed"`
	SeedingTime            int64   `json:"seeding_time"`
	Connections            int64   `json:"nb_connections"`
	ConnectionsLimit       int64   `json:"nb_connections_limit"`
	ShareRatio             float64 `json:"share_ratio"`
	AdditionDate           int64   `json:"addition_date"`
	CompletionDate         int64   `json:"completion_date"`
	CreatedBy              string  `json:"created_by"`
	DownloadSpeedAverage   int64   `json:"dl_speed_avg"`
	DownloadSpeed          int64   `json:"dl_speed"`
	ETA                    int64   `json:"eta"`
	LastSeen               int64   `json:"last_seen"`
	Peers                  int64   `json:"peers"`
	PeersTotal             int64   `json:"peers_total"`
	PiecesHave             int64   `json:"pieces_have"`
	PiecesNum              int64   `json:"pieces_num"`
	Reannounce             int64   `json:"reannounce"`
	Seeds                  int64   `json:"seeds"`
	SeedsTotal             int64   `json:"seeds_total"`
	TotalSize              int64   `json:"total_size"`
	UploadSpeedAverage     int64   `json:"up_speed_avg"`
	UploadSpeed            int64   `json:"up_speed"`
	IsPrivate              bool    `json:"isPrivate"`
}

// TorrentContent 表示 qBittorrent 种子中的单个文件。
type TorrentContent struct {
	Index        int     `json:"index"`
	Name         string  `json:"name"`
	Size         int64   `json:"size"`
	Progress     float64 `json:"progress"`
	Priority     int     `json:"priority"`
	IsSeed       bool    `json:"is_seed"`
	PieceRange   []int   `json:"piece_range"`
	Availability float64 `json:"availability"`
}

// Category 表示 qBittorrent 下载分类。
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// AddTorrentURL 通过 URL 或 magnet 链接添加新种子。
func (c *Client) AddTorrentURL(ctx context.Context, opts AddOptions) error {
	urls := append([]string{}, opts.URLs...)
	if strings.TrimSpace(opts.URL) != "" {
		urls = append([]string{opts.URL}, urls...)
	}
	if len(nonEmptyStrings(urls)) == 0 {
		return fmt.Errorf("torrent url is required")
	}
	slog.Info("qb add torrent url started", "category", opts.Category, "tags", strings.Join(opts.Tags, ","), "auth_mode", c.authMode())
	form, err := c.addOptionsForm(opts)
	if err != nil {
		return err
	}
	form.Set("urls", strings.Join(nonEmptyStrings(urls), "\n"))
	_, err = c.postMultipart(ctx, "/api/v2/torrents/add", form, nil)
	return err
}

// AddTorrentFile 上传本地已下载的 torrent 文件到 qBittorrent。
func (c *Client) AddTorrentFile(ctx context.Context, opts AddOptions) error {
	_, err := c.addTorrentFileWithResult(ctx, opts)
	return err
}

func (c *Client) addTorrentFileWithResult(ctx context.Context, opts AddOptions) (torrentAddResult, error) {
	if len(opts.Data) == 0 {
		return torrentAddResult{}, fmt.Errorf("torrent data is required")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "nexusbridge.torrent"
	}
	slog.Info("qb add torrent file started", "name", name, "bytes", len(opts.Data), "category", opts.Category, "tags", strings.Join(opts.Tags, ","), "auth_mode", c.authMode())
	form, err := c.addOptionsForm(opts)
	if err != nil {
		return torrentAddResult{}, err
	}
	return c.postMultipart(ctx, "/api/v2/torrents/add", form, []torrentFile{{Name: name, Data: opts.Data}})
}

// ListTorrents 查询全部 qBittorrent 种子任务。
func (c *Client) ListTorrents(ctx context.Context) ([]TorrentInfo, error) {
	return c.ListTorrentsWithOptions(ctx, TorrentListOptions{})
}

// ListTorrentsWithOptions 按 qBittorrent 标准筛选参数查询种子任务。
func (c *Client) ListTorrentsWithOptions(ctx context.Context, opts TorrentListOptions) ([]TorrentInfo, error) {
	values := url.Values{}
	if opts.Filter != "" {
		values.Set("filter", opts.Filter)
	}
	if opts.Category != nil {
		values.Set("category", *opts.Category)
	}
	if opts.Tag != nil {
		values.Set("tag", *opts.Tag)
	}
	if opts.Sort != "" {
		values.Set("sort", opts.Sort)
	}
	if opts.Reverse {
		values.Set("reverse", "true")
	}
	if opts.Limit > 0 {
		values.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset != nil {
		values.Set("offset", strconv.Itoa(*opts.Offset))
	}
	if len(opts.Hashes) > 0 {
		values.Set("hashes", strings.Join(nonEmptyStrings(opts.Hashes), "|"))
	}
	var torrents []TorrentInfo
	if err := c.getJSON(ctx, "/api/v2/torrents/info", values, "torrents/info", &torrents); err != nil {
		return nil, err
	}
	return torrents, nil
}

// GetTorrentProperties 查询指定种子的通用属性。
func (c *Client) GetTorrentProperties(ctx context.Context, hash string) (TorrentProperties, error) {
	if strings.TrimSpace(hash) == "" {
		return TorrentProperties{}, fmt.Errorf("torrent hash is required")
	}
	values := url.Values{"hash": {hash}}
	var props TorrentProperties
	if err := c.getJSON(ctx, "/api/v2/torrents/properties", values, "torrents/properties", &props); err != nil {
		return TorrentProperties{}, err
	}
	return props, nil
}

// GetTorrentContents 查询指定种子的文件列表。
func (c *Client) GetTorrentContents(ctx context.Context, hash string, indexes []int) ([]TorrentContent, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, fmt.Errorf("torrent hash is required")
	}
	values := url.Values{"hash": {hash}}
	if len(indexes) > 0 {
		parts := make([]string, 0, len(indexes))
		for _, index := range indexes {
			parts = append(parts, strconv.Itoa(index))
		}
		values.Set("indexes", strings.Join(parts, "|"))
	}
	var contents []TorrentContent
	if err := c.getJSON(ctx, "/api/v2/torrents/files", values, "torrents/files", &contents); err != nil {
		return nil, err
	}
	return contents, nil
}

// SetTorrentCategory 设置一个或多个种子的分类。
func (c *Client) SetTorrentCategory(ctx context.Context, hashes []string, category string) error {
	values := url.Values{"hashes": {joinHashes(hashes)}, "category": {category}}
	return c.postForm(ctx, "/api/v2/torrents/setCategory", values, "torrents/setCategory")
}

// GetCategories 查询 qBittorrent 中的全部分类。
func (c *Client) GetCategories(ctx context.Context) (map[string]Category, error) {
	var categories map[string]Category
	if err := c.getJSON(ctx, "/api/v2/torrents/categories", nil, "torrents/categories", &categories); err != nil {
		return nil, err
	}
	if categories == nil {
		categories = map[string]Category{}
	}
	return categories, nil
}

// CreateCategory 创建 qBittorrent 分类。
func (c *Client) CreateCategory(ctx context.Context, category, savePath string) error {
	values := url.Values{"category": {category}}
	if savePath != "" {
		values.Set("savePath", savePath)
	}
	return c.postForm(ctx, "/api/v2/torrents/createCategory", values, "torrents/createCategory")
}

// EditCategory 修改 qBittorrent 分类的保存路径。
func (c *Client) EditCategory(ctx context.Context, category, savePath string) error {
	values := url.Values{"category": {category}, "savePath": {savePath}}
	return c.postForm(ctx, "/api/v2/torrents/editCategory", values, "torrents/editCategory")
}

// RemoveCategories 删除一个或多个 qBittorrent 分类。
func (c *Client) RemoveCategories(ctx context.Context, categories []string) error {
	return c.postForm(ctx, "/api/v2/torrents/removeCategories", url.Values{"categories": {strings.Join(nonEmptyStrings(categories), "\n")}}, "torrents/removeCategories")
}

// AddTorrentTags 给一个或多个种子添加标签。
func (c *Client) AddTorrentTags(ctx context.Context, hashes []string, tags []string) error {
	return c.postForm(ctx, "/api/v2/torrents/addTags", torrentTagsForm(hashes, tags), "torrents/addTags")
}

// RemoveTorrentTags 移除一个或多个种子的标签。
func (c *Client) RemoveTorrentTags(ctx context.Context, hashes []string, tags []string) error {
	return c.postForm(ctx, "/api/v2/torrents/removeTags", torrentTagsForm(hashes, tags), "torrents/removeTags")
}

// GetTags 查询 qBittorrent 中的全部标签。
func (c *Client) GetTags(ctx context.Context) ([]string, error) {
	var tags []string
	if err := c.getJSON(ctx, "/api/v2/torrents/tags", nil, "torrents/tags", &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// CreateTags 创建一个或多个 qBittorrent 标签。
func (c *Client) CreateTags(ctx context.Context, tags []string) error {
	return c.postForm(ctx, "/api/v2/torrents/createTags", url.Values{"tags": {strings.Join(nonEmptyStrings(tags), ",")}}, "torrents/createTags")
}

// DeleteTags 删除一个或多个 qBittorrent 标签。
func (c *Client) DeleteTags(ctx context.Context, tags []string) error {
	return c.postForm(ctx, "/api/v2/torrents/deleteTags", url.Values{"tags": {strings.Join(nonEmptyStrings(tags), ",")}}, "torrents/deleteTags")
}

// DeleteTorrents 删除一个或多个 qBittorrent 种子任务。
func (c *Client) DeleteTorrents(ctx context.Context, hashes []string, deleteFiles bool) error {
	values := url.Values{"hashes": {joinHashes(hashes)}, "deleteFiles": {strconv.FormatBool(deleteFiles)}}
	return c.postForm(ctx, "/api/v2/torrents/delete", values, "torrents/delete")
}

// StopTorrents 暂停一个或多个 qBittorrent 种子任务。
func (c *Client) StopTorrents(ctx context.Context, hashes []string) error {
	return c.postForm(ctx, "/api/v2/torrents/stop", url.Values{"hashes": {joinHashes(hashes)}}, "torrents/stop")
}

// StartTorrents 恢复一个或多个 qBittorrent 种子任务。
func (c *Client) StartTorrents(ctx context.Context, hashes []string) error {
	return c.postForm(ctx, "/api/v2/torrents/start", url.Values{"hashes": {joinHashes(hashes)}}, "torrents/start")
}

func torrentTagsForm(hashes, tags []string) url.Values {
	return url.Values{"hashes": {joinHashes(hashes)}, "tags": {strings.Join(nonEmptyStrings(tags), ",")}}
}

func (c *Client) postMultipart(ctx context.Context, path string, values url.Values, files []torrentFile) (torrentAddResult, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return torrentAddResult{}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, fieldValues := range values {
		for _, value := range fieldValues {
			if err := writer.WriteField(name, value); err != nil {
				return torrentAddResult{}, err
			}
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile("torrents", file.Name)
		if err != nil {
			return torrentAddResult{}, err
		}
		if _, err := part.Write(file.Data); err != nil {
			return torrentAddResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return torrentAddResult{}, err
	}
	resp, err := c.doAuthed(ctx, http.MethodPost, path, &body, writer.FormDataContentType())
	if err != nil {
		return torrentAddResult{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	slog.Info("qb add torrent response", "status", resp.StatusCode, "body", strings.TrimSpace(string(respBody)))
	if resp.StatusCode == http.StatusConflict {
		return torrentAddResult{}, fmt.Errorf("%w: %s", ErrTorrentConflict, trimBody(respBody))
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return torrentAddResult{}, fmt.Errorf("qbittorrent torrents/add status %d: %s", resp.StatusCode, trimBody(respBody))
	}
	var addResult torrentAddResult
	if json.Unmarshal(respBody, &addResult) == nil && addResult.FailureCount > 0 {
		return addResult, fmt.Errorf("qbittorrent add failed: %s", trimBody(respBody))
	}
	if text := strings.TrimSpace(string(respBody)); text != "" && !strings.Contains(strings.ToLower(text), "ok") {
		if !strings.Contains(text, "pending_count") && !strings.Contains(text, "success_count") {
			return addResult, fmt.Errorf("qbittorrent add failed: %s", text)
		}
	}
	return addResult, nil
}

func (c *Client) addOptionsForm(opts AddOptions) (url.Values, error) {
	values := url.Values{}
	if opts.SavePath != "" {
		values.Set("savepath", opts.SavePath)
	}
	if opts.Category != "" {
		values.Set("category", opts.Category)
	}
	if len(opts.Tags) > 0 {
		values.Set("tags", strings.Join(nonEmptyStrings(opts.Tags), ","))
	}
	if opts.SkipChecking {
		values.Set("skip_checking", "true")
	}
	if opts.Paused {
		values.Set("paused", "true")
	}
	if opts.RootFolder != nil {
		values.Set("root_folder", strconv.FormatBool(*opts.RootFolder))
	}
	if opts.Rename != "" {
		values.Set("rename", opts.Rename)
	}
	if opts.UploadLimit > 0 {
		values.Set("upLimit", strconv.Itoa(opts.UploadLimit))
	}
	if opts.DownloadLimit > 0 {
		values.Set("dlLimit", strconv.Itoa(opts.DownloadLimit))
	}
	if opts.RatioLimit != nil {
		values.Set("ratioLimit", strconv.FormatFloat(*opts.RatioLimit, 'f', -1, 64))
	}
	if opts.SeedingTimeLimit != nil {
		values.Set("seedingTimeLimit", strconv.Itoa(*opts.SeedingTimeLimit))
	}
	if opts.AutoTMM != nil {
		values.Set("autoTMM", strconv.FormatBool(*opts.AutoTMM))
	}
	if opts.SequentialDownload {
		values.Set("sequentialDownload", "true")
	}
	if opts.FirstLastPiecePrio {
		values.Set("firstLastPiecePrio", "true")
	}
	return values, nil
}

type torrentFile struct {
	Name string
	Data []byte
}
