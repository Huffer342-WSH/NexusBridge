package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
)

type TorrentRecord struct {
	SiteID             string
	TorrentID          string
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
	Total   int
	Changed []TorrentRecord
}

type TorrentListQuery struct {
	SiteID string
	Search string
	Limit  int
	Offset int
}

type TorrentKey struct {
	SiteID    string
	TorrentID string
}

type RuleRecord struct {
	ID         string
	Name       string
	Enabled    bool
	SiteIDs    []string
	Categories []string
	Tags       []string
	Include    string
	Exclude    string
	Promotion  string
	MinSize    int64
	MaxSize    int64
	MinSeeders int
	Action     string
}

type DownloadTaskRecord struct {
	ID           string
	SiteID       string
	TorrentID    string
	RuleID       string
	Status       string
	TorrentTitle string
	DownloadURL  string
	QBHash       string
	Error        string
	ContentPath  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
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
		err := tx.QueryRowContext(ctx, `
SELECT title || '|' || category || '|' || download_url || '|' || tags_json || '|' || tag_ids_json || '|' || promotion || '|' || subtitle || '|' ||
       size_bytes || '|' || seeders || '|' || leechers || '|' || snatches
FROM torrents WHERE site_id = ? AND torrent_id = ?
`, record.SiteID, record.TorrentID).Scan(&existingSig)
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
		newSig := strings.Join([]string{
			record.Title,
			record.Category,
			record.DownloadURL,
			string(tagsJSON),
			string(tagIDsJSON),
			record.Promotion,
			record.Subtitle,
			strconv.FormatInt(record.SizeBytes, 10),
			strconv.Itoa(record.Seeders),
			strconv.Itoa(record.Leechers),
			strconv.Itoa(record.Snatches),
		}, "|")
		if existingSig == "" || existingSig != newSig {
			result.Changed = append(result.Changed, record)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO torrents (
	site_id, torrent_id, title, category, category_query, detail_url, download_url, cover_url,
	tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
	subtitle, size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
	sticky_level, bookmarked, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
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
`, record.SiteID, record.TorrentID, record.Title, record.Category, record.CategoryQuery, record.DetailURL, record.DownloadURL,
			record.CoverURL, string(tagsJSON), string(tagIDsJSON), record.Promotion, record.PromotionClass, record.PromotionEndsAt,
			record.PromotionRemaining, record.Description, record.Subtitle, record.SizeText, record.SizeBytes, record.Seeders,
			record.Leechers, record.Snatches, record.Comments, record.PublishedAt, record.PublishedText,
			record.StickyLevel, boolInt(record.Bookmarked))
		if err != nil {
			return result, fmt.Errorf("upsert torrent: %w", err)
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
	args = append(args, limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, title, category, category_query, detail_url, download_url, cover_url,
       tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
       detail_title, subtitle, product_url, detail_info_hash, detail_description, detail_raw_text, detail_fetched_at,
       size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
       sticky_level, bookmarked, first_seen_at, last_seen_at
FROM torrents
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY last_seen_at DESC
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
		if err := rows.Scan(&record.SiteID, &record.TorrentID, &record.Title, &record.Category, &record.CategoryQuery,
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

// GetTorrent 按站点和种子 ID 读取单条种子。
func (s *SQLiteStore) GetTorrent(ctx context.Context, siteID, torrentID string) (TorrentRecord, bool, error) {
	var record TorrentRecord
	var tagsJSON, tagIDsJSON string
	var bookmarked int
	var firstSeen, lastSeen string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, torrent_id, title, category, category_query, detail_url, download_url, cover_url,
       tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
       detail_title, subtitle, product_url, detail_info_hash, detail_description, detail_raw_text, detail_fetched_at,
       size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
       sticky_level, bookmarked, first_seen_at, last_seen_at
FROM torrents
WHERE site_id = ? AND torrent_id = ?
	`, siteID, torrentID).Scan(&record.SiteID, &record.TorrentID, &record.Title, &record.Category, &record.CategoryQuery,
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
	siteIDs, _ := json.Marshal(rule.SiteIDs)
	categories, _ := json.Marshal(rule.Categories)
	tags, _ := json.Marshal(rule.Tags)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO rules (id, name, enabled, site_ids_json, categories_json, tags_json, include_text, exclude_text, promotion, min_size, max_size, min_seeders, action, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	enabled = excluded.enabled,
	site_ids_json = excluded.site_ids_json,
	categories_json = excluded.categories_json,
	tags_json = excluded.tags_json,
	include_text = excluded.include_text,
	exclude_text = excluded.exclude_text,
	promotion = excluded.promotion,
	min_size = excluded.min_size,
	max_size = excluded.max_size,
	min_seeders = excluded.min_seeders,
	action = excluded.action,
	updated_at = CURRENT_TIMESTAMP
`, rule.ID, rule.Name, boolInt(rule.Enabled), string(siteIDs), string(categories), string(tags), rule.Include, rule.Exclude,
		rule.Promotion, rule.MinSize, rule.MaxSize, rule.MinSeeders, rule.Action)
	return err
}

func (s *SQLiteStore) ListRules(ctx context.Context) ([]RuleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, enabled, site_ids_json, categories_json, tags_json, include_text, exclude_text, promotion, min_size, max_size, min_seeders, action
FROM rules ORDER BY id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []RuleRecord{}
	for rows.Next() {
		var rule RuleRecord
		var enabled int
		var siteIDs, categories, tags string
		if err := rows.Scan(&rule.ID, &rule.Name, &enabled, &siteIDs, &categories, &tags, &rule.Include, &rule.Exclude,
			&rule.Promotion, &rule.MinSize, &rule.MaxSize, &rule.MinSeeders, &rule.Action); err != nil {
			return nil, err
		}
		rule.Enabled = enabled != 0
		_ = json.Unmarshal([]byte(siteIDs), &rule.SiteIDs)
		_ = json.Unmarshal([]byte(categories), &rule.Categories)
		_ = json.Unmarshal([]byte(tags), &rule.Tags)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *SQLiteStore) CreateDownloadTaskIfAbsent(ctx context.Context, task DownloadTaskRecord) (bool, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO download_tasks (id, site_id, torrent_id, rule_id, status, torrent_title, download_url, qb_hash, error, content_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, task.ID, task.SiteID, task.TorrentID, task.RuleID, task.Status, task.TorrentTitle, task.DownloadURL, task.QBHash, task.Error, task.ContentPath)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "constraint") {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
	rows, err := s.db.QueryContext(ctx, `
SELECT id, site_id, torrent_id, rule_id, status, torrent_title, download_url, qb_hash, error, content_path, created_at, updated_at
FROM download_tasks ORDER BY updated_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []DownloadTaskRecord{}
	for rows.Next() {
		var task DownloadTaskRecord
		var created, updated string
		if err := rows.Scan(&task.ID, &task.SiteID, &task.TorrentID, &task.RuleID, &task.Status, &task.TorrentTitle,
			&task.DownloadURL, &task.QBHash, &task.Error, &task.ContentPath, &created, &updated); err != nil {
			return nil, err
		}
		task.CreatedAt = parseDBTime(created)
		task.UpdatedAt = parseDBTime(updated)
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// ListDownloadTasksByTorrent 查询指定本地种子的下载任务。
func (s *SQLiteStore) ListDownloadTasksByTorrent(ctx context.Context, siteID, torrentID string) ([]DownloadTaskRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, site_id, torrent_id, rule_id, status, torrent_title, download_url, qb_hash, error, content_path, created_at, updated_at
FROM download_tasks
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
	rows, err := s.db.QueryContext(ctx, `
SELECT id, site_id, torrent_id, rule_id, status, torrent_title, download_url, qb_hash, error, content_path, created_at, updated_at
FROM download_tasks
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
		var task DownloadTaskRecord
		var created, updated string
		if err := rows.Scan(&task.ID, &task.SiteID, &task.TorrentID, &task.RuleID, &task.Status, &task.TorrentTitle,
			&task.DownloadURL, &task.QBHash, &task.Error, &task.ContentPath, &created, &updated); err != nil {
			return nil, err
		}
		task.CreatedAt = parseDBTime(created)
		task.UpdatedAt = parseDBTime(updated)
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
