package core

import (
	"context"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"nexusbridge/internal/parser"
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
	searchMode := RecoverySearchURL
	if strings.TrimSpace(request.TorrentURL) == "" {
		var err error
		searchMode, err = normalizeRecoverySearchMode(request.SearchMode)
		if err != nil {
			return RecoveryPreview{}, err
		}
	}
	target, err := scanRecoveryPath(request.Path)
	if err != nil {
		return RecoveryPreview{}, err
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
func (a *App) ControlRecoveryTorrent(ctx context.Context, hash, action string) (RecoveryActionResult, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(hash) {
		return RecoveryActionResult{}, fmt.Errorf("invalid recovery torrent hash")
	}
	qb, err := a.qbClient(ctx)
	if err != nil {
		return RecoveryActionResult{}, err
	}
	if _, found, err := qb.FindTorrentByHash(ctx, hash); err != nil {
		return RecoveryActionResult{}, err
	} else if !found {
		return RecoveryActionResult{}, fmt.Errorf("qbittorrent torrent %s does not exist", hash)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	result := RecoveryActionResult{Hash: hash, Action: action}
	switch action {
	case "start":
		if err := qb.StartTorrents(ctx, []string{hash}); err != nil {
			return RecoveryActionResult{}, err
		}
		torrent, found, err := qb.FindTorrentByHash(ctx, hash)
		if err != nil {
			return RecoveryActionResult{}, err
		}
		if found {
			status := statusFromQBTorrent(torrent, storage.DownloadTaskRecord{}, "recovery_action", time.Now())
			result.QBStatus = &status
		}
	case "delete":
		if err := qb.DeleteTorrents(ctx, []string{hash}, false); err != nil {
			return RecoveryActionResult{}, err
		}
		result.Deleted = true
	default:
		return RecoveryActionResult{}, fmt.Errorf("unsupported recovery action %q", action)
	}
	return result, nil
}

// verifyRecoveredTorrent 重试触发强制校验，并只接受暂停且完整的校验结果。
func verifyRecoveredTorrent(ctx context.Context, qb *qbittorrent.Client, hash string) (qbittorrent.TorrentInfo, int, error) {
	var last qbittorrent.TorrentInfo
	var lastErr error
	for attempt := 1; attempt <= recoveryRecheckAttempts; attempt++ {
		if err := qb.RecheckTorrents(ctx, []string{hash}); err != nil {
			lastErr = fmt.Errorf("trigger recheck attempt %d: %w", attempt, err)
		} else {
			torrent, started, err := waitRecoveryRecheckStarted(ctx, qb, hash)
			last = torrent
			if err != nil {
				lastErr = err
			} else if started {
				checked, err := waitRecoveryRecheckFinished(ctx, qb, hash)
				last = checked
				if err != nil {
					return last, attempt, err
				}
				if err := qb.StopTorrents(ctx, []string{hash}); err != nil {
					return last, attempt, fmt.Errorf("pause checked torrent: %w", err)
				}
				paused, err := waitRecoveryTorrentStopped(ctx, qb, hash)
				if err != nil {
					return last, attempt, err
				}
				if !recoveryTorrentComplete(paused) {
					return paused, attempt, fmt.Errorf("recheck completed but torrent is incomplete: state=%s progress=%.6f amount_left=%d", paused.State, paused.Progress, paused.AmountLeft)
				}
				return paused, attempt, nil
			} else {
				lastErr = fmt.Errorf("recheck attempt %d did not enter a checking state", attempt)
			}
		}
		if attempt < recoveryRecheckAttempts {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return last, attempt, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return last, recoveryRecheckAttempts, lastErr
}

// waitRecoveryRecheckStarted 等待 qB 确实进入 checking 状态。
func waitRecoveryRecheckStarted(ctx context.Context, qb *qbittorrent.Client, hash string) (qbittorrent.TorrentInfo, bool, error) {
	deadline := time.Now().Add(recoveryRecheckStartTimeout)
	var last qbittorrent.TorrentInfo
	for time.Now().Before(deadline) {
		torrent, found, err := qb.FindTorrentByHash(ctx, hash)
		if err != nil {
			return last, false, err
		}
		if !found {
			return last, false, fmt.Errorf("recovered torrent disappeared before recheck")
		}
		last = torrent
		if recoveryTorrentChecking(torrent.State) {
			return torrent, true, nil
		}
		if err := waitRecoveryPoll(ctx); err != nil {
			return last, false, err
		}
	}
	return last, false, nil
}

// waitRecoveryRecheckFinished 等待已开始的强制校验结束。
func waitRecoveryRecheckFinished(ctx context.Context, qb *qbittorrent.Client, hash string) (qbittorrent.TorrentInfo, error) {
	deadline := time.Now().Add(recoveryRecheckFinishTimeout)
	for time.Now().Before(deadline) {
		torrent, found, err := qb.FindTorrentByHash(ctx, hash)
		if err != nil {
			return qbittorrent.TorrentInfo{}, err
		}
		if !found {
			return qbittorrent.TorrentInfo{}, fmt.Errorf("recovered torrent disappeared during recheck")
		}
		if !recoveryTorrentChecking(torrent.State) {
			return torrent, nil
		}
		if err := waitRecoveryPoll(ctx); err != nil {
			return torrent, err
		}
	}
	return qbittorrent.TorrentInfo{}, fmt.Errorf("recheck did not finish within %s", recoveryRecheckFinishTimeout)
}

// waitRecoveryTorrentStopped 等待校验后的任务回到暂停状态。
func waitRecoveryTorrentStopped(ctx context.Context, qb *qbittorrent.Client, hash string) (qbittorrent.TorrentInfo, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		torrent, found, err := qb.FindTorrentByHash(ctx, hash)
		if err != nil {
			return qbittorrent.TorrentInfo{}, err
		}
		if !found {
			return qbittorrent.TorrentInfo{}, fmt.Errorf("recovered torrent disappeared after recheck")
		}
		state := strings.ToLower(torrent.State)
		if strings.HasPrefix(state, "paused") || strings.HasPrefix(state, "stopped") {
			return torrent, nil
		}
		if err := waitRecoveryPoll(ctx); err != nil {
			return torrent, err
		}
	}
	return qbittorrent.TorrentInfo{}, fmt.Errorf("checked torrent did not enter a stopped state")
}

// recoveryTorrentChecking 判断 qB 状态是否正在校验。
func recoveryTorrentChecking(state string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(state)), "checking")
}

// recoveryTorrentComplete 判断任务是否已校验完成且保持暂停。
func recoveryTorrentComplete(torrent qbittorrent.TorrentInfo) bool {
	state := strings.ToLower(strings.TrimSpace(torrent.State))
	return torrent.Progress >= 1 && torrent.AmountLeft == 0 && (state == "pausedup" || state == "stoppedup")
}

// waitRecoveryPoll 等待下一次恢复状态轮询。
func waitRecoveryPoll(ctx context.Context) error {
	timer := time.NewTimer(recoveryRecheckPollInterval)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// recoveryErrorString 把可选校验错误转换为响应文本。
func recoveryErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// normalizeRecoverySearchMode 校验恢复检索模式，空值使用数据库优先模式。
func normalizeRecoverySearchMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return RecoverySearchDatabaseThenSite, nil
	}
	switch value {
	case RecoverySearchDatabase, RecoverySearchSite, RecoverySearchDatabaseThenSite:
		return value, nil
	default:
		return "", fmt.Errorf("invalid recovery search_mode %q", value)
	}
}

// recoveryCategory 使用本地分类快照为预览快速推断分类。
func (a *App) recoveryCategory(ctx context.Context, requested, savePath string) string {
	return a.recoveryCategoryFromCatalog(ctx, requested, savePath, false)
}

// recoveryCategoryForExecution 刷新 qB 分类后按最终保存目录推断恢复分类。
func (a *App) recoveryCategoryForExecution(ctx context.Context, requested, savePath string) string {
	return a.recoveryCategoryFromCatalog(ctx, requested, savePath, true)
}

// recoveryCategoryFromCatalog 按分类的有效保存目录精确匹配最深层分类。
func (a *App) recoveryCategoryFromCatalog(ctx context.Context, requested, savePath string, refresh bool) string {
	if category := strings.TrimSpace(requested); category != "" {
		return category
	}
	if strings.TrimSpace(savePath) == "" {
		return ""
	}
	categories, err := a.GetQBCategories(ctx, refresh)
	if err != nil {
		return ""
	}
	defaultSavePath := ""
	needsDefaultSavePath := false
	for _, category := range categories.Items {
		if strings.TrimSpace(category.SavePath) == "" && !qbCategoryHasExplicitParentPath(category, categories.Items) {
			needsDefaultSavePath = true
			break
		}
	}
	if needsDefaultSavePath {
		if qb, qbErr := a.qbClient(ctx); qbErr == nil {
			defaultSavePath, _ = qb.GetDefaultSavePath(ctx)
		}
	}
	matchedName := ""
	matchedDepth := -1
	for _, category := range categories.Items {
		effectivePath := qbCategoryEffectiveSavePath(category, categories.Items, defaultSavePath)
		if effectivePath == "" || !sameFilesystemPath(effectivePath, savePath) {
			continue
		}
		depth := len(qbCategorySegments(category.Name))
		if depth > matchedDepth {
			matchedName = category.Name
			matchedDepth = depth
		}
	}
	return matchedName
}

// qbCategoryEffectiveSavePath 解析分类自身、最近父分类或全局默认目录继承后的有效保存路径。
func qbCategoryEffectiveSavePath(category QBCategory, categories []QBCategory, defaultSavePath string) string {
	if savePath := strings.TrimSpace(category.SavePath); savePath != "" {
		return filepath.Clean(savePath)
	}
	segments := qbCategorySegments(category.Name)
	if len(segments) == 0 {
		return ""
	}
	byName := make(map[string]QBCategory, len(categories))
	for _, candidate := range categories {
		name := strings.ToLower(strings.Join(qbCategorySegments(candidate.Name), "/"))
		if name != "" {
			byName[name] = candidate
		}
	}
	for depth := len(segments) - 1; depth > 0; depth-- {
		parent, exists := byName[strings.ToLower(strings.Join(segments[:depth], "/"))]
		if !exists || strings.TrimSpace(parent.SavePath) == "" {
			continue
		}
		parts := append([]string{strings.TrimSpace(parent.SavePath)}, segments[depth:]...)
		return filepath.Clean(filepath.Join(parts...))
	}
	if strings.TrimSpace(defaultSavePath) == "" {
		return ""
	}
	parts := append([]string{strings.TrimSpace(defaultSavePath)}, segments...)
	return filepath.Clean(filepath.Join(parts...))
}

// qbCategoryHasExplicitParentPath 判断空路径分类能否从任一父分类继承保存目录。
func qbCategoryHasExplicitParentPath(category QBCategory, categories []QBCategory) bool {
	segments := qbCategorySegments(category.Name)
	if len(segments) < 2 {
		return false
	}
	parents := make(map[string]struct{}, len(categories))
	for _, candidate := range categories {
		if strings.TrimSpace(candidate.SavePath) == "" {
			continue
		}
		parents[strings.ToLower(strings.Join(qbCategorySegments(candidate.Name), "/"))] = struct{}{}
	}
	for depth := len(segments) - 1; depth > 0; depth-- {
		if _, exists := parents[strings.ToLower(strings.Join(segments[:depth], "/"))]; exists {
			return true
		}
	}
	return false
}

// qbCategorySegments 返回去除空白层级后的 qB 分类路径片段。
func qbCategorySegments(name string) []string {
	segments := make([]string, 0, strings.Count(name, "/")+1)
	for _, segment := range strings.Split(name, "/") {
		if segment = strings.TrimSpace(segment); segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func trimmedRecoverySiteIDs(siteIDs []string) []string {
	result := make([]string, 0, len(siteIDs))
	seen := map[string]struct{}{}
	for _, siteID := range siteIDs {
		siteID = strings.TrimSpace(siteID)
		if siteID == "" {
			continue
		}
		if _, exists := seen[siteID]; exists {
			continue
		}
		seen[siteID] = struct{}{}
		result = append(result, siteID)
	}
	return result
}

func (a *App) recoveryMatch(ctx context.Context, target recoveryTarget, file storage.TorrentFileRecord, source string) (RecoveryMatch, []recoveryFileMapping, bool, error) {
	torrent, err := a.getTorrent(ctx, file.SiteID, file.TorrentID)
	if err != nil {
		return RecoveryMatch{}, nil, false, err
	}
	return a.recoveryMatchData(target, file.Data, torrent, source)
}

// recoveryMatchData 按完整文件大小多重集合匹配，并生成不改动磁盘的 qB 文件路径映射。
func (a *App) recoveryMatchData(target recoveryTarget, data []byte, torrent Torrent, source string) (RecoveryMatch, []recoveryFileMapping, bool, error) {
	metadata, err := qbittorrent.ParseTorrentMetadata(data)
	if err != nil {
		return RecoveryMatch{}, nil, false, err
	}
	type torrentFile struct {
		path       string
		normalized string
		size       int64
	}
	torrentFiles := make([]torrentFile, 0, len(metadata.Files))
	var totalSize int64
	for _, item := range metadata.Files {
		if item.Padding {
			continue
		}
		normalized, ok := normalizeRecoveryRelativePath(item.Path)
		if !ok {
			return RecoveryMatch{}, nil, false, nil
		}
		torrentFiles = append(torrentFiles, torrentFile{path: filepath.ToSlash(item.Path), normalized: normalized, size: item.Size})
		totalSize += item.Size
	}
	if len(target.DiskFiles) == 0 || len(target.DiskFiles) != len(torrentFiles) {
		return RecoveryMatch{}, nil, false, nil
	}
	if target.IsFile && len(torrentFiles) != 1 {
		return RecoveryMatch{}, nil, false, nil
	}

	diskPaths := make([]string, 0, len(target.DiskFiles))
	for path := range target.DiskFiles {
		diskPaths = append(diskPaths, path)
	}
	sort.Strings(diskPaths)
	used := make(map[int]struct{}, len(diskPaths))
	selectedByDisk := make(map[string]int, len(diskPaths))
	assignUnique := func(matches func(string, torrentFile) bool) bool {
		changed := false
		for _, diskPath := range diskPaths {
			if _, assigned := selectedByDisk[diskPath]; assigned {
				continue
			}
			candidates := make([]int, 0)
			for index, item := range torrentFiles {
				if _, exists := used[index]; !exists && item.size == target.DiskFiles[diskPath] && matches(diskPath, item) {
					candidates = append(candidates, index)
				}
			}
			if len(candidates) == 1 {
				selectedByDisk[diskPath] = candidates[0]
				used[candidates[0]] = struct{}{}
				changed = true
			}
		}
		return changed
	}
	assignUnique(func(diskPath string, item torrentFile) bool { return item.normalized == diskPath })
	assignUnique(func(diskPath string, item torrentFile) bool {
		return recoveryNameEqual(pathpkg.Base(item.normalized), pathpkg.Base(diskPath))
	})
	for assignUnique(func(_ string, _ torrentFile) bool { return true }) {
	}
	// 没有路径或文件名线索时，同尺寸文件仍可建立稳定的一一映射；最终由 qB 强制校验确认内容。
	for _, diskPath := range diskPaths {
		if _, assigned := selectedByDisk[diskPath]; assigned {
			continue
		}
		for index, item := range torrentFiles {
			if _, exists := used[index]; !exists && item.size == target.DiskFiles[diskPath] {
				selectedByDisk[diskPath] = index
				used[index] = struct{}{}
				break
			}
		}
	}
	if len(selectedByDisk) != len(diskPaths) {
		return RecoveryMatch{}, nil, false, nil
	}

	mappings := make([]recoveryFileMapping, 0, len(torrentFiles))
	for _, diskPath := range diskPaths {
		selected := selectedByDisk[diskPath]
		size := target.DiskFiles[diskPath]
		originalDiskPath := firstNonEmpty(target.DiskFilePaths[diskPath], diskPath)
		newPath := originalDiskPath
		if !target.IsFile {
			newPath = filepath.ToSlash(filepath.Join(filepath.Base(target.Path), filepath.FromSlash(originalDiskPath)))
		}
		if normalized, ok := normalizeRecoveryRelativePath(newPath); !ok || normalized == "" {
			return RecoveryMatch{}, nil, false, nil
		}
		mappings = append(mappings, recoveryFileMapping{
			TorrentPath: torrentFiles[selected].path, NewPath: filepath.ToSlash(newPath), Size: size,
		})
	}
	return RecoveryMatch{
		Torrent: torrent, Source: source, OriginalName: metadata.Name,
		SavePath: filepath.Dir(target.Path), RootFolder: false,
		InfoHashV1: metadata.Hashes.V1, InfoHashV2: metadata.Hashes.V2,
		FileCount: len(torrentFiles), TotalSize: totalSize, MatchMethod: "size", MappingComplete: true,
	}, mappings, true, nil
}

func recoveryTargetSizeCounts(target recoveryTarget) map[int64]int {
	result := make(map[int64]int, len(target.DiskFiles))
	for _, size := range target.DiskFiles {
		result[size]++
	}
	return result
}

func recoveryTargetTotalSize(target recoveryTarget) int64 {
	var result int64
	for _, size := range target.DiskFiles {
		result += size
	}
	return result
}

func uniqueRecoveryCandidate(candidates []int, matches func(int) bool) int {
	selected := -1
	for _, candidate := range candidates {
		if !matches(candidate) {
			continue
		}
		if selected >= 0 {
			return -1
		}
		selected = candidate
	}
	return selected
}

// applyRecoveryFileMappings 使用 qB 返回的完整相对路径直接重命名文件。
func (a *App) applyRecoveryFileMappings(ctx context.Context, qb *qbittorrent.Client, hash string, mappings []recoveryFileMapping) error {
	contents, err := waitRecoveryTorrentContents(ctx, qb, hash)
	if err != nil {
		return err
	}
	used := make(map[int]struct{}, len(mappings))
	type rename struct {
		contentIndex int
		oldPath      string
		newPath      string
	}
	renames := make([]rename, 0, len(mappings))
	newPaths := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		selected := selectRecoveryQBContent(mapping, contents, used)
		if selected < 0 {
			return fmt.Errorf("cannot identify qB file for torrent path %s", mapping.TorrentPath)
		}
		used[selected] = struct{}{}
		oldPath := filepath.ToSlash(contents[selected].Name)
		newNormalized, newOK := normalizeRecoveryRelativePath(mapping.NewPath)
		if !newOK {
			return fmt.Errorf("invalid mapped qB file path %s", mapping.NewPath)
		}
		if _, exists := newPaths[newNormalized]; exists {
			return fmt.Errorf("multiple existing files map to qB path %s", mapping.NewPath)
		}
		newPaths[newNormalized] = struct{}{}
		if oldNormalized, oldOK := normalizeRecoveryRelativePath(oldPath); oldOK {
			if oldNormalized == newNormalized {
				continue
			}
		}
		renames = append(renames, rename{contentIndex: selected, oldPath: oldPath, newPath: mapping.NewPath})
	}
	for _, item := range renames {
		newPath, _ := normalizeRecoveryRelativePath(item.newPath)
		for index, content := range contents {
			if index == item.contentIndex {
				continue
			}
			contentPath, ok := normalizeRecoveryRelativePath(content.Name)
			if ok && contentPath == newPath {
				return fmt.Errorf("mapped qB path conflicts with torrent file %s", content.Name)
			}
		}
	}
	for _, item := range renames {
		if err := qb.RenameTorrentFile(ctx, hash, item.oldPath, item.newPath); err != nil {
			return fmt.Errorf("rename %s to %s: %w", item.oldPath, item.newPath, err)
		}
		if err := waitRecoveryTorrentFilePath(ctx, qb, hash, item.newPath); err != nil {
			return fmt.Errorf("confirm renamed qB file %s: %w", item.newPath, err)
		}
	}
	return nil
}

// waitRecoveryTorrentFilePath 等待 qB 文件列表反映重命名结果。
func waitRecoveryTorrentFilePath(ctx context.Context, qb *qbittorrent.Client, hash, expectedPath string) error {
	expected, ok := normalizeRecoveryRelativePath(expectedPath)
	if !ok {
		return fmt.Errorf("invalid qB file path %s", expectedPath)
	}
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		contents, err := qb.GetTorrentContents(ctx, hash, nil)
		if err == nil {
			for _, content := range contents {
				if actual, valid := normalizeRecoveryRelativePath(content.Name); valid && actual == expected {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("timed out waiting for qB file path %s", expectedPath)
}

func waitRecoveryTorrentContents(ctx context.Context, qb *qbittorrent.Client, hash string) ([]qbittorrent.TorrentContent, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		contents, err := qb.GetTorrentContents(ctx, hash, nil)
		if err == nil && len(contents) > 0 {
			return contents, nil
		}
		lastErr = err
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("wait for qB torrent files: %w", lastErr)
	}
	return nil, fmt.Errorf("wait for qB torrent files timed out")
}

func selectRecoveryQBContent(mapping recoveryFileMapping, contents []qbittorrent.TorrentContent, used map[int]struct{}) int {
	torrentPath, _ := normalizeRecoveryRelativePath(mapping.TorrentPath)
	candidates := make([]int, 0)
	for index, content := range contents {
		if _, exists := used[index]; exists || content.Size != mapping.Size {
			continue
		}
		candidates = append(candidates, index)
	}
	selected := uniqueRecoveryCandidate(candidates, func(index int) bool {
		contentPath, ok := normalizeRecoveryRelativePath(contents[index].Name)
		return ok && (contentPath == torrentPath || strings.HasSuffix(contentPath, "/"+torrentPath))
	})
	if selected < 0 && len(candidates) == 1 {
		selected = candidates[0]
	}
	return selected
}

func scanRecoveryPath(value string) (recoveryTarget, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return recoveryTarget{}, fmt.Errorf("recovery path is required")
	}
	target, err := filepath.Abs(value)
	if err != nil {
		return recoveryTarget{}, err
	}
	target = filepath.Clean(target)
	info, err := os.Stat(target)
	if err != nil {
		return recoveryTarget{}, fmt.Errorf("inspect recovery path: %w", err)
	}
	if info.Mode().IsRegular() {
		relative, ok := normalizeRecoveryRelativePath(filepath.Base(target))
		if !ok {
			return recoveryTarget{}, fmt.Errorf("invalid recovery file name: %s", filepath.Base(target))
		}
		return recoveryTarget{
			Path: target, SearchName: filepath.Base(target), DiskFiles: map[string]int64{relative: info.Size()},
			DiskFilePaths: map[string]string{relative: filepath.Base(target)}, IsFile: true,
		}, nil
	}
	if !info.IsDir() {
		return recoveryTarget{}, fmt.Errorf("recovery path must be a regular file or directory")
	}
	files := map[string]int64{}
	filePaths := map[string]string{}
	err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("recovery path contains non-regular file: %s", path)
		}
		relative, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		normalized, ok := normalizeRecoveryRelativePath(filepath.ToSlash(relative))
		if !ok {
			return fmt.Errorf("invalid recovery relative path: %s", relative)
		}
		files[normalized] = info.Size()
		filePaths[normalized] = filepath.ToSlash(relative)
		return nil
	})
	if err != nil {
		return recoveryTarget{}, err
	}
	return recoveryTarget{Path: target, SearchName: filepath.Base(target), DiskFiles: files, DiskFilePaths: filePaths}, nil
}

func normalizeRecoveryRelativePath(value string) (string, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") {
		return "", false
	}
	cleaned := pathpkg.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, true
}

func recoveryNameEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (a *App) recoverySiteIDs(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return append([]string(nil), a.siteIDs...), nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(requested))
	for _, siteID := range requested {
		siteID = strings.TrimSpace(siteID)
		if siteID == "" {
			continue
		}
		if _, err := a.findSite(siteID); err != nil {
			return nil, err
		}
		if _, exists := seen[siteID]; exists {
			continue
		}
		seen[siteID] = struct{}{}
		result = append(result, siteID)
	}
	return result, nil
}

func (a *App) searchRecoverySite(ctx context.Context, siteID, keyword string, targetTotalSize int64) ([]Torrent, RecoverySearchAttempt) {
	attempt := RecoverySearchAttempt{SiteID: siteID, Status: "failed"}
	site, err := a.findSite(siteID)
	if err != nil {
		attempt.Error = err.Error()
		return nil, attempt
	}
	records := make([]storage.TorrentRecord, 0)
	seen := map[storage.TorrentKey]struct{}{}
	attempted := make([]string, 0)
	errors := make([]string, 0)
	succeeded := 0
	for _, candidateKeyword := range recoverySearchKeywords(keyword) {
		attempted = append(attempted, candidateKeyword)
		targetURL, err := recoverySearchURL(site, candidateKeyword)
		if err != nil {
			errors = append(errors, candidateKeyword+": "+err.Error())
			continue
		}
		result, err := a.fetchSiteResource(ctx, site, targetURL, siteRequestOptions{
			Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
		})
		if err != nil {
			errors = append(errors, candidateKeyword+": "+err.Error())
			continue
		}
		parsed, err := parser.ParsePageWithDefinition(result.Body, site.Definition)
		if err != nil {
			errors = append(errors, candidateKeyword+": "+err.Error())
			continue
		}
		succeeded++
		for index, entry := range parsed.Torrents {
			record := recordFromParser(entry, index)
			key := storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			records = append(records, record)
		}
	}
	attempt.Keyword = strings.Join(attempted, " | ")
	if succeeded == 0 {
		attempt.Error = strings.Join(errors, "; ")
		return nil, attempt
	}
	if _, err := a.store.UpsertTorrents(ctx, records); err != nil {
		attempt.Error = err.Error()
		return nil, attempt
	}
	allTorrents := make([]Torrent, 0, len(records))
	torrents := make([]Torrent, 0, len(records))
	a.mu.Lock()
	for _, record := range records {
		torrent := torrentFromRecord(record)
		allTorrents = append(allTorrents, torrent)
		if recoverySiteSizeCompatible(torrent.SizeBytes, targetTotalSize) {
			torrents = append(torrents, torrent)
		}
		a.cache[torrentKey(torrent)] = torrent
	}
	a.mu.Unlock()
	attempt.Candidates = len(allTorrents)
	attempt.FilesSaved, attempt.FilesFailed, err = a.ensureTorrentFiles(ctx, torrents)
	if err != nil {
		attempt.Error = err.Error()
		return torrents, attempt
	}
	attempt.Status = "ok"
	if len(errors) > 0 {
		attempt.Error = strings.Join(errors, "; ")
	}
	return torrents, attempt
}

func recoverySiteSizeCompatible(reported, target int64) bool {
	if reported <= 0 || target <= 0 {
		return true
	}
	difference := reported - target
	if difference < 0 {
		difference = -difference
	}
	tolerance := target / 20
	if tolerance < 1<<20 {
		tolerance = 1 << 20
	}
	return difference <= tolerance
}

var recoveryBracketPattern = regexp.MustCompile(`\[([^\]]+)\]`)

func recoverySearchKeywords(folderName string) []string {
	result := []string{}
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || len([]rune(value)) < 4 {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	add(folderName)
	add(strings.TrimSuffix(folderName, filepath.Ext(folderName)))
	segments := []string{}
	for _, match := range recoveryBracketPattern.FindAllStringSubmatch(folderName, -1) {
		if len(match) > 1 {
			segments = append(segments, strings.TrimSpace(match[1]))
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return len([]rune(segments[i])) > len([]rune(segments[j])) })
	for _, segment := range segments {
		prefix := segment
		if index := strings.IndexAny(prefix, "~～〜"); index >= 0 {
			prefix = prefix[:index]
		}
		add(prefix)
		add(segment)
	}
	parts := strings.FieldsFunc(folderName, func(r rune) bool {
		return strings.ContainsRune("[](){}<>_- .~～〜/\\", r)
	})
	sort.SliceStable(parts, func(i, j int) bool { return len([]rune(parts[i])) > len([]rune(parts[j])) })
	for _, part := range parts {
		add(part)
	}
	runes := []rune(strings.TrimSpace(folderName))
	if len(runes) > 24 {
		add(string(runes[:24]))
	}
	const maxRecoverySearchKeywords = 5
	if len(result) > maxRecoverySearchKeywords {
		result = result[:maxRecoverySearchKeywords]
	}
	return result
}

func recoverySearchURL(site runtimeSite, keyword string) (string, error) {
	definition := site.Definition.HTML.Search
	if len(definition.Paths) > 0 && definition.Paths[0].Method != "" && !strings.EqualFold(definition.Paths[0].Method, "get") {
		return "", fmt.Errorf("site %s recovery search only supports GET", site.ID)
	}
	parsed, err := url.Parse(parser.SiteConfigFromDefinition(site.Definition).URL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	keywordApplied := false
	for name, raw := range definition.Params {
		value := fmt.Sprint(raw)
		for _, token := range []string{"{{keyword}}", "{keyword}", "{{value}}"} {
			if strings.Contains(value, token) {
				value = strings.ReplaceAll(value, token, keyword)
				keywordApplied = true
			}
		}
		query.Set(name, value)
	}
	if !keywordApplied {
		name := "search"
		if definition.Fields.Keyword != nil && strings.TrimSpace(definition.Fields.Keyword.Name) != "" {
			name = definition.Fields.Keyword.Name
		}
		query.Set(name, keyword)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func sortRecoveryMatches(matches []RecoveryMatch) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Torrent.SiteID != matches[j].Torrent.SiteID {
			return matches[i].Torrent.SiteID < matches[j].Torrent.SiteID
		}
		return matches[i].Torrent.ID < matches[j].Torrent.ID
	})
}
