// Package storage 提供 qBittorrent 状态快照的 SQLite 持久化。
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// QBSnapshotRecord 表示本地种子关联的 qBittorrent 状态快照。
type QBSnapshotRecord struct {
	SiteID        string
	TorrentID     string
	Added         bool
	QBHash        string
	Name          string
	State         string
	Progress      float64
	Category      string
	Tags          []string
	SavePath      string
	ContentPath   string
	TotalSize     int64
	AmountLeft    int64
	Downloaded    int64
	Uploaded      int64
	DownloadSpeed int64
	UploadSpeed   int64
	ETA           int64
	Ratio         float64
	Tracker       string
	IsPrivate     bool
	AddedOn       int64
	CompletionOn  int64
	CreationDate  int64
	PieceSize     int64
	Comment       string
	CreatedBy     string
	SyncedAt      time.Time
}

// SaveQBSnapshot 保存单个种子的 qBittorrent 状态快照。
func (s *SQLiteStore) SaveQBSnapshot(ctx context.Context, record QBSnapshotRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if err := upsertQBSnapshot(ctx, tx, record); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceQBSnapshots 用一次成功的 qB 全量查询替换当前匹配状态。
func (s *SQLiteStore) ReplaceQBSnapshots(ctx context.Context, records []QBSnapshotRecord) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer rollbackUnlessCommitted(tx)
	previouslyAdded := map[TorrentKey]struct{}{}
	rows, err := tx.QueryContext(ctx, `SELECT site_id, torrent_id FROM torrent_qb_snapshots WHERE added = 1`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var key TorrentKey
		if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
			rows.Close()
			return 0, err
		}
		previouslyAdded[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE torrent_qb_snapshots
SET added = 0, state = 'missing', progress = 0, download_speed = 0, upload_speed = 0,
	eta = 0, amount_left = 0, synced_at = CURRENT_TIMESTAMP
WHERE added = 1
`)
	if err != nil {
		return 0, err
	}
	for _, record := range records {
		if err := upsertQBSnapshot(ctx, tx, record); err != nil {
			return 0, err
		}
		if record.Added {
			delete(previouslyAdded, TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID})
		}
	}
	return len(previouslyAdded), tx.Commit()
}

// ListQBSnapshots 返回指定本地种子的 qBittorrent 快照。
func (s *SQLiteStore) ListQBSnapshots(ctx context.Context, keys []TorrentKey) (map[TorrentKey]QBSnapshotRecord, error) {
	result := make(map[TorrentKey]QBSnapshotRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	query, args := torrentKeyQuery(qbSnapshotSelect+" WHERE ", keys)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanQBSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = record
	}
	return result, rows.Err()
}

// GetQBSnapshot 读取单个本地种子的 qBittorrent 快照。
func (s *SQLiteStore) GetQBSnapshot(ctx context.Context, key TorrentKey) (QBSnapshotRecord, bool, error) {
	record, err := scanQBSnapshot(s.db.QueryRowContext(ctx, qbSnapshotSelect+" WHERE site_id = ? AND torrent_id = ?", key.SiteID, key.TorrentID))
	if err == sql.ErrNoRows {
		return QBSnapshotRecord{}, false, nil
	}
	if err != nil {
		return QBSnapshotRecord{}, false, err
	}
	return record, true, nil
}

const qbSnapshotSelect = `
SELECT site_id, torrent_id, added, qb_hash, name, state, progress, category, tags_json,
	save_path, content_path, total_size, amount_left, downloaded, uploaded, download_speed,
	upload_speed, eta, ratio, tracker, is_private, added_on, completion_on, creation_date,
	piece_size, comment, created_by, synced_at
FROM torrent_qb_snapshots`

// qbSnapshotScanner 统一 sql.Row 和 sql.Rows 的扫描接口。
type qbSnapshotScanner interface {
	Scan(dest ...any) error
}

// scanQBSnapshot 从数据库行解析 qB 状态快照。
func scanQBSnapshot(scanner qbSnapshotScanner) (QBSnapshotRecord, error) {
	var record QBSnapshotRecord
	var added, private int
	var tagsJSON, syncedAt string
	err := scanner.Scan(&record.SiteID, &record.TorrentID, &added, &record.QBHash, &record.Name, &record.State,
		&record.Progress, &record.Category, &tagsJSON, &record.SavePath, &record.ContentPath, &record.TotalSize,
		&record.AmountLeft, &record.Downloaded, &record.Uploaded, &record.DownloadSpeed, &record.UploadSpeed,
		&record.ETA, &record.Ratio, &record.Tracker, &private, &record.AddedOn, &record.CompletionOn,
		&record.CreationDate, &record.PieceSize, &record.Comment, &record.CreatedBy, &syncedAt)
	if err != nil {
		return QBSnapshotRecord{}, err
	}
	record.Added = added != 0
	record.IsPrivate = private != 0
	_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
	record.SyncedAt = parseDBTime(syncedAt)
	return record, nil
}

// upsertQBSnapshot 在事务内新增或覆盖 qB 状态快照。
func upsertQBSnapshot(ctx context.Context, tx *sql.Tx, record QBSnapshotRecord) error {
	tagsJSON, err := json.Marshal(record.Tags)
	if err != nil {
		return err
	}
	syncedAt := record.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now()
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO torrent_qb_snapshots (
	site_id, torrent_id, added, qb_hash, name, state, progress, category, tags_json,
	save_path, content_path, total_size, amount_left, downloaded, uploaded, download_speed,
	upload_speed, eta, ratio, tracker, is_private, added_on, completion_on, creation_date,
	piece_size, comment, created_by, synced_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	added = excluded.added, qb_hash = excluded.qb_hash, name = excluded.name, state = excluded.state,
	progress = excluded.progress, category = excluded.category, tags_json = excluded.tags_json,
	save_path = excluded.save_path, content_path = excluded.content_path, total_size = excluded.total_size,
	amount_left = excluded.amount_left, downloaded = excluded.downloaded, uploaded = excluded.uploaded,
	download_speed = excluded.download_speed, upload_speed = excluded.upload_speed, eta = excluded.eta,
	ratio = excluded.ratio, tracker = excluded.tracker, is_private = excluded.is_private,
	added_on = excluded.added_on, completion_on = excluded.completion_on, creation_date = excluded.creation_date,
	piece_size = excluded.piece_size, comment = excluded.comment, created_by = excluded.created_by,
	synced_at = excluded.synced_at
`, record.SiteID, record.TorrentID, boolInt(record.Added), record.QBHash, record.Name, record.State,
		record.Progress, record.Category, string(tagsJSON), record.SavePath, record.ContentPath, record.TotalSize,
		record.AmountLeft, record.Downloaded, record.Uploaded, record.DownloadSpeed, record.UploadSpeed,
		record.ETA, record.Ratio, record.Tracker, boolInt(record.IsPrivate), record.AddedOn, record.CompletionOn,
		record.CreationDate, record.PieceSize, record.Comment, record.CreatedBy, syncedAt.UTC().Format(time.RFC3339))
	return err
}
