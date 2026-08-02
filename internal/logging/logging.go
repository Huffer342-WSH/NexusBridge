package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexusbridge/internal/config"
)

const (
	logTimeLayout = "2006/01/02 15:04:05"
	defaultModule = "app"
)

type bracketHandler struct {
	writer io.Writer
	level  slog.Leveler
	mu     *sync.Mutex
	attrs  []scopedAttr
	groups []string
}

type scopedAttr struct {
	groups []string
	attr   slog.Attr
}

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

	handler := newBracketHandler(io.MultiWriter(writers...), level)
	slog.SetDefault(slog.New(handler))
	log.SetFlags(0)
	log.SetPrefix("")
	return func() error {
		if file != nil {
			return file.Close()
		}
		return nil
	}, nil
}

func newBracketHandler(writer io.Writer, level slog.Leveler) *bracketHandler {
	return &bracketHandler{writer: writer, level: level, mu: &sync.Mutex{}}
}

func (h *bracketHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *bracketHandler) Handle(_ context.Context, record slog.Record) error {
	loggedAt := record.Time
	if loggedAt.IsZero() {
		loggedAt = time.Now()
	}

	module := moduleFromRecord(record)
	fields := make([]string, 0, len(h.attrs)+record.NumAttrs())
	for _, item := range h.attrs {
		appendAttr(&fields, &module, item.groups, item.attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendAttr(&fields, &module, h.groups, attr)
		return true
	})

	var line strings.Builder
	fmt.Fprintf(
		&line,
		"[%s][%s][%s] %s",
		loggedAt.Local().Format(logTimeLayout),
		record.Level.String(),
		sanitizeModule(module),
		sanitizeMessage(record.Message),
	)
	if len(fields) > 0 {
		line.WriteByte(' ')
		line.WriteString(strings.Join(fields, " "))
	}
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.writer, line.String())
	return err
}

func (h *bracketHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append([]scopedAttr(nil), h.attrs...)
	for _, attr := range attrs {
		clone.attrs = append(clone.attrs, scopedAttr{
			groups: append([]string(nil), h.groups...),
			attr:   attr,
		})
	}
	return &clone
}

func (h *bracketHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func appendAttr(fields *[]string, module *string, groups []string, attr slog.Attr) {
	value := attr.Value.Resolve()
	if attr.Key == "" && value.Kind() != slog.KindGroup {
		return
	}
	if value.Kind() == slog.KindGroup {
		nestedGroups := append([]string(nil), groups...)
		if attr.Key != "" {
			nestedGroups = append(nestedGroups, attr.Key)
		}
		for _, nested := range value.Group() {
			appendAttr(fields, module, nestedGroups, nested)
		}
		return
	}
	if len(groups) == 0 && attr.Key == "module" {
		if valueText := strings.TrimSpace(value.String()); valueText != "" {
			*module = valueText
		}
		return
	}
	keyParts := append(append([]string(nil), groups...), attr.Key)
	*fields = append(*fields, strings.Join(keyParts, ".")+"="+formatValue(value))
}

func formatValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return quoteValue(value.String())
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindAny:
		return quoteValue(fmt.Sprint(value.Any()))
	default:
		return value.String()
	}
}

func quoteValue(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return r <= ' ' || strings.ContainsRune("=\\\"[]", r)
	}) == -1 {
		return value
	}
	return strconv.Quote(value)
}

func moduleFromRecord(record slog.Record) string {
	if record.PC == 0 {
		return defaultModule
	}
	frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
	function := strings.TrimPrefix(frame.Function, "nexusbridge/")
	parts := strings.Split(function, "/")
	if len(parts) >= 2 && parts[0] == "internal" {
		return parts[1]
	}
	if len(parts) >= 2 && parts[0] == "cmd" {
		return parts[1]
	}
	if len(parts) > 1 {
		return parts[0]
	}
	if packageEnd := strings.IndexByte(function, '.'); packageEnd > 0 {
		return function[:packageEnd]
	}
	return defaultModule
}

func sanitizeModule(module string) string {
	module = strings.TrimSpace(module)
	if module == "" {
		return defaultModule
	}
	return strings.NewReplacer("\r", "_", "\n", "_", "]", "_").Replace(module)
}

func sanitizeMessage(message string) string {
	return strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(message)
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
