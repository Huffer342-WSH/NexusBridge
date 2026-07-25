// Package storage 提供 qBittorrent 稳定关联信息的 SQLite 持久化。
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// QBSnapshotRecord 同时承载稳定关联字段和仅供内存/API 使用的实时字段。
// State、Progress、速度、流量、ETA 和 Ratio 永远不会写入数据库。
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

// SaveQBSnapshot 仅在稳定关联字段变化时保存单个 qB 关联。
func (s *SQLiteStore) SaveQBSnapshot(ctx context.Context, record QBSnapshotRecord) error {
	return s.SaveQBSnapshots(ctx, []QBSnapshotRecord{record})
}

// SaveQBSnapshots 在一个事务中保存一批稳定关联。
func (s *SQLiteStore) SaveQBSnapshots(ctx context.Context, records []QBSnapshotRecord) error {
	if len(records) == 0 {
		return nil
	}
	return s.withIndexWriteTx(ctx, func(tx *sql.Tx) error {
		for _, record := range records {
			if err := upsertQBAssociation(ctx, tx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReplaceQBSnapshots 用一次成功全量查询批量更新稳定关联，不保存实时状态。
func (s *SQLiteStore) ReplaceQBSnapshots(ctx context.Context, records []QBSnapshotRecord) (int, error) {
	missing := 0
	err := s.withIndexWriteTx(ctx, func(tx *sql.Tx) error {
		previouslyAdded := map[TorrentKey]struct{}{}
		rows, err := tx.QueryContext(ctx, `SELECT site_id, torrent_id FROM torrent_qb_associations WHERE added = 1`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var key TorrentKey
			if err := rows.Scan(&key.SiteID, &key.TorrentID); err != nil {
				_ = rows.Close()
				return err
			}
			previouslyAdded[key] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, record := range records {
			key := TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}
			if !record.Added {
				if _, wasAdded := previouslyAdded[key]; wasAdded {
					missing++
				}
			}
			if err := upsertQBAssociation(ctx, tx, record); err != nil {
				return err
			}
			delete(previouslyAdded, key)
		}
		for key := range previouslyAdded {
			result, err := tx.ExecContext(ctx, `
UPDATE torrent_qb_associations
SET added = 0, synced_at = CURRENT_TIMESTAMP
WHERE site_id = ? AND torrent_id = ? AND added = 1
`, key.SiteID, key.TorrentID)
			if err != nil {
				return err
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			missing += int(affected)
		}
		return nil
	})
	return missing, err
}

// ListQBSnapshots 返回指定种子的持久化稳定关联；实时字段保持零值。
func (s *SQLiteStore) ListQBSnapshots(ctx context.Context, keys []TorrentKey) (map[TorrentKey]QBSnapshotRecord, error) {
	result := make(map[TorrentKey]QBSnapshotRecord, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	query, args := torrentKeyQuery(qbAssociationSelect+" WHERE ", keys)
	rows, err := s.indexDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanQBAssociation(rows)
		if err != nil {
			return nil, err
		}
		result[TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}] = record
	}
	return result, rows.Err()
}

const qbAssociationSelect = `
SELECT site_id, torrent_id, added, qb_hash, name, category, tags_json, save_path, content_path,
	total_size, tracker, is_private, added_on, completion_on, creation_date, piece_size, comment,
	created_by, synced_at
FROM torrent_qb_associations`

type qbAssociationScanner interface {
	Scan(dest ...any) error
}

func scanQBAssociation(scanner qbAssociationScanner) (QBSnapshotRecord, error) {
	var record QBSnapshotRecord
	var added, private int
	var tagsJSON, syncedAt string
	err := scanner.Scan(
		&record.SiteID, &record.TorrentID, &added, &record.QBHash, &record.Name, &record.Category,
		&tagsJSON, &record.SavePath, &record.ContentPath, &record.TotalSize, &record.Tracker, &private,
		&record.AddedOn, &record.CompletionOn, &record.CreationDate, &record.PieceSize, &record.Comment,
		&record.CreatedBy, &syncedAt,
	)
	if err != nil {
		return QBSnapshotRecord{}, err
	}
	record.Added = added != 0
	record.IsPrivate = private != 0
	_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
	record.SyncedAt = parseDBTime(syncedAt)
	return record, nil
}

func upsertQBAssociation(ctx context.Context, tx *sql.Tx, record QBSnapshotRecord) error {
	syncedAt := record.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now()
	}
	if !record.Added {
		_, err := tx.ExecContext(ctx, `
INSERT INTO torrent_qb_associations (site_id, torrent_id, added, qb_hash, synced_at)
VALUES (?, ?, 0, ?, ?)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	added = 0,
	qb_hash = COALESCE(NULLIF(excluded.qb_hash, ''), torrent_qb_associations.qb_hash),
	synced_at = excluded.synced_at
WHERE torrent_qb_associations.added <> 0
	OR (excluded.qb_hash <> '' AND torrent_qb_associations.qb_hash <> excluded.qb_hash)
`, record.SiteID, record.TorrentID, record.QBHash, syncedAt.UTC().Format(time.RFC3339))
		return err
	}
	tagsJSON, err := json.Marshal(record.Tags)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO torrent_qb_associations (
	site_id, torrent_id, added, qb_hash, name, category, tags_json, save_path, content_path,
	total_size, tracker, is_private, added_on, completion_on, creation_date, piece_size, comment,
	created_by, synced_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(site_id, torrent_id) DO UPDATE SET
	added = excluded.added,
	qb_hash = excluded.qb_hash,
	name = excluded.name,
	category = excluded.category,
	tags_json = excluded.tags_json,
	save_path = excluded.save_path,
	content_path = excluded.content_path,
	total_size = excluded.total_size,
	tracker = excluded.tracker,
	is_private = excluded.is_private,
	added_on = excluded.added_on,
	completion_on = excluded.completion_on,
	creation_date = excluded.creation_date,
	piece_size = excluded.piece_size,
	comment = excluded.comment,
	created_by = excluded.created_by,
	synced_at = excluded.synced_at
WHERE added <> excluded.added
	OR qb_hash <> excluded.qb_hash
	OR name <> excluded.name
	OR category <> excluded.category
	OR tags_json <> excluded.tags_json
	OR save_path <> excluded.save_path
	OR content_path <> excluded.content_path
	OR total_size <> excluded.total_size
	OR tracker <> excluded.tracker
	OR is_private <> excluded.is_private
	OR added_on <> excluded.added_on
	OR completion_on <> excluded.completion_on
	OR creation_date <> excluded.creation_date
	OR piece_size <> excluded.piece_size
	OR comment <> excluded.comment
	OR created_by <> excluded.created_by
`, record.SiteID, record.TorrentID, boolInt(record.Added), record.QBHash, record.Name, record.Category,
		string(tagsJSON), record.SavePath, record.ContentPath, record.TotalSize, record.Tracker,
		boolInt(record.IsPrivate), record.AddedOn, record.CompletionOn, record.CreationDate, record.PieceSize,
		record.Comment, record.CreatedBy, syncedAt.UTC().Format(time.RFC3339))
	return err
}
