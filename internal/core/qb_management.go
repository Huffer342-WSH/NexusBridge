package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidQBManagement 表示管理请求参数无效。
var ErrInvalidQBManagement = errors.New("invalid qB management request")

// QBManagementRequest 指定分类或目标目录，两者不可同时提供。
type QBManagementRequest struct {
	Category *string `json:"category,omitempty"`
	Location *string `json:"location,omitempty"`
}

// ManageQBTorrent 将分类与目录变更交给 qB，不直接操作磁盘文件。
func (a *App) ManageQBTorrent(ctx context.Context, hash string, request QBManagementRequest) error {
	hash, err := normalizeQBTorrentHash(hash)
	if err != nil {
		return err
	}
	if (request.Category == nil) == (request.Location == nil) {
		return fmt.Errorf("%w: specify category or location", ErrInvalidQBManagement)
	}
	if request.Location != nil {
		location := strings.TrimSpace(*request.Location)
		if location == "" || strings.ContainsAny(location, "\x00\r\n") || !(strings.HasPrefix(location, "/") || strings.HasPrefix(location, `\\`) || (len(location) > 2 && location[1] == ':' && (location[2] == 92 || location[2] == '/'))) {
			return fmt.Errorf("%w: absolute qB save path required", ErrInvalidQBManagement)
		}
		request.Location = &location
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return err
	}
	if _, found, err := qb.FindTorrentByHash(ctx, hash); err != nil {
		return err
	} else if !found {
		return ErrQBTorrentNotFound
	}
	defer func() { a.qbPollMu.Lock(); a.qbPollLastAt = time.Time{}; a.qbPollMu.Unlock() }()
	if request.Category != nil {
		category := *request.Category
		if category != "" {
			categories, err := qb.GetCategories(ctx)
			if err != nil {
				return err
			}
			if _, ok := categories[category]; !ok {
				return fmt.Errorf("%w: category does not exist", ErrInvalidQBManagement)
			}
		}
		if err := qb.SetTorrentCategory(ctx, []string{hash}, category); err != nil {
			return err
		}
		if err := qb.SetTorrentAutoManagement(ctx, hash, true); err != nil {
			return fmt.Errorf("分类已修改，但启用自动管理失败，请检查 qB 实际状态: %w", err)
		}
		return nil
	}
	if err := qb.SetTorrentAutoManagement(ctx, hash, false); err != nil {
		return err
	}
	if err := qb.SetTorrentLocation(ctx, hash, *request.Location); err != nil {
		return fmt.Errorf("任务已切换为手动管理，但移动请求失败: %w", err)
	}
	return nil
}
