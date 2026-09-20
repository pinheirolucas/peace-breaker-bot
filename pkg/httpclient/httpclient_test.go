package httpclient

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func userAgentServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
	}))
	t.Cleanup(srv.Close)

	return srv, &got
}

func do(t *testing.T, req *http.Request) {
	t.Helper()

	resp, err := New().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
}

func TestNewSetsUserAgentWhenAbsent(t *testing.T) {
	srv, got := userAgentServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	do(t, req)

	if *got != UserAgent {
		t.Errorf("User-Agent = %q, want %q", *got, UserAgent)
	}
}

func TestNewPreservesCallerUserAgent(t *testing.T) {
	srv, got := userAgentServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("User-Agent", "custom/2.0")
	do(t, req)

	if *got != "custom/2.0" {
		t.Errorf("User-Agent = %q, want the caller's custom/2.0", *got)
	}
}

func TestNewDoesNotMutateTheCallerRequest(t *testing.T) {
	srv, _ := userAgentServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	do(t, req)

	if ua, ok := req.Header["User-Agent"]; ok {
		t.Errorf("caller's request gained User-Agent %q", ua)
	}
}

func TestNewSetsATimeout(t *testing.T) {
	if New().Timeout == 0 {
		t.Error("Timeout = 0, want a bound so a stalled myinstants.com cannot hang a request")
	}
}

func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}

func TestNewLogsEachRequestAtDebug(t *testing.T) {
	logs := captureDebugLogs(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/clip.mp3", nil)
	do(t, req)

	for _, want := range []string{"level=DEBUG", `msg="upstream request"`, "method=GET", srv.URL + "/clip.mp3", "status=418", "durationMs="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q does not contain %q", logs.String(), want)
		}
	}
}

func TestNewLogsATransportErrorWithoutAStatus(t *testing.T) {
	logs := captureDebugLogs(t)

	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if _, err := New().Do(req); err == nil {
		t.Fatal("Do succeeded against a closed server")
	}

	if !strings.Contains(logs.String(), "err=") || strings.Contains(logs.String(), "status=") {
		t.Errorf("log %q should carry err and no status", logs.String())
	}
}

func TestNewDoesNotLogHeaders(t *testing.T) {
	logs := captureDebugLogs(t)

	srv, _ := userAgentServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer super-secret")
	do(t, req)

	if strings.Contains(logs.String(), "super-secret") || strings.Contains(logs.String(), UserAgent) {
		t.Errorf("log %q leaks a header value", logs.String())
	}
}
