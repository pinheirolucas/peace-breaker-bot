package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
)

// Format is the layout of a log record.
type Format int32

const (
	// FormatText is key=value pairs on one line.
	FormatText Format = iota
	// FormatJSON is one JSON object per line.
	FormatJSON
)

var format atomic.Int32

// ParseFormat converts a log.format setting into a Format.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "text":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	}

	return 0, fmt.Errorf("invalid log format %q: want text or json", s)
}

// SetFormat changes the format of the default logger.
func SetFormat(f Format) {
	format.Store(int32(f))
}

// CurrentFormat returns the format the default logger is using.
func CurrentFormat() Format {
	return Format(format.Load())
}

type formatHandler struct {
	text slog.Handler
	json slog.Handler
}

func (h formatHandler) current() slog.Handler {
	if CurrentFormat() == FormatJSON {
		return h.json
	}

	return h.text
}

func (h formatHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.current().Enabled(ctx, l)
}

func (h formatHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.current().Handle(ctx, r)
}

func (h formatHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return formatHandler{text: h.text.WithAttrs(attrs), json: h.json.WithAttrs(attrs)}
}

func (h formatHandler) WithGroup(name string) slog.Handler {
	return formatHandler{text: h.text.WithGroup(name), json: h.json.WithGroup(name)}
}
