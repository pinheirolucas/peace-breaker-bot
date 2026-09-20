package instant

import (
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func seedCache(t *testing.T, links ...string) []string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir := filepath.Join(home, ".instants")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating cache dir: %v", err)
	}

	paths := make([]string, 0, len(links))
	for _, link := range links {
		path := filepath.Join(dir, fmt.Sprintf("%x.mp3", md5.Sum([]byte(link))))
		if err := os.WriteFile(path, []byte("not really an mp3"), 0o644); err != nil {
			t.Fatalf("writing cache fixture: %v", err)
		}
		paths = append(paths, path)
	}

	return paths
}

func waitFor(t *testing.T, ch <-chan string) string {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Play to return")
		return ""
	}
}

func playAsync(p *Player, link string) (<-chan string, <-chan error) {
	reason := make(chan string, 1)
	errc := make(chan error, 1)

	go func() {
		r, err := p.Play(link)
		errc <- err
		reason <- r
	}()

	return reason, errc
}

func mustSubmit(t *testing.T, p *Player, path string) *Playback {
	t.Helper()

	pb, err := p.submit(p.nextTicket(), path)
	if err != nil {
		t.Fatalf("submit(%q): %v", path, err)
	}

	return pb
}

func TestPlayRejectsInvalidLinkBeforeAnyIO(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	reason, err := p.Play("not a url")

	if err != ErrInvalidLink {
		t.Errorf("err = %v, want ErrInvalidLink", err)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestNextReceivesThePathAndEndCompletesPlayback(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	path := seedCache(t, "https://example.com/a.mp3")[0]

	reason, errc := playAsync(p, "https://example.com/a.mp3")

	pb, ok := p.Next()
	if !ok {
		t.Fatal("Next() returned false on an open player")
	}
	if pb.Path() != path {
		t.Errorf("Path() = %q, want %q", pb.Path(), path)
	}

	pb.End()

	if err := <-errc; err != nil {
		t.Fatalf("Play returned error: %v", err)
	}
	if r := waitFor(t, reason); r != "end" {
		t.Errorf("Play returned %q, want \"end\"", r)
	}
}

func TestEndIsIdempotent(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	pb := mustSubmit(t, p, "a.mp3")
	pb.End()
	pb.End()

	if pb.reason != "end" {
		t.Errorf("reason = %q, want \"end\"", pb.reason)
	}
}

func TestStopMidPlaybackCancelsTheContext(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	seedCache(t, "https://example.com/b.mp3")

	reason, errc := playAsync(p, "https://example.com/b.mp3")
	pb, _ := p.Next()

	p.Stop()

	if err := <-errc; err != nil {
		t.Fatalf("Play returned error: %v", err)
	}
	if r := waitFor(t, reason); r != "stop" {
		t.Errorf("Play returned %q, want \"stop\"", r)
	}
	if pb.Context().Err() == nil {
		t.Error("playback context was not cancelled")
	}
}

func TestStopWithNothingPlayingDoesNotAffectTheNextPlay(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	seedCache(t, "https://example.com/c.mp3")

	p.Stop()

	reason, _ := playAsync(p, "https://example.com/c.mp3")
	pb, _ := p.Next()
	pb.End()

	if r := waitFor(t, reason); r != "end" {
		t.Errorf("Play returned %q, want \"end\"", r)
	}
}

func TestPlayWhileAlreadyPlayingStopsTheFirstClip(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	seedCache(t, "https://example.com/first.mp3", "https://example.com/second.mp3")

	firstReason, _ := playAsync(p, "https://example.com/first.mp3")
	first, _ := p.Next()

	secondReason, _ := playAsync(p, "https://example.com/second.mp3")

	if r := waitFor(t, firstReason); r != "stop" {
		t.Errorf("first Play returned %q, want \"stop\"", r)
	}
	if first.Context().Err() == nil {
		t.Error("first playback context was not cancelled")
	}

	second, _ := p.Next()
	second.End()

	if r := waitFor(t, secondReason); r != "end" {
		t.Errorf("second Play returned %q, want \"end\"", r)
	}
}

func TestEndOfAReplacedPlaybackDoesNotFinishTheNewOne(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	a := mustSubmit(t, p, "a.mp3")
	b := mustSubmit(t, p, "b.mp3")

	a.End()

	select {
	case <-b.done:
		t.Fatal("End() of the replaced playback finished the new one")
	default:
	}

	p.Stop()

	if b.reason != "stop" {
		t.Errorf("reason = %q, want \"stop\"", b.reason)
	}
}

func TestPlaybackReplacedBeforePickupIsSkipped(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	a := mustSubmit(t, p, "a.mp3")
	b := mustSubmit(t, p, "b.mp3")

	got, _ := p.Next()

	if got != b {
		t.Errorf("Next() returned %q, want %q", got.Path(), b.Path())
	}
	if a.reason != "stop" {
		t.Errorf("replaced playback reason = %q, want \"stop\"", a.reason)
	}
}

func TestOlderRequestDoesNotReplaceANewerOne(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	older := p.nextTicket()
	newer := p.nextTicket()

	b, err := p.submit(newer, "b.mp3")
	if err != nil {
		t.Fatalf("submit(newer): %v", err)
	}
	if _, err := p.submit(older, "a.mp3"); !errors.Is(err, errSuperseded) {
		t.Fatalf("submit(older) error = %v, want errSuperseded", err)
	}

	select {
	case <-b.done:
		t.Fatal("the newer playback was stopped by the older request")
	default:
	}
}

func TestStopDropsRequestsStillBeingFetched(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	ticket := p.nextTicket()
	p.Stop()

	if _, err := p.submit(ticket, "a.mp3"); !errors.Is(err, errSuperseded) {
		t.Fatalf("submit error = %v, want errSuperseded", err)
	}
}

func TestCloseReleasesEveryone(t *testing.T) {
	p := NewPlayer()

	seedCache(t, "https://example.com/d.mp3")

	reason, _ := playAsync(p, "https://example.com/d.mp3")
	p.Next()

	p.Close()

	if r := waitFor(t, reason); r != "stop" {
		t.Errorf("Play returned %q, want \"stop\"", r)
	}
	if _, ok := p.Next(); ok {
		t.Error("Next() returned true on a closed player")
	}
	if _, err := p.Play("https://example.com/d.mp3"); err != ErrClosed {
		t.Errorf("Play after Close error = %v, want ErrClosed", err)
	}
}

func TestConcurrentPlaysAndStopsAlwaysReturn(t *testing.T) {
	p := NewPlayer()
	defer p.Close()

	links := []string{
		"https://example.com/1.mp3",
		"https://example.com/2.mp3",
		"https://example.com/3.mp3",
	}
	seedCache(t, links...)

	go func() {
		for {
			pb, ok := p.Next()
			if !ok {
				return
			}

			time.Sleep(time.Millisecond)
			pb.End()
		}
	}()

	var wg sync.WaitGroup
	results := make(chan string, 200)

	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()

			reason, err := p.Play(links[i%len(links)])
			if err != nil {
				t.Errorf("Play returned error: %v", err)
				return
			}
			results <- reason
		}()

		go func() {
			defer wg.Done()

			if i%5 == 0 {
				p.Stop()
			}
		}()
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a Play never returned")
	}

	close(results)
	for reason := range results {
		if reason != "end" && reason != "stop" {
			t.Errorf("reason = %q, want \"end\" or \"stop\"", reason)
		}
	}
}
