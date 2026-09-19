package instant

import (
	"testing"
	"time"
)

func TestStopImmediatelyAfterHandoffIsNotLost(t *testing.T) {
	for i := 0; i < 200; i++ {
		p := NewPlayer()
		seedCache(t, "https://example.com/w.mp3")

		reason := make(chan string, 1)
		go func() {
			r, _ := p.Play("https://example.com/w.mp3")
			reason <- r
		}()

		pb, _ := p.Next()
		p.Stop()

		select {
		case r := <-reason:
			if r != "stop" {
				t.Fatalf("iteration %d: reason = %q, want stop", i, r)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("iteration %d: Play never returned after Stop()", i)
		}

		if pb.Context().Err() == nil {
			t.Fatalf("iteration %d: playback context was not cancelled", i)
		}

		p.Close()
	}
}
