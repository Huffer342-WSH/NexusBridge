package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// OrganizeTaskRecord 表示媒体整理任务的数据库记录。
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

// CreateOrganizeTaskIfAbsent 在任务不存在时创建整理任务。
func (s *SQLiteStore) CreateOrganizeTaskIfAbsent(ctx context.Context, task OrganizeTaskRecord) (bool, error) {
	_, err := s.execWriteContext(ctx, `
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

// UpdateOrganizeTask 更新整理任务的输出和执行状态。
func (s *SQLiteStore) UpdateOrganizeTask(ctx context.Context, id, status, relativeDir, filename, targetPath, response, errText string, confidence float64) error {
	_, err := s.execWriteContext(ctx, `
UPDATE organize_tasks
SET status = ?, relative_dir = ?, filename = ?, target_path = ?, llm_response = ?, confidence = ?, error = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, relativeDir, filename, targetPath, response, confidence, errText, id)
	return err
}

// ListOrganizeTasks 返回全部或仅待处理的整理任务。
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
