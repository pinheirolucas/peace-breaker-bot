package instant

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
)

const (
	reasonEnd  = "end"
	reasonStop = "stop"
)

var (
	ErrInvalidLink = errors.New("invalid link")
	ErrClosed      = errors.New("player closed")

	errSuperseded = errors.New("superseded by a newer request")
)

// Playback is a single clip handed to the consumer by Player.Next.
type Playback struct {
	player *Player
	path   string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	reason string
	once   sync.Once
}

func newPlayback(player *Player, path string) *Playback {
	ctx, cancel := context.WithCancel(context.Background())

	return &Playback{
		player: player,
		path:   path,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// Path is the cached file to play.
func (pb *Playback) Path() string {
	return pb.path
}

// Context is cancelled when the playback is stopped or replaced.
func (pb *Playback) Context() context.Context {
	return pb.ctx
}

// End reports that the consumer is done with the playback. It has no effect
// on a playback that was already stopped.
func (pb *Playback) End() {
	pb.player.release(pb)
	pb.finish(reasonEnd)
}

func (pb *Playback) stop() {
	pb.finish(reasonStop)
}

func (pb *Playback) finish(reason string) {
	pb.once.Do(func() {
		pb.reason = reason
		close(pb.done)
		pb.cancel()
	})
}

// Player plays one clip at a time. A newer Play replaces the current clip.
type Player struct {
	mu      sync.Mutex
	seq     uint64
	floor   uint64
	current *Playback
	pending *Playback

	wake chan struct{}
	quit chan struct{}
}

func NewPlayer() *Player {
	return &Player{
		wake: make(chan struct{}, 1),
		quit: make(chan struct{}),
	}
}

// Close stops the current clip and releases every blocked Play and Next.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.quit:
		return
	default:
	}

	close(p.quit)
	p.dropCurrent()

	slog.Debug("player closed")
}

// Play blocks until the clip ends or is stopped and returns "end" or "stop".
// A request is stopped if a newer one arrived after it, or if Stop was called
// while it was still being fetched.
func (p *Player) Play(link string) (string, error) {
	if !IsLinkValid(link) {
		return "", ErrInvalidLink
	}

	ticket := p.nextTicket()
	slog.Debug("play requested", "ticket", ticket, "link", link)

	f, err := fsutil.GetFromCache(link)
	if err != nil {
		return "", err
	}
	defer f.Close()

	slog.Debug("clip resolved", "ticket", ticket, "path", f.Name())

	pb, err := p.submit(ticket, f.Name())
	switch {
	case errors.Is(err, errSuperseded):
		return reasonStop, nil
	case err != nil:
		return "", err
	}

	start := time.Now()
	<-pb.done

	slog.Debug("playback finished",
		"ticket", ticket,
		"reason", pb.reason,
		"path", pb.path,
		"durationMs", time.Since(start).Milliseconds(),
	)

	return pb.reason, nil
}

// Stop stops the current clip and any request still being fetched.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.floor = p.seq
	slog.Debug("stop requested", "floor", p.floor, "hadCurrent", p.current != nil)
	p.dropCurrent()
}

// Next blocks until there is a clip to play. It returns false once the player
// is closed.
func (p *Player) Next() (*Playback, bool) {
	for {
		select {
		case <-p.wake:
		case <-p.quit:
			return nil, false
		}

		p.mu.Lock()
		pb := p.pending
		p.pending = nil
		p.mu.Unlock()

		if pb != nil && pb.ctx.Err() == nil {
			return pb, true
		}
		if pb != nil {
			slog.Debug("skipping stale playback", "path", pb.path)
		}
	}
}

func (p *Player) nextTicket() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.seq++

	return p.seq
}

func (p *Player) submit(ticket uint64, path string) (*Playback, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.quit:
		return nil, ErrClosed
	default:
	}

	if ticket <= p.floor {
		slog.Debug("play superseded", "ticket", ticket, "floor", p.floor)
		return nil, errSuperseded
	}
	p.floor = ticket

	slog.Debug("playback submitted", "ticket", ticket, "replacedCurrent", p.current != nil)
	p.dropCurrent()

	pb := newPlayback(p, path)
	p.current, p.pending = pb, pb

	select {
	case p.wake <- struct{}{}:
	default:
	}

	return pb, nil
}

func (p *Player) release(pb *Playback) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.current == pb {
		p.current = nil
	}
}

func (p *Player) dropCurrent() {
	if p.current != nil {
		p.current.stop()
	}

	p.current, p.pending = nil, nil
}
