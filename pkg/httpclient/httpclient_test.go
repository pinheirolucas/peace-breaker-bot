package httpclient

import (
	"net/http"
	"net/http/httptest"
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
