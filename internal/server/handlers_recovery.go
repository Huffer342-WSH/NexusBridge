package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexusbridge/internal/core"
)

// handlePreviewRecovery 按本地目录结构返回精确 torrent 候选。
func (s *Server) handlePreviewRecovery(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.PreviewRecovery(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRecoverFolder 重新校验指定候选并恢复 qB 任务。
func (s *Server) handleRecoverFolder(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.RecoverFolder(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

// handleRecoveryAction 启动或删除校验失败后保留的 qB 任务。
func (s *Server) handleRecoveryAction(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryActionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.ControlRecoveryTorrent(r.Context(), chi.URLParam(r, "hash"), request.Action)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleTorrentSizeIndexStatus 返回恢复文件大小索引状态。
func (s *Server) handleTorrentSizeIndexStatus(w http.ResponseWriter, r *http.Request) {
	result, err := s.recovery.GetTorrentSizeIndexStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRebuildTorrentSizeIndex 手动重建全部已保存 torrent 的大小索引。
func (s *Server) handleRebuildTorrentSizeIndex(w http.ResponseWriter, r *http.Request) {
	result, err := s.recovery.RebuildTorrentSizeIndex(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleScanRecoveryCandidates 递归预览可能丢失的 qB 任务。
func (s *Server) handleScanRecoveryCandidates(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryScanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.ScanRecoveryCandidates(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRecoverFolders 串行执行扫描阶段已经唯一选定的恢复候选。
func (s *Server) handleRecoverFolders(w http.ResponseWriter, r *http.Request) {
	var request core.RecoveryBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.recovery.RecoverFolders(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handlePreviewBatchDownload(w http.ResponseWriter, r *http.Request) {
	var request core.BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.batch.PreviewBatchDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExecuteBatchDownload(w http.ResponseWriter, r *http.Request) {
	var request core.BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.batch.ExecuteBatchDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleRetryDownloadTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.batch.RetryDownloadTask(r.Context(), chi.URLParam(r, "task_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

// handlePreviewTorrentDownload 返回手动下载前的 LLM 标题预览。
func (s *Server) handlePreviewTorrentDownload(w http.ResponseWriter, r *http.Request) {
	request := core.ManualDownloadRequest{
		SiteID:    chi.URLParam(r, "site_id"),
		TorrentID: chi.URLParam(r, "torrent_id"),
	}
	preview, err := s.actions.PreviewTorrentDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// handleSendTorrentDownload 发送单个种子到 qBittorrent。
func (s *Server) handleSendTorrentDownload(w http.ResponseWriter, r *http.Request) {
	request := core.ManualDownloadRequest{
		SiteID:    chi.URLParam(r, "site_id"),
		TorrentID: chi.URLParam(r, "torrent_id"),
	}
	var body struct {
		FormattedTitle string `json:"formatted_title"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
		request.FormattedTitle = body.FormattedTitle
	}
	task, err := s.actions.SendTorrentDownload(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}
