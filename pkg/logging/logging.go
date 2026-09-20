package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

var level slog.LevelVar

func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}

	return 0, fmt.Errorf("invalid log level %q: want debug, info, warn or error", s)
}

func Setup(w io.Writer) {
	slog.SetDefault(slog.New(newHandler(w, &level)))
}

func SetLevel(l slog.Level) {
	level.Set(l)
}

func newHandler(w io.Writer, l slog.Leveler) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: l,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05"))
			}
			return a
		},
	})
}

func Floor(h slog.Handler, min slog.Level) slog.Handler {
	return floorHandler{Handler: h, min: min}
}

type floorHandler struct {
	slog.Handler
	min slog.Level
}

func (f floorHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= f.min && f.Handler.Enabled(ctx, l)
}

func (f floorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return floorHandler{Handler: f.Handler.WithAttrs(attrs), min: f.min}
}

func (f floorHandler) WithGroup(name string) slog.Handler {
	return floorHandler{Handler: f.Handler.WithGroup(name), min: f.min}
}
