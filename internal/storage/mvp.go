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

const (
	// MaxTorrentListLimit 是单次种子列表查询允许的最大记录数。
	MaxTorrentListLimit     = 500
	defaultTorrentListLimit = 100
	QBittorrentSettingKey   = "qbittorrent"
	LLMSettingKey           = "llm"
	// MihomoSettingKey 是 Mihomo 配置目录偏好的存储键。
	MihomoSettingKey = "mihomo"
)

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

type TorrentUpsertResult struct {
	Total    int
	Inserted []TorrentRecord
	Changed  []TorrentRecord
}

type TorrentListQuery struct {
	SiteID        string
	Search        string
	SortBy        string
	SortDirection string
	Limit         int
	Offset        int
}

type TorrentKey struct {
	SiteID    string
	TorrentID string
}

type RuleRecord struct {
	Name                   string
	SiteIDs                []string
	SiteCategories         []string
	SiteTags               []string
	SubtitleTags           []string
	TitleExpression        string
	Promotions             []string
	MinSize                int64
	MaxSize                int64
	MinSeeders             int
	MaxSeeders             int
	MinLeechers            int
	MaxLeechers            int
	MinSnatches            int
	MaxSnatches            int
	PublishedWithinMinutes int
	SortBy                 string
	SortDirection          string
	Action                 string
}

type RuleFacetRecord struct {
	SiteCategories []string
	SiteTags       []string
	SubtitleTags   []string
}

type DownloadTaskRecord struct {
	ID             string
	SiteID         string
	TorrentID      string
	RuleName       string
	SubscriptionID string
	Trigger        string
	Status         string
	TorrentTitle   string
	DownloadURL    string
	QBHash         string
	Error          string
	ContentPath    string
	PlanCategory   string
	PlanSavePath   string
	PlanTags       []string
	PlanRename     string
	PlanPaused     bool
	ReasonCode     string
	AttemptCount   int
	RetryCount     int
	LastAttemptAt  time.Time
	SentAt         time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type OrganizeTaskRecord struct {
	ID             string
	DownloadTaskID string
	Status         string
	Title          string
	SourcePath     string
	RelativeDir    string
	Filename       string
	TargetPath     string
	LLMResponse    string
	Confidence     float64
	Error          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s *SQLiteStore) SaveSetting(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
`, key, string(data))
	return err
}

func (s *SQLiteStore) LoadSetting(ctx context.Context, key string, value any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) UpsertTorrents(ctx context.Context, records []TorrentRecord) (TorrentUpsertResult, error) {
	result := TorrentUpsertResult{Total: len(records)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollbackUnlessCommitted(tx)

	for _, record := range records {
		if strings.TrimSpace(record.SiteID) == "" || strings.TrimSpace(record.TorrentID) == "" {
			continue
		}
		existingSig := ""
		existingSubtitle := ""
		err := tx.QueryRowContext(ctx, `
SELECT source_order || '|' || title || '|' || category || '|' || category_query || '|' || detail_url || '|' || download_url || '|' || cover_url || '|' ||
       tags_json || '|' || tag_ids_json || '|' || promotion || '|' || promotion_class || '|' || promotion_ends_at || '|' || promotion_remaining || '|' ||
       description || '|' || subtitle || '|' || size_text || '|' || size_bytes || '|' || seeders || '|' || leechers || '|' || snatches || '|' ||
       comments || '|' || published_at || '|' || published_text || '|' || sticky_level || '|' || bookmarked,
       subtitle
FROM torrents WHERE site_id = ? AND torrent_id = ?
`, record.SiteID, record.TorrentID).Scan(&existingSig, &existingSubtitle)
		exists := err == nil
		if err != nil && err != sql.ErrNoRows {
			return result, fmt.Errorf("check torrent: %w", err)
		}

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
			effectiveSubtitle = existingSubtitle
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
			strconv.Itoa(record.StickyLevel),
			strconv.Itoa(boolInt(record.Bookmarked)),
		}, "|")
		if !exists {
			result.Inserted = append(result.Inserted, record)
		} else if existingSig != newSig {
			result.Changed = append(result.Changed, record)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO torrents (
	site_id, torrent_id, source_order, title, category, category_query, detail_url, download_url, cover_url,
	tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
	subtitle, size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
	sticky_level, bookmarked, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
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
	sticky_level = excluded.sticky_level,
	bookmarked = excluded.bookmarked,
	last_seen_at = CURRENT_TIMESTAMP,
	updated_at = CURRENT_TIMESTAMP
`, record.SiteID, record.TorrentID, record.SourceOrder, record.Title, record.Category, record.CategoryQuery, record.DetailURL, record.DownloadURL,
			record.CoverURL, string(tagsJSON), string(tagIDsJSON), record.Promotion, record.PromotionClass, record.PromotionEndsAt,
			record.PromotionRemaining, record.Description, record.Subtitle, record.SizeText, record.SizeBytes, record.Seeders,
			record.Leechers, record.Snatches, record.Comments, record.PublishedAt, record.PublishedText,
			record.StickyLevel, boolInt(record.Bookmarked))
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

	return result, tx.Commit()
}

func (s *SQLiteStore) ListTorrents(ctx context.Context, query TorrentListQuery) ([]TorrentRecord, error) {
	limit := query.Limit
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	clauses := []string{"1=1"}
	args := []any{}
	if query.SiteID != "" {
		clauses = append(clauses, "site_id = ?")
		args = append(args, query.SiteID)
	}
	if query.Search != "" {
		clauses = append(clauses, "title LIKE ?")
		args = append(args, "%"+query.Search+"%")
	}
	orderBy := "last_seen_at DESC, source_order ASC, site_id ASC, torrent_id ASC"
	if column, ok := map[string]string{
		"source_order": "source_order", "published_at": "published_at", "size_bytes": "size_bytes",
		"seeders": "seeders", "leechers": "leechers", "snatches": "snatches",
	}[strings.ToLower(strings.TrimSpace(query.SortBy))]; ok {
		direction := "ASC"
		if strings.EqualFold(strings.TrimSpace(query.SortDirection), "desc") {
			direction = "DESC"
		}
		orderBy = column + " " + direction + ", site_id ASC, torrent_id ASC"
	}
	args = append(args, limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, source_order, title, category, category_query, detail_url, download_url, cover_url,
       tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
       detail_title, subtitle, product_url, detail_info_hash, detail_description, detail_raw_text, detail_fetched_at,
       size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
       sticky_level, bookmarked, first_seen_at, last_seen_at
FROM torrents
WHERE `+strings.Join(clauses, " AND ")+`
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
			&record.Comments, &record.PublishedAt, &record.PublishedText, &record.StickyLevel, &bookmarked,
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
       sticky_level, bookmarked, first_seen_at, last_seen_at
FROM torrents
WHERE site_id = ? AND torrent_id = ?
	`, siteID, torrentID).Scan(&record.SiteID, &record.TorrentID, &record.SourceOrder, &record.Title, &record.Category, &record.CategoryQuery,
		&record.DetailURL, &record.DownloadURL, &record.CoverURL, &tagsJSON, &tagIDsJSON, &record.Promotion,
		&record.PromotionClass, &record.PromotionEndsAt, &record.PromotionRemaining, &record.Description,
		&record.DetailTitle, &record.Subtitle, &record.ProductURL, &record.DetailInfoHash, &record.DetailDescription, &record.DetailRawText, &record.DetailFetchedAt,
		&record.SizeText, &record.SizeBytes, &record.Seeders, &record.Leechers, &record.Snatches,
		&record.Comments, &record.PublishedAt, &record.PublishedText, &record.StickyLevel, &bookmarked,
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
	_, err := s.db.ExecContext(ctx, `
UPDATE torrents
SET detail_title = ?, subtitle = ?, product_url = ?, detail_info_hash = ?, detail_description = ?, detail_raw_text = ?, detail_fetched_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ?
`, record.DetailTitle, record.Subtitle, record.ProductURL, record.DetailInfoHash, record.DetailDescription, record.DetailRawText, record.DetailFetchedAt,
		record.SiteID, record.TorrentID)
	return err
}

func (s *SQLiteStore) SaveRule(ctx context.Context, rule RuleRecord) error {
	if rule.Action == "" {
		rule.Action = "download"
	}
	if rule.SortBy == "" {
		rule.SortBy = "source_order"
	}
	if rule.SortDirection == "" {
		rule.SortDirection = "asc"
	}
	siteIDs, _ := json.Marshal(rule.SiteIDs)
	siteCategories, _ := json.Marshal(rule.SiteCategories)
	siteTags, _ := json.Marshal(rule.SiteTags)
	subtitleTags, _ := json.Marshal(rule.SubtitleTags)
	promotions, _ := json.Marshal(rule.Promotions)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO rules (
	name, site_ids_json, site_categories_json, site_tags_json, subtitle_tags_json, title_expression, promotions_json,
	min_size, max_size, min_seeders, max_seeders,
	min_leechers, max_leechers, min_snatches, max_snatches, published_within_minutes,
	sort_by, sort_direction, action, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(name) DO UPDATE SET
	site_ids_json = excluded.site_ids_json,
	site_categories_json = excluded.site_categories_json,
	site_tags_json = excluded.site_tags_json,
	subtitle_tags_json = excluded.subtitle_tags_json,
	title_expression = excluded.title_expression,
	promotions_json = excluded.promotions_json,
	min_size = excluded.min_size,
	max_size = excluded.max_size,
	min_seeders = excluded.min_seeders,
	max_seeders = excluded.max_seeders,
	min_leechers = excluded.min_leechers,
	max_leechers = excluded.max_leechers,
	min_snatches = excluded.min_snatches,
	max_snatches = excluded.max_snatches,
	published_within_minutes = excluded.published_within_minutes,
	sort_by = excluded.sort_by,
	sort_direction = excluded.sort_direction,
	action = excluded.action,
	updated_at = CURRENT_TIMESTAMP
`, rule.Name, string(siteIDs), string(siteCategories), string(siteTags), string(subtitleTags), rule.TitleExpression, string(promotions),
		rule.MinSize, rule.MaxSize, rule.MinSeeders, rule.MaxSeeders,
		rule.MinLeechers, rule.MaxLeechers, rule.MinSnatches, rule.MaxSnatches, rule.PublishedWithinMinutes,
		rule.SortBy, rule.SortDirection, rule.Action)
	return err
}

// RenameRule 事务级联修改规则名称和全部本地引用。
func (s *SQLiteStore) RenameRule(ctx context.Context, oldName string, rule RuleRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rules WHERE name = ? COLLATE NOCASE`, oldName).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if !strings.EqualFold(strings.TrimSpace(oldName), strings.TrimSpace(rule.Name)) {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rules WHERE name = ? COLLATE NOCASE`, rule.Name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("rule name %q already exists", rule.Name)
		}
	}
	siteIDs, _ := json.Marshal(rule.SiteIDs)
	siteCategories, _ := json.Marshal(rule.SiteCategories)
	siteTags, _ := json.Marshal(rule.SiteTags)
	subtitleTags, _ := json.Marshal(rule.SubtitleTags)
	promotions, _ := json.Marshal(rule.Promotions)
	result, err := tx.ExecContext(ctx, `
UPDATE rules SET name = ?, site_ids_json = ?, site_categories_json = ?, site_tags_json = ?,
	subtitle_tags_json = ?, title_expression = ?, promotions_json = ?, min_size = ?, max_size = ?,
	min_seeders = ?, max_seeders = ?, min_leechers = ?, max_leechers = ?, min_snatches = ?,
	max_snatches = ?, published_within_minutes = ?, sort_by = ?, sort_direction = ?, action = ?,
	updated_at = CURRENT_TIMESTAMP
WHERE name = ? COLLATE NOCASE
`, rule.Name, string(siteIDs), string(siteCategories), string(siteTags), string(subtitleTags), rule.TitleExpression,
		string(promotions), rule.MinSize, rule.MaxSize, rule.MinSeeders, rule.MaxSeeders, rule.MinLeechers,
		rule.MaxLeechers, rule.MinSnatches, rule.MaxSnatches, rule.PublishedWithinMinutes, rule.SortBy,
		rule.SortDirection, rule.Action, oldName)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	for _, table := range []string{"subscriptions", "subscription_candidates", "download_tasks"} {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET rule_name = ? WHERE rule_name = ? COLLATE NOCASE`, rule.Name, oldName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListRules(ctx context.Context) ([]RuleRecord, error) {
	rows, err := s.db.QueryContext(ctx, ruleSelect+` ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []RuleRecord{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// GetRule 按名称读取筛选规则。
func (s *SQLiteStore) GetRule(ctx context.Context, name string) (RuleRecord, bool, error) {
	rule, err := scanRule(s.db.QueryRowContext(ctx, ruleSelect+` WHERE name = ? COLLATE NOCASE`, name))
	if err == sql.ErrNoRows {
		return RuleRecord{}, false, nil
	}
	if err != nil {
		return RuleRecord{}, false, err
	}
	return rule, true, nil
}

// DeleteRule 删除筛选规则；关联检查由业务层负责。
func (s *SQLiteStore) DeleteRule(ctx context.Context, name string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE name = ? COLLATE NOCASE`, name)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

const ruleSelect = `
SELECT name, site_ids_json, site_categories_json, site_tags_json, subtitle_tags_json, title_expression, promotions_json,
	min_size, max_size, min_seeders, max_seeders,
	min_leechers, max_leechers, min_snatches, max_snatches, published_within_minutes,
	sort_by, sort_direction, action
FROM rules`

func scanRule(scanner rowScanner) (RuleRecord, error) {
	var rule RuleRecord
	var siteIDs, siteCategories, siteTags, subtitleTags, promotions string
	err := scanner.Scan(&rule.Name, &siteIDs, &siteCategories, &siteTags, &subtitleTags, &rule.TitleExpression, &promotions,
		&rule.MinSize, &rule.MaxSize, &rule.MinSeeders, &rule.MaxSeeders,
		&rule.MinLeechers, &rule.MaxLeechers, &rule.MinSnatches, &rule.MaxSnatches, &rule.PublishedWithinMinutes,
		&rule.SortBy, &rule.SortDirection, &rule.Action)
	if err != nil {
		return RuleRecord{}, err
	}
	_ = json.Unmarshal([]byte(siteIDs), &rule.SiteIDs)
	_ = json.Unmarshal([]byte(siteCategories), &rule.SiteCategories)
	_ = json.Unmarshal([]byte(siteTags), &rule.SiteTags)
	_ = json.Unmarshal([]byte(subtitleTags), &rule.SubtitleTags)
	_ = json.Unmarshal([]byte(promotions), &rule.Promotions)
	return rule, nil
}

func (s *SQLiteStore) CreateDownloadTaskIfAbsent(ctx context.Context, task DownloadTaskRecord) (bool, error) {
	planTags, err := json.Marshal(task.PlanTags)
	if err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO download_tasks (
	id, site_id, torrent_id, rule_name, subscription_id, trigger, status, torrent_title, download_url,
	qb_hash, error, content_path, plan_category, plan_save_path, plan_tags_json, plan_rename,
	plan_paused, reason_code, attempt_count, retry_count, last_attempt_at, sent_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, task.ID, task.SiteID, task.TorrentID, task.RuleName, task.SubscriptionID, task.Trigger, task.Status, task.TorrentTitle,
		task.DownloadURL, task.QBHash, task.Error, task.ContentPath, task.PlanCategory, task.PlanSavePath,
		string(planTags), task.PlanRename, boolInt(task.PlanPaused), task.ReasonCode, task.AttemptCount, task.RetryCount,
		formatDBTime(task.LastAttemptAt), formatDBTime(task.SentAt))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "constraint") {
			return false, nil
		}
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *SQLiteStore) UpdateDownloadTask(ctx context.Context, id, status, qbHash, contentPath, errText string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE download_tasks
SET status = ?, qb_hash = COALESCE(NULLIF(?, ''), qb_hash), content_path = COALESCE(NULLIF(?, ''), content_path), error = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, qbHash, contentPath, errText, id)
	return err
}

// UpdateDownloadTaskHash 回写下载任务关联的 qBittorrent hash。
func (s *SQLiteStore) UpdateDownloadTaskHash(ctx context.Context, id, qbHash string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(qbHash) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE download_tasks
SET qb_hash = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, qbHash, id)
	return err
}

func (s *SQLiteStore) ListDownloadTasks(ctx context.Context) ([]DownloadTaskRecord, error) {
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+` ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDownloadTasks(rows)
}

// ListDownloadTasksByTorrent 查询指定本地种子的下载任务。
func (s *SQLiteStore) ListDownloadTasksByTorrent(ctx context.Context, siteID, torrentID string) ([]DownloadTaskRecord, error) {
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+`
WHERE site_id = ? AND torrent_id = ?
ORDER BY updated_at DESC
`, siteID, torrentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDownloadTasks(rows)
}

// ListDownloadTasksByTorrents 批量查询指定本地种子的下载任务。
func (s *SQLiteStore) ListDownloadTasksByTorrents(ctx context.Context, keys []TorrentKey) (map[string][]DownloadTaskRecord, error) {
	result := make(map[string][]DownloadTaskRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		if strings.TrimSpace(key.SiteID) == "" || strings.TrimSpace(key.TorrentID) == "" {
			continue
		}
		clauses = append(clauses, "(site_id = ? AND torrent_id = ?)")
		args = append(args, key.SiteID, key.TorrentID)
	}
	if len(clauses) == 0 {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+`
WHERE `+strings.Join(clauses, " OR ")+`
ORDER BY updated_at DESC
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanDownloadTasks(rows)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		key := task.SiteID + ":" + task.TorrentID
		result[key] = append(result[key], task)
	}
	return result, nil
}

// GetDownloadTask 按 ID 读取下载任务。
func (s *SQLiteStore) GetDownloadTask(ctx context.Context, id string) (DownloadTaskRecord, bool, error) {
	task, err := scanDownloadTask(s.db.QueryRowContext(ctx, downloadTaskSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return DownloadTaskRecord{}, false, nil
	}
	if err != nil {
		return DownloadTaskRecord{}, false, err
	}
	return task, true, nil
}

// GetDownloadTaskByRule 按种子联合键和规则名称读取幂等任务。
func (s *SQLiteStore) GetDownloadTaskByRule(ctx context.Context, siteID, torrentID, ruleName string) (DownloadTaskRecord, bool, error) {
	task, err := scanDownloadTask(s.db.QueryRowContext(ctx, downloadTaskSelect+`
WHERE site_id = ? AND torrent_id = ? AND rule_name = ? COLLATE NOCASE
`, siteID, torrentID, ruleName))
	if err == sql.ErrNoRows {
		return DownloadTaskRecord{}, false, nil
	}
	if err != nil {
		return DownloadTaskRecord{}, false, err
	}
	return task, true, nil
}

// ListDownloadTasksByStatus 按订阅和状态分页读取下载任务。
func (s *SQLiteStore) ListDownloadTasksByStatus(ctx context.Context, subscriptionID string, statuses []string, limit int) ([]DownloadTaskRecord, error) {
	clauses := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(subscriptionID) != "" {
		clauses = append(clauses, "subscription_id = ?")
		args = append(args, subscriptionID)
	}
	if len(statuses) > 0 {
		placeholders := make([]string, 0, len(statuses))
		for _, status := range statuses {
			placeholders = append(placeholders, "?")
			args = append(args, status)
		}
		clauses = append(clauses, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	if limit <= 0 || limit > MaxTorrentListLimit {
		limit = defaultTorrentListLimit
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, downloadTaskSelect+`
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDownloadTasks(rows)
}

// ClaimDownloadTask 以单条条件更新安全领取任务，并记录本次尝试。
func (s *SQLiteStore) ClaimDownloadTask(ctx context.Context, id string, fromStatuses []string, incrementRetry bool) (DownloadTaskRecord, bool, error) {
	if len(fromStatuses) == 0 {
		fromStatuses = []string{"pending"}
	}
	placeholders := make([]string, 0, len(fromStatuses))
	args := []any{boolInt(incrementRetry), id}
	for _, status := range fromStatuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE download_tasks
SET status = 'processing', attempt_count = attempt_count + 1,
	retry_count = retry_count + ?, last_attempt_at = CURRENT_TIMESTAMP,
	error = '', reason_code = '', updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND status IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return DownloadTaskRecord{}, false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return DownloadTaskRecord{}, false, err
	}
	return s.GetDownloadTask(ctx, id)
}

// UpdateDownloadTaskRecord 覆盖下载任务的执行状态和下载计划快照。
func (s *SQLiteStore) UpdateDownloadTaskRecord(ctx context.Context, task DownloadTaskRecord) error {
	_, err := s.UpdateDownloadTaskRecordIfStatus(ctx, task, nil)
	return err
}

// UpdateDownloadTaskRecordIfStatus 仅在任务仍处于指定旧状态时更新执行状态和计划快照。
func (s *SQLiteStore) UpdateDownloadTaskRecordIfStatus(ctx context.Context, task DownloadTaskRecord, fromStatuses []string) (bool, error) {
	planTags, err := json.Marshal(task.PlanTags)
	if err != nil {
		return false, err
	}
	where := "id = ?"
	args := []any{task.SubscriptionID, task.Trigger, task.Status, task.TorrentTitle, task.DownloadURL, task.QBHash,
		task.Error, task.ContentPath, task.PlanCategory, task.PlanSavePath, string(planTags), task.PlanRename,
		boolInt(task.PlanPaused), task.ReasonCode, task.AttemptCount, task.RetryCount,
		formatDBTime(task.LastAttemptAt), formatDBTime(task.SentAt), task.ID}
	if len(fromStatuses) > 0 {
		placeholders := make([]string, 0, len(fromStatuses))
		for _, status := range fromStatuses {
			placeholders = append(placeholders, "?")
			args = append(args, status)
		}
		where += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE download_tasks
SET subscription_id = ?, trigger = ?, status = ?, torrent_title = ?, download_url = ?, qb_hash = ?,
	error = ?, content_path = ?, plan_category = ?, plan_save_path = ?, plan_tags_json = ?,
	plan_rename = ?, plan_paused = ?, reason_code = ?, attempt_count = ?, retry_count = ?,
	last_attempt_at = ?, sent_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE `+where, args...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// CountDownloadTasksByStatus 统计订阅在指定状态下的任务数。
func (s *SQLiteStore) CountDownloadTasksByStatus(ctx context.Context, subscriptionID string, statuses []string) (int, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	placeholders := make([]string, 0, len(statuses))
	args := []any{subscriptionID}
	for _, status := range statuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM download_tasks
WHERE subscription_id = ? AND status IN (`+strings.Join(placeholders, ",")+`)`, args...).Scan(&count)
	return count, err
}

// CountSentDownloadTasksSince 统计订阅从指定时间起成功发送的任务数。
func (s *SQLiteStore) CountSentDownloadTasksSince(ctx context.Context, subscriptionID string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM download_tasks
WHERE subscription_id = ? AND status = 'sent' AND sent_at >= ?
`, subscriptionID, formatDBTime(since)).Scan(&count)
	return count, err
}

const downloadTaskSelect = `
SELECT id, site_id, torrent_id, rule_name, subscription_id, trigger, status, torrent_title, download_url,
	qb_hash, error, content_path, plan_category, plan_save_path, plan_tags_json, plan_rename,
	plan_paused, reason_code, attempt_count, retry_count, last_attempt_at, sent_at, created_at, updated_at
FROM download_tasks`

func scanDownloadTask(scanner rowScanner) (DownloadTaskRecord, error) {
	var task DownloadTaskRecord
	var planTagsJSON, lastAttemptAt, sentAt, createdAt, updatedAt string
	var planPaused int
	err := scanner.Scan(&task.ID, &task.SiteID, &task.TorrentID, &task.RuleName, &task.SubscriptionID,
		&task.Trigger, &task.Status, &task.TorrentTitle, &task.DownloadURL, &task.QBHash, &task.Error,
		&task.ContentPath, &task.PlanCategory, &task.PlanSavePath, &planTagsJSON, &task.PlanRename,
		&planPaused, &task.ReasonCode, &task.AttemptCount, &task.RetryCount, &lastAttemptAt, &sentAt,
		&createdAt, &updatedAt)
	if err != nil {
		return DownloadTaskRecord{}, err
	}
	_ = json.Unmarshal([]byte(planTagsJSON), &task.PlanTags)
	task.PlanPaused = planPaused != 0
	task.LastAttemptAt = parseDBTime(lastAttemptAt)
	task.SentAt = parseDBTime(sentAt)
	task.CreatedAt = parseDBTime(createdAt)
	task.UpdatedAt = parseDBTime(updatedAt)
	return task, nil
}

func (s *SQLiteStore) CreateOrganizeTaskIfAbsent(ctx context.Context, task OrganizeTaskRecord) (bool, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO organize_tasks (id, download_task_id, status, title, source_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, task.ID, task.DownloadTaskID, task.Status, task.Title, task.SourcePath)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "constraint") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func scanDownloadTasks(rows *sql.Rows) ([]DownloadTaskRecord, error) {
	tasks := []DownloadTaskRecord{}
	for rows.Next() {
		task, err := scanDownloadTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SQLiteStore) UpdateOrganizeTask(ctx context.Context, id, status, relativeDir, filename, targetPath, response, errText string, confidence float64) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE organize_tasks
SET status = ?, relative_dir = ?, filename = ?, target_path = ?, llm_response = ?, confidence = ?, error = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, relativeDir, filename, targetPath, response, confidence, errText, id)
	return err
}

func (s *SQLiteStore) ListOrganizeTasks(ctx context.Context, onlyPending bool) ([]OrganizeTaskRecord, error) {
	query := `
SELECT id, download_task_id, status, title, source_path, relative_dir, filename, target_path, llm_response, confidence, error, created_at, updated_at
FROM organize_tasks`
	args := []any{}
	if onlyPending {
		query += ` WHERE status = ?`
		args = append(args, "pending")
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []OrganizeTaskRecord{}
	for rows.Next() {
		var task OrganizeTaskRecord
		var created, updated string
		if err := rows.Scan(&task.ID, &task.DownloadTaskID, &task.Status, &task.Title, &task.SourcePath,
			&task.RelativeDir, &task.Filename, &task.TargetPath, &task.LLMResponse, &task.Confidence,
			&task.Error, &created, &updated); err != nil {
			return nil, err
		}
		task.CreatedAt = parseDBTime(created)
		task.UpdatedAt = parseDBTime(updated)
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseDBTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func formatDBTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
