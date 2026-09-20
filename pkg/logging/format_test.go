package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"", FormatText, false},
		{"text", FormatText, false},
		{" JSON ", FormatJSON, false},
		{"logfmt", 0, true},
		{"xml", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseFormat(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseFormat(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("ParseFormat(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func setupBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		SetFormat(FormatText)
		SetColor(false)
		SetLevel(slog.LevelInfo)
	})

	var buf bytes.Buffer
	Setup(&buf)

	return &buf
}

func TestSetFormatSwitchesTheDefaultLogger(t *testing.T) {
	buf := setupBuffer(t)

	slog.Info("as text", "k", "v")
	if !strings.Contains(buf.String(), `msg="as text" k=v`) {
		t.Fatalf("text line = %q", buf.String())
	}

	buf.Reset()
	SetFormat(FormatJSON)
	slog.Info("as json", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json line %q: %v", buf.String(), err)
	}
	if rec["msg"] != "as json" || rec["k"] != "v" || rec["level"] != "INFO" {
		t.Errorf("json record = %v", rec)
	}
}

func TestDerivedLoggersFollowTheFormat(t *testing.T) {
	buf := setupBuffer(t)
	derived := slog.Default().With("name", "bot").WithGroup("g")

	SetFormat(FormatJSON)
	derived.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json line %q: %v", buf.String(), err)
	}
	group, _ := rec["g"].(map[string]any)
	if rec["name"] != "bot" || group["k"] != "v" {
		t.Errorf("attrs or group lost through With: %v", rec)
	}

	buf.Reset()
	SetFormat(FormatText)
	derived.Info("hello", "k", "v")
	if !strings.Contains(buf.String(), "name=bot") || !strings.Contains(buf.String(), "g.k=v") {
		t.Errorf("text line after switching back = %q", buf.String())
	}
}

func TestJSONIsNeverColored(t *testing.T) {
	buf := setupBuffer(t)
	SetColor(true)
	SetFormat(FormatJSON)

	slog.Error("boom", "err", "bad")
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("json line carries escapes: %q", buf.String())
	}
}

func TestFloorWorksOnTheSwitchableHandler(t *testing.T) {
	buf := setupBuffer(t)
	SetLevel(slog.LevelDebug)

	logger := slog.New(Floor(slog.Default().Handler(), slog.LevelInfo)).With("k", "v")
	logger.Debug("dropped")
	logger.Info("kept")

	if strings.Contains(buf.String(), "dropped") || !strings.Contains(buf.String(), "kept") {
		t.Errorf("floor over the switchable handler: %q", buf.String())
	}
}
