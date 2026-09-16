// Package httpclient builds the client every request to myinstants.com goes
// through, setting a non-default User-Agent required to get past Cloudflare.
package httpclient

import (
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
	if req.Header.Get("User-Agent") != "" {
		return t.base.RoundTrip(req)
	}

	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", UserAgent)

	return t.base.RoundTrip(req)
}

// New returns a client that identifies itself with UserAgent.
func New() *http.Client {
	return &http.Client{
		Transport: &userAgentTransport{base: http.DefaultTransport},
		Timeout:   timeout,
	}
}
