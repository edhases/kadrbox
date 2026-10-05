// Package logging wires the process-wide structured logger (stdlib log/slog).
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New builds a logger writing to stdout. level is one of debug|info|warn|error
// (unknown values fall back to info); format is json|text (unknown -> text).
func New(level, format string) *slog.Logger {
	return slog.New(NewHandler(level, format, os.Stdout))
}

// NewHandler is New with a caller-supplied writer, for tests.
func NewHandler(level, format string, w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: ParseLevel(level)}
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

// ParseLevel maps a configuration string onto an slog.Level.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
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

// SetDefault installs l as the process logger (slog.SetDefault).
func SetDefault(l *slog.Logger) {
	if l == nil {
		return
	}
	slog.SetDefault(l)
}

// Init builds a logger from configuration and installs it as the default.
func Init(level, format string) *slog.Logger {
	l := New(level, format)
	SetDefault(l)
	return l
}

// L returns the logger all middleware must use instead of the log package.
func L() *slog.Logger { return slog.Default() }
