// Package storage 提供 NexusBridge 的 SQLite 持久化实现。
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

const (
	sqliteBusyTimeoutMillis = 10_000
	sqliteMaxOpenConns      = 4
)

// OpenSQLite 打开 SQLite，并为连接池中的每条连接设置锁等待策略。
func OpenSQLite(ctx context.Context, path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", sqliteDataSourceName(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(sqliteMaxOpenConns)
	db.SetMaxIdleConns(sqliteMaxOpenConns)
	store := &SQLiteStore{db: db}
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite journal mode: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// sqliteDataSourceName 确保 modernc 为连接池后续创建的每条连接执行 busy_timeout。
func sqliteDataSourceName(path string) string {
	query := url.Values{}
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMillis))
	return filepath.ToSlash(path) + "?" + query.Encode()
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cookies (
	scope TEXT NOT NULL,
	name TEXT NOT NULL,
	value TEXT NOT NULL,
	path TEXT NOT NULL DEFAULT '',
	domain TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (scope, name)
);

CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_credentials (
	site_id TEXT PRIMARY KEY,
	base_url TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	headers_json TEXT NOT NULL DEFAULT '{}',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS torrents (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	source_order INTEGER NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	category_query TEXT NOT NULL DEFAULT '',
	detail_url TEXT NOT NULL DEFAULT '',
	download_url TEXT NOT NULL DEFAULT '',
	cover_url TEXT NOT NULL DEFAULT '',
	tags_json TEXT NOT NULL DEFAULT '[]',
	tag_ids_json TEXT NOT NULL DEFAULT '[]',
	promotion TEXT NOT NULL DEFAULT '',
	promotion_class TEXT NOT NULL DEFAULT '',
	promotion_ends_at TEXT NOT NULL DEFAULT '',
	promotion_remaining TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	detail_title TEXT NOT NULL DEFAULT '',
	subtitle TEXT NOT NULL DEFAULT '',
	product_url TEXT NOT NULL DEFAULT '',
	detail_info_hash TEXT NOT NULL DEFAULT '',
	detail_description TEXT NOT NULL DEFAULT '',
	detail_raw_text TEXT NOT NULL DEFAULT '',
	detail_fetched_at TEXT NOT NULL DEFAULT '',
	size_text TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	seeders INTEGER NOT NULL DEFAULT 0,
	leechers INTEGER NOT NULL DEFAULT 0,
	snatches INTEGER NOT NULL DEFAULT 0,
	comments INTEGER NOT NULL DEFAULT 0,
	published_at TEXT NOT NULL DEFAULT '',
	published_text TEXT NOT NULL DEFAULT '',
	sticky_level INTEGER NOT NULL DEFAULT 0,
	bookmarked INTEGER NOT NULL DEFAULT 0,
	first_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);

CREATE TABLE IF NOT EXISTS cover_cache (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	source_url TEXT NOT NULL,
	local_path TEXT NOT NULL DEFAULT '',
	mime_type TEXT NOT NULL DEFAULT '',
	file_size INTEGER NOT NULL DEFAULT 0,
	sha256 TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	last_error TEXT NOT NULL DEFAULT '',
	last_success_at TEXT NOT NULL DEFAULT '',
	last_failed_at TEXT NOT NULL DEFAULT '',
	fail_count INTEGER NOT NULL DEFAULT 0,
	attempt_version INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);

CREATE TABLE IF NOT EXISTS torrent_files (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	data BLOB,
	info_hash_v1 TEXT NOT NULL DEFAULT '',
	info_hash_v2 TEXT NOT NULL DEFAULT '',
	original_name TEXT NOT NULL DEFAULT '',
	byte_size INTEGER NOT NULL DEFAULT 0,
	fetched_at TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);

CREATE TABLE IF NOT EXISTS torrent_file_size_index (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	file_size INTEGER NOT NULL,
	occurrence_count INTEGER NOT NULL,
	PRIMARY KEY (site_id, torrent_id, file_size)
);

CREATE TABLE IF NOT EXISTS torrent_qb_snapshots (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	added INTEGER NOT NULL DEFAULT 0,
	qb_hash TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL DEFAULT '',
	progress REAL NOT NULL DEFAULT 0,
	category TEXT NOT NULL DEFAULT '',
	tags_json TEXT NOT NULL DEFAULT '[]',
	save_path TEXT NOT NULL DEFAULT '',
	content_path TEXT NOT NULL DEFAULT '',
	total_size INTEGER NOT NULL DEFAULT 0,
	amount_left INTEGER NOT NULL DEFAULT 0,
	downloaded INTEGER NOT NULL DEFAULT 0,
	uploaded INTEGER NOT NULL DEFAULT 0,
	download_speed INTEGER NOT NULL DEFAULT 0,
	upload_speed INTEGER NOT NULL DEFAULT 0,
	eta INTEGER NOT NULL DEFAULT 0,
	ratio REAL NOT NULL DEFAULT 0,
	tracker TEXT NOT NULL DEFAULT '',
	is_private INTEGER NOT NULL DEFAULT 0,
	added_on INTEGER NOT NULL DEFAULT 0,
	completion_on INTEGER NOT NULL DEFAULT 0,
	creation_date INTEGER NOT NULL DEFAULT 0,
	piece_size INTEGER NOT NULL DEFAULT 0,
	comment TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	synced_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);

CREATE TABLE IF NOT EXISTS rules (
	name TEXT PRIMARY KEY COLLATE NOCASE,
	site_ids_json TEXT NOT NULL DEFAULT '[]',
	site_categories_json TEXT NOT NULL DEFAULT '[]',
	site_tags_json TEXT NOT NULL DEFAULT '[]',
	subtitle_tags_json TEXT NOT NULL DEFAULT '[]',
	title_expression TEXT NOT NULL DEFAULT '',
	promotions_json TEXT NOT NULL DEFAULT '[]',
	min_size INTEGER NOT NULL DEFAULT 0,
	max_size INTEGER NOT NULL DEFAULT 0,
	min_seeders INTEGER NOT NULL DEFAULT 0,
	max_seeders INTEGER NOT NULL DEFAULT 0,
	min_leechers INTEGER NOT NULL DEFAULT 0,
	max_leechers INTEGER NOT NULL DEFAULT 0,
	min_snatches INTEGER NOT NULL DEFAULT 0,
	max_snatches INTEGER NOT NULL DEFAULT 0,
	published_within_minutes INTEGER NOT NULL DEFAULT 0,
	sort_by TEXT NOT NULL DEFAULT 'source_order',
	sort_direction TEXT NOT NULL DEFAULT 'asc',
	action TEXT NOT NULL DEFAULT 'download',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subscriptions (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	rule_name TEXT NOT NULL COLLATE NOCASE,
	enabled INTEGER NOT NULL DEFAULT 0,
	site_ids_json TEXT NOT NULL DEFAULT '[]',
	priority INTEGER NOT NULL DEFAULT 0,
	qb_category TEXT NOT NULL DEFAULT '',
	save_path_template TEXT NOT NULL DEFAULT '',
	qb_tags_json TEXT NOT NULL DEFAULT '[]',
	filename_template TEXT NOT NULL DEFAULT '',
	paused INTEGER NOT NULL DEFAULT 0,
	max_concurrent INTEGER NOT NULL DEFAULT 0,
	daily_limit INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_schedules (
	site_id TEXT PRIMARY KEY,
	enabled INTEGER NOT NULL DEFAULT 0,
	interval_minutes INTEGER NOT NULL DEFAULT 15,
	last_run_at TEXT NOT NULL DEFAULT '',
	next_run_at TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_fetch_jobs (
	id TEXT PRIMARY KEY,
	site_id TEXT NOT NULL,
	trigger TEXT NOT NULL,
	mode TEXT NOT NULL,
	requested_pages INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,
	current_page INTEGER NOT NULL DEFAULT 0,
	pages_fetched INTEGER NOT NULL DEFAULT 0,
	fetched_count INTEGER NOT NULL DEFAULT 0,
	inserted_count INTEGER NOT NULL DEFAULT 0,
	changed_count INTEGER NOT NULL DEFAULT 0,
	matched_count INTEGER NOT NULL DEFAULT 0,
	sent_count INTEGER NOT NULL DEFAULT 0,
	files_saved INTEGER NOT NULL DEFAULT 0,
	files_failed INTEGER NOT NULL DEFAULT 0,
	stop_reason TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subscription_candidates (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	subscription_id TEXT NOT NULL,
	rule_name TEXT NOT NULL COLLATE NOCASE,
	source_order INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'unread',
	reason_code TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	processed_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (site_id, torrent_id)
);

CREATE TABLE IF NOT EXISTS subscription_ingest_queue (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	source_order INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
);

CREATE TABLE IF NOT EXISTS subscription_runs (
	id TEXT PRIMARY KEY,
	subscription_id TEXT NOT NULL DEFAULT '',
	site_id TEXT NOT NULL DEFAULT '',
	trigger TEXT NOT NULL,
	status TEXT NOT NULL,
	fetched_count INTEGER NOT NULL DEFAULT 0,
	inserted_count INTEGER NOT NULL DEFAULT 0,
	matched_count INTEGER NOT NULL DEFAULT 0,
	attempted_count INTEGER NOT NULL DEFAULT 0,
	sent_count INTEGER NOT NULL DEFAULT 0,
	exists_count INTEGER NOT NULL DEFAULT 0,
	failed_count INTEGER NOT NULL DEFAULT 0,
	skipped_count INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS qb_categories (
	name TEXT PRIMARY KEY,
	save_path TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	synced_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS qb_tags (
	name TEXT PRIMARY KEY,
	last_error TEXT NOT NULL DEFAULT '',
	synced_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS qb_cache_state (
	kind TEXT PRIMARY KEY,
	last_error TEXT NOT NULL DEFAULT '',
	synced_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS download_tasks (
	id TEXT PRIMARY KEY,
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	rule_name TEXT NOT NULL COLLATE NOCASE,
	subscription_id TEXT NOT NULL DEFAULT '',
	trigger TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	torrent_title TEXT NOT NULL,
	download_url TEXT NOT NULL,
	qb_hash TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	content_path TEXT NOT NULL DEFAULT '',
	plan_category TEXT NOT NULL DEFAULT '',
	plan_save_path TEXT NOT NULL DEFAULT '',
	plan_tags_json TEXT NOT NULL DEFAULT '[]',
	plan_rename TEXT NOT NULL DEFAULT '',
	plan_paused INTEGER NOT NULL DEFAULT 0,
	reason_code TEXT NOT NULL DEFAULT '',
	attempt_count INTEGER NOT NULL DEFAULT 0,
	retry_count INTEGER NOT NULL DEFAULT 0,
	last_attempt_at TEXT NOT NULL DEFAULT '',
	sent_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (site_id, torrent_id, rule_name)
);

CREATE TABLE IF NOT EXISTS organize_tasks (
	id TEXT PRIMARY KEY,
	download_task_id TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL,
	title TEXT NOT NULL,
	source_path TEXT NOT NULL,
	relative_dir TEXT NOT NULL DEFAULT '',
	filename TEXT NOT NULL DEFAULT '',
	target_path TEXT NOT NULL DEFAULT '',
	llm_response TEXT NOT NULL DEFAULT '',
	confidence REAL NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_download_tasks_site_torrent ON download_tasks (site_id, torrent_id);
CREATE INDEX IF NOT EXISTS idx_download_tasks_qb_hash ON download_tasks (qb_hash);
CREATE INDEX IF NOT EXISTS idx_torrent_files_info_hash_v1 ON torrent_files (info_hash_v1 COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_torrent_qb_snapshots_hash ON torrent_qb_snapshots (qb_hash COLLATE NOCASE);
`)
	if err != nil {
		return err
	}
	tableColumns := map[string][]struct {
		name       string
		definition string
	}{
		"torrents": {
			{"source_order", "INTEGER NOT NULL DEFAULT 0"},
			{"detail_title", "TEXT NOT NULL DEFAULT ''"},
			{"subtitle", "TEXT NOT NULL DEFAULT ''"},
			{"tag_ids_json", "TEXT NOT NULL DEFAULT '[]'"},
			{"product_url", "TEXT NOT NULL DEFAULT ''"},
			{"detail_info_hash", "TEXT NOT NULL DEFAULT ''"},
			{"detail_description", "TEXT NOT NULL DEFAULT ''"},
			{"detail_raw_text", "TEXT NOT NULL DEFAULT ''"},
			{"detail_fetched_at", "TEXT NOT NULL DEFAULT ''"},
		},
		"torrent_files": {
			{"original_name", "TEXT NOT NULL DEFAULT ''"},
			{"content_file_count", "INTEGER NOT NULL DEFAULT 0"},
			{"content_total_size", "INTEGER NOT NULL DEFAULT 0"},
			{"size_index_version", "INTEGER NOT NULL DEFAULT 0"},
			{"size_indexed_at", "TEXT NOT NULL DEFAULT ''"},
			{"size_index_error", "TEXT NOT NULL DEFAULT ''"},
		},
		"cover_cache": {
			{"attempt_version", "INTEGER NOT NULL DEFAULT 0"},
		},
		"download_tasks": {
			{"subscription_id", "TEXT NOT NULL DEFAULT ''"},
			{"trigger", "TEXT NOT NULL DEFAULT ''"},
			{"plan_category", "TEXT NOT NULL DEFAULT ''"},
			{"plan_save_path", "TEXT NOT NULL DEFAULT ''"},
			{"plan_tags_json", "TEXT NOT NULL DEFAULT '[]'"},
			{"plan_rename", "TEXT NOT NULL DEFAULT ''"},
			{"plan_paused", "INTEGER NOT NULL DEFAULT 0"},
			{"reason_code", "TEXT NOT NULL DEFAULT ''"},
			{"attempt_count", "INTEGER NOT NULL DEFAULT 0"},
			{"retry_count", "INTEGER NOT NULL DEFAULT 0"},
			{"last_attempt_at", "TEXT NOT NULL DEFAULT ''"},
			{"sent_at", "TEXT NOT NULL DEFAULT ''"},
		},
		"subscription_runs": {
			{"inserted_count", "INTEGER NOT NULL DEFAULT 0"},
			{"attempted_count", "INTEGER NOT NULL DEFAULT 0"},
		},
	}
	for table, columns := range tableColumns {
		for _, column := range columns {
			if err := s.addColumnIfMissing(ctx, table, column.name, column.definition); err != nil {
				return err
			}
		}
	}
	_, err = s.db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_torrents_site_source_order ON torrents (site_id, source_order, torrent_id);
CREATE INDEX IF NOT EXISTS idx_torrents_published ON torrents (published_at DESC, site_id, torrent_id);
CREATE INDEX IF NOT EXISTS idx_torrents_site_published ON torrents (site_id, published_at DESC, torrent_id);
CREATE INDEX IF NOT EXISTS idx_torrents_site_sticky_published ON torrents (site_id, sticky_level DESC, published_at DESC, torrent_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_enabled_priority ON subscriptions (enabled, priority DESC, id);
CREATE INDEX IF NOT EXISTS idx_site_schedules_due ON site_schedules (enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_site_fetch_jobs_site_created ON site_fetch_jobs (site_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_site_fetch_jobs_status ON site_fetch_jobs (status, updated_at);
CREATE INDEX IF NOT EXISTS idx_subscription_candidates_status ON subscription_candidates (subscription_id, status, source_order, site_id, torrent_id);
CREATE INDEX IF NOT EXISTS idx_subscription_ingest_site_order ON subscription_ingest_queue (site_id, source_order, torrent_id);
CREATE INDEX IF NOT EXISTS idx_subscription_runs_subscription ON subscription_runs (subscription_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_download_tasks_subscription_status ON download_tasks (subscription_id, status, sent_at);
CREATE INDEX IF NOT EXISTS idx_torrent_file_size_lookup ON torrent_file_size_index (file_size, occurrence_count, site_id, torrent_id);
CREATE TRIGGER IF NOT EXISTS cleanup_torrent_file_size_index
AFTER DELETE ON torrent_files
BEGIN
	DELETE FROM torrent_file_size_index WHERE site_id = OLD.site_id AND torrent_id = OLD.torrent_id;
END;
CREATE TRIGGER IF NOT EXISTS prevent_duplicate_download_task_rule
BEFORE INSERT ON download_tasks
WHEN EXISTS (
	SELECT 1 FROM download_tasks
	WHERE site_id = NEW.site_id AND torrent_id = NEW.torrent_id AND rule_name = NEW.rule_name
)
BEGIN
	SELECT RAISE(IGNORE);
END;
`)
	return err
}

// addColumnIfMissing 为旧数据库补齐缺失列。
func (s *SQLiteStore) addColumnIfMissing(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if strings.EqualFold(name, column) {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
