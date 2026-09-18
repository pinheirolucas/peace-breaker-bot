package instant

import (
	"errors"
	"sync"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
)

var ErrInvalidLink = errors.New("invalid link")

type Player struct {
	sync.Mutex

	playing    bool
	generation uint64

	playChan     chan string
	endChan      chan bool
	internalStop chan bool
	StopChan     chan bool
}

func NewPlayer() *Player {
	return &Player{
		playChan:     make(chan string, 1),
		endChan:      make(chan bool, 1),
		internalStop: make(chan bool, 1),
		StopChan:     make(chan bool, 1),
	}
}

func (p *Player) Close() {
	close(p.playChan)
	close(p.endChan)
	close(p.StopChan)
}

func (p *Player) Play(link string) (string, error) {
	if !IsLinkValid(link) {
		return "", ErrInvalidLink
	}

	p.Stop()

	p.Lock()
	p.generation++
	gen := p.generation
	p.Unlock()

	f, err := fsutil.GetFromCache(link)
	if err != nil {
		return "", err
	}
	defer f.Close()

	p.Lock()
	if p.generation != gen {
		// A Stop (or a Play that superseded this one) arrived while the clip
		// was still downloading, before there was anything on StopChan to
		// interrupt — report it the same way an interrupted playback would.
		p.Unlock()
		return "stop", nil
	}
	p.playing = true
	p.Unlock()

	p.playChan <- f.Name()

	select {
	case <-p.endChan:
		return "end", nil
	case <-p.internalStop:
		return "stop", nil
	}
}

func (p *Player) claimNotPlaying() bool {
	p.Lock()
	defer p.Unlock()

	if !p.playing {
		return false
	}

	p.playing = false

	return true
}

func (p *Player) Stop() {
	p.Lock()
	p.generation++
	p.Unlock()

	if !p.claimNotPlaying() {
		return
	}

	p.StopChan <- true
	p.internalStop <- true
}

func (p *Player) End() {
	if !p.claimNotPlaying() {
		return
	}

	p.endChan <- true
}

func (p *Player) GetNextPlay() string {
	return <-p.playChan
}
