package core

import (
	"context"

	"nexusbridge/internal/qbittorrent"
	"nexusbridge/internal/storage"
)

type subscriptionExecutionCounts struct {
	attempted int
	sent      int
	exists    int
	failed    int
	skipped   int
}

type preparedSubscriptionCandidate struct {
	record   storage.SubscriptionCandidateRecord
	torrent  Torrent
	data     []byte
	metadata qbittorrent.TorrentMetadata
	plan     DownloadPlan
}

// runSiteSubscriptions 抓取站点、按优先级独占分配新种子并执行全部启用订阅。
func (a *App) runSiteSubscriptions(ctx context.Context, siteID, trigger string) (FetchResult, error) {
	job, err := a.runTrackedSiteFetch(ctx, siteID, trigger, SiteFetchRequest{Mode: "incremental"})
	result := FetchResult{
		SiteID: job.SiteID, Status: "ok", Fetched: job.Fetched, Inserted: job.Inserted, Changed: job.Changed,
		Matched: job.Matched, DownloadSent: job.DownloadSent, FilesSaved: job.FilesSaved, FilesFailed: job.FilesFailed,
	}
	if err != nil {
		result.Status = "failed"
	}
	return result, err
}

func (a *App) assignInsertedTorrents(ctx context.Context, torrents []Torrent, subscriptions []storage.SubscriptionRecord) (map[string]int, error) {
	assigned := map[string]int{}
	rules := make(map[string]Rule, len(subscriptions))
	for _, subscription := range subscriptions {
		record, ok, err := a.store.GetRule(ctx, subscription.RuleName)
		if err != nil {
			return nil, err
		}
		if ok {
			rules[subscription.ID] = ruleFromRecord(record)
		}
	}
	for _, torrent := range torrents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, subscription := range subscriptions {
			rule, ok := rules[subscription.ID]
			if !ok || !MatchRule(rule, torrent).Matched {
				continue
			}
			created, err := a.store.CreateSubscriptionCandidateIfAbsent(ctx, storage.SubscriptionCandidateRecord{
				SiteID: torrent.SiteID, TorrentID: torrent.ID, SubscriptionID: subscription.ID,
				RuleName: rule.Name, SourceOrder: torrent.SourceOrder, Status: storage.SubscriptionCandidateUnread,
				ReasonCode: "matched",
			})
			if err != nil {
				return nil, err
			}
			if created {
				assigned[subscription.ID]++
			}
			// 无论是否已存在候选，同一种子都不能回退到低优先级订阅。
			break
		}
	}
	return assigned, nil
}

func (a *App) assignPendingSiteTorrents(ctx context.Context, siteID string, subscriptions []storage.SubscriptionRecord) (map[string]int, error) {
	records, err := a.store.ListPendingSubscriptionIngest(ctx, siteID, storage.MaxTorrentListLimit)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return map[string]int{}, nil
	}
	torrents := make([]Torrent, 0, len(records))
	for _, record := range records {
		torrent, err := a.getTorrent(ctx, record.SiteID, record.TorrentID)
		if err != nil {
			return nil, err
		}
		// 候选顺序固定为首次入库时的页面位置，不随后续抓取重排。
		torrent.SourceOrder = record.SourceOrder
		torrents = append(torrents, torrent)
	}
	assigned, err := a.assignInsertedTorrents(ctx, torrents, subscriptions)
	if err != nil {
		return nil, err
	}
	if err := a.store.DeletePendingSubscriptionIngest(ctx, records); err != nil {
		return nil, err
	}
	return assigned, nil
}
