package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"nexusbridge/internal/config"
)

// Setup 初始化终端和文件日志输出。
func Setup(cfg config.LoggingConfig) (func() error, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	writers := []io.Writer{os.Stdout}
	var file *os.File
	if strings.TrimSpace(cfg.File) != "" {
		if dir := filepath.Dir(cfg.File); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create log directory: %w", err)
			}
		}
		file, err = os.OpenFile(cfg.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		writers = append(writers, file)
	}

	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
	return func() error {
		if file != nil {
			return file.Close()
		}
		return nil
	}, nil
}

// parseLevel 解析日志等级。
func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", value)
	}
}
