package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type SiteCredentialRecord struct {
	SiteID    string            `json:"site_id"`
	BaseURL   string            `json:"base_url"`
	UserAgent string            `json:"user_agent"`
	Headers   map[string]string `json:"headers"`
	HasCookie bool              `json:"has_cookie"`
}

// SaveSiteCredential 保存站点请求凭据元数据。
func (s *SQLiteStore) SaveSiteCredential(ctx context.Context, credential SiteCredentialRecord) error {
	if strings.TrimSpace(credential.SiteID) == "" {
		return fmt.Errorf("site id is required")
	}
	headers := credential.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO site_credentials (site_id, base_url, user_agent, headers_json, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(site_id) DO UPDATE SET
	base_url = excluded.base_url,
	user_agent = excluded.user_agent,
	headers_json = excluded.headers_json,
	updated_at = CURRENT_TIMESTAMP
`, credential.SiteID, credential.BaseURL, credential.UserAgent, string(data))
	return err
}

// LoadSiteCredential 读取站点请求凭据元数据。
func (s *SQLiteStore) LoadSiteCredential(ctx context.Context, siteID string) (SiteCredentialRecord, bool, error) {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return SiteCredentialRecord{}, false, fmt.Errorf("site id is required")
	}
	var record SiteCredentialRecord
	var headersJSON string
	err := s.db.QueryRowContext(ctx, `
SELECT site_id, base_url, user_agent, headers_json
FROM site_credentials
WHERE site_id = ?
`, siteID).Scan(&record.SiteID, &record.BaseURL, &record.UserAgent, &headersJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return SiteCredentialRecord{}, false, nil
		}
		return SiteCredentialRecord{}, false, err
	}
	_ = json.Unmarshal([]byte(headersJSON), &record.Headers)
	record.HasCookie = s.HasCookies(ctx, record.BaseURL)
	return record, true, nil
}

// HasCookies 判断指定 scope 是否已保存 cookie。
func (s *SQLiteStore) HasCookies(ctx context.Context, scope string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return false
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cookies WHERE scope = ?`, scope).Scan(&count)
	return err == nil && count > 0
}
