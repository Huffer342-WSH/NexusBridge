package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

// ControlRecoveryTorrent 控制恢复任务对应的 qBittorrent 种子状态。
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
