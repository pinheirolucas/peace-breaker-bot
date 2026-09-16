package soundbuttons

import (
	"encoding/json"
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

func TestUnescapeJSStringDecodesUnicodeEscapesAndDoubledSlashes(t *testing.T) {
	// What the site actually sends: json_encode's default \/ escaping,
	// further escaped once more for embedding in a single-quoted JS string,
	// plus \uXXXX in place of literal double quotes.
	in := `{"title":"Boom","url":"https:\\\/\\\/cdn.soundbuttons.io\\\/a.mp3"}`

	got := unescapeJSString(in)

	want := `{"title":"Boom","url":"https:\/\/cdn.soundbuttons.io\/a.mp3"}`
	if got != want {
		t.Fatalf("unescapeJSString =\n%s\nwant\n%s", got, want)
	}

	var c card
	if err := json.Unmarshal([]byte(got), &c); err != nil {
		t.Fatalf("resulting string is not valid JSON: %v", err)
	}
	if c.Title != "Boom" || c.URL != "https://cdn.soundbuttons.io/a.mp3" {
		t.Errorf("decoded = %+v", c)
	}
}

func TestParseListReadsTitlesAndURLs(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "full.html")), 1)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != pageSize {
		t.Fatalf("got %d instants, want %d", len(got.Instants), pageSize)
	}

	want := provider.Instant{
		Name: "[Ultrakill] Wiggle Meme",
		URL:  "https://cdn.soundbuttons.io/sounds/2520e4a92c184efc19211cdfcfdc9b8f.mp3",
	}
	if got.Instants[0] != want {
		t.Errorf("Instants[0] = %+v, want %+v", got.Instants[0], want)
	}
}

func TestParseListInfersAnotherPageAfterAFullOne(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "full.html")), 2)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if got.Pages != 3 {
		t.Errorf("Pages = %d, want 3", got.Pages)
	}
}

func TestParseListTreatsAShortPageAsTheLast(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "short.html")), 1)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}

	if len(got.Instants) != 3 {
		t.Errorf("got %d instants, want 3", len(got.Instants))
	}
	if got.Pages != 1 {
		t.Errorf("Pages = %d, want 1", got.Pages)
	}
}

func TestParseListHandlesAPageWithNoResults(t *testing.T) {
	got, err := parseList(strings.NewReader(fixture(t, "empty.html")), 1)
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

func TestParseListFailsLoudOnAnUnreadableClickPayload(t *testing.T) {
	html := `<article class="sc" @click="$store.player.play(JSON.parse('not json'))"></article>`

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

func TestListBrowsesTrendingOnPageOne(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "empty.html")))
	})

	if _, err := p.List(provider.ListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/trending"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
}

func TestListBrowsesDeeperPagesThroughTheFeedAPI(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(`{"html":"","nextUrl":""}`))
	})

	if _, err := p.List(provider.ListParams{Page: 3}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/api/feed/trending?sort=trending&page=3"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
}

func TestListDegradesToAnEmptyPageWhenTheFeedAPIChangesShape(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not the feed JSON shape anymore</html>"))
	})

	got, err := p.List(provider.ListParams{Page: 2})
	if err != nil {
		t.Fatalf("List: %v, want a graceful empty page instead", err)
	}
	if len(got.Instants) != 0 {
		t.Errorf("got %d instants, want none", len(got.Instants))
	}
}

func TestListSearchesByName(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "empty.html")))
	})

	if _, err := p.List(provider.ListParams{Page: 2, Search: "vine boom"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/search?q=vine+boom&page=2"; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
}

func TestListIgnoresRegion(t *testing.T) {
	var gotPath string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(fixture(t, "empty.html")))
	})

	if _, err := p.List(provider.ListParams{Region: "br"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/trending"; gotPath != want {
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
