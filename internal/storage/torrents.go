package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TorrentRecord 表示站点种子的数据库记录。
type TorrentRecord struct {
	SiteID             string
	TorrentID          string
	SourceOrder        int
	Title              string
	Category           string
	CategoryQuery      string
	DetailURL          string
	DownloadURL        string
	CoverURL           string
	Tags               []string
	TagIDs             []string
	Promotion          string
	PromotionClass     string
	PromotionEndsAt    string
	PromotionRemaining string
	Description        string
	DetailTitle        string
	Subtitle           string
	ProductURL         string
	DetailInfoHash     string
	DetailDescription  string
	DetailRawText      string
	DetailFetchedAt    string
	SizeText           string
	SizeBytes          int64
	Seeders            int
	Leechers           int
	Snatches           int
	Comments           int
	PublishedAt        string
	PublishedText      string
	StickyLevel        int
	Bookmarked         bool
	FirstSeenAt        time.Time
	LastSeenAt         time.Time
}

// TorrentUpsertResult 汇总一批种子的新增和变化记录。
type TorrentUpsertResult struct {
	Total    int
	Inserted []TorrentRecord
	Changed  []TorrentRecord
}

// TorrentListQuery 描述种子数据库查询条件。
type TorrentListQuery struct {
	SiteID         string
	Search         string
	SearchSiteIDs  []string
	SortBy         string
	SortDirection  string
	QBTask         string
	QBProgressKeys map[TorrentKey]struct{}
	Categories     []string
	SiteCheckboxes []TorrentCheckboxFilter
	Promotions     []string
	Limit          int
	Offset         int
	ExcludeKeys    []TorrentKey
}

// TorrentCheckboxFilter 表示一个站点 checkbox 分组内按 OR 匹配的标签值。
type TorrentCheckboxFilter struct {
	Name   string
	Values []string
}

// TorrentKey 唯一标识一个站点种子。
type TorrentKey struct {
	SiteID    string
	TorrentID string
}

// UpsertTorrents 新增或更新一批种子记录。
func (s *SQLiteStore) UpsertTorrents(ctx context.Context, records []TorrentRecord) (TorrentUpsertResult, error) {
	return s.upsertTorrents(ctx, records)
}

// UpsertTorrentPage 写入一页站点列表；置顶信息仅由核心层保存在内存。
func (s *SQLiteStore) UpsertTorrentPage(ctx context.Context, records []TorrentRecord) (TorrentUpsertResult, error) {
	return s.upsertTorrents(ctx, records)
}

func (s *SQLiteStore) upsertTorrents(ctx context.Context, records []TorrentRecord) (TorrentUpsertResult, error) {
	result := TorrentUpsertResult{Total: len(records)}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollbackUnlessCommitted(tx)
	type existingTorrent struct {
		signature string
		subtitle  string
	}
	existing := make(map[TorrentKey]existingTorrent, len(records))
	keys := make([]TorrentKey, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.SiteID) != "" && strings.TrimSpace(record.TorrentID) != "" {
			keys = append(keys, TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID})
		}
	}
	if len(keys) > 0 {
		query, args := torrentKeyQuery(`
SELECT site_id, torrent_id,
       source_order || '|' || title || '|' || category || '|' || category_query || '|' || detail_url || '|' || download_url || '|' || cover_url || '|' ||
       tags_json || '|' || tag_ids_json || '|' || promotion || '|' || promotion_class || '|' || promotion_ends_at || '|' || promotion_remaining || '|' ||
       description || '|' || subtitle || '|' || size_text || '|' || size_bytes || '|' || seeders || '|' || leechers || '|' || snatches || '|' ||
       comments || '|' || published_at || '|' || published_text || '|' || bookmarked,
       subtitle
FROM torrents WHERE `, keys)
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return result, fmt.Errorf("load existing torrents: %w", err)
		}
		for rows.Next() {
			var key TorrentKey
			var item existingTorrent
			if err := rows.Scan(&key.SiteID, &key.TorrentID, &item.signature, &item.subtitle); err != nil {
				_ = rows.Close()
				return result, err
			}
			existing[key] = item
		}
		if err := rows.Close(); err != nil {
			return result, err
		}
	}
	for _, record := range records {
		if strings.TrimSpace(record.SiteID) == "" || strings.TrimSpace(record.TorrentID) == "" {
			continue
		}
		existingRecord, exists := existing[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}]

		tagsJSON, err := json.Marshal(record.Tags)
		if err != nil {
			return result, err
		}
		tagIDsJSON, err := json.Marshal(record.TagIDs)
		if err != nil {
			return result, err
		}
		effectiveSubtitle := record.Subtitle
		if exists && effectiveSubtitle == "" {
			effectiveSubtitle = existingRecord.subtitle
		}
		newSig := strings.Join([]string{
			strconv.Itoa(record.SourceOrder),
			record.Title,
			record.Category,
			record.CategoryQuery,
			record.DetailURL,
			record.DownloadURL,
			record.CoverURL,
			string(tagsJSON),
			string(tagIDsJSON),
			record.Promotion,
			record.PromotionClass,
			record.PromotionEndsAt,
			record.PromotionRemaining,
			record.Description,
			effectiveSubtitle,
			record.SizeText,
			strconv.FormatInt(record.SizeBytes, 10),
			strconv.Itoa(record.Seeders),
			strconv.Itoa(record.Leechers),
			strconv.Itoa(record.Snatches),
			strconv.Itoa(record.Comments),
			record.PublishedAt,
			record.PublishedText,
			strconv.Itoa(boolInt(record.Bookmarked)),
		}, "|")
		if !exists {
			result.Inserted = append(result.Inserted, record)
		} else if existingRecord.signature != newSig {
			result.Changed = append(result.Changed, record)
		} else {
			continue
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO torrents (
	site_id, torrent_id, source_order, title, category, category_query, detail_url, download_url, cover_url,
	tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
	subtitle, size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
	bookmarked, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	source_order = excluded.source_order,
	title = excluded.title,
	category = excluded.category,
	category_query = excluded.category_query,
	detail_url = excluded.detail_url,
	download_url = excluded.download_url,
	cover_url = excluded.cover_url,
	tags_json = excluded.tags_json,
	tag_ids_json = excluded.tag_ids_json,
	promotion = excluded.promotion,
	promotion_class = excluded.promotion_class,
	promotion_ends_at = excluded.promotion_ends_at,
	promotion_remaining = excluded.promotion_remaining,
	description = excluded.description,
	subtitle = COALESCE(NULLIF(excluded.subtitle, ''), subtitle),
	size_text = excluded.size_text,
	size_bytes = excluded.size_bytes,
	seeders = excluded.seeders,
	leechers = excluded.leechers,
	snatches = excluded.snatches,
	comments = excluded.comments,
	published_at = excluded.published_at,
	published_text = excluded.published_text,
	bookmarked = excluded.bookmarked,
	last_seen_at = CURRENT_TIMESTAMP,
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, record.TorrentID, record.SourceOrder, record.Title, record.Category, record.CategoryQuery, record.DetailURL, record.DownloadURL,
			record.CoverURL, string(tagsJSON), string(tagIDsJSON), record.Promotion, record.PromotionClass, record.PromotionEndsAt,
			record.PromotionRemaining, record.Description, record.Subtitle, record.SizeText, record.SizeBytes, record.Seeders,
			record.Leechers, record.Snatches, record.Comments, record.PublishedAt, record.PublishedText,
			boolInt(record.Bookmarked))
		if err != nil {
			return result, fmt.Errorf("upsert torrent: %w", err)
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO subscription_ingest_queue (site_id, torrent_id, source_order, created_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
`, record.SiteID, record.TorrentID, record.SourceOrder); err != nil {
				return result, fmt.Errorf("queue inserted torrent for subscription matching: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}
	if err := s.updateTorrentSearch(ctx, records); err != nil {
		return result, err
	}
	return result, nil
}

type torrentQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func buildTorrentWhere(query TorrentListQuery) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	if query.SiteID != "" {
		clauses = append(clauses, "site_id = ?")
		args = append(args, query.SiteID)
	}
	if query.Search != "" {
		searchClauses := []string{"title LIKE ?", "category LIKE ?", "promotion LIKE ?", "site_id LIKE ?"}
		pattern := "%" + query.Search + "%"
		args = append(args, pattern, pattern, pattern, pattern)
		for range query.SearchSiteIDs {
			searchClauses = append(searchClauses, "site_id = ?")
		}
		for _, siteID := range query.SearchSiteIDs {
			args = append(args, siteID)
		}
		clauses = append(clauses, "("+strings.Join(searchClauses, " OR ")+")")
	}
	if len(query.ExcludeKeys) > 0 {
		exclusions := make([]string, 0, len(query.ExcludeKeys))
		for _, key := range query.ExcludeKeys {
			exclusions = append(exclusions, "(site_id = ? AND torrent_id = ?)")
			args = append(args, key.SiteID, key.TorrentID)
		}
		clauses = append(clauses, "NOT ("+strings.Join(exclusions, " OR ")+")")
	}
	return strings.Join(clauses, " AND "), args
}

// ListTorrents 返回符合筛选条件的种子范围。
func (s *SQLiteStore) ListTorrents(ctx context.Context, query TorrentListQuery) ([]TorrentRecord, error) {
	return listTorrents(ctx, s.db, query)
}

func listTorrents(ctx context.Context, db torrentQueryer, query TorrentListQuery) ([]TorrentRecord, error) {
	limit := query.Limit
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	where, args := buildTorrentWhere(query)
	orderBy := "CASE WHEN published_at = '' OR datetime(published_at) IS NULL THEN 1 ELSE 0 END ASC, published_at DESC, site_id ASC, torrent_id ASC"
	if column, ok := map[string]string{
		"source_order": "source_order", "published_at": "published_at", "size_bytes": "size_bytes",
		"seeders": "seeders", "leechers": "leechers", "snatches": "snatches",
	}[strings.ToLower(strings.TrimSpace(query.SortBy))]; ok {
		direction := "ASC"
		if strings.EqualFold(strings.TrimSpace(query.SortDirection), "desc") {
			direction = "DESC"
		}
		if column == "published_at" && direction == "DESC" {
			orderBy = "CASE WHEN published_at = '' OR datetime(published_at) IS NULL THEN 1 ELSE 0 END ASC, published_at DESC, site_id ASC, torrent_id ASC"
		} else {
			orderBy = column + " " + direction + ", site_id ASC, torrent_id ASC"
		}
	}
	args = append(args, limit, query.Offset)
	rows, err := db.QueryContext(ctx, `
SELECT site_id, torrent_id, source_order, title, category, category_query, detail_url, download_url, cover_url,
       tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
       detail_title, subtitle, product_url, detail_info_hash, detail_description, detail_raw_text, detail_fetched_at,
       size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
       bookmarked, first_seen_at, last_seen_at
FROM torrents
WHERE `+where+`
ORDER BY `+orderBy+`
LIMIT ? OFFSET ?
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []TorrentRecord{}
	for rows.Next() {
		var record TorrentRecord
		var tagsJSON, tagIDsJSON string
		var bookmarked int
		var firstSeen, lastSeen string
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.SourceOrder, &record.Title, &record.Category, &record.CategoryQuery,
			&record.DetailURL, &record.DownloadURL, &record.CoverURL, &tagsJSON, &tagIDsJSON, &record.Promotion,
			&record.PromotionClass, &record.PromotionEndsAt, &record.PromotionRemaining, &record.Description,
			&record.DetailTitle, &record.Subtitle, &record.ProductURL, &record.DetailInfoHash, &record.DetailDescription, &record.DetailRawText, &record.DetailFetchedAt,
			&record.SizeText, &record.SizeBytes, &record.Seeders, &record.Leechers, &record.Snatches,
			&record.Comments, &record.PublishedAt, &record.PublishedText, &bookmarked,
			&firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
		_ = json.Unmarshal([]byte(tagIDsJSON), &record.TagIDs)
		record.Bookmarked = bookmarked != 0
		record.FirstSeenAt = parseDBTime(firstSeen)
		record.LastSeenAt = parseDBTime(lastSeen)
		records = append(records, record)
	}
	return records, rows.Err()
}

func countTorrents(ctx context.Context, db torrentQueryer, query TorrentListQuery) (int, error) {
	where, args := buildTorrentWhere(query)
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM torrents WHERE `+where, args...).Scan(&count)
	return count, err
}

// ListTorrentPage 在同一只读事务中返回筛选总数和当前范围。
func (s *SQLiteStore) ListTorrentPage(ctx context.Context, query TorrentListQuery) ([]TorrentRecord, int, error) {
	if query.QBTask != "" || len(query.Categories) > 0 || len(query.SiteCheckboxes) > 0 || len(query.Promotions) > 0 {
		return s.listFilteredTorrentPage(ctx, query)
	}
	if len([]rune(strings.TrimSpace(query.Search))) >= 3 {
		return s.searchTorrentPage(ctx, query)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer rollbackUnlessCommitted(tx)
	total, err := countTorrents(ctx, tx, query)
	if err != nil {
		return nil, 0, err
	}
	items, err := listTorrents(ctx, tx, query)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListRuleFacets 汇总指定站点已抓取种子的分类和两类标签。
func (s *SQLiteStore) ListRuleFacets(ctx context.Context, siteID string) (RuleFacetRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT category, tags_json, tag_ids_json
FROM torrents
WHERE site_id = ?
`, siteID)
	if err != nil {
		return RuleFacetRecord{}, err
	}
	defer rows.Close()
	categories := map[string]string{}
	siteTags := map[string]string{}
	subtitleTags := map[string]string{}
	add := func(values map[string]string, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values[strings.ToLower(value)] = value
		}
	}
	for rows.Next() {
		var category, tagsJSON, tagIDsJSON string
		if err := rows.Scan(&category, &tagsJSON, &tagIDsJSON); err != nil {
			return RuleFacetRecord{}, err
		}
		add(categories, category)
		var tags, tagIDs []string
		_ = json.Unmarshal([]byte(tagsJSON), &tags)
		_ = json.Unmarshal([]byte(tagIDsJSON), &tagIDs)
		for _, value := range tags {
			add(subtitleTags, value)
		}
		for _, value := range tagIDs {
			add(siteTags, value)
		}
	}
	if err := rows.Err(); err != nil {
		return RuleFacetRecord{}, err
	}
	toSorted := func(values map[string]string) []string {
		result := make([]string, 0, len(values))
		for _, value := range values {
			result = append(result, value)
		}
		sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
		return result
	}
	return RuleFacetRecord{
		SiteCategories: toSorted(categories), SiteTags: toSorted(siteTags), SubtitleTags: toSorted(subtitleTags),
	}, nil
}

// GetTorrent 按站点和种子 ID 读取单条种子。
func (s *SQLiteStore) GetTorrent(ctx context.Context, siteID, torrentID string) (TorrentRecord, bool, error) {
	var record TorrentRecord
	var tagsJSON, tagIDsJSON string
	var bookmarked int
	var firstSeen, lastSeen string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, torrent_id, source_order, title, category, category_query, detail_url, download_url, cover_url,
       tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
       detail_title, subtitle, product_url, detail_info_hash, detail_description, detail_raw_text, detail_fetched_at,
       size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
       bookmarked, first_seen_at, last_seen_at
FROM torrents
WHERE site_id = ? AND torrent_id = ?
	`, siteID, torrentID).Scan(&record.SiteID, &record.TorrentID, &record.SourceOrder, &record.Title, &record.Category, &record.CategoryQuery,
		&record.DetailURL, &record.DownloadURL, &record.CoverURL, &tagsJSON, &tagIDsJSON, &record.Promotion,
		&record.PromotionClass, &record.PromotionEndsAt, &record.PromotionRemaining, &record.Description,
		&record.DetailTitle, &record.Subtitle, &record.ProductURL, &record.DetailInfoHash, &record.DetailDescription, &record.DetailRawText, &record.DetailFetchedAt,
		&record.SizeText, &record.SizeBytes, &record.Seeders, &record.Leechers, &record.Snatches,
		&record.Comments, &record.PublishedAt, &record.PublishedText, &bookmarked,
		&firstSeen, &lastSeen)
	if err == sql.ErrNoRows {
		return TorrentRecord{}, false, nil
	}
	if err != nil {
		return TorrentRecord{}, false, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
	_ = json.Unmarshal([]byte(tagIDsJSON), &record.TagIDs)
	record.Bookmarked = bookmarked != 0
	record.FirstSeenAt = parseDBTime(firstSeen)
	record.LastSeenAt = parseDBTime(lastSeen)
	return record, true, nil
}

// UpdateTorrentDetail 只更新种子的详情页字段。
func (s *SQLiteStore) UpdateTorrentDetail(ctx context.Context, record TorrentRecord) error {
	_, err := s.execWriteContext(ctx, `
UPDATE torrents
SET detail_title = ?, subtitle = ?, product_url = ?, detail_info_hash = ?, detail_description = ?, detail_raw_text = ?, detail_fetched_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ?
`, record.DetailTitle, record.Subtitle, record.ProductURL, record.DetailInfoHash, record.DetailDescription, record.DetailRawText, record.DetailFetchedAt,
		record.SiteID, record.TorrentID)
	return err
}
