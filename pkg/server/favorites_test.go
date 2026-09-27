package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/favorites"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
)

type favoritesEnvelope struct {
	Label string         `json:"label"`
	Data  favorites.List `json:"data"`
}

func favoritesServer(t *testing.T) (http.Handler, string) {
	t.Helper()

	dir := t.TempDir()
	s := New(instant.NewPlayer(), connectedBot(), WithFavorites(favorites.NewStore(dir), "PinheiroLucas"))

	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.pattern, rt.handler)
	}

	return mux, dir
}

func doFavorites(t *testing.T, h http.Handler, method, body string) (int, favoritesEnvelope) {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/favorites", strings.NewReader(body)))

	var out favoritesEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s response is not JSON: %v: %s", method, err, rec.Body.String())
	}

	return rec.Code, out
}

const twoFavorites = `[
	{"name": "Ai que delícia", "url": "https://www.myinstants.com/media/sounds/ai-que-delicia.mp3", "key": "a"},
	{"name": "Bruh", "url": "https://www.myinstants.com/media/sounds/movie_1.mp3"}
]`

func TestGetFavoritesOfANewOwnerIsEmpty(t *testing.T) {
	h, _ := favoritesServer(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"data":{"owner":"pinheirolucas","revision":0,"instants":[]}}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestPutFavoritesThenGet(t *testing.T) {
	h, _ := favoritesServer(t)

	code, put := doFavorites(t, h, http.MethodPut, `{"owner": "pinheirolucas", "baseRevision": 0, "instants": `+twoFavorites+`}`)
	if code != http.StatusOK {
		t.Fatalf("PUT status = %d (%s), want 200", code, put.Label)
	}
	if put.Data.Revision != 1 || len(put.Data.Instants) != 2 || put.Data.UpdatedAt == nil {
		t.Errorf("PUT data = %+v, want revision 1 with 2 instants and updatedAt", put.Data)
	}

	code, got := doFavorites(t, h, http.MethodGet, "")
	if code != http.StatusOK || got.Data.Revision != 1 || got.Data.Instants[0].Key != "a" {
		t.Errorf("GET = %d %+v, want the stored list", code, got.Data)
	}
}

func TestPutFavoritesAcceptsTheOwnerInAnyCase(t *testing.T) {
	h, _ := favoritesServer(t)

	if code, out := doFavorites(t, h, http.MethodPut, `{"owner": " PINHEIROLUCAS ", "baseRevision": 0, "instants": []}`); code != http.StatusOK {
		t.Errorf("status = %d (%s), want 200", code, out.Label)
	}
}

func TestPutFavoritesErrors(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
		label  string
	}{
		{"not json", `{`, http.StatusBadRequest, "invalid_body"},
		{"no owner", `{"baseRevision": 0, "instants": []}`, http.StatusBadRequest, "invalid_body"},
		{"no base revision", `{"owner": "pinheirolucas", "instants": []}`, http.StatusBadRequest, "invalid_body"},
		{"no instants", `{"owner": "pinheirolucas", "baseRevision": 0}`, http.StatusBadRequest, "invalid_body"},
		{"null instants", `{"owner": "pinheirolucas", "baseRevision": 0, "instants": null}`, http.StatusBadRequest, "invalid_body"},
		{"other owner", `{"owner": "someoneelse", "baseRevision": 0, "instants": []}`, http.StatusConflict, "owner_mismatch"},
		{"owner that is not a username", `{"owner": "../x", "baseRevision": 0, "instants": []}`, http.StatusConflict, "owner_mismatch"},
		{"stale revision", `{"owner": "pinheirolucas", "baseRevision": 3, "instants": []}`, http.StatusConflict, "favorites_conflict"},
		{"invalid list", `{"owner": "pinheirolucas", "baseRevision": 0, "instants": [{"name": "a", "url": "nope"}]}`, http.StatusBadRequest, "invalid_favorites"},
		{"too large", `{"owner": "pinheirolucas", "baseRevision": 0, "instants": [{"name": "` + strings.Repeat("a", maxFavoritesBody) + `"}]}`, http.StatusRequestEntityTooLarge, "favorites_too_large"},
	}

	for _, tt := range tests {
		h, _ := favoritesServer(t)

		code, out := doFavorites(t, h, http.MethodPut, tt.body)
		if code != tt.status || out.Label != tt.label {
			t.Errorf("%s: got %d %q, want %d %q", tt.name, code, out.Label, tt.status, tt.label)
		}
	}
}

func TestPutFavoritesConflictKeepsTheStoredList(t *testing.T) {
	h, _ := favoritesServer(t)

	doFavorites(t, h, http.MethodPut, `{"owner": "pinheirolucas", "baseRevision": 0, "instants": `+twoFavorites+`}`)

	if code, out := doFavorites(t, h, http.MethodPut, `{"owner": "pinheirolucas", "baseRevision": 0, "instants": []}`); code != http.StatusConflict || out.Label != "favorites_conflict" {
		t.Fatalf("second PUT from revision 0 = %d %q, want 409 favorites_conflict", code, out.Label)
	}

	if _, got := doFavorites(t, h, http.MethodGet, ""); got.Data.Revision != 1 || len(got.Data.Instants) != 2 {
		t.Errorf("list after conflict = %+v, want revision 1 untouched", got.Data)
	}
}

func TestFavoritesWithoutAStoreAreUnavailable(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/favorites", strings.NewReader(`{}`))
		if method == http.MethodGet {
			s.handleGetFavorites(rec, req)
		} else {
			s.handlePutFavorites(rec, req)
		}

		if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"favorites_unavailable"`) {
			t.Errorf("%s = %d %s, want 500 favorites_unavailable", method, rec.Code, rec.Body.String())
		}
	}
}

func TestFavoritesErrorsAreTranslated(t *testing.T) {
	h, _ := favoritesServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/favorites", strings.NewReader(`{"owner": "someoneelse", "baseRevision": 0, "instants": []}`))
	req.Header.Set("Accept-Language", "pt-BR")
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "dono deste bot") {
		t.Errorf("body = %s, want the pt-BR message", rec.Body.String())
	}
}
