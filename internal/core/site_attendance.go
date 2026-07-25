package core

import (
	"context"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"nexusbridge/internal/storage"
)

const defaultAttendanceTimeOfDay = "09:00"

// GetSiteAttendance 返回站点签到配置，未保存时返回关闭的默认值。
func (a *App) GetSiteAttendance(ctx context.Context, siteID string) (SiteAttendance, error) {
	if _, err := a.findSite(siteID); err != nil {
		return SiteAttendance{}, err
	}
	record, ok, err := a.store.GetSiteAttendanceSchedule(ctx, siteID)
	if err != nil {
		return SiteAttendance{}, err
	}
	if !ok {
		return SiteAttendance{
			SiteID:     siteID,
			Configured: false,
			Enabled:    false,
			TimeOfDay:  defaultAttendanceTimeOfDay,
			Timezone:   "",
		}, nil
	}
	return siteAttendanceFromRecord(record, true), nil
}

// SaveSiteAttendance 保存站点签到配置并唤醒后台调度器。
func (a *App) SaveSiteAttendance(ctx context.Context, attendance SiteAttendance) (SiteAttendance, error) {
	if _, err := a.findSite(attendance.SiteID); err != nil {
		return SiteAttendance{}, err
	}
	attendance.TimeOfDay = strings.TrimSpace(attendance.TimeOfDay)
	attendance.Timezone = strings.TrimSpace(attendance.Timezone)
	if _, _, err := parseAttendanceTime(attendance.TimeOfDay); err != nil {
		return SiteAttendance{}, err
	}
	location, err := time.LoadLocation(attendance.Timezone)
	if err != nil {
		return SiteAttendance{}, fmt.Errorf("invalid attendance timezone %q: %w", attendance.Timezone, err)
	}

	record := storage.SiteAttendanceScheduleRecord{
		SiteID:    attendance.SiteID,
		Enabled:   attendance.Enabled,
		TimeOfDay: attendance.TimeOfDay,
		Timezone:  attendance.Timezone,
	}
	if attendance.Enabled {
		nextRunAt, err := nextAttendanceRun(time.Now(), attendance.TimeOfDay, location)
		if err != nil {
			return SiteAttendance{}, err
		}
		record.NextRunAt = nextRunAt
	}
	if err := a.store.SaveSiteAttendanceSchedule(ctx, record); err != nil {
		return SiteAttendance{}, err
	}
	a.wakeAutomation()
	return a.GetSiteAttendance(ctx, attendance.SiteID)
}

// runSiteAttendance 使用站点凭据访问签到页面。
func (a *App) runSiteAttendance(ctx context.Context, siteID string) error {
	site, err := a.findSite(siteID)
	if err != nil {
		return err
	}
	lock := a.siteMutex(site.ID)
	if err := lock.Lock(ctx); err != nil {
		return err
	}
	defer lock.Unlock()

	_, err = a.fetchSiteResource(ctx, site, site.AttendanceURL, siteRequestOptions{
		RequireCookies: true,
		LogRequest:     true,
	})
	return err
}

func nextAttendanceRun(after time.Time, timeOfDay string, location *time.Location) (time.Time, error) {
	hour, minute, err := parseAttendanceTime(timeOfDay)
	if err != nil {
		return time.Time{}, err
	}
	local := after.In(location)
	next := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, location)
	if !next.After(local) {
		next = time.Date(local.Year(), local.Month(), local.Day()+1, hour, minute, 0, 0, location)
	}
	return next, nil
}

func parseAttendanceTime(value string) (int, int, error) {
	if len(value) != len("09:00") {
		return 0, 0, fmt.Errorf("attendance time_of_day must use HH:mm")
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, fmt.Errorf("attendance time_of_day must use HH:mm")
	}
	return parsed.Hour(), parsed.Minute(), nil
}

func siteAttendanceFromRecord(record storage.SiteAttendanceScheduleRecord, configured bool) SiteAttendance {
	result := SiteAttendance{
		SiteID:     record.SiteID,
		Configured: configured,
		Enabled:    record.Enabled,
		TimeOfDay:  record.TimeOfDay,
		Timezone:   record.Timezone,
		LastError:  record.LastError,
	}
	if !record.LastRunAt.IsZero() {
		value := record.LastRunAt
		result.LastRunAt = &value
	}
	if !record.NextRunAt.IsZero() {
		value := record.NextRunAt
		result.NextRunAt = &value
	}
	if !record.UpdatedAt.IsZero() {
		value := record.UpdatedAt
		result.UpdatedAt = &value
	}
	return result
}
