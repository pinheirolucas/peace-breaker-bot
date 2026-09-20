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
