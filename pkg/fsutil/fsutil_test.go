package fsutil

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newCache wires a Cache at a temp dir and a fixture server, so nothing here
// touches ~/.instants or the network.
func newCache(t *testing.T, h http.HandlerFunc) (*Cache, string) {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &Cache{Client: srv.Client(), Dir: t.TempDir()}, srv.URL
}

func serveFile(t *testing.T, name string) http.HandlerFunc {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	return func(w http.ResponseWriter, r *http.Request) { w.Write(body) }
}

func TestGetDownloadsAndCachesAnMp3(t *testing.T) {
	var hits int
	c, base := newCache(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		serveFile(t, "valid.mp3")(w, r)
	})

	link := base + "/a.mp3"

	f, err := c.Get(link)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	f.Close()

	want := filepath.Join(c.Dir, fmt.Sprintf("%x.mp3", md5.Sum([]byte(link))))
	if f.Name() != want {
		t.Errorf("cached at %q, want %q", f.Name(), want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("cache file missing: %v", err)
	}

	// Second call must be served from disk without re-fetching.
	f2, err := c.Get(link)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	f2.Close()

	if hits != 1 {
		t.Errorf("server received %d requests, want 1 — the cache was not reused", hits)
	}
}

func TestGetReturnsFileReadableFromTheStart(t *testing.T) {
	c, base := newCache(t, serveFile(t, "valid.mp3"))

	f, err := c.Get(base + "/a.mp3")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer f.Close()

	head := make([]byte, 3)
	if _, err := f.Read(head); err != nil {
		t.Fatalf("reading returned file: %v", err)
	}
	if string(head) != "ID3" {
		t.Errorf("first bytes = %q, want the mp3 to be seeked back to the start", head)
	}
}

func TestGetRejectsNonMp3Content(t *testing.T) {
	c, base := newCache(t, serveFile(t, "not-an-mp3.wav"))

	_, err := c.Get(base + "/a.mp3")

	if err != ErrUnsuportedAudioFormat {
		t.Errorf("err = %v, want ErrUnsuportedAudioFormat", err)
	}
}

func TestGetMapsNotFoundToErrNotFound(t *testing.T) {
	c, base := newCache(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	_, err := c.Get(base + "/missing.mp3")

	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetSurfacesOtherStatusCodes(t *testing.T) {
	c, base := newCache(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := c.Get(base + "/a.mp3")

	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Errorf("err = %v, want ErrUpstreamUnavailable", err)
	}
	for _, e := range []error{ErrNotFound, ErrUnsuportedAudioFormat} {
		if errors.Is(err, e) {
			t.Errorf("err = %v, want ErrUpstreamUnavailable rather than %v", err, e)
		}
	}
}

func TestDirOrCreateUsesTheOverrideAndCreatesIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	c := &Cache{Dir: dir}

	got, err := c.DirOrCreate()
	if err != nil {
		t.Fatalf("DirOrCreate: %v", err)
	}
	if got != dir {
		t.Errorf("DirOrCreate() = %q, want %q", got, dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("directory was not created: %v", err)
	}
}

func TestDirOrCreateFallsBackToHomeInstants(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := (&Cache{}).DirOrCreate()
	if err != nil {
		t.Fatalf("DirOrCreate: %v", err)
	}

	want := filepath.Join(home, ".instants")
	if got != want {
		t.Errorf("DirOrCreate() = %q, want %q", got, want)
	}
}

func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}

func TestGetTracesADownloadThenACacheHit(t *testing.T) {
	logs := captureDebugLogs(t)
	c, base := newCache(t, serveFile(t, "valid.mp3"))
	link := base + "/a.mp3"

	for i := 0; i < 2; i++ {
		f, err := c.Get(link)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		f.Close()
	}

	for _, want := range []string{"cache miss, downloading", "download finished", "bytes=", "cache hit"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q does not contain %q", logs.String(), want)
		}
	}
	if strings.Count(logs.String(), "cache miss") != 1 {
		t.Errorf("want exactly one miss in %q", logs.String())
	}
}

func TestGetTracesWhyADownloadWasRejected(t *testing.T) {
	cases := map[string]struct {
		handler http.HandlerFunc
		want    string
	}{
		"status":  {func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }, "status=403"},
		"not-mp3": {func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>nope</html>")) }, "reason=not-mp3"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureDebugLogs(t)
			c, base := newCache(t, tc.handler)

			if _, err := c.Get(base + "/a.mp3"); err == nil {
				t.Fatal("Get succeeded")
			}

			if !strings.Contains(logs.String(), "download rejected") || !strings.Contains(logs.String(), tc.want) {
				t.Errorf("log %q does not contain a rejection with %q", logs.String(), tc.want)
			}
		})
	}
}
