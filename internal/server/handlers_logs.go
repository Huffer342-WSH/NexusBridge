package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nexusbridge/internal/logging"
)

const (
	defaultLogLimit = 500
	maximumLogLimit = 2000
)

// handleLogs 返回当前进程内最近的格式化日志。
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limit, err := logLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, logging.Recent(limit))
}

// handleLogStream 通过 SSE 先发送快照，再持续推送新日志。
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	limit, err := logLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming is not supported"))
		return
	}

	snapshot, updates, cancel := logging.Subscribe(limit)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := writeLogEvent(w, "snapshot", snapshot); err != nil {
		return
	}
	flusher.Flush()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case entry, open := <-updates:
			if !open {
				return
			}
			if err := writeLogEvent(w, "log", entry); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func logLimit(r *http.Request) (int, error) {
	limit, err := queryInt(r, "limit", defaultLogLimit)
	if err != nil {
		return 0, err
	}
	if limit < 1 || limit > maximumLogLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximumLogLimit)
	}
	return limit, nil
}

func writeLogEvent(w http.ResponseWriter, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}
