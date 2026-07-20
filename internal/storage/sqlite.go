// Package storage 提供 NexusBridge 的 SQLite 持久化实现。
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

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

// Close 关闭 SQLite 连接池。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
