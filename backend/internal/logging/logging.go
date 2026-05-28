package logging

import (
	"log/slog"
	"os"
	"strings"
)

var sensitiveKeys = []string{"password", "secret", "key", "token", "credential"}

func Init(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
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
