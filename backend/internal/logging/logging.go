// Package logging builds the application-wide slog logger: JSON (or text) to
// stdout, optionally mirrored into a size-rotated file via lumberjack.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

var sensitiveKeys = []string{"password", "secret", "key", "token", "credential"}

// Options configures Init. Zero values fall back to sane defaults
// (level info, json format, stdout only).
type Options struct {
	Level          string // debug | info | warn | warning | error
	Format         string // "json" (default) | "text"
	File           string // log file path; "" = stdout only
	FileMaxSizeMB  int    // rotate after this size; default 10
	FileMaxBackups int    // rotated files to keep; default 5
	FileMaxAgeDays int    // days to keep rotated files; 0 = no age pruning
}

// Init builds the logger. The returned close func flushes the file sink and is
// a no-op when logging to stdout only — always defer it in main. It returns an
// error only for an unwritable File path: lumberjack would otherwise swallow
// the problem until the first write, so we probe-open and fail loudly at startup.
func Init(o Options) (*slog.Logger, func() error, error) {
	var w io.Writer = os.Stdout
	closeFn := func() error { return nil }

	if o.File != "" {
		if err := os.MkdirAll(filepath.Dir(o.File), 0o750); err != nil {
			return nil, nil, fmt.Errorf("create log file directory: %w", err)
		}
		probe, err := os.OpenFile(o.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file %q: %w", o.File, err)
		}
		_ = probe.Close() // probe only; lumberjack owns the real handle

		lj := &lumberjack.Logger{
			Filename:   o.File,
			MaxSize:    orDefault(o.FileMaxSizeMB, 10),
			MaxBackups: orDefault(o.FileMaxBackups, 5),
			MaxAge:     o.FileMaxAgeDays,
		}
		w = io.MultiWriter(os.Stdout, lj)
		closeFn = lj.Close
	}

	opts := &slog.HandlerOptions{Level: parseLevel(o.Level)}
	var h slog.Handler
	if strings.ToLower(o.Format) == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h), closeFn, nil
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func SafeLogAttrs(attrs ...slog.Attr) []slog.Attr {
	result := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		if isSensitiveKey(a.Key) {
			result[i] = slog.String(a.Key, "[REDACTED]")
			continue
		}
		result[i] = a
	}
	return result
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
