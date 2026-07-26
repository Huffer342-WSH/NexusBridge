package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *SQLiteStore) rebuildDerivedIndexes(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT site_id FROM torrents ORDER BY site_id`)
	if err != nil {
		return err
	}
	var siteIDs []string
	for rows.Next() {
		var siteID string
		if err := rows.Scan(&siteID); err != nil {
			_ = rows.Close()
			return err
		}
		siteIDs = append(siteIDs, siteID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, siteID := range siteIDs {
		if err := s.rebuildTorrentSearchSite(ctx, siteID); err != nil {
			return err
		}
	}
	sizeRows, err := s.db.QueryContext(ctx, `
SELECT site_id, torrent_id, size_signature, content_file_count, content_total_size, size_index_version
FROM torrent_files WHERE size_signature <> '' AND size_index_version = ?
`, TorrentSizeIndexVersion)
	if err != nil {
		return err
	}
	defer sizeRows.Close()
	return s.withIndexWriteTx(ctx, func(tx *sql.Tx) error {
		for sizeRows.Next() {
			var key TorrentKey
			var signature string
			var fileCount, version int
			var totalSize int64
			if err := sizeRows.Scan(&key.SiteID, &key.TorrentID, &signature, &fileCount, &totalSize, &version); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO torrent_size_signatures (site_id, torrent_id, signature, file_count, total_size, version)
VALUES (?, ?, ?, ?, ?, ?)
`, key.SiteID, key.TorrentID, signature, fileCount, totalSize, version); err != nil {
				return err
			}
		}
		return sizeRows.Err()
	})
}

func (s *SQLiteStore) updateTorrentSearch(ctx context.Context, records []TorrentRecord) error {
	if len(records) == 0 {
		return nil
	}
	return s.withIndexWriteTx(ctx, func(tx *sql.Tx) error {
		for _, record := range records {
			if err := upsertTorrentSearch(ctx, tx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLiteStore) rebuildTorrentSearchSite(ctx context.Context, siteID string) error {
	records := make([]TorrentRecord, 0)
	for offset := 0; ; offset += MaxTorrentListLimit {
		page, err := listTorrents(ctx, s.db, TorrentListQuery{
			SiteID: siteID, Limit: MaxTorrentListLimit, Offset: offset,
		})
		if err != nil {
			return err
		}
		records = append(records, page...)
		if len(page) < MaxTorrentListLimit {
			break
		}
	}
	return s.withIndexWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM torrent_search_fts WHERE site_id = ?`, siteID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM torrent_search WHERE site_id = ?`, siteID); err != nil {
			return err
		}
		for _, record := range records {
			if err := upsertTorrentSearch(ctx, tx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertTorrentSearch(ctx context.Context, tx *sql.Tx, record TorrentRecord) error {
	tagIDsJSON, err := json.Marshal(record.TagIDs)
	if err != nil {
		return err
	}
	var unchanged int
	err = tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM torrent_search
	WHERE site_id = ? AND torrent_id = ? AND title = ? AND category = ? AND promotion = ? AND promotion_class = ? AND tag_ids_json = ?
	AND source_order = ? AND published_at = ? AND size_bytes = ? AND seeders = ? AND leechers = ?
	AND snatches = ?
`, record.SiteID, record.TorrentID, record.Title, record.Category, record.Promotion, record.PromotionClass, string(tagIDsJSON), record.SourceOrder,
		record.PublishedAt, record.SizeBytes, record.Seeders, record.Leechers, record.Snatches).Scan(&unchanged)
	if err != nil {
		return err
	}
	if unchanged == 1 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO torrent_search (
	site_id, torrent_id, title, category, promotion, promotion_class, tag_ids_json, source_order, published_at,
	size_bytes, seeders, leechers, snatches
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	title = excluded.title,
	category = excluded.category,
	promotion = excluded.promotion,
	promotion_class = excluded.promotion_class,
	tag_ids_json = excluded.tag_ids_json,
	source_order = excluded.source_order,
	published_at = excluded.published_at,
	size_bytes = excluded.size_bytes,
	seeders = excluded.seeders,
	leechers = excluded.leechers,
	snatches = excluded.snatches
`, record.SiteID, record.TorrentID, record.Title, record.Category, record.Promotion, record.PromotionClass, string(tagIDsJSON), record.SourceOrder,
		record.PublishedAt, record.SizeBytes, record.Seeders, record.Leechers, record.Snatches); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM torrent_search_fts WHERE site_id = ? AND torrent_id = ?`, record.SiteID, record.TorrentID); err != nil {
		return err
	}
	searchText := strings.Join([]string{record.Title, record.Category, record.Promotion, record.SiteID}, "\n")
	_, err = tx.ExecContext(ctx, `
INSERT INTO torrent_search_fts (site_id, torrent_id, search_text) VALUES (?, ?, ?)
`, record.SiteID, record.TorrentID, searchText)
	return err
}

func (s *SQLiteStore) searchTorrentPage(ctx context.Context, query TorrentListQuery) ([]TorrentRecord, int, error) {
	keys, total, err := s.searchTorrentKeys(ctx, query)
	if err != nil || len(keys) == 0 {
		return []TorrentRecord{}, total, err
	}
	records, err := s.listTorrentsByKeys(ctx, keys)
	return records, total, err
}

func (s *SQLiteStore) searchTorrentKeys(ctx context.Context, query TorrentListQuery) ([]TorrentKey, int, error) {
	search := strings.TrimSpace(query.Search)
	if len([]rune(search)) < 3 {
		return nil, 0, fmt.Errorf("trigram search requires at least three characters")
	}
	matchedSQL := `SELECT site_id, torrent_id FROM torrent_search_fts WHERE torrent_search_fts MATCH ?`
	args := []any{`"` + strings.ReplaceAll(search, `"`, `""`) + `"`}
	if len(query.SearchSiteIDs) > 0 {
		placeholders := make([]string, len(query.SearchSiteIDs))
		for i, siteID := range query.SearchSiteIDs {
			placeholders[i] = "?"
			args = append(args, siteID)
		}
		matchedSQL += ` UNION SELECT site_id, torrent_id FROM torrent_search WHERE site_id IN (` + strings.Join(placeholders, ",") + `)`
	}
	where := []string{"1=1"}
	filterArgs := []any{}
	if query.SiteID != "" {
		where = append(where, "p.site_id = ?")
		filterArgs = append(filterArgs, query.SiteID)
	}
	if len(query.ExcludeKeys) > 0 {
		exclusions := make([]string, 0, len(query.ExcludeKeys))
		for _, key := range query.ExcludeKeys {
			exclusions = append(exclusions, "(p.site_id = ? AND p.torrent_id = ?)")
			filterArgs = append(filterArgs, key.SiteID, key.TorrentID)
		}
		where = append(where, "NOT ("+strings.Join(exclusions, " OR ")+")")
	}
	orderBy := torrentSearchOrder(query)
	fromClause := `
FROM torrent_search p
JOIN matched m ON m.site_id = p.site_id AND m.torrent_id = p.torrent_id
WHERE ` + strings.Join(where, " AND ")
	countArgs := append(append([]any{}, args...), filterArgs...)
	var total int
	if err := s.indexDB.QueryRowContext(ctx, `WITH matched AS (`+matchedSQL+`)
SELECT COUNT(*) `+fromClause, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listArgs := append(countArgs, query.Limit, query.Offset)
	rows, err := s.indexDB.QueryContext(ctx, `WITH matched AS (`+matchedSQL+`)
SELECT p.site_id, p.torrent_id `+fromClause+`
ORDER BY `+orderBy+` LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var keys []TorrentKey
	for rows.Next() {
		var key TorrentKey
		if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
			return nil, 0, err
		}
		keys = append(keys, key)
	}
	return keys, total, rows.Err()
}

// listFilteredTorrentPage 在派生索引中先完成媒体条件筛选和范围计算，再按键读取主库记录。
func (s *SQLiteStore) listFilteredTorrentPage(ctx context.Context, query TorrentListQuery) ([]TorrentRecord, int, error) {
	keys, total, err := s.listFilteredTorrentKeys(ctx, query)
	if err != nil || len(keys) == 0 {
		return []TorrentRecord{}, total, err
	}
	records, err := s.listTorrentsByKeys(ctx, keys)
	return records, total, err
}

func (s *SQLiteStore) listFilteredTorrentKeys(ctx context.Context, query TorrentListQuery) ([]TorrentKey, int, error) {
	prefix := ""
	args := []any{}
	joins := `
FROM torrent_search p
LEFT JOIN torrent_qb_associations q
	ON q.site_id = p.site_id AND q.torrent_id = p.torrent_id`
	search := strings.TrimSpace(query.Search)
	where := []string{"1=1"}
	if len([]rune(search)) >= 3 {
		matchedSQL := `SELECT site_id, torrent_id FROM torrent_search_fts WHERE torrent_search_fts MATCH ?`
		args = append(args, `"`+strings.ReplaceAll(search, `"`, `""`)+`"`)
		if len(query.SearchSiteIDs) > 0 {
			placeholders := make([]string, len(query.SearchSiteIDs))
			for i, siteID := range query.SearchSiteIDs {
				placeholders[i] = "?"
				args = append(args, siteID)
			}
			matchedSQL += ` UNION SELECT site_id, torrent_id FROM torrent_search WHERE site_id IN (` + strings.Join(placeholders, ",") + `)`
		}
		prefix = `WITH matched AS (` + matchedSQL + `) `
		joins += `
JOIN matched m ON m.site_id = p.site_id AND m.torrent_id = p.torrent_id`
	} else if search != "" {
		searchClauses := []string{"p.title LIKE ?", "p.category LIKE ?", "p.promotion LIKE ?", "p.site_id LIKE ?"}
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern, pattern)
		for range query.SearchSiteIDs {
			searchClauses = append(searchClauses, "p.site_id = ?")
		}
		for _, siteID := range query.SearchSiteIDs {
			args = append(args, siteID)
		}
		where = append(where, "("+strings.Join(searchClauses, " OR ")+")")
	}
	if query.SiteID != "" {
		where = append(where, "p.site_id = ?")
		args = append(args, query.SiteID)
	}
	if len(query.Categories) > 0 {
		placeholders := make([]string, len(query.Categories))
		for i, category := range query.Categories {
			placeholders[i] = "?"
			args = append(args, category)
		}
		where = append(where, "p.category COLLATE NOCASE IN ("+strings.Join(placeholders, ",")+")")
	}
	for _, group := range query.SiteCheckboxes {
		if len(group.Values) == 0 {
			continue
		}
		placeholders := make([]string, len(group.Values))
		for i, value := range group.Values {
			placeholders[i] = "?"
			args = append(args, value)
		}
		where = append(where, `EXISTS (
			SELECT 1 FROM json_each(p.tag_ids_json) AS site_tag
			WHERE site_tag.value COLLATE NOCASE IN (`+strings.Join(placeholders, ",")+`)
		)`)
	}
	if len(query.Promotions) > 0 {
		promotionClauses := make([]string, 0, len(query.Promotions))
		for _, promotion := range query.Promotions {
			if strings.EqualFold(promotion, "normal") {
				promotionClauses = append(promotionClauses, "TRIM(p.promotion_class) = ''")
				continue
			}
			promotionClauses = append(promotionClauses, "(p.promotion_class = ? OR p.promotion_class LIKE ?)")
			args = append(args, promotion, promotion+" %")
		}
		where = append(where, "("+strings.Join(promotionClauses, " OR ")+")")
	}
	switch query.QBTask {
	case "present":
		where = append(where, "COALESCE(q.added, 0) = 1")
	case "absent":
		where = append(where, "COALESCE(q.added, 0) = 0")
	case "":
	default:
		return nil, 0, fmt.Errorf("unsupported qB task filter %q", query.QBTask)
	}
	rows, err := s.indexDB.QueryContext(ctx, prefix+`
SELECT p.site_id, p.torrent_id
`+joins+`
WHERE `+strings.Join(where, " AND ")+`
ORDER BY `+torrentSearchOrder(query), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	excluded := make(map[TorrentKey]struct{}, len(query.ExcludeKeys))
	for _, key := range query.ExcludeKeys {
		excluded[key] = struct{}{}
	}
	keys := make([]TorrentKey, 0, query.Limit)
	total := 0
	for rows.Next() {
		var key TorrentKey
		if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
			return nil, 0, err
		}
		if _, skip := excluded[key]; skip {
			continue
		}
		if query.QBProgressKeys != nil {
			if _, matches := query.QBProgressKeys[key]; !matches {
				continue
			}
		}
		if total >= query.Offset && len(keys) < query.Limit {
			keys = append(keys, key)
		}
		total++
	}
	return keys, total, rows.Err()
}

func torrentSearchOrder(query TorrentListQuery) string {
	column := map[string]string{
		"source_order": "p.source_order",
		"published_at": "p.published_at",
		"size_bytes":   "p.size_bytes",
		"seeders":      "p.seeders",
		"leechers":     "p.leechers",
		"snatches":     "p.snatches",
	}[strings.ToLower(strings.TrimSpace(query.SortBy))]
	if column == "" {
		column = "p.published_at"
	}
	direction := "ASC"
	if strings.EqualFold(query.SortDirection, "desc") || query.SortDirection == "" && column == "p.published_at" {
		direction = "DESC"
	}
	return column + " " + direction + ", p.site_id, p.torrent_id"
}

func (s *SQLiteStore) listTorrentsByKeys(ctx context.Context, keys []TorrentKey) ([]TorrentRecord, error) {
	query, args := torrentKeyQuery(`
SELECT site_id, torrent_id, source_order, title, category, category_query, detail_url, download_url, cover_url,
	tags_json, tag_ids_json, promotion, promotion_class, promotion_ends_at, promotion_remaining, description,
	detail_title, subtitle, product_url, detail_info_hash, detail_description, detail_raw_text, detail_fetched_at,
	size_text, size_bytes, seeders, leechers, snatches, comments, published_at, published_text,
	bookmarked, first_seen_at, last_seen_at
FROM torrents WHERE `, keys)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	records, err := scanTorrentRows(rows)
	if err != nil {
		return nil, err
	}
	byKey := make(map[TorrentKey]TorrentRecord, len(records))
	for _, record := range records {
		byKey[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = record
	}
	ordered := make([]TorrentRecord, 0, len(keys))
	for _, key := range keys {
		if record, ok := byKey[key]; ok {
			ordered = append(ordered, record)
		}
	}
	return ordered, nil
}

// ListTorrentsByKeys 按输入顺序读取一批种子，供内存态信息与持久化记录合并。
func (s *SQLiteStore) ListTorrentsByKeys(ctx context.Context, keys []TorrentKey) ([]TorrentRecord, error) {
	if len(keys) == 0 {
		return []TorrentRecord{}, nil
	}
	return s.listTorrentsByKeys(ctx, keys)
}

func scanTorrentRows(rows *sql.Rows) ([]TorrentRecord, error) {
	defer rows.Close()
	records := make([]TorrentRecord, 0)
	for rows.Next() {
		var record TorrentRecord
		var tagsJSON, tagIDsJSON string
		var bookmarked int
		var firstSeen, lastSeen string
		if err := rows.Scan(
			&record.SiteID, &record.TorrentID, &record.SourceOrder, &record.Title, &record.Category,
			&record.CategoryQuery, &record.DetailURL, &record.DownloadURL, &record.CoverURL, &tagsJSON,
			&tagIDsJSON, &record.Promotion, &record.PromotionClass, &record.PromotionEndsAt,
			&record.PromotionRemaining, &record.Description, &record.DetailTitle, &record.Subtitle,
			&record.ProductURL, &record.DetailInfoHash, &record.DetailDescription, &record.DetailRawText,
			&record.DetailFetchedAt, &record.SizeText, &record.SizeBytes, &record.Seeders, &record.Leechers,
			&record.Snatches, &record.Comments, &record.PublishedAt, &record.PublishedText,
			&bookmarked, &firstSeen, &lastSeen,
		); err != nil {
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
