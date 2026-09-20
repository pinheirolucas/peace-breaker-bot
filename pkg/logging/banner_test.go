package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestBannerPlainHasNoEscapes(t *testing.T) {
	got := banner(false, "v0.0.3")

	if strings.Contains(got, "\x1b") {
		t.Errorf("plain banner carries escapes: %q", got)
	}
	for _, want := range []string{"v0.0.3", "╭", "@", "█▀█ █▀▀ ▄▀▄ █▀▀ █▀▀  █▀▄ █▀▄ █▀▀ ▄▀▄ █ █ █▀▀ █▀▄  █▀▄ █▀█ ▀█▀"} {
		if !strings.Contains(got, want) {
			t.Errorf("banner missing %q:\n%s", want, got)
		}
	}
}

func TestBannerColorsEachLetterWithAPaletteSlot(t *testing.T) {
	got := banner(true, "v0.0.3")

	for i, style := range slotColors {
		if !strings.Contains(got, style) {
			t.Errorf("slot %d color missing from the banner", i)
		}
	}
	if !strings.Contains(got, ansiBlue+"█▀█"+ansiReset+" "+ansiRed+"█▀▀"+ansiReset) {
		t.Errorf("first letters P and E are not blue and red: %q", got)
	}
}

func TestBannerColoredMatchesPlainOnceStripped(t *testing.T) {
	colored := banner(true, "v0.0.3")

	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("colored banner has no escapes: %q", colored)
	}
	if got, want := ansi.ReplaceAllString(colored, ""), banner(false, "v0.0.3"); got != want {
		t.Errorf("stripped banner differs from the plain one:\n got %q\nwant %q", got, want)
	}
}

func TestBannerRowsAlign(t *testing.T) {
	rows := strings.Split(strings.TrimRight(banner(false, "v0.0.3"), "\n"), "\n")
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}

	for i, row := range rows {
		cells := []rune(row)
		if len(cells) > 80 {
			t.Errorf("row %d is %d columns, want at most 80: %q", i, len(cells), row)
		}
		if len(cells) < 16 {
			t.Fatalf("row %d too short: %q", i, row)
		}
		if edge := cells[15]; i == 0 && edge != '╮' || i == 4 && edge != '╯' || i > 0 && i < 4 && edge != '│' {
			t.Errorf("row %d does not close the cassette at column 16: %q", i, row)
		}
	}
}

func TestDisplayVersion(t *testing.T) {
	tests := map[string]string{"0.0.3": "v0.0.3", "1.4.0-rc.1": "v1.4.0-rc.1", "v0.0.3": "v0.0.3", "dev": "dev", "": ""}

	for in, want := range tests {
		if got := displayVersion(in); got != want {
			t.Errorf("displayVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintBannerShowsTheVersionWithAPrefix(t *testing.T) {
	setupBuffer(t)

	var buf bytes.Buffer
	PrintBanner(&buf, "0.0.3")
	if !strings.Contains(buf.String(), "v0.0.3") {
		t.Errorf("banner missing the prefixed version: %q", buf.String())
	}
}

func TestPrintBannerSkipsJSON(t *testing.T) {
	setupBuffer(t)

	var buf bytes.Buffer
	PrintBanner(&buf, "v0.0.3")
	if !strings.Contains(buf.String(), "v0.0.3") {
		t.Errorf("text format printed no banner: %q", buf.String())
	}

	buf.Reset()
	SetFormat(FormatJSON)
	PrintBanner(&buf, "v0.0.3")
	if buf.Len() != 0 {
		t.Errorf("json format printed a banner: %q", buf.String())
	}
}
