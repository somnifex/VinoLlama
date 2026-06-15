package logging

import (
	"fmt"
	"io"
	"log/slog"
)

func New(level string, out io.Writer) (*slog.Logger, error) {
	var slogLevel slog.Level
	switch level {
	case "", "info":
		slogLevel = slog.LevelInfo
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("unsupported log level %q", level)
	}
	handler := slog.NewTextHandler(out, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(handler), nil
}
