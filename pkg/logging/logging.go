// Package logging builds the process-wide slog handler and owns the
// configurable log level.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// level is shared by the default handler so it can be changed after Setup
// without rebuilding the handler or calling slog.SetDefault again.
var level slog.LevelVar

// ParseLevel maps a config value to a slog level. It accepts debug, info,
// warn and error in any case, and treats empty as info. It is stricter than
// slog.Level.UnmarshalText on purpose, which also takes forms like "INFO+2".
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

// Setup installs the default slog logger, writing to w at the level set by
// SetLevel (info until then).
func Setup(w io.Writer) {
	slog.SetDefault(slog.New(newHandler(w, &level)))
}

// SetLevel changes the level of the logger installed by Setup.
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

// Floor wraps h so records below min are dropped even when h would accept
// them. It never lowers h's own threshold: a record has to clear both.
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
