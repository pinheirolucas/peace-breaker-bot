package instantsmeme

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

func fixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	return string(b)
}

func TestParseListReadsNamesAndLinks(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-full.html")), 1)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != pageSize {
		t.Fatalf("got %d instants, want %d", len(got.Instants), pageSize)
	}

	want := provider.Instant{Name: "VINE BOOM SOUND", URL: "https://cdn.instants.meme/2026/01/18/vine-boom-sound.mp3"}
	if got.Instants[0] != want {
		t.Errorf("Instants[0] = %+v, want %+v", got.Instants[0], want)
	}
}

func TestParseListInfersAnotherPageAfterAFullOne(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-full.html")), 2)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if got.Pages != 3 {
		t.Errorf("Pages = %d, want 3", got.Pages)
	}
}

func TestParseListTreatsAShortPageAsTheLast(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-short.html")), 1)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != 5 {
		t.Errorf("got %d instants, want 5", len(got.Instants))
	}
	if got.Pages != 1 {
		t.Errorf("Pages = %d, want 1", got.Pages)
	}
}

func TestParseListHandlesAPageWithNoResults(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-empty.html")), 1)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != 0 {
		t.Errorf("got %d instants, want none", len(got.Instants))
	}
	if got.Pages != 1 {
		t.Errorf("Pages = %d, want 1", got.Pages)
	}
}

func TestParseListFailsLoudOnAButtonMissingTitle(t *testing.T) {
	html := `<button class="sound-btn" data-src="https://cdn.instants.meme/a.mp3"></button>`

	_, err := parseList(strings.NewReader(html), 1)

	if !errors.Is(err, provider.ErrUnexpectedMarkup) {
		t.Errorf("err = %v, want provider.ErrUnexpectedMarkup", err)
	}
}

func newTestProvider(t *testing.T, h http.HandlerFunc) *Provider {
	t.Helper()

	upstream := httptest.NewServer(h)
	t.Cleanup(upstream.Close)

	return &Provider{BaseURL: upstream.URL, Client: upstream.Client()}
}

func TestListBrowsesPopular(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-short.html")))
	})

	if _, err := p.List(provider.ListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}

	if want := "/popular/?page=1"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
}

func TestListSearchesByName(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-short.html")))
	})

	if _, err := p.List(provider.ListParams{Page: 2, Search: "vine boom"}); err != nil {
		t.Fatalf("List: %v", err)
	}

	if want := "/?s=vine+boom&paged=2"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
}

func TestListIgnoresRegion(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-empty.html")))
	})

	if _, err := p.List(provider.ListParams{Region: "br"}); err != nil {
		t.Fatalf("List: %v", err)
	}

	if want := "/popular/?page=1"; gotPath != want {
		t.Errorf("upstream path = %q, want %q — region should be ignored", gotPath, want)
	}
}

func TestListTreatsUpstream404AsEmpty(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	got, err := p.List(provider.ListParams{Search: "nothing"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if got.Instants == nil || len(got.Instants) != 0 {
		t.Errorf("Instants = %v, want a non-nil empty slice", got.Instants)
	}
	if got.Pages != 1 {
		t.Errorf("Pages = %d, want 1", got.Pages)
	}
}

// /popular/ answers an out-of-range page by 301-redirecting to page 1
// instead of 404ing — this must not come back looking like real page-N
// content.
func TestListTreatsARedirectedPageAsPastTheEnd(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if page := r.URL.Query().Get("page"); page == "" || page == "1" {
			w.Write([]byte(fixture(t, "search-full.html")))
			return
		}

		http.Redirect(w, r, "/popular/", http.StatusMovedPermanently)
	})

	got, err := p.List(provider.ListParams{Page: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(got.Instants) != 0 {
		t.Errorf("got %d instants, want none (redirected past the end)", len(got.Instants))
	}
	if got.Pages != 4 {
		t.Errorf("Pages = %d, want 4", got.Pages)
	}
}

func TestListSurfacesUpstreamErrorStatus(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := p.List(provider.ListParams{})

	if !errors.Is(err, provider.ErrBadUpstreamStatus) {
		t.Errorf("err = %v, want provider.ErrBadUpstreamStatus", err)
	}
}
