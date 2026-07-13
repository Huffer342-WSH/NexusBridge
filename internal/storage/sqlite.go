// Package storage 提供 NexusBridge 的 SQLite 持久化实现。
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(ctx context.Context, path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(path))
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{db: db}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
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

CREATE TABLE IF NOT EXISTS torrent_files (
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	data BLOB,
	info_hash_v1 TEXT NOT NULL DEFAULT '',
	info_hash_v2 TEXT NOT NULL DEFAULT '',
	byte_size INTEGER NOT NULL DEFAULT 0,
	fetched_at TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (site_id, torrent_id)
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
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 0,
	site_ids_json TEXT NOT NULL DEFAULT '[]',
	categories_json TEXT NOT NULL DEFAULT '[]',
	tags_json TEXT NOT NULL DEFAULT '[]',
	include_text TEXT NOT NULL DEFAULT '',
	exclude_text TEXT NOT NULL DEFAULT '',
	promotion TEXT NOT NULL DEFAULT '',
	min_size INTEGER NOT NULL DEFAULT 0,
	max_size INTEGER NOT NULL DEFAULT 0,
	min_seeders INTEGER NOT NULL DEFAULT 0,
	action TEXT NOT NULL DEFAULT 'download',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS download_tasks (
	id TEXT PRIMARY KEY,
	site_id TEXT NOT NULL,
	torrent_id TEXT NOT NULL,
	rule_id TEXT NOT NULL,
	status TEXT NOT NULL,
	torrent_title TEXT NOT NULL,
	download_url TEXT NOT NULL,
	qb_hash TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	content_path TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (site_id, torrent_id, rule_id)
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
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"detail_title", "TEXT NOT NULL DEFAULT ''"},
		{"subtitle", "TEXT NOT NULL DEFAULT ''"},
		{"tag_ids_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"product_url", "TEXT NOT NULL DEFAULT ''"},
		{"detail_info_hash", "TEXT NOT NULL DEFAULT ''"},
		{"detail_description", "TEXT NOT NULL DEFAULT ''"},
		{"detail_raw_text", "TEXT NOT NULL DEFAULT ''"},
		{"detail_fetched_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.addColumnIfMissing(ctx, "torrents", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

// addColumnIfMissing 为旧数据库补齐缺失列。
func (s *SQLiteStore) addColumnIfMissing(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
