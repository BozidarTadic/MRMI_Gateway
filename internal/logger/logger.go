package logger

import (
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

var l atomic.Pointer[slog.Logger]

func init() { l.Store(slog.Default()) }

// Init configures the global logger from config values.
// level: "debug" | "info" | "warn" | "error" (default: "info")
// format: "json" | "text" (default: "text")
func Init(level, format string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("ts", a.Value.Time().UTC().Format(time.RFC3339))
			}
			return a
		},
	}

	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}

	nl := slog.New(h)
	l.Store(nl)
	slog.SetDefault(nl)
}

func Info(msg string, args ...any)  { l.Load().Info(msg, args...) }
func Warn(msg string, args ...any)  { l.Load().Warn(msg, args...) }
func Error(msg string, args ...any) { l.Load().Error(msg, args...) }
func Debug(msg string, args ...any) { l.Load().Debug(msg, args...) }
