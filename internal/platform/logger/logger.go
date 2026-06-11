package logger

import (
	"log/slog"
	"os"
)

func New(level string) *slog.Logger {
	opts := &slog.HandlerOptions{}
	switch level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
