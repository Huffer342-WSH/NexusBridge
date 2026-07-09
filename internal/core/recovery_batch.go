package core

import (
	"context"
	"fmt"
	"strings"
)

const maxRecoveryBatchItems = 500

// RecoverFolders 串行恢复扫描阶段已经唯一确定的候选，并继续处理单项失败。
func (a *App) RecoverFolders(ctx context.Context, request RecoveryBatchRequest) (RecoveryBatchResult, error) {
	if len(request.Items) == 0 {
		return RecoveryBatchResult{}, fmt.Errorf("recovery batch items must not be empty")
	}
	if len(request.Items) > maxRecoveryBatchItems {
		return RecoveryBatchResult{}, fmt.Errorf("recovery batch items must not exceed %d", maxRecoveryBatchItems)
	}
	for index, item := range request.Items {
		if strings.TrimSpace(item.Path) == "" {
			return RecoveryBatchResult{}, fmt.Errorf("recovery batch item %d path is required", index)
		}
		if strings.TrimSpace(item.SiteID) == "" || strings.TrimSpace(item.TorrentID) == "" {
			return RecoveryBatchResult{}, fmt.Errorf("recovery batch item %d site_id and torrent_id are required", index)
		}
	}

	searchMode := RecoverySearchDatabase
	if request.WebSearch {
		searchMode = RecoverySearchDatabaseThenSite
	}
	result := RecoveryBatchResult{Items: make([]RecoveryBatchItemResult, 0, len(request.Items))}
	seenPaths := make(map[string]struct{}, len(request.Items))
	seenTorrents := make(map[string]struct{}, len(request.Items))
	for _, item := range request.Items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		pathKey := normalizedFilesystemPath(item.Path)
		torrentKey := strings.ToLower(strings.TrimSpace(item.SiteID)) + "\x00" + strings.TrimSpace(item.TorrentID)
		itemResult := RecoveryBatchItemResult{
			Path: strings.TrimSpace(item.Path), SiteID: strings.TrimSpace(item.SiteID), TorrentID: strings.TrimSpace(item.TorrentID),
		}
		if _, exists := seenPaths[pathKey]; exists {
			itemResult.Status = "skipped"
			itemResult.Error = "duplicate recovery path in batch"
			result.Skipped++
			result.Items = append(result.Items, itemResult)
			continue
		}
		if _, exists := seenTorrents[torrentKey]; exists {
			itemResult.Status = "skipped"
			itemResult.Error = "duplicate torrent candidate in batch"
			result.Skipped++
			result.Items = append(result.Items, itemResult)
			continue
		}
		seenPaths[pathKey] = struct{}{}
		seenTorrents[torrentKey] = struct{}{}
		result.Attempted++

		recovered, err := a.RecoverFolder(ctx, RecoveryRequest{
			Path: itemResult.Path, SiteID: itemResult.SiteID, TorrentID: itemResult.TorrentID,
			SearchMode: searchMode, Category: strings.TrimSpace(item.Category),
		})
		if err != nil {
			itemResult.Status = "failed"
			itemResult.Error = err.Error()
			result.Failed++
		} else if !recovered.Started {
			itemResult.Status = "needs_attention"
			itemResult.Error = recovered.VerificationError
			itemResult.Result = &recovered
			result.NeedsAttention++
		} else {
			itemResult.Status = "recovered"
			itemResult.Result = &recovered
			result.Recovered++
		}
		result.Items = append(result.Items, itemResult)
	}
	return result, nil
}
