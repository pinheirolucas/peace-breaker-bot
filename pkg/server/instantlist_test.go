package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testBase = "https://www.myinstants.com"

func fixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	return string(b)
}

func TestParseInstantListReadsNamesAndLinks(t *testing.T) {
	got, err := parseInstantList(strings.NewReader(fixture(t, "search.html")), testBase, 1)
	if err != nil {
		t.Fatalf("parseInstantList: %v", err)
	}

	if len(got.Instants) != pageSize {
		t.Fatalf("got %d instants, want %d", len(got.Instants), pageSize)
	}

	want := []instantButton{
		{Name: "VINE BOOM SOUND", URL: testBase + "/media/sounds/vine-boom.mp3"},
		{Name: "RUN vine", URL: testBase + "/media/sounds/run-vine-sound-effect.mp3"},
	}
	for i, w := range want {
		if got.Instants[i].Name != w.Name {
			t.Errorf("Instants[%d].Name = %q, want %q", i, got.Instants[i].Name, w.Name)
		}
		if got.Instants[i].URL != w.URL {
			t.Errorf("Instants[%d].URL = %q, want %q", i, got.Instants[i].URL, w.URL)
		}
	}
}

func TestParseInstantListInfersAnotherPageAfterAFullOne(t *testing.T) {
	got, err := parseInstantList(strings.NewReader(fixture(t, "search.html")), testBase, 2)
	if err != nil {
		t.Fatalf("parseInstantList: %v", err)
	}

	if got.Pages != 3 {
		t.Errorf("Pages = %d, want 3 (a full page 2 means there may be a page 3)", got.Pages)
	}
}

func TestParseInstantListTreatsAShortPageAsTheLast(t *testing.T) {
	got, err := parseInstantList(strings.NewReader(fixture(t, "search-last-page.html")), testBase, 4)
	if err != nil {
		t.Fatalf("parseInstantList: %v", err)
	}

	if got.Pages != 4 {
		t.Errorf("Pages = %d, want 4 (a short page is the last)", got.Pages)
	}
	if len(got.Instants) != 3 {
		t.Errorf("got %d instants, want 3", len(got.Instants))
	}
}

func TestParseInstantListHandlesAPageWithNoResults(t *testing.T) {
	got, err := parseInstantList(strings.NewReader(fixture(t, "search-empty.html")), testBase, 1)
	if err != nil {
		t.Fatalf("parseInstantList: %v", err)
	}

	if len(got.Instants) != 0 {
		t.Errorf("got %d instants, want none", len(got.Instants))
	}
	if got.Pages != 1 {
		t.Errorf("Pages = %d, want 1", got.Pages)
	}
}

func TestParseInstantListPutsAnEmptyPageBeforeItself(t *testing.T) {
	// An empty page past the end must not report itself as the total.
	got, err := parseInstantList(strings.NewReader(fixture(t, "search-empty.html")), testBase, 99)
	if err != nil {
		t.Fatalf("parseInstantList: %v", err)
	}

	if got.Pages != 98 {
		t.Errorf("Pages = %d, want 98", got.Pages)
	}
}

func TestParseInstantListRejectsMismatchedNamesAndLinks(t *testing.T) {
	// A name with no matching play button — the shape a markup change would take.
	html := `<a class="instant-link">Orphan</a><a class="instant-link">Another</a>
	         <button class="small-button" onclick="play('/media/sounds/a.mp3', 'loader-1', 'a-1')"></button>`

	_, err := parseInstantList(strings.NewReader(html), testBase, 1)

	if err != errNameLinkMismatch {
		t.Errorf("err = %v, want errNameLinkMismatch", err)
	}
}

func newTestServer(t *testing.T, h http.HandlerFunc) *Server {
	t.Helper()

	upstream := httptest.NewServer(h)
	t.Cleanup(upstream.Close)

	return &Server{myInstantsBaseURL: upstream.URL, client: upstream.Client()}
}

func upstreamPath(t *testing.T, query string) string {
	t.Helper()

	var gotPath string
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-empty.html")))
	})

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants"+query, nil))

	return gotPath
}

func TestHandleInstantListServesScrapedResults(t *testing.T) {
	var gotPath string
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search.html")))
	})

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants?page=2&search=risada%20do%20mal", nil))

	if want := "/search/?page=2&name=risada+do+mal"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}

	var body struct {
		Data instantListResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(body.Data.Instants) != pageSize || body.Data.Pages != 3 {
		t.Errorf("got %d instants / %d pages, want %d / 3", len(body.Data.Instants), body.Data.Pages, pageSize)
	}
}

func TestHandleInstantListBrowsesTheDefaultRegionOnPageOne(t *testing.T) {
	if got, want := upstreamPath(t, ""), "/en/index/us/?page=1"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestHandleInstantListBrowsesTheRequestedRegion(t *testing.T) {
	if got, want := upstreamPath(t, "?page=3&region=br"), "/en/index/br/?page=3"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestHandleInstantListNormalisesTheRegion(t *testing.T) {
	if got, want := upstreamPath(t, "?region=%20BR%20"), "/en/index/br/?page=1"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestHandleInstantListSearchIgnoresTheRegion(t *testing.T) {
	if got, want := upstreamPath(t, "?search=vine&region=br"), "/search/?page=1&name=vine"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestHandleInstantListTreatsANonNumericPageAsTheFirst(t *testing.T) {
	// The UI sends page=undefined when it has no page.
	if got, want := upstreamPath(t, "?page=undefined"), "/en/index/us/?page=1"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestHandleInstantListRejectsAnInvalidRegion(t *testing.T) {
	for _, region := range []string{"bra", "b", "b1", "..", "br%2F..%2F"} {
		t.Run(region, func(t *testing.T) {
			called := false
			s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { called = true })

			rec := httptest.NewRecorder()
			s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants?region="+region, nil))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if body := rec.Body.String(); !strings.Contains(body, `"label":"invalid_region"`) {
				t.Errorf("body = %s, want an invalid_region error", body)
			}
			if called {
				t.Error("an invalid region still reached myinstants.com")
			}
		})
	}
}

func TestHandleInstantListTreatsUpstream404AsEmpty(t *testing.T) {
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	raw := rec.Body.String()
	if !strings.Contains(raw, `"instants":[]`) {
		t.Errorf("body = %s, want an explicit empty instants array", raw)
	}

	var body struct {
		Data instantListResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Data.Instants == nil || len(body.Data.Instants) != 0 {
		t.Errorf("Instants = %v, want a non-nil empty slice", body.Data.Instants)
	}
	if body.Data.Pages != 1 {
		t.Errorf("Pages = %d, want 1", body.Data.Pages)
	}
}

func TestHandleInstantListSurfacesUpstreamErrorStatus(t *testing.T) {
	// What Cloudflare answers when the request carries a User-Agent it denies.
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	rec := httptest.NewRecorder()
	s.handleListInstants(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instants", nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"label":"bad_http_status"`) {
		t.Errorf("body = %s, want a bad_http_status error", body)
	}
}
