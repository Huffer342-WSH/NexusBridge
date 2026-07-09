// recovery_batch_api_test.go 验证自动批量恢复 HTTP 接口在连接 qB 前拒绝不安全请求。
package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	httpserver "nexusbridge/internal/server"
)

// TestRecoveryBatchAPIValidation 验证空批次、缺少唯一候选键和超限批次均被拒绝。
func TestRecoveryBatchAPIValidation(t *testing.T) {
	root := t.TempDir()
	sitesDir := filepath.Join(root, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Storage.Path = filepath.Join(root, "batch-api.db")
	cfg.SitesDir = sitesDir
	cfg.QBittorrent.AutoSync = false
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	handler := httpserver.New(cfg, app, "").Handler()

	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/recovery/batch", core.RecoveryBatchRequest{}, http.StatusBadRequest, nil)
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/recovery/batch", core.RecoveryBatchRequest{
		Items: []core.RecoveryBatchItemRequest{{Path: filepath.Join(root, "candidate")}},
	}, http.StatusBadRequest, nil)

	tooMany := make([]core.RecoveryBatchItemRequest, 501)
	for index := range tooMany {
		tooMany[index] = core.RecoveryBatchItemRequest{Path: filepath.Join(root, "candidate"), SiteID: "demo", TorrentID: "1"}
	}
	task1JSONRequest(t, handler, http.MethodPost, "/api/qb/recovery/batch", core.RecoveryBatchRequest{Items: tooMany}, http.StatusBadRequest, nil)
}
