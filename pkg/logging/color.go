package logging

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// ColorMode is the log.color setting.
type ColorMode int

const (
	// ColorAuto colors only when stdout is a terminal, honoring the color environment variables.
	ColorAuto ColorMode = iota
	// ColorAlways colors regardless of the terminal.
	ColorAlways
	// ColorNever never colors.
	ColorNever
)

var color atomic.Bool

// ParseColorMode converts a log.color setting into a ColorMode.
func ParseColorMode(s string) (ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	}

	return 0, fmt.Errorf("invalid log color %q: want auto, always or never", s)
}

// ResolveColor decides whether to color stdout, and prepares the terminal to render it.
func ResolveColor(mode ColorMode) bool {
	if !resolveColor(mode, os.Getenv, isTerminal(os.Stdout)) {
		return false
	}

	return enableVirtualTerminal(os.Stdout) || mode == ColorAlways
}

func resolveColor(mode ColorMode, getenv func(string) string, tty bool) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}

	switch {
	case getenv("NO_COLOR") != "":
		return false
	case forced(getenv("FORCE_COLOR")) || forced(getenv("CLICOLOR_FORCE")):
		return true
	case getenv("TERM") == "dumb":
		return false
	}

	return tty
}

func forced(v string) bool {
	return v != "" && v != "0"
}

// SetColor turns colored output on or off for the default logger.
func SetColor(on bool) {
	color.Store(on)
}

// ColorEnabled reports whether the default logger colors its output.
func ColorEnabled() bool {
	return color.Load()
}

type colorWriter struct {
	w io.Writer
}

func (c *colorWriter) Write(p []byte) (int, error) {
	if !ColorEnabled() {
		return c.w.Write(p)
	}

	if _, err := c.w.Write([]byte(tint(string(p)))); err != nil {
		return 0, err
	}

	return len(p), nil
}

const (
	ansiReset     = "\x1b[0m"
	ansiBold      = "\x1b[1m"
	ansiDim       = "\x1b[2m"
	ansiUnderline = "\x1b[4m"
	ansiRed       = "\x1b[31m"
	ansiGreen     = "\x1b[32m"
	ansiYellow    = "\x1b[33m"
	ansiBlue      = "\x1b[34m"
	ansiMagenta   = "\x1b[35m"
	ansiCyan      = "\x1b[36m"
)

func paint(b *strings.Builder, style, text string) {
	if style == "" {
		b.WriteString(text)
		return
	}

	b.WriteString(style)
	b.WriteString(text)
	b.WriteString(ansiReset)
}

func tint(line string) string {
	body := strings.TrimSuffix(line, "\n")

	var b strings.Builder
	var level string

	for i := 0; i < len(body); {
		if body[i] == ' ' {
			b.WriteByte(' ')
			i++
			continue
		}

		eq := i
		for eq < len(body) && body[eq] != '=' && body[eq] != ' ' {
			eq++
		}
		if eq == len(body) || body[eq] == ' ' {
			b.WriteString(body[i:eq])
			i = eq
			continue
		}

		key := body[i:eq]
		end := valueEnd(body, eq+1)
		raw := body[eq+1 : end]
		val := strings.Trim(raw, `"`)

		switch key {
		case "time":
			paint(&b, ansiDim, key+"="+raw)
		case "level":
			level = val
			paint(&b, ansiDim, key+"=")
			paint(&b, levelStyle(val), raw)
		case "msg":
			paint(&b, ansiDim, key+"=")
			style := ansiBold
			if level == "ERROR" {
				style += ansiRed
			}
			paint(&b, style, raw)
		default:
			paint(&b, ansiCyan, key)
			paint(&b, ansiDim, "=")
			paint(&b, valueStyle(key, val), raw)
		}

		i = end
	}

	return b.String() + line[len(body):]
}

func valueEnd(s string, i int) int {
	if i < len(s) && s[i] == '"' {
		i++
		for i < len(s) && s[i] != '"' {
			if s[i] == '\\' && i+1 < len(s) {
				i++
			}
			i++
		}
		if i < len(s) {
			i++
		}
		return i
	}

	for i < len(s) && s[i] != ' ' {
		i++
	}

	return i
}

func levelStyle(level string) string {
	switch level {
	case "DEBUG":
		return ansiDim
	case "INFO":
		return ansiBold + ansiGreen
	case "WARN":
		return ansiBold + ansiYellow
	case "ERROR":
		return ansiBold + ansiRed
	}

	return ""
}

func valueStyle(key, val string) string {
	base := strings.ToLower(key[strings.LastIndexByte(key, '.')+1:])

	switch {
	case base == "err" || base == "error":
		return ansiRed
	case strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://"):
		return ansiBlue + ansiUnderline
	case val == "true" || val == "false":
		return ansiYellow
	case strings.HasSuffix(base, "ms"):
		return durationStyle(val)
	case strings.HasSuffix(base, "path") || strings.HasSuffix(base, "file") || strings.HasSuffix(base, "dir"):
		return ansiDim
	case strings.HasSuffix(base, "id") || base == "ticket" || base == "version":
		return ansiMagenta
	}

	return ""
}

func durationStyle(val string) string {
	ms, err := strconv.ParseFloat(val, 64)
	switch {
	case err != nil:
		return ""
	case ms < 200:
		return ansiGreen
	case ms < 1000:
		return ansiYellow
	}

	return ansiRed
}
