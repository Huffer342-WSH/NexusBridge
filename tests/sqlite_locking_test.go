// sqlite_locking_test.go 验证 SQLite 在并发读写锁竞争后可以自行恢复。
package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexusbridge/internal/storage"
)

// TestSQLiteLockContentionRecovers 验证调度读取不受写事务阻塞，等待中的写入会在锁释放后继续。
func TestSQLiteLockContentionRecovers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "locking.db")
	store, err := storage.OpenSQLite(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.SaveSiteSchedule(t.Context(), storage.SiteScheduleRecord{
		SiteID:          "test-site",
		Enabled:         true,
		IntervalMinutes: 30,
		NextRunAt:       now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("save site schedule: %v", err)
	}

	locker, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open locking connection: %v", err)
	}
	defer locker.Close()
	locker.SetMaxOpenConns(1)

	var journalMode string
	if err := locker.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("expected WAL journal mode, got %q", journalMode)
	}

	conn, err := locker.Conn(t.Context())
	if err != nil {
		t.Fatalf("reserve locking connection: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), `BEGIN EXCLUSIVE`); err != nil {
		t.Fatalf("begin exclusive transaction: %v", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if _, err := conn.ExecContext(t.Context(), `UPDATE site_schedules SET last_error = 'locked' WHERE site_id = 'test-site'`); err != nil {
		t.Fatalf("hold write lock: %v", err)
	}

	readCtx, cancelRead := context.WithTimeout(t.Context(), time.Second)
	due, err := store.ListDueSiteSchedules(readCtx, now, 10)
	cancelRead()
	if err != nil {
		t.Fatalf("list due schedules while writer holds lock: %v", err)
	}
	if len(due) != 1 || due[0].SiteID != "test-site" {
		t.Fatalf("unexpected due schedules: %#v", due)
	}

	type writeResult struct {
		index int
		err   error
	}
	const writerCount = 4
	results := make(chan writeResult, writerCount)
	for i := 0; i < writerCount; i++ {
		go func(index int) {
			results <- writeResult{
				index: index,
				err:   store.SaveSetting(t.Context(), fmt.Sprintf("lock-test-%d", index), index),
			}
		}(i)
	}

	select {
	case result := <-results:
		t.Fatalf("writer %d returned before lock release: %v", result.index, result.err)
	case <-time.After(150 * time.Millisecond):
	}

	if _, err := conn.ExecContext(t.Context(), `COMMIT`); err != nil {
		t.Fatalf("release write lock: %v", err)
	}
	committed = true
	for i := 0; i < writerCount; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Errorf("writer %d did not recover after lock release: %v", result.index, result.err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for writers after lock release")
		}
	}
}
