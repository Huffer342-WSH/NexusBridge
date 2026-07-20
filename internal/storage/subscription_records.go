package storage

import (
	"database/sql"
	"encoding/json"
	"sort"
	"time"
)

const (
	// SubscriptionCandidateUnread 表示候选尚未执行。
	SubscriptionCandidateUnread = "unread"
	// SubscriptionCandidateProcessing 表示候选已被执行器领取。
	SubscriptionCandidateProcessing = "processing"
	// SubscriptionCandidateProcessed 表示候选已完成处理。
	SubscriptionCandidateProcessed = "processed"
	// SubscriptionCandidateFailed 表示候选执行失败且等待显式处理。
	SubscriptionCandidateFailed = "failed"
)

// SubscriptionRecord 表示引用筛选规则的自动下载订阅。
type SubscriptionRecord struct {
	ID               string
	Name             string
	RuleName         string
	Enabled          bool
	SiteIDs          []string
	Priority         int
	QBCategory       string
	SavePathTemplate string
	QBTags           []string
	FilenameTemplate string
	Paused           bool
	MaxConcurrent    int
	DailyLimit       int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SiteScheduleRecord 表示单个站点的周期抓取配置和最近状态。
type SiteScheduleRecord struct {
	SiteID          string
	Enabled         bool
	IntervalMinutes int
	LastRunAt       time.Time
	NextRunAt       time.Time
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SubscriptionCandidateRecord 表示已独占分配给订阅的种子候选。
type SubscriptionCandidateRecord struct {
	SiteID         string
	TorrentID      string
	SubscriptionID string
	RuleName       string
	SourceOrder    int
	Status         string
	ReasonCode     string
	Error          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ProcessedAt    time.Time
}

// SubscriptionCandidateQuery 描述订阅候选的分页筛选条件。
type SubscriptionCandidateQuery struct {
	SubscriptionID string
	SiteID         string
	Status         string
	Limit          int
	Offset         int
}

// SubscriptionIngestRecord 表示尚未完成首次订阅匹配的新入库种子。
type SubscriptionIngestRecord struct {
	SiteID      string
	TorrentID   string
	SourceOrder int
	CreatedAt   time.Time
}

// ListPendingSubscriptionIngest 按初始列表顺序返回站点尚未完成首次匹配的新种子。

// SubscriptionRunRecord 表示一次订阅执行的持久化汇总。
type SubscriptionRunRecord struct {
	ID             string
	SubscriptionID string
	SiteID         string
	Trigger        string
	Status         string
	FetchedCount   int
	InsertedCount  int
	MatchedCount   int
	AttemptedCount int
	SentCount      int
	ExistsCount    int
	FailedCount    int
	SkippedCount   int
	Error          string
	StartedAt      time.Time
	FinishedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SubscriptionRunQuery 描述订阅运行记录的分页筛选条件。
type SubscriptionRunQuery struct {
	SubscriptionID string
	SiteID         string
	Trigger        string
	Status         string
	Limit          int
	Offset         int
}

// SaveSubscription 新增或覆盖订阅配置。

const subscriptionSelect = `
SELECT id, name, rule_name, enabled, site_ids_json, priority, qb_category, save_path_template,
	qb_tags_json, filename_template, paused, max_concurrent, daily_limit, created_at, updated_at
FROM subscriptions`

func scanSubscription(scanner rowScanner) (SubscriptionRecord, error) {
	var record SubscriptionRecord
	var enabled, paused int
	var siteIDsJSON, qbTagsJSON, createdAt, updatedAt string
	err := scanner.Scan(&record.ID, &record.Name, &record.RuleName, &enabled, &siteIDsJSON, &record.Priority,
		&record.QBCategory, &record.SavePathTemplate, &qbTagsJSON, &record.FilenameTemplate,
		&paused, &record.MaxConcurrent, &record.DailyLimit, &createdAt, &updatedAt)
	if err != nil {
		return SubscriptionRecord{}, err
	}
	record.Enabled = enabled != 0
	record.Paused = paused != 0
	_ = json.Unmarshal([]byte(siteIDsJSON), &record.SiteIDs)
	_ = json.Unmarshal([]byte(qbTagsJSON), &record.QBTags)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}

func scanSubscriptions(rows *sql.Rows) ([]SubscriptionRecord, error) {
	var records []SubscriptionRecord
	for rows.Next() {
		record, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

const siteScheduleSelect = `
SELECT site_id, enabled, interval_minutes, last_run_at, next_run_at, last_error, created_at, updated_at
FROM site_schedules`

func scanSiteSchedule(scanner rowScanner) (SiteScheduleRecord, error) {
	var record SiteScheduleRecord
	var enabled int
	var lastRunAt, nextRunAt, createdAt, updatedAt string
	err := scanner.Scan(&record.SiteID, &enabled, &record.IntervalMinutes, &lastRunAt, &nextRunAt,
		&record.LastError, &createdAt, &updatedAt)
	if err != nil {
		return SiteScheduleRecord{}, err
	}
	record.Enabled = enabled != 0
	record.LastRunAt = parseDBTime(lastRunAt)
	record.NextRunAt = parseDBTime(nextRunAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}

const subscriptionCandidateSelect = `
SELECT site_id, torrent_id, subscription_id, rule_name, source_order, status, reason_code,
	error, created_at, updated_at, processed_at
FROM subscription_candidates`

func scanSubscriptionCandidate(scanner rowScanner) (SubscriptionCandidateRecord, error) {
	var record SubscriptionCandidateRecord
	var createdAt, updatedAt, processedAt string
	err := scanner.Scan(&record.SiteID, &record.TorrentID, &record.SubscriptionID, &record.RuleName,
		&record.SourceOrder, &record.Status, &record.ReasonCode, &record.Error, &createdAt, &updatedAt, &processedAt)
	if err != nil {
		return SubscriptionCandidateRecord{}, err
	}
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	record.ProcessedAt = parseDBTime(processedAt)
	return record, nil
}

func scanSubscriptionCandidates(rows *sql.Rows) ([]SubscriptionCandidateRecord, error) {
	var records []SubscriptionCandidateRecord
	for rows.Next() {
		record, err := scanSubscriptionCandidate(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].SourceOrder != records[j].SourceOrder {
			return records[i].SourceOrder < records[j].SourceOrder
		}
		if records[i].SiteID != records[j].SiteID {
			return records[i].SiteID < records[j].SiteID
		}
		return records[i].TorrentID < records[j].TorrentID
	})
	return records, nil
}

const subscriptionRunSelect = `
SELECT id, subscription_id, site_id, trigger, status, fetched_count, inserted_count, matched_count,
	attempted_count, sent_count, exists_count, failed_count, skipped_count, error, started_at, finished_at, created_at, updated_at
FROM subscription_runs`

func scanSubscriptionRun(scanner rowScanner) (SubscriptionRunRecord, error) {
	var record SubscriptionRunRecord
	var startedAt, finishedAt, createdAt, updatedAt string
	err := scanner.Scan(&record.ID, &record.SubscriptionID, &record.SiteID, &record.Trigger, &record.Status,
		&record.FetchedCount, &record.InsertedCount, &record.MatchedCount, &record.AttemptedCount, &record.SentCount, &record.ExistsCount, &record.FailedCount,
		&record.SkippedCount, &record.Error, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if err != nil {
		return SubscriptionRunRecord{}, err
	}
	record.StartedAt = parseDBTime(startedAt)
	record.FinishedAt = parseDBTime(finishedAt)
	record.CreatedAt = parseDBTime(createdAt)
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}
