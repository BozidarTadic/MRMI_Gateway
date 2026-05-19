package logger

import (
	"log/slog"
	"os"
	"time"
)

var l *slog.Logger = slog.Default()

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

	l = slog.New(h)
	slog.SetDefault(l)
}

func Info(msg string, args ...any)  { l.Info(msg, args...) }
func Warn(msg string, args ...any)  { l.Warn(msg, args...) }
func Error(msg string, args ...any) { l.Error(msg, args...) }
func Debug(msg string, args ...any) { l.Debug(msg, args...) }
