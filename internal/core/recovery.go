package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

const (
	RecoverySearchDatabase         = "database"
	RecoverySearchSite             = "site"
	RecoverySearchDatabaseThenSite = "database_then_site"
	RecoverySearchURL              = "url"
	recoveryRecheckAttempts        = 3
	recoveryRecheckStartTimeout    = 5 * time.Second
	recoveryRecheckFinishTimeout   = 30 * time.Minute
	recoveryRecheckPollInterval    = 250 * time.Millisecond
)

type recoveryTarget struct {
	Path          string
	SearchName    string
	DiskFiles     map[string]int64
	DiskFilePaths map[string]string
	IsFile        bool
}

type recoveryFileMapping struct {
	TorrentPath string
	NewPath     string
	Size        int64
}

// PreviewRecovery 按保留文件的完整大小集合查找可恢复的 torrent，不修改 qB 状态。
func (a *App) PreviewRecovery(ctx context.Context, request RecoveryPreviewRequest) (RecoveryPreview, error) {
	target, err := scanRecoveryPath(request.Path)
	if err != nil {
		return RecoveryPreview{}, err
	}
	return a.previewRecoveryTarget(ctx, request, target)
}

func (a *App) previewRecoveryTarget(ctx context.Context, request RecoveryPreviewRequest, target recoveryTarget) (RecoveryPreview, error) {
	searchMode := RecoverySearchURL
	if strings.TrimSpace(request.TorrentURL) == "" {
		var err error
		searchMode, err = normalizeRecoverySearchMode(request.SearchMode)
		if err != nil {
			return RecoveryPreview{}, err
		}
	}
	preview := RecoveryPreview{
		Path: target.Path, FolderName: target.SearchName, SavePath: filepath.Dir(target.Path),
		SearchMode: searchMode, Matches: []RecoveryMatch{},
	}
	if searchMode == RecoverySearchURL {
		data, torrent, err := a.downloadRecoveryTorrentURL(ctx, request.TorrentURL, request.SiteIDs)
		if err != nil {
			return RecoveryPreview{}, err
		}
		preview.Evaluated = 1
		match, _, matched, err := a.recoveryMatchData(target, data, torrent, "url")
		if err != nil {
			return RecoveryPreview{}, err
		}
		if matched {
			preview.Matches = append(preview.Matches, match)
			preview.Source, preview.SavePath = "url", match.SavePath
		} else {
			preview.Source = "none"
		}
		preview.Category = a.recoveryCategory(ctx, request.Category, preview.SavePath)
		return preview, nil
	}
	requestedSiteIDs := trimmedRecoverySiteIDs(request.SiteIDs)
	seen := map[storage.TorrentKey]struct{}{}
	if searchMode != RecoverySearchSite {
		status, err := a.store.GetTorrentSizeIndexStatus(ctx)
		if err != nil {
			return RecoveryPreview{}, err
		}
		if status.Pending > 0 {
			return RecoveryPreview{}, fmt.Errorf("torrent size index is incomplete: %d of %d torrent files indexed; rebuild it first", status.Indexed, status.Total)
		}
		databaseFiles, err := a.store.ListTorrentFilesBySizes(ctx, recoveryTargetSizeCounts(target), requestedSiteIDs)
		if err != nil {
			return RecoveryPreview{}, err
		}
		for _, file := range databaseFiles {
			key := storage.TorrentKey{SiteID: file.SiteID, TorrentID: file.TorrentID}
			seen[key] = struct{}{}
			preview.Evaluated++
			match, _, matched, err := a.recoveryMatch(ctx, target, file, "database")
			if err != nil {
				return RecoveryPreview{}, err
			}
			if matched {
				preview.Matches = append(preview.Matches, match)
			}
		}
		if len(preview.Matches) > 0 || searchMode == RecoverySearchDatabase {
			preview.Source = "database"
			sortRecoveryMatches(preview.Matches)
			if len(preview.Matches) == 0 {
				preview.Source = "none"
			} else {
				preview.SavePath = preview.Matches[0].SavePath
			}
			preview.Category = a.recoveryCategory(ctx, request.Category, preview.SavePath)
			return preview, nil
		}
	}
	siteIDs, err := a.recoverySiteIDs(requestedSiteIDs)
	if err != nil {
		return RecoveryPreview{}, err
	}
	for _, siteID := range siteIDs {
		torrents, attempt := a.searchRecoverySite(ctx, siteID, preview.FolderName, recoveryTargetTotalSize(target))
		preview.SearchAttempts = append(preview.SearchAttempts, attempt)
		for _, torrent := range torrents {
			key := storage.TorrentKey{SiteID: torrent.SiteID, TorrentID: torrent.ID}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			file, ok, err := a.store.GetTorrentFile(ctx, key)
			if err != nil {
				return RecoveryPreview{}, err
			}
			if !ok || !file.HasData {
				continue
			}
			preview.Evaluated++
			match, _, matched, err := a.recoveryMatch(ctx, target, file, "site_search")
			if err != nil {
				return RecoveryPreview{}, err
			}
			if matched {
				preview.Matches = append(preview.Matches, match)
			}
		}
	}
	if len(preview.Matches) > 0 {
		preview.Source = "site_search"
	} else {
		preview.Source = "none"
	}
	sortRecoveryMatches(preview.Matches)
	if len(preview.Matches) > 0 {
		preview.SavePath = preview.Matches[0].SavePath
	}
	preview.Category = a.recoveryCategory(ctx, request.Category, preview.SavePath)
	return preview, nil
}

// RecoverFolder 按完整文件大小集合恢复 qB 任务，并只重命名 qB 内部文件路径。
func (a *App) RecoverFolder(ctx context.Context, request RecoveryRequest) (RecoveryResult, error) {
	previewRequest := RecoveryPreviewRequest{
		Path: request.Path, SearchMode: request.SearchMode, TorrentURL: request.TorrentURL, Category: request.Category,
	}
	if siteID := strings.TrimSpace(request.SiteID); siteID != "" {
		previewRequest.SiteIDs = []string{siteID}
	}
	preview, err := a.PreviewRecovery(ctx, previewRequest)
	if err != nil {
		return RecoveryResult{}, err
	}
	torrentID := strings.TrimSpace(request.TorrentID)
	matches := make([]RecoveryMatch, 0, len(preview.Matches))
	for _, match := range preview.Matches {
		if torrentID == "" || match.Torrent.ID == torrentID {
			matches = append(matches, match)
		}
	}
	if len(matches) == 0 {
		return RecoveryResult{}, fmt.Errorf("no torrent matches recovery folder %s", preview.Path)
	}
	if len(matches) > 1 {
		keys := make([]string, 0, len(matches))
		for _, match := range matches {
			keys = append(keys, match.Torrent.SiteID+"/"+match.Torrent.ID)
		}
		return RecoveryResult{}, fmt.Errorf("multiple torrents match recovery folder %s: %s", preview.Path, strings.Join(keys, ", "))
	}
	selected := matches[0]
	target, err := scanRecoveryPath(request.Path)
	if err != nil {
		return RecoveryResult{}, err
	}
	var data []byte
	var match RecoveryMatch
	var mappings []recoveryFileMapping
	var matched bool
	if strings.TrimSpace(request.TorrentURL) != "" {
		var torrent Torrent
		data, torrent, err = a.downloadRecoveryTorrentURL(ctx, request.TorrentURL, previewRequest.SiteIDs)
		if err == nil {
			match, mappings, matched, err = a.recoveryMatchData(target, data, torrent, "url")
		}
	} else {
		key := storage.TorrentKey{SiteID: selected.Torrent.SiteID, TorrentID: selected.Torrent.ID}
		var file storage.TorrentFileRecord
		var ok bool
		file, ok, err = a.store.GetTorrentFile(ctx, key)
		if err == nil && (!ok || !file.HasData) {
			err = fmt.Errorf("torrent file %s/%s is not persisted", key.SiteID, key.TorrentID)
		}
		if err == nil {
			data = file.Data
			match, mappings, matched, err = a.recoveryMatch(ctx, target, file, selected.Source)
		}
	}
	if err != nil {
		return RecoveryResult{}, err
	}
	if !matched {
		return RecoveryResult{}, fmt.Errorf("recovery files no longer match the selected torrent file sizes")
	}
	metadata, err := qbittorrent.ParseTorrentMetadata(data)
	if err != nil {
		return RecoveryResult{}, err
	}
	if metadata.Hashes.V1 == "" {
		return RecoveryResult{}, fmt.Errorf("pure v2 torrent recovery is not supported")
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return RecoveryResult{}, err
	}
	if _, exists, err := qb.FindTorrentByHash(ctx, metadata.Hashes.V1); err != nil {
		return RecoveryResult{}, err
	} else if exists {
		return RecoveryResult{}, fmt.Errorf("qbittorrent torrent %s already exists", metadata.Hashes.V1)
	}
	rootFolder, autoTMM := false, false
	qbCfg := a.effectiveQBConfig(ctx)
	category := firstNonEmpty(a.recoveryCategoryForExecution(ctx, request.Category, match.SavePath), qbCfg.Category)
	added, err := qb.AddTorrentFileVerifiedResult(ctx, qbittorrent.AddOptions{
		Name: torrentFileName(match.Torrent), Data: data, SavePath: match.SavePath,
		Category: category, Tags: qbCfg.Tags, Paused: true, SkipChecking: true, RootFolder: &rootFolder, AutoTMM: &autoTMM,
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	if !added.Added {
		return RecoveryResult{}, fmt.Errorf("qbittorrent torrent %s already exists", metadata.Hashes.V1)
	}
	hash := firstNonEmpty(added.Torrent.Hash, metadata.Hashes.V1)
	if category != "" {
		if err := qb.SetTorrentCategory(ctx, []string{hash}, category); err != nil {
			if cleanupErr := qb.DeleteTorrents(ctx, []string{hash}, false); cleanupErr != nil {
				return RecoveryResult{}, fmt.Errorf("set recovered torrent category %s: %w; cleanup failed: %v", category, err, cleanupErr)
			}
			return RecoveryResult{}, fmt.Errorf("set recovered torrent category %s: %w", category, err)
		}
	}
	if len(mappings) > 0 {
		if err := a.applyRecoveryFileMappings(ctx, qb, hash, mappings); err != nil {
			if cleanupErr := qb.DeleteTorrents(ctx, []string{hash}, false); cleanupErr != nil {
				return RecoveryResult{}, fmt.Errorf("configure recovered torrent files: %w; cleanup failed: %v", err, cleanupErr)
			}
			return RecoveryResult{}, fmt.Errorf("configure recovered torrent files: %w", err)
		}
	}
	qbTorrent, attempts, verifyErr := verifyRecoveredTorrent(ctx, qb, hash)
	verificationStatus := "verified"
	started := false
	canStart, canDelete := false, false
	if verifyErr != nil {
		verificationStatus = "failed"
		canStart, canDelete = true, true
		_ = qb.StopTorrents(ctx, []string{hash})
		if current, found, findErr := qb.FindTorrentByHash(ctx, hash); findErr == nil && found {
			qbTorrent = current
		}
	} else {
		if err := qb.StartTorrents(ctx, []string{hash}); err != nil {
			verificationStatus = "start_failed"
			verifyErr = fmt.Errorf("start verified recovered torrent: %w", err)
			canStart, canDelete = true, true
		} else {
			started = true
			if current, found, findErr := qb.FindTorrentByHash(ctx, hash); findErr == nil && found {
				qbTorrent = current
			}
		}
	}
	if qbTorrent.Hash == "" {
		qbTorrent = added.Torrent
		qbTorrent.Hash = hash
	}
	status := statusFromQBTorrent(qbTorrent, storage.DownloadTaskRecord{}, "recovery", time.Now())
	if selected.Source != "url" {
		if _, err := a.refreshTorrentQBSnapshot(ctx, match.Torrent, false); err != nil && ctx.Err() == nil {
			// qB 恢复已成功，快照失败不回滚新任务。
			status.Error = err.Error()
		}
	}
	return RecoveryResult{
		Path: target.Path, SavePath: match.SavePath, Category: category, Match: match, QBStatus: status,
		VerificationStatus: verificationStatus, VerificationError: recoveryErrorString(verifyErr), RecheckAttempts: attempts,
		Started: started, CanStart: canStart, CanDelete: canDelete,
	}, nil
}

// ControlRecoveryTorrent 对校验失败后保留的唯一 qB 任务执行显式启动或删除。
