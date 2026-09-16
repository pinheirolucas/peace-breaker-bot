package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

// fakeProvider is a minimal provider.Provider double so dispatch/envelope
// tests don't need to depend on any one provider's markup or fixtures.
type fakeProvider struct {
	key    string
	result *provider.ListResult
	err    error

	gotParams provider.ListParams
}

func (f *fakeProvider) Key() string         { return f.key }
func (f *fakeProvider) DisplayName() string { return f.key }

func (f *fakeProvider) List(params provider.ListParams) (*provider.ListResult, error) {
	f.gotParams = params
	return f.result, f.err
}

func (f *fakeProvider) AllowedContentHosts() []string { return nil }

func TestHandleInstantListDispatchesToTheDefaultProviderWhenNoneIsRequested(t *testing.T) {
	fp := &fakeProvider{key: "myinstants", result: &provider.ListResult{
		Instants: []provider.Instant{{Name: "Bruh", URL: "https://example.com/bruh.mp3"}},
		Pages:    1,
	}}
	s := &Server{registry: provider.Registry{"myinstants": fp}}

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants?page=2&search=vine&region=br", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fp.gotParams != (provider.ListParams{Page: 2, Search: "vine", Region: "br"}) {
		t.Errorf("List called with %+v", fp.gotParams)
	}

	var body struct {
		Data instantListResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(body.Data.Instants) != 1 || body.Data.Instants[0].Name != "Bruh" {
		t.Errorf("Instants = %+v", body.Data.Instants)
	}
	if body.Data.Pages != 1 {
		t.Errorf("Pages = %d, want 1", body.Data.Pages)
	}
}

func TestHandleInstantListDispatchesToTheRequestedProvider(t *testing.T) {
	fp := &fakeProvider{key: "other", result: &provider.ListResult{Instants: []provider.Instant{}, Pages: 1}}
	s := &Server{registry: provider.Registry{
		"myinstants": &fakeProvider{key: "myinstants", result: &provider.ListResult{Instants: []provider.Instant{}, Pages: 1}},
		"other":      fp,
	}}

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants?provider=other", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fp.gotParams.Page != 1 {
		t.Errorf("the requested provider was not called: %+v", fp.gotParams)
	}
}

func TestHandleInstantListRejectsAnUnknownProvider(t *testing.T) {
	s := &Server{registry: provider.Registry{"myinstants": &fakeProvider{key: "myinstants"}}}

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants?provider=doesnotexist", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := decodeBody(t, rec)["label"]; got != "provider_not_found" {
		t.Errorf("label = %v, want provider_not_found", got)
	}
}

func TestHandleInstantListMapsProviderErrorsToTheirStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		label  string
	}{
		{"invalid region", provider.ErrInvalidRegion, http.StatusBadRequest, "invalid_region"},
		{"upstream unavailable", provider.ErrUpstreamUnavailable, http.StatusBadGateway, "http_request"},
		{"bad upstream status", provider.ErrBadUpstreamStatus, http.StatusBadGateway, "bad_http_status"},
		{"unexpected markup", provider.ErrUnexpectedMarkup, http.StatusInternalServerError, "name_link_not_matched"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &Server{registry: provider.Registry{"myinstants": &fakeProvider{key: "myinstants", err: c.err}}}

			rec := httptest.NewRecorder()
			s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants", nil))

			if rec.Code != c.status {
				t.Errorf("status = %d, want %d", rec.Code, c.status)
			}
			if got := decodeBody(t, rec)["label"]; got != c.label {
				t.Errorf("label = %v, want %v", got, c.label)
			}
		})
	}
}

// A smoke test against the real, unswapped registry — guards New() wiring
// up a usable default myinstants provider without hitting the network.
func TestHandleInstantListUsesARealMyInstantsProviderByDefault(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBotStatus())

	p, ok := s.providers().Get("myinstants")
	if !ok {
		t.Fatal(`default registry has no "myinstants" provider`)
	}
	if p.Key() != "myinstants" {
		t.Errorf("Key() = %q, want myinstants", p.Key())
	}
}
