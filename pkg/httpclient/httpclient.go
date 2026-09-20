// Package httpclient builds the client every request to myinstants.com goes
// through, setting a non-default User-Agent required to get past Cloudflare.
package httpclient

import (
	"log/slog"
	"net/http"
	"time"
)

// UserAgent identifies the app to myinstants.com.
const UserAgent = "peace-breaker-bot/1.0"

const timeout = 30 * time.Second

type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", UserAgent)
	}

	start := time.Now()
	res, err := t.base.RoundTrip(req)

	attrs := []any{"method", req.Method, "url", req.URL, "durationMs", time.Since(start).Milliseconds()}
	if err != nil {
		attrs = append(attrs, "err", err)
	} else {
		attrs = append(attrs, "status", res.StatusCode)
	}
	slog.Debug("upstream request", attrs...)

	return res, err
}

// New returns a client that identifies itself with UserAgent.
func New() *http.Client {
	return &http.Client{
		Transport: &userAgentTransport{base: http.DefaultTransport},
		Timeout:   timeout,
	}
}
