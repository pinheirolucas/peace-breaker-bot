package logging

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

func TestParseColorMode(t *testing.T) {
	tests := []struct {
		in      string
		want    ColorMode
		wantErr bool
	}{
		{"", ColorAuto, false},
		{"auto", ColorAuto, false},
		{" Always ", ColorAlways, false},
		{"NEVER", ColorNever, false},
		{"true", 0, true},
		{"on", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseColorMode(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseColorMode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("ParseColorMode(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestResolveColor(t *testing.T) {
	tests := []struct {
		name string
		mode ColorMode
		env  map[string]string
		tty  bool
		want bool
	}{
		{"auto on a terminal", ColorAuto, nil, true, true},
		{"auto piped", ColorAuto, nil, false, false},
		{"NO_COLOR wins over a terminal", ColorAuto, map[string]string{"NO_COLOR": "1"}, true, false},
		{"NO_COLOR wins over FORCE_COLOR", ColorAuto, map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, false, false},
		{"empty NO_COLOR is ignored", ColorAuto, map[string]string{"NO_COLOR": ""}, true, true},
		{"FORCE_COLOR forces piped output", ColorAuto, map[string]string{"FORCE_COLOR": "1"}, false, true},
		{"CLICOLOR_FORCE forces piped output", ColorAuto, map[string]string{"CLICOLOR_FORCE": "1"}, false, true},
		{"FORCE_COLOR=0 does not force", ColorAuto, map[string]string{"FORCE_COLOR": "0"}, false, false},
		{"dumb terminal", ColorAuto, map[string]string{"TERM": "dumb"}, true, false},
		{"always beats NO_COLOR", ColorAlways, map[string]string{"NO_COLOR": "1"}, false, true},
		{"never beats FORCE_COLOR", ColorNever, map[string]string{"FORCE_COLOR": "1"}, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveColor(tt.mode, func(k string) string { return tt.env[k] }, tt.tty)
			if got != tt.want {
				t.Errorf("resolveColor = %v, want %v", got, tt.want)
			}
		})
	}
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestTintRoles(t *testing.T) {
	line := `time="2026-09-20 10:42:07" level=ERROR msg="play failed" url=https://a.example/x.mp3 err="bad status" durationMs=1187 guildId=812 tokenSet=true path=/root/.instants/a.mp3 name=bot` + "\n"
	got := tint(line)

	wants := []string{
		ansiDim + `time="2026-09-20 10:42:07"` + ansiReset,
		"level=" + ansiReset + ansiBold + ansiRed + "ERROR" + ansiReset,
		ansiBold + ansiRed + `"play failed"` + ansiReset,
		ansiCyan + "url" + ansiReset,
		ansiBlue + ansiUnderline + "https://a.example/x.mp3" + ansiReset,
		ansiRed + `"bad status"` + ansiReset,
		ansiRed + "1187" + ansiReset,
		ansiMagenta + "812" + ansiReset,
		ansiYellow + "true" + ansiReset,
		ansiDim + "/root/.instants/a.mp3" + ansiReset,
		ansiCyan + "name" + ansiReset + ansiDim + "=" + ansiReset + "bot",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("tinted line missing %q\n%q", want, got)
		}
	}

	if plain := ansi.ReplaceAllString(got, ""); plain != line {
		t.Errorf("stripping the codes changed the text:\n got %q\nwant %q", plain, line)
	}
}

func TestTintLevels(t *testing.T) {
	tests := map[string]string{
		"DEBUG": ansiDim,
		"INFO":  ansiBold + ansiGreen,
		"WARN":  ansiBold + ansiYellow,
		"ERROR": ansiBold + ansiRed,
	}

	for level, style := range tests {
		got := tint("level=" + level + " msg=x\n")
		if !strings.Contains(got, style+level+ansiReset) {
			t.Errorf("%s not styled with %q: %q", level, style, got)
		}
	}
}

func TestTintDurationBands(t *testing.T) {
	tests := map[string]string{"40": ansiGreen, "412": ansiYellow, "1187": ansiRed}

	for ms, style := range tests {
		got := tint("durationMs=" + ms + "\n")
		if !strings.Contains(got, style+ms+ansiReset) {
			t.Errorf("durationMs=%s not styled with %q: %q", ms, style, got)
		}
	}
}

func TestTintKeepsQuotedValuesWhole(t *testing.T) {
	line := `msg="a b=c \"d\" e" k=v` + "\n"
	got := tint(line)

	if plain := ansi.ReplaceAllString(got, ""); plain != line {
		t.Errorf("stripping the codes changed the text:\n got %q\nwant %q", plain, line)
	}
	if !strings.Contains(got, ansiBold+`"a b=c \"d\" e"`+ansiReset) {
		t.Errorf("quoted msg split apart: %q", got)
	}
}

func TestColoredOutputNeverCarriesEscapesFromValues(t *testing.T) {
	buf := setupBuffer(t)
	SetColor(true)

	slog.Warn("hostile", "err", "\x1b[2J\x1b[31mgotcha", "url", "https://a.example/\x1b]0;x\x07")

	if plain := ansi.ReplaceAllString(buf.String(), ""); strings.Contains(plain, "\x1b") {
		t.Errorf("a raw escape from a value reached the output: %q", buf.String())
	}
}

func TestColorWriterPassesThroughWhenOff(t *testing.T) {
	buf := setupBuffer(t)

	slog.Info("hello", "k", "v")
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("escapes with color off: %q", buf.String())
	}

	buf.Reset()
	SetColor(true)
	slog.Info("hello", "k", "v")
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("no escapes with color on: %q", buf.String())
	}
}

func TestColorWriterReportsTheInputLength(t *testing.T) {
	SetColor(true)
	t.Cleanup(func() { SetColor(false) })

	var buf bytes.Buffer
	w := &colorWriter{w: &buf}
	in := []byte("level=INFO msg=x\n")

	n, err := w.Write(in)
	if err != nil || n != len(in) {
		t.Errorf("Write = %d, %v; want %d, nil", n, err, len(in))
	}
}
