package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

func (s *SQLiteStore) SaveCookies(ctx context.Context, scope string, cookies []*http.Cookie) error {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return fmt.Errorf("cookie scope is required")
	}

	tx, release, err := s.beginWriteTx(ctx)
	if err != nil {
		return err
	}
	defer release()
	defer rollbackUnlessCommitted(tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM cookies WHERE scope = ?`, scope); err != nil {
		return fmt.Errorf("clear cookies: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO cookies (scope, name, value, path, domain, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(scope, name) DO UPDATE SET
	value = excluded.value,
	path = excluded.path,
	domain = excluded.domain,
	updated_at = CURRENT_TIMESTAMP
`)
	if err != nil {
		return fmt.Errorf("prepare cookie insert: %w", err)
	}
	defer stmt.Close()

	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, scope, cookie.Name, cookie.Value, cookie.Path, cookie.Domain); err != nil {
			return fmt.Errorf("save cookie: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) LoadCookies(ctx context.Context, scope string) ([]*http.Cookie, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return nil, fmt.Errorf("cookie scope is required")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT name, value, path, domain
FROM cookies
WHERE scope = ?
ORDER BY name
`, scope)
	if err != nil {
		return nil, fmt.Errorf("load cookies: %w", err)
	}
	defer rows.Close()

	cookies := []*http.Cookie{}
	for rows.Next() {
		cookie := &http.Cookie{}
		if err := rows.Scan(&cookie.Name, &cookie.Value, &cookie.Path, &cookie.Domain); err != nil {
			return nil, fmt.Errorf("scan cookie: %w", err)
		}
		cookies = append(cookies, cookie)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cookies, nil
}

func rollbackUnlessCommitted(tx *sql.Tx) {
	_ = tx.Rollback()
}
