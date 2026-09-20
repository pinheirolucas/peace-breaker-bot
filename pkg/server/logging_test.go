package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

// captureLogs routes the default slog logger into the returned buffer at the
// given level for the duration of the test.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}

func TestHandleInstantListLogsUnexpectedMarkupAsAnError(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)

	s := &Server{registry: provider.Registry{"myinstants": &fakeProvider{key: "myinstants", err: provider.ErrUnexpectedMarkup}}}
	s.handleListInstants(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/instants", nil))

	for _, want := range []string{"level=ERROR", "provider=myinstants"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q does not contain %q", logs.String(), want)
		}
	}
}

func TestHandleInstantContentLogsAnUpstreamFailureAsAWarning(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/a.mp3"
	upstreamURL, _ := url.Parse(upstream.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/"+link+"/content", nil)
	req.SetPathValue("url", link)

	serverAllowingHost(upstreamURL.Hostname()).handleInstantContent(httptest.NewRecorder(), req)

	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "status 403") {
		t.Errorf("log %q does not warn about the 403", logs.String())
	}
}

func TestLoggingMiddlewareRecordsTheHandlersStatus(t *testing.T) {
	logs := captureLogs(t, slog.LevelDebug)

	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", nil))

	if rec.Code != http.StatusConflict {
		t.Errorf("response status = %d, want %d passed through", rec.Code, http.StatusConflict)
	}
	for _, want := range []string{`msg="api request"`, "method=POST", "path=/api/v1/bot/play", "status=409", "durationMs="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q does not contain %q", logs.String(), want)
		}
	}
}

func TestLoggingMiddlewareReportsTheImplicit200(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"write only": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) },
		"no write":   func(w http.ResponseWriter, r *http.Request) {},
	}

	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureLogs(t, slog.LevelDebug)

			loggingMiddleware(handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

			if !strings.Contains(logs.String(), "status=200") {
				t.Errorf("log %q does not report status 200", logs.String())
			}
		})
	}
}

func TestLoggingMiddlewareStaysQuietAboveDebug(t *testing.T) {
	logs := captureLogs(t, slog.LevelInfo)

	called := false
	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if !called {
		t.Error("handler not called")
	}
	if logs.Len() != 0 {
		t.Errorf("logged at INFO: %q", logs.String())
	}
}

func TestHandleInstantListLogsWhatWasListed(t *testing.T) {
	logs := captureLogs(t, slog.LevelDebug)

	result := &provider.ListResult{Instants: []provider.Instant{{Name: "a", URL: "u"}}, Pages: 3}
	s := &Server{registry: provider.Registry{"myinstants": &fakeProvider{key: "myinstants", result: result}}}
	s.handleListInstants(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/instants?page=2&search=wow", nil))

	for _, want := range []string{`msg="instants listed"`, "provider=myinstants", "page=2", "search=wow", "count=1", "pages=3"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q does not contain %q", logs.String(), want)
		}
	}
}

func TestHandleInstantContentLogsWhyAURLWasRejected(t *testing.T) {
	logs := captureLogs(t, slog.LevelDebug)

	link := "https://evil.example/a.mp3"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/x/content", nil)
	req.SetPathValue("url", link)

	serverAllowingHost("www.myinstants.com").handleInstantContent(httptest.NewRecorder(), req)

	if !strings.Contains(logs.String(), "reason=host-not-allowed") || !strings.Contains(logs.String(), "host=evil.example") {
		t.Errorf("log %q does not explain the rejection", logs.String())
	}
}
