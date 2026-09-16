package soundboardguy

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

func TestParseListReadsNamesAndJoinsTheAudioSource(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-full.html")), 1, searchPageSize)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != 10 {
		t.Fatalf("got %d instants, want 10", len(got.Instants))
	}
	for _, i := range got.Instants {
		if strings.Contains(i.URL, "decoy") {
			t.Errorf("instant %+v came from the Discover fallback grid, not real results", i)
		}
	}
}

func TestParseListIgnoresTheDiscoverFallbackGrid(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-empty.html")), 1, searchPageSize)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != 0 {
		t.Errorf("got %d instants, want none — the Discover grid should have been ignored: %+v", len(got.Instants), got.Instants)
	}
}

func TestParseListTreatsAShortPageAsTheLast(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-short.html")), 1, searchPageSize)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != 2 {
		t.Errorf("got %d instants, want 2", len(got.Instants))
	}
	if got.Pages != 1 {
		t.Errorf("Pages = %d, want 1", got.Pages)
	}
}

func TestParseListInfersAnotherPageAfterAFullOne(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "search-full.html")), 2, searchPageSize)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if got.Pages != 3 {
		t.Errorf("Pages = %d, want 3", got.Pages)
	}
}

func TestParseListFailsLoudOnATriggerWithNoMatchingAudio(t *testing.T) {
	html := `<div class="sbg-big-grid">
		<a class="shareable--trigger" data-name="Orphan" data-audio="missing-id"></a>
	</div>`

	_, err := parseList(strings.NewReader(html), 1, searchPageSize)

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

func TestListBrowsesSounds(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-empty.html")))
	})

	if _, err := p.List(provider.ListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/sounds/"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}

	if _, err := p.List(provider.ListParams{Page: 3}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/sounds/page/3/"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
}

func TestListSearchesByName(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "search-empty.html")))
	})

	if _, err := p.List(provider.ListParams{Search: "vine boom"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/?s=vine+boom"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}

	if _, err := p.List(provider.ListParams{Page: 2, Search: "vine boom"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/page/2/?s=vine+boom"; gotPath != want {
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
	if want := "/sounds/"; gotPath != want {
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

func TestListSurfacesUpstreamErrorStatus(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := p.List(provider.ListParams{})

	if !errors.Is(err, provider.ErrBadUpstreamStatus) {
		t.Errorf("err = %v, want provider.ErrBadUpstreamStatus", err)
	}
}
