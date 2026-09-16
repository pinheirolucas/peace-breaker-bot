package myinstants

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

const testBase = "https://www.myinstants.com"

func fixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	return string(b)
}

func TestParseListReadsNamesAndLinks(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search.html")), testBase, 1)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != pageSize {
		t.Fatalf("got %d instants, want %d", len(got.Instants), pageSize)
	}

	want := []provider.Instant{
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

func TestParseListInfersAnotherPageAfterAFullOne(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search.html")), testBase, 2)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if got.Pages != 3 {
		t.Errorf("Pages = %d, want 3 (a full page 2 means there may be a page 3)", got.Pages)
	}
}

func TestParseListTreatsAShortPageAsTheLast(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-last-page.html")), testBase, 4)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if got.Pages != 4 {
		t.Errorf("Pages = %d, want 4 (a short page is the last)", got.Pages)
	}
	if len(got.Instants) != 3 {
		t.Errorf("got %d instants, want 3", len(got.Instants))
	}
}

func TestParseListHandlesAPageWithNoResults(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-empty.html")), testBase, 1)
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

func TestParseListPutsAnEmptyPageBeforeItself(t *testing.T) {
	// An empty page past the end must not report itself as the total.
	got, err := parseList(strings.NewReader(fixture(t, "search-empty.html")), testBase, 99)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if got.Pages != 98 {
		t.Errorf("Pages = %d, want 98", got.Pages)
	}
}

func TestParseListRejectsMismatchedNamesAndLinks(t *testing.T) {
	// A name with no matching play button — the shape a markup change would take.
	html := `<a class="instant-link">Orphan</a><a class="instant-link">Another</a>
	         <button class="small-button" onclick="play('/media/sounds/a.mp3', 'loader-1', 'a-1')"></button>`

	_, err := parseList(strings.NewReader(html), testBase, 1)

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

func upstreamPath(t *testing.T, params provider.ListParams) string {
	t.Helper()

	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-empty.html")))
	})

	if _, err := p.List(params); err != nil {
		t.Fatalf("List: %v", err)
	}

	return gotPath
}

func TestListServesScrapedResults(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search.html")))
	})

	got, err := p.List(provider.ListParams{Page: 2, Search: "risada do mal"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if want := "/search/?page=2&name=risada+do+mal"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
	if len(got.Instants) != pageSize || got.Pages != 3 {
		t.Errorf("got %d instants / %d pages, want %d / 3", len(got.Instants), got.Pages, pageSize)
	}
}

func TestListBrowsesTheDefaultRegionOnPageOne(t *testing.T) {
	if got, want := upstreamPath(t, provider.ListParams{}), "/en/index/us/?page=1"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestListBrowsesTheRequestedRegion(t *testing.T) {
	if got, want := upstreamPath(t, provider.ListParams{Page: 3, Region: "br"}), "/en/index/br/?page=3"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestListNormalisesTheRegion(t *testing.T) {
	if got, want := upstreamPath(t, provider.ListParams{Region: " BR "}), "/en/index/br/?page=1"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestListSearchIgnoresTheRegion(t *testing.T) {
	if got, want := upstreamPath(t, provider.ListParams{Search: "vine", Region: "br"}), "/search/?page=1&name=vine"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestListTreatsANonPositivePageAsTheFirst(t *testing.T) {
	if got, want := upstreamPath(t, provider.ListParams{Page: 0}), "/en/index/us/?page=1"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestListRejectsAnInvalidRegion(t *testing.T) {
	for _, region := range []string{"bra", "b", "b1", "..", "br%2F..%2F"} {
		t.Run(region, func(t *testing.T) {
			called := false
			p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) { called = true })

			_, err := p.List(provider.ListParams{Region: region})

			if err != provider.ErrInvalidRegion {
				t.Errorf("err = %v, want provider.ErrInvalidRegion", err)
			}
			if called {
				t.Error("an invalid region still reached myinstants.com")
			}
		})
	}
}

func TestListTreatsUpstream404AsEmpty(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	got, err := p.List(provider.ListParams{})
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

func TestListSurfacesUpstreamErrorStatus(t *testing.T) {
	// What Cloudflare answers when the request carries a User-Agent it denies.
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := p.List(provider.ListParams{})

	if !errors.Is(err, provider.ErrBadUpstreamStatus) {
		t.Errorf("err = %v, want provider.ErrBadUpstreamStatus", err)
	}
}
