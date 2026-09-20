package logging

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"", slog.LevelInfo, false},
		{"info", slog.LevelInfo, false},
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{" Warn ", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"verbose", 0, true},
		{"INFO+2", 0, true},
		{"warning", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseLevel(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseLevel(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseLevelErrorNamesTheValue(t *testing.T) {
	_, err := ParseLevel("verbose")
	if err == nil || !strings.Contains(err.Error(), `"verbose"`) {
		t.Errorf("error = %v, want it to quote the rejected value", err)
	}
}

func TestSetupFollowsSetLevel(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		SetLevel(slog.LevelInfo)
	})

	var buf bytes.Buffer
	Setup(&buf)

	slog.Debug("hidden")
	if buf.Len() != 0 {
		t.Fatalf("debug logged at the default level: %q", buf.String())
	}

	SetLevel(slog.LevelDebug)
	slog.Debug("shown")
	if !strings.Contains(buf.String(), "shown") {
		t.Errorf("debug not logged after SetLevel(debug): %q", buf.String())
	}
}

func TestSetupFormatsTime(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	Setup(&buf)
	slog.Info("hello")

	want := regexp.MustCompile(`^time="\d{4}-\d\d-\d\d \d\d:\d\d:\d\d" level=INFO msg=hello\n$`)
	if !want.MatchString(buf.String()) {
		t.Errorf("unexpected line: %q", buf.String())
	}
}

func TestFloorDropsRecordsBelowMinimum(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(Floor(inner, slog.LevelInfo))

	logger.Debug("dropped")
	logger.Info("kept")

	out := buf.String()
	if strings.Contains(out, "dropped") {
		t.Errorf("debug record passed the floor: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("info record missing: %q", out)
	}
}

func TestFloorKeepsTheInnerThreshold(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	logger := slog.New(Floor(inner, slog.LevelInfo))

	logger.Warn("below inner level")
	if buf.Len() != 0 {
		t.Errorf("floor lowered the inner threshold: %q", buf.String())
	}
}

func TestFloorSurvivesWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(Floor(inner, slog.LevelInfo)).With("name", "bot").WithGroup("g")

	logger.Debug("dropped")
	if buf.Len() != 0 {
		t.Errorf("floor lost after With/WithGroup: %q", buf.String())
	}

	logger.Info("kept", "k", "v")
	if !strings.Contains(buf.String(), "name=bot") || !strings.Contains(buf.String(), "g.k=v") {
		t.Errorf("attrs/group lost: %q", buf.String())
	}
}
