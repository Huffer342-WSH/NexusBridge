package qbittorrent

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// Category 表示 qBittorrent 分类及其保存路径。
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// UnmarshalJSON 同时兼容 qBittorrent 返回的 savePath 和 save_path 字段。
func (c *Category) UnmarshalJSON(data []byte) error {
	var value struct {
		Name          string  `json:"name"`
		SavePath      *string `json:"savePath"`
		SavePathSnake *string `json:"save_path"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	c.Name = value.Name
	c.SavePath = ""
	if value.SavePath != nil {
		c.SavePath = *value.SavePath
	} else if value.SavePathSnake != nil {
		c.SavePath = *value.SavePathSnake
	}
	return nil
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
	for name, category := range categories {
		if strings.TrimSpace(category.Name) == "" {
			category.Name = name
			categories[name] = category
		}
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
