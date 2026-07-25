package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// RuleRecord 表示筛选规则的数据库记录。
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

// RuleFacetRecord 汇总站点中可用于规则筛选的维度。
type RuleFacetRecord struct {
	SiteCategories []string
	SiteTags       []string
	SubtitleTags   []string
}

// SaveRule 新增或更新筛选规则。
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
	_, err := s.execWriteContext(ctx, `
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
	tx, release, err := s.beginWriteTx(ctx)
	if err != nil {
		return err
	}
	defer release()
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

// ListRules 按名称返回全部筛选规则。
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
	result, err := s.execWriteContext(ctx, `DELETE FROM rules WHERE name = ? COLLATE NOCASE`, name)
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
