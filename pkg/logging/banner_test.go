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
	for _, want := range []string{"Peace Breaker Bot", "v0.0.3", "instants on demand", "╭", "@"} {
		if !strings.Contains(got, want) {
			t.Errorf("banner missing %q:\n%s", want, got)
		}
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
		cassette := []rune(row)
		if len(cassette) < 20 {
			t.Fatalf("row %d too short: %q", i, row)
		}
		if edge := cassette[19]; i == 0 && edge != '╮' || i == 4 && edge != '╯' || i > 0 && i < 4 && edge != '│' {
			t.Errorf("row %d does not close the cassette at column 20: %q", i, row)
		}
	}
}

func TestPrintBannerSkipsJSON(t *testing.T) {
	setupBuffer(t)

	var buf bytes.Buffer
	PrintBanner(&buf, "v0.0.3")
	if !strings.Contains(buf.String(), "Peace Breaker Bot") {
		t.Errorf("text format printed no banner: %q", buf.String())
	}

	buf.Reset()
	SetFormat(FormatJSON)
	PrintBanner(&buf, "v0.0.3")
	if buf.Len() != 0 {
		t.Errorf("json format printed a banner: %q", buf.String())
	}
}
