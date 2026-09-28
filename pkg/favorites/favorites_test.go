package favorites

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()

	dir := t.TempDir()
	s := NewStore(dir)
	s.now = func() time.Time { return time.Date(2026, 9, 27, 14, 3, 11, 500, time.UTC) }

	return s, dir
}

func sample() []Instant {
	return []Instant{
		{Name: "Ai que delícia", URL: "https://www.myinstants.com/media/sounds/ai-que-delicia.mp3", Key: "a"},
		{Name: "Bruh", URL: "https://www.myinstants.com/media/sounds/movie_1.mp3"},
	}
}

func TestOwnerKey(t *testing.T) {
	tests := []struct {
		owner string
		want  string
		ok    bool
	}{
		{"pinheirolucas", "pinheirolucas", true},
		{"  PinheiroLucas ", "pinheirolucas", true},
		{"lucas.pinheiro_2", "lucas.pinheiro_2", true},
		{"", "", false},
		{"a", "", false},
		{strings.Repeat("a", 33), "", false},
		{"lucas..pinheiro", "", false},
		{"../etc", "", false},
		{"lucas/pinheiro", "", false},
		{"Name#1234", "", false},
	}

	for _, tt := range tests {
		got, err := OwnerKey(tt.owner)
		if tt.ok && (err != nil || got != tt.want) {
			t.Errorf("OwnerKey(%q) = %q, %v; want %q", tt.owner, got, err, tt.want)
		}
		if !tt.ok && !errors.Is(err, ErrInvalidOwner) {
			t.Errorf("OwnerKey(%q) error = %v, want ErrInvalidOwner", tt.owner, err)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := func(name, url, key string) Instant { return Instant{Name: name, URL: url, Key: key} }
	const u1, u2 = "https://example.com/a.mp3", "https://example.com/b.mp3"

	tests := []struct {
		name     string
		instants []Instant
		valid    bool
	}{
		{"empty list", nil, true},
		{"sample", sample(), true},
		{"http url", []Instant{ok("a", "http://192.168.0.12:9001/a.mp3", "")}, true},
		{"name at the limit", []Instant{ok(strings.Repeat("é", MaxNameLength), u1, "")}, true},
		{"blank name", []Instant{ok("   ", u1, "")}, false},
		{"name over the limit", []Instant{ok(strings.Repeat("é", MaxNameLength+1), u1, "")}, false},
		{"relative url", []Instant{ok("a", "/media/a.mp3", "")}, false},
		{"ftp url", []Instant{ok("a", "ftp://example.com/a.mp3", "")}, false},
		{"url without host", []Instant{ok("a", "https://", "")}, false},
		{"duplicate url", []Instant{ok("a", u1, ""), ok("b", u1, "")}, false},
		{"uppercase key", []Instant{ok("a", u1, "A")}, false},
		{"punctuation key", []Instant{ok("a", u1, ";")}, false},
		{"two-letter key", []Instant{ok("a", u1, "ab")}, false},
		{"duplicate key", []Instant{ok("a", u1, "x"), ok("b", u2, "x")}, false},
	}

	for _, tt := range tests {
		err := Validate(tt.instants)
		if tt.valid && err != nil {
			t.Errorf("%s: Validate = %v, want nil", tt.name, err)
		}
		if !tt.valid && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Validate = %v, want ErrInvalid", tt.name, err)
		}
	}
}

func TestValidateRejectsTooManyItems(t *testing.T) {
	instants := make([]Instant, MaxInstants+1)
	for i := range instants {
		instants[i] = Instant{Name: "clip", URL: "https://example.com/" + strings.Repeat("a", i+1)}
	}

	if err := Validate(instants); !errors.Is(err, ErrInvalid) {
		t.Errorf("Validate = %v, want ErrInvalid", err)
	}
	if err := Validate(instants[:MaxInstants]); err != nil {
		t.Errorf("Validate at the limit = %v, want nil", err)
	}
}

func TestGetOfAnUnknownOwnerIsEmpty(t *testing.T) {
	s, _ := testStore(t)

	list, err := s.Get("PinheiroLucas")
	if err != nil {
		t.Fatal(err)
	}
	if list.Owner != "pinheirolucas" || list.Revision != 0 || list.UpdatedAt != nil || list.Instants == nil || len(list.Instants) != 0 {
		t.Errorf("Get = %+v, want an empty list at revision 0", list)
	}
}

func TestPutThenGetRoundTrips(t *testing.T) {
	s, dir := testStore(t)

	put, err := s.Put("pinheirolucas", 0, sample())
	if err != nil {
		t.Fatal(err)
	}
	if put.Revision != 1 {
		t.Errorf("revision = %d, want 1", put.Revision)
	}
	if want := time.Date(2026, 9, 27, 14, 3, 11, 0, time.UTC); put.UpdatedAt == nil || !put.UpdatedAt.Equal(want) {
		t.Errorf("updatedAt = %v, want %v", put.UpdatedAt, want)
	}

	reopened := NewStore(dir)
	got, err := reopened.Get("pinheirolucas")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 1 || len(got.Instants) != 2 || got.Instants[0] != sample()[0] {
		t.Errorf("reloaded list = %+v, want the stored one", got)
	}

	if _, err := os.Stat(filepath.Join(dir, "favorites", "pinheirolucas.json")); err != nil {
		t.Errorf("list file: %v", err)
	}
}

func TestPutIncrementsTheRevision(t *testing.T) {
	s, _ := testStore(t)

	for base := int64(0); base < 3; base++ {
		list, err := s.Put("pinheirolucas", base, sample()[:1])
		if err != nil {
			t.Fatal(err)
		}
		if list.Revision != base+1 {
			t.Errorf("revision = %d, want %d", list.Revision, base+1)
		}
	}
}

func TestPutWithAStaleBaseConflicts(t *testing.T) {
	s, _ := testStore(t)

	if _, err := s.Put("pinheirolucas", 0, sample()); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Put("pinheirolucas", 0, nil); !errors.Is(err, ErrConflict) {
		t.Errorf("Put with a stale base = %v, want ErrConflict", err)
	}
	if _, err := s.Put("pinheirolucas", 5, nil); !errors.Is(err, ErrConflict) {
		t.Errorf("Put with a future base = %v, want ErrConflict", err)
	}

	got, _ := s.Get("pinheirolucas")
	if got.Revision != 1 || len(got.Instants) != 2 {
		t.Errorf("list after conflicts = %+v, want revision 1 untouched", got)
	}
}

func TestPutRejectsAnInvalidList(t *testing.T) {
	s, dir := testStore(t)

	bad := []Instant{{Name: "a", URL: "not a url"}}
	if _, err := s.Put("pinheirolucas", 0, bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("Put = %v, want ErrInvalid", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "favorites")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("favorites dir exists after a rejected Put: %v", err)
	}
}

func TestOwnersHaveSeparateLists(t *testing.T) {
	s, _ := testStore(t)

	if _, err := s.Put("pinheirolucas", 0, sample()); err != nil {
		t.Fatal(err)
	}

	other, err := s.Get("someoneelse")
	if err != nil {
		t.Fatal(err)
	}
	if other.Revision != 0 || len(other.Instants) != 0 {
		t.Errorf("other owner's list = %+v, want empty", other)
	}

	if _, err := s.Put("someoneelse", 0, sample()[1:]); err != nil {
		t.Errorf("Put for another owner at revision 0 = %v", err)
	}
}

func TestInvalidOwnerIsRejected(t *testing.T) {
	s, _ := testStore(t)

	if _, err := s.Get("../escape"); !errors.Is(err, ErrInvalidOwner) {
		t.Errorf("Get = %v, want ErrInvalidOwner", err)
	}
	if _, err := s.Put("../escape", 0, nil); !errors.Is(err, ErrInvalidOwner) {
		t.Errorf("Put = %v, want ErrInvalidOwner", err)
	}
}

func TestALeftoverTempFileIsIgnored(t *testing.T) {
	s, dir := testStore(t)

	if _, err := s.Put("pinheirolucas", 0, sample()); err != nil {
		t.Fatal(err)
	}

	stray := filepath.Join(dir, "favorites", ".pinheirolucas-123.tmp")
	if err := os.WriteFile(stray, []byte(`{"revision": 99, "instants": [{"name": "half`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := NewStore(dir).Get("pinheirolucas")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 1 || len(got.Instants) != 2 {
		t.Errorf("list = %+v, want revision 1 from the renamed file", got)
	}
}

func TestAFailedWriteKeepsTheOldList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores directory permissions")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	s, dir := testStore(t)

	if _, err := s.Put("pinheirolucas", 0, sample()); err != nil {
		t.Fatal(err)
	}

	listDir := filepath.Join(dir, "favorites")
	if err := os.Chmod(listDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(listDir, 0o755) })

	if _, err := s.Put("pinheirolucas", 1, nil); err == nil {
		t.Fatal("Put into a read-only dir succeeded")
	}

	got, err := s.Get("pinheirolucas")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 1 || len(got.Instants) != 2 {
		t.Errorf("list after a failed write = %+v, want revision 1", got)
	}

	if _, err := s.Put("pinheirolucas", 1, nil); err == nil {
		t.Fatal("second Put into a read-only dir succeeded")
	}
}

func TestACorruptFileIsAnError(t *testing.T) {
	_, dir := testStore(t)

	listDir := filepath.Join(dir, "favorites")
	if err := os.MkdirAll(listDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(listDir, "pinheirolucas.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(dir)
	if _, err := s.Get("pinheirolucas"); err == nil {
		t.Error("Get of a corrupt file succeeded")
	}
	if _, err := s.Put("pinheirolucas", 0, nil); err == nil || errors.Is(err, ErrConflict) {
		t.Errorf("Put over a corrupt file = %v, want a read error", err)
	}
}
