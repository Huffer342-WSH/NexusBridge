package core

import (
	"strconv"
	"time"

	"nexusbridge/internal/config"
	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

func recordFromParser(entry parser.TorrentEntry, sourceOrder int) storage.TorrentRecord {
	return storage.TorrentRecord{
		SiteID:             entry.SiteID,
		TorrentID:          strconv.Itoa(entry.ID),
		Title:              entry.Title,
		Category:           entry.Category,
		CategoryQuery:      entry.CategoryQuery,
		DetailURL:          entry.DetailURL,
		DownloadURL:        entry.DownloadURL,
		CoverURL:           entry.CoverURL,
		Tags:               entry.Tags,
		TagIDs:             entry.TagIDs,
		Promotion:          entry.Promotion,
		PromotionClass:     entry.PromotionClass,
		PromotionEndsAt:    entry.PromotionEndsAt,
		PromotionRemaining: entry.PromotionRemaining,
		Subtitle:           entry.Subtitle,
		Description:        entry.Description,
		SizeText:           entry.SizeText,
		SizeBytes:          entry.SizeBytes,
		Seeders:            entry.Seeders,
		Leechers:           entry.Leechers,
		Snatches:           entry.Snatches,
		Comments:           entry.Comments,
		PublishedAt:        entry.PublishedAt,
		PublishedText:      entry.PublishedText,
		StickyLevel:        entry.StickyLevel,
		Bookmarked:         entry.Bookmarked,
		SourceOrder:        sourceOrder,
	}
}

func torrentFromRecord(record storage.TorrentRecord) Torrent {
	var published *time.Time
	if record.PublishedAt != "" {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", record.PublishedAt, time.Local); err == nil {
			published = &parsed
		}
	}
	return Torrent{
		ID:                record.TorrentID,
		SiteID:            record.SiteID,
		Category:          record.Category,
		CategoryQuery:     record.CategoryQuery,
		Title:             record.Title,
		DetailURL:         record.DetailURL,
		DownloadURL:       record.DownloadURL,
		CoverURL:          record.CoverURL,
		Tags:              record.Tags,
		TagIDs:            record.TagIDs,
		Description:       record.Description,
		DetailTitle:       record.DetailTitle,
		Subtitle:          record.Subtitle,
		ProductURL:        record.ProductURL,
		DetailInfoHash:    record.DetailInfoHash,
		DetailDescription: record.DetailDescription,
		DetailRawText:     record.DetailRawText,
		DetailFetchedAt:   record.DetailFetchedAt,
		SizeBytes:         record.SizeBytes,
		Seeders:           record.Seeders,
		Leechers:          record.Leechers,
		Snatches:          record.Snatches,
		Comments:          record.Comments,
		Promotion:         record.Promotion,
		PromotionClass:    record.PromotionClass,
		PromotionEndsAt:   record.PromotionEndsAt,
		PublishedAt:       published,
		PublishedText:     record.PublishedText,
		FirstSeenAt:       record.FirstSeenAt,
		LastSeenAt:        record.LastSeenAt,
		SourceOrder:       record.SourceOrder,
		StickyLevel:       record.StickyLevel,
	}
}

func ruleFromConfig(rule config.RuleConfig) Rule {
	return Rule{
		Name: rule.Name, SiteIDs: rule.SiteIDs, SiteCategories: rule.SiteCategories, SiteTags: rule.SiteTags,
		SubtitleTags: rule.SubtitleTags, TitleExpression: rule.TitleExpression, Promotions: rule.Promotions,
		MinSize: rule.MinSize, MaxSize: rule.MaxSize,
		MinSeeders: rule.MinSeeders, MaxSeeders: rule.MaxSeeders, MinLeechers: rule.MinLeechers,
		MaxLeechers: rule.MaxLeechers, MinSnatches: rule.MinSnatches, MaxSnatches: rule.MaxSnatches,
		PublishedWithinMinutes: rule.PublishedWithinMinutes, SortBy: rule.SortBy, SortDirection: rule.SortDirection,
		Action: firstNonEmpty(rule.Action, "download"),
	}
}

func ruleToRecord(rule Rule) storage.RuleRecord {
	return storage.RuleRecord{
		Name: rule.Name, SiteIDs: rule.SiteIDs, SiteCategories: rule.SiteCategories, SiteTags: rule.SiteTags,
		SubtitleTags: rule.SubtitleTags, TitleExpression: rule.TitleExpression, Promotions: rule.Promotions,
		MinSize: rule.MinSize, MaxSize: rule.MaxSize,
		MinSeeders: rule.MinSeeders, MaxSeeders: rule.MaxSeeders, MinLeechers: rule.MinLeechers,
		MaxLeechers: rule.MaxLeechers, MinSnatches: rule.MinSnatches, MaxSnatches: rule.MaxSnatches,
		PublishedWithinMinutes: rule.PublishedWithinMinutes, SortBy: rule.SortBy, SortDirection: rule.SortDirection,
		Action: firstNonEmpty(rule.Action, "download"),
	}
}

func ruleFromRecord(record storage.RuleRecord) Rule {
	return Rule{
		Name: record.Name, SiteIDs: record.SiteIDs, SiteCategories: record.SiteCategories, SiteTags: record.SiteTags,
		SubtitleTags: record.SubtitleTags, TitleExpression: record.TitleExpression, Promotions: record.Promotions,
		MinSize: record.MinSize, MaxSize: record.MaxSize,
		MinSeeders: record.MinSeeders, MaxSeeders: record.MaxSeeders, MinLeechers: record.MinLeechers,
		MaxLeechers: record.MaxLeechers, MinSnatches: record.MinSnatches, MaxSnatches: record.MaxSnatches,
		PublishedWithinMinutes: record.PublishedWithinMinutes, SortBy: record.SortBy, SortDirection: record.SortDirection,
		Action: firstNonEmpty(record.Action, "download"),
	}
}

func downloadTaskID(siteID, torrentID, ruleName string) string {
	return siteID + ":" + torrentID + ":" + ruleName
}

func downloadTaskToRecord(task DownloadTask) storage.DownloadTaskRecord {
	return storage.DownloadTaskRecord{
		ID: task.ID, SiteID: task.SiteID, TorrentID: task.TorrentID, RuleName: task.RuleName,
		SubscriptionID: task.SubscriptionID, Trigger: task.Trigger, Status: task.Status,
		TorrentTitle: task.TorrentTitle, DownloadURL: task.DownloadURL, QBHash: task.QBHash,
		Error: task.Error, ContentPath: task.ContentPath, PlanCategory: task.Category,
		PlanSavePath: task.SavePath, PlanTags: task.Tags, PlanRename: task.Rename,
		PlanPaused: task.Paused, ReasonCode: task.ReasonCode, AttemptCount: task.AttemptCount,
		RetryCount: task.RetryCount, LastAttemptAt: timeValue(task.LastAttemptAt), SentAt: timeValue(task.SentAt),
	}
}

func downloadTaskFromRecord(record storage.DownloadTaskRecord) DownloadTask {
	task := DownloadTask{
		ID: record.ID, SiteID: record.SiteID, TorrentID: record.TorrentID, RuleName: record.RuleName,
		SubscriptionID: record.SubscriptionID, Trigger: record.Trigger, Status: record.Status,
		TorrentTitle: record.TorrentTitle, DownloadURL: record.DownloadURL, QBHash: record.QBHash,
		Error: record.Error, ContentPath: record.ContentPath, Category: record.PlanCategory,
		SavePath: record.PlanSavePath, Tags: record.PlanTags, Rename: record.PlanRename,
		Paused: record.PlanPaused, ReasonCode: record.ReasonCode, AttemptCount: record.AttemptCount,
		RetryCount: record.RetryCount, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if !record.LastAttemptAt.IsZero() {
		value := record.LastAttemptAt
		task.LastAttemptAt = &value
	}
	if !record.SentAt.IsZero() {
		value := record.SentAt
		task.SentAt = &value
	}
	return task
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func organizeTaskFromRecord(record storage.OrganizeTaskRecord) OrganizeTask {
	return OrganizeTask{
		ID:             record.ID,
		DownloadTaskID: record.DownloadTaskID,
		Status:         record.Status,
		Title:          record.Title,
		SourcePath:     record.SourcePath,
		RelativeDir:    record.RelativeDir,
		Filename:       record.Filename,
		TargetPath:     record.TargetPath,
		LLMResponse:    record.LLMResponse,
		Confidence:     record.Confidence,
		Error:          record.Error,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}
