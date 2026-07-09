package core

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

const (
	automationPollInterval  = 5 * time.Second
	defaultScheduleInterval = 15 * time.Minute
	maxDueSchedulesPerPoll  = 100
)

// AutomationTicker 抽象调度器的周期信号，便于使用可控时钟验证调度行为。
type AutomationTicker interface {
	C() <-chan time.Time
	Stop()
}

// AutomationClock 为订阅调度器提供当前时间和周期信号。
type AutomationClock interface {
	Now() time.Time
	NewTicker(time.Duration) AutomationTicker
}

type realAutomationClock struct{}

func (realAutomationClock) Now() time.Time { return time.Now() }

func (realAutomationClock) NewTicker(interval time.Duration) AutomationTicker {
	return realAutomationTicker{Ticker: time.NewTicker(interval)}
}

type realAutomationTicker struct{ *time.Ticker }

func (ticker realAutomationTicker) C() <-chan time.Time { return ticker.Ticker.C }

type contextMutex struct{ token chan struct{} }

func newContextMutex() *contextMutex {
	mutex := &contextMutex{token: make(chan struct{}, 1)}
	mutex.token <- struct{}{}
	return mutex
}

func (mutex *contextMutex) Lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-mutex.token:
		return nil
	}
}

func (mutex *contextMutex) Unlock() { mutex.token <- struct{}{} }

// StartAutomation 启动仅供常驻进程使用的站点订阅调度器。
func (a *App) StartAutomation(ctx context.Context) {
	a.StartAutomationWithClock(ctx, realAutomationClock{})
}

// StartAutomationWithClock 使用指定时钟启动常驻调度器，主要供确定性集成测试使用。
func (a *App) StartAutomationWithClock(ctx context.Context, clock AutomationClock) {
	if clock == nil {
		clock = realAutomationClock{}
	}
	a.automationOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		a.automationMu.Lock()
		a.automationCancel = cancel
		a.automationMu.Unlock()
		a.automationWG.Add(1)
		go func() {
			defer a.automationWG.Done()
			a.automationLoop(runCtx, clock)
		}()
	})
}

// wakeAutomation 通知调度器立即重新检查站点计划。
func (a *App) wakeAutomation() {
	select {
	case a.automationWake <- struct{}{}:
	default:
	}
}

// siteMutex 返回指定站点共用的防重入锁。
func (a *App) siteMutex(siteID string) *contextMutex {
	a.siteLockMu.Lock()
	defer a.siteLockMu.Unlock()
	lock, ok := a.siteLocks[siteID]
	if !ok {
		lock = newContextMutex()
		a.siteLocks[siteID] = lock
	}
	return lock
}

// subscriptionMutex 返回指定订阅共用的配额与候选领取锁。
func (a *App) subscriptionMutex(subscriptionID string) *contextMutex {
	a.subscriptionLockMu.Lock()
	defer a.subscriptionLockMu.Unlock()
	lock, ok := a.subscriptionLocks[subscriptionID]
	if !ok {
		lock = newContextMutex()
		a.subscriptionLocks[subscriptionID] = lock
	}
	return lock
}

// hashMutex 返回同一 info hash 从查询到写入 qB 的进程内互斥锁。
func (a *App) hashMutex(hash string) *contextMutex {
	hash = strings.ToLower(strings.TrimSpace(hash))
	a.hashLockMu.Lock()
	defer a.hashLockMu.Unlock()
	if lock, ok := a.hashLocks[hash]; ok {
		return lock
	}
	lock := newContextMutex()
	a.hashLocks[hash] = lock
	return lock
}

// automationLoop 按周期或显式唤醒信号检查到期站点。
func (a *App) automationLoop(ctx context.Context, clock AutomationClock) {
	ticker := clock.NewTicker(automationPollInterval)
	defer ticker.Stop()

	a.runDueSiteSchedules(ctx, clock, clock.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C():
			a.runDueSiteSchedules(ctx, clock, now)
		case <-a.automationWake:
			a.runDueSiteSchedules(ctx, clock, clock.Now())
		}
	}
}

// runDueSiteSchedules 领取并执行当前已经到期的站点计划。
func (a *App) runDueSiteSchedules(ctx context.Context, clock AutomationClock, now time.Time) {
	if err := ctx.Err(); err != nil {
		return
	}
	schedules, err := a.store.ListDueSiteSchedules(ctx, now, maxDueSchedulesPerPoll)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("list due site schedules failed", "error", err)
		}
		return
	}
	for _, schedule := range schedules {
		if err := ctx.Err(); err != nil {
			return
		}
		claimedAt := now
		interval := time.Duration(schedule.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = defaultScheduleInterval
		}
		claimed, err := a.store.ClaimDueSiteSchedule(ctx, schedule.SiteID, claimedAt, claimedAt.Add(interval))
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("claim due site schedule failed", "site_id", schedule.SiteID, "error", err)
			}
			continue
		}
		if !claimed {
			continue
		}

		_, runErr := a.runSiteSubscriptions(ctx, schedule.SiteID, "scheduled")
		if ctx.Err() != nil {
			return
		}
		finishedAt := clock.Now()
		nextRunAt := finishedAt.Add(interval)
		if latest, ok, getErr := a.store.GetSiteSchedule(ctx, schedule.SiteID); getErr == nil && ok {
			latestInterval := time.Duration(latest.IntervalMinutes) * time.Minute
			if latestInterval > 0 {
				nextRunAt = finishedAt.Add(latestInterval)
			}
		} else if getErr != nil {
			slog.Warn("reload site schedule after run failed", "site_id", schedule.SiteID, "error", getErr)
		}
		errText := ""
		if runErr != nil {
			errText = runErr.Error()
		}
		if err := a.store.UpdateSiteScheduleResult(ctx, schedule.SiteID, finishedAt, nextRunAt, errText); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("save site schedule result failed", "site_id", schedule.SiteID, "error", err)
			}
		}
	}
}
