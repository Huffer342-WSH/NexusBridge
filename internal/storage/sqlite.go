// Package storage 提供 NexusBridge 的 SQLite 持久化实现。
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	metadataApplicationID     = 0x4e584252 // NXBR
	indexApplicationID        = 0x4e584249 // NXBI
	metadataSchemaVersion     = 1
	derivedIndexSchemaVersion = 2
)

// SQLiteOptions 描述 SQLite 连接池和页缓存策略。
type SQLiteOptions struct {
	BusyTimeoutMillis int
	MaxOpenConns      int
	CacheKiB          int
	MmapBytes         int64
	Synchronous       string
}

// DefaultSQLiteOptions 返回适合单机运行的平衡配置。
func DefaultSQLiteOptions() SQLiteOptions {
	return SQLiteOptions{
		BusyTimeoutMillis: 10_000,
		MaxOpenConns:      4,
		CacheKiB:          16 * 1024,
		MmapBytes:         64 * 1024 * 1024,
		Synchronous:       "NORMAL",
	}
}

// SQLiteStore 同时管理业务元数据、可重建索引和 torrent 文件目录。
type SQLiteStore struct {
	db         *sql.DB
	indexDB    *sql.DB
	path       string
	indexPath  string
	torrentDir string
	writeMu    sync.Mutex
	indexMu    sync.Mutex
}

// OpenSQLite 打开 SQLite，并初始化独立的元数据和派生索引数据库。
func OpenSQLite(ctx context.Context, path string) (*SQLiteStore, error) {
	return OpenSQLiteWithOptions(ctx, path, DefaultSQLiteOptions())
}

// OpenSQLiteWithOptions 使用指定参数打开 SQLite。
func OpenSQLiteWithOptions(ctx context.Context, path string, options SQLiteOptions) (*SQLiteStore, error) {
	options = normalizeSQLiteOptions(options)
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	if err := prepareMetadataDatabase(ctx, absolutePath); err != nil {
		return nil, err
	}

	indexPath := derivedIndexPath(absolutePath)
	rebuildMarkerPath := indexRebuildMarkerPath(indexPath)
	if _, err := os.Stat(rebuildMarkerPath); err == nil {
		if err := removeIndexSQLiteFiles(indexPath); err != nil {
			return nil, fmt.Errorf("discard interrupted index rebuild: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect index rebuild marker: %w", err)
	}
	rebuildIndexes, err := prepareIndexDatabase(ctx, indexPath)
	if err != nil {
		return nil, err
	}
	if rebuildIndexes {
		if err := createIndexRebuildMarker(rebuildMarkerPath); err != nil {
			return nil, err
		}
	}
	db, err := openSQLitePool(ctx, absolutePath, options)
	if err != nil {
		return nil, fmt.Errorf("open metadata database: %w", err)
	}
	indexDB, err := openSQLitePool(ctx, indexPath, options)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open index database: %w", err)
	}
	store := &SQLiteStore{
		db:         db,
		indexDB:    indexDB,
		path:       absolutePath,
		indexPath:  indexPath,
		torrentDir: filepath.Join(filepath.Dir(absolutePath), "torrents"),
	}
	if err := os.MkdirAll(store.torrentDir, 0o755); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create torrent directory: %w", err)
	}
	if err := store.InitializeSchema(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	if rebuildIndexes {
		if err := store.rebuildDerivedIndexes(ctx); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("rebuild derived indexes: %w", err)
		}
	}
	if err := store.checkDatabaseHealth(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	if rebuildIndexes {
		if err := os.Remove(rebuildMarkerPath); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("remove index rebuild marker: %w", err)
		}
	}
	return store, nil
}

func normalizeSQLiteOptions(options SQLiteOptions) SQLiteOptions {
	defaults := DefaultSQLiteOptions()
	if options.BusyTimeoutMillis <= 0 {
		options.BusyTimeoutMillis = defaults.BusyTimeoutMillis
	}
	if options.MaxOpenConns <= 0 {
		options.MaxOpenConns = defaults.MaxOpenConns
	}
	if options.CacheKiB <= 0 {
		options.CacheKiB = defaults.CacheKiB
	}
	if options.MmapBytes < 0 {
		options.MmapBytes = defaults.MmapBytes
	}
	options.Synchronous = strings.ToUpper(strings.TrimSpace(options.Synchronous))
	if options.Synchronous == "" {
		options.Synchronous = defaults.Synchronous
	}
	return options
}

func openSQLitePool(ctx context.Context, path string, options SQLiteOptions) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteDataSourceName(path, options))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(options.MaxOpenConns)
	db.SetMaxIdleConns(options.MaxOpenConns)
	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: sqlite returned %q", journalMode)
	}
	return db, nil
}

// sqliteDataSourceName 确保连接池创建的每条连接使用相同的本地数据库策略。
func sqliteDataSourceName(path string, options SQLiteOptions) string {
	query := url.Values{}
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", options.BusyTimeoutMillis))
	query.Add("_pragma", fmt.Sprintf("synchronous(%s)", options.Synchronous))
	query.Add("_pragma", "temp_store(MEMORY)")
	query.Add("_pragma", fmt.Sprintf("cache_size(-%d)", options.CacheKiB))
	query.Add("_pragma", fmt.Sprintf("mmap_size(%d)", options.MmapBytes))
	query.Add("_pragma", "foreign_keys(ON)")
	query.Set("_txlock", "immediate")
	return filepath.ToSlash(path) + "?" + query.Encode()
}

func derivedIndexPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".index" + ext
}

func indexRebuildMarkerPath(indexPath string) string {
	return indexPath + ".rebuilding"
}

func createIndexRebuildMarker(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create index rebuild marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync index rebuild marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close index rebuild marker: %w", err)
	}
	return nil
}

func prepareMetadataDatabase(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect metadata database: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return fmt.Errorf("inspect metadata database: %w", err)
	}
	defer db.Close()

	var applicationID, userVersion, tableCount int
	if err := db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
		return fmt.Errorf("inspect metadata application id: %w", err)
	}
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("inspect metadata schema version: %w", err)
	}
	if applicationID == metadataApplicationID {
		if userVersion != metadataSchemaVersion {
			return fmt.Errorf("unsupported NexusBridge database schema version %d; expected %d", userVersion, metadataSchemaVersion)
		}
		return nil
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect metadata schema: %w", err)
	}
	if tableCount == 0 {
		return nil
	}
	legacy, err := isLegacyNexusBridgeDatabase(ctx, db)
	if err != nil {
		return err
	}
	if !legacy {
		return fmt.Errorf("refuse to replace unknown database at %s", path)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close legacy database: %w", err)
	}
	return removeSQLiteFiles(path)
}

func isLegacyNexusBridgeDatabase(ctx context.Context, db *sql.DB) (bool, error) {
	required := []string{"torrents", "torrent_files", "torrent_qb_snapshots", "site_credentials"}
	for _, table := range required {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			return false, fmt.Errorf("inspect legacy table %s: %w", table, err)
		}
		if count != 1 {
			return false, nil
		}
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(torrent_files)`)
	if err != nil {
		return false, fmt.Errorf("inspect legacy torrent files: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if strings.EqualFold(name, "data") && strings.EqualFold(columnType, "BLOB") {
			return true, nil
		}
	}
	return false, rows.Err()
}

func removeSQLiteFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		err := os.Remove(candidate)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove legacy sqlite file %s: %w", candidate, err)
		}
	}
	indexPath := derivedIndexPath(path)
	if err := removeIndexSQLiteFiles(indexPath); err != nil {
		return err
	}
	return nil
}

func removeIndexSQLiteFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		err := os.Remove(candidate)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove derived sqlite file %s: %w", candidate, err)
		}
	}
	return nil
}

func prepareIndexDatabase(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect index database: %w", err)
	}
	if info.Size() == 0 {
		return true, nil
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return false, fmt.Errorf("inspect index database: %w", err)
	}
	var applicationID, userVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
		_ = db.Close()
		return false, fmt.Errorf("inspect index application id: %w", err)
	}
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		_ = db.Close()
		return false, fmt.Errorf("inspect index schema version: %w", err)
	}
	if applicationID == indexApplicationID && userVersion == derivedIndexSchemaVersion {
		return false, db.Close()
	}
	var knownTables int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM sqlite_master
WHERE type IN ('table', 'view') AND name IN ('torrent_search', 'torrent_search_fts', 'torrent_size_signatures', 'torrent_qb_associations')
`).Scan(&knownTables); err != nil {
		_ = db.Close()
		return false, err
	}
	if err := db.Close(); err != nil {
		return false, err
	}
	if applicationID != 0 && applicationID != indexApplicationID || knownTables == 0 {
		return false, fmt.Errorf("refuse to replace unknown index database at %s", path)
	}
	if err := removeIndexSQLiteFiles(path); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) withWriteTx(ctx context.Context, fn func(*sql.Tx) error) error {
	started := time.Now()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		logSQLiteBusy("metadata", started, err)
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if err := fn(tx); err != nil {
		logSQLiteBusy("metadata", started, err)
		return err
	}
	err = tx.Commit()
	logSQLiteBusy("metadata", started, err)
	return err
}

func (s *SQLiteStore) checkDatabaseHealth(ctx context.Context) error {
	for _, database := range []struct {
		name          string
		db            *sql.DB
		applicationID int
		schemaVersion int
	}{
		{name: "metadata", db: s.db, applicationID: metadataApplicationID, schemaVersion: metadataSchemaVersion},
		{name: "index", db: s.indexDB, applicationID: indexApplicationID, schemaVersion: derivedIndexSchemaVersion},
	} {
		var applicationID, userVersion int
		var journalMode string
		if err := database.db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
			return fmt.Errorf("check %s application id: %w", database.name, err)
		}
		if err := database.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
			return fmt.Errorf("check %s schema version: %w", database.name, err)
		}
		if err := database.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			return fmt.Errorf("check %s WAL mode: %w", database.name, err)
		}
		if applicationID != database.applicationID || userVersion != database.schemaVersion || !strings.EqualFold(journalMode, "wal") {
			return fmt.Errorf("%s database health check failed: application_id=%d user_version=%d journal_mode=%s",
				database.name, applicationID, userVersion, journalMode)
		}
	}
	return nil
}

func (s *SQLiteStore) beginWriteTx(ctx context.Context) (*sql.Tx, func(), error) {
	s.writeMu.Lock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.writeMu.Unlock()
		return nil, func() {}, err
	}
	return tx, s.writeMu.Unlock, nil
}

func (s *SQLiteStore) execWriteContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	started := time.Now()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	result, err := s.db.ExecContext(ctx, query, args...)
	logSQLiteBusy("metadata", started, err)
	return result, err
}

func (s *SQLiteStore) withIndexWriteTx(ctx context.Context, fn func(*sql.Tx) error) error {
	started := time.Now()
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	tx, err := s.indexDB.BeginTx(ctx, nil)
	if err != nil {
		logSQLiteBusy("index", started, err)
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if err := fn(tx); err != nil {
		logSQLiteBusy("index", started, err)
		return err
	}
	err = tx.Commit()
	logSQLiteBusy("index", started, err)
	return err
}

func logSQLiteBusy(database string, started time.Time, err error) {
	if err == nil {
		return
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "sqlite_busy") || strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") {
		slog.Warn("sqlite busy", "database", database, "wait", time.Since(started), "error", err)
	}
}

// Optimize 让 SQLite 按需更新查询规划统计，不执行 VACUUM。
func (s *SQLiteStore) Optimize(ctx context.Context) error {
	if _, err := s.execWriteContext(ctx, `PRAGMA optimize`); err != nil {
		return err
	}
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	_, err := s.indexDB.ExecContext(ctx, `PRAGMA optimize`)
	return err
}

// Close 优化并关闭两个 SQLite 连接池。
func (s *SQLiteStore) Close() error {
	if s == nil {
		return nil
	}
	if s.db != nil {
		_, _ = s.db.Exec(`PRAGMA optimize`)
	}
	if s.indexDB != nil {
		_, _ = s.indexDB.Exec(`PRAGMA optimize`)
	}
	var firstErr error
	if s.indexDB != nil {
		firstErr = s.indexDB.Close()
	}
	if s.db != nil {
		if err := s.db.Close(); firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
