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

		p.GetNextPlay()
		p.Stop()

		select {
		case r := <-reason:
			if r != "stop" {
				t.Fatalf("iteration %d: reason = %q, want stop", i, r)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("iteration %d: Stop() right after the handoff was lost — Play never returned", i)
		}
		mustReceiveStop(t, p)
	}
}
