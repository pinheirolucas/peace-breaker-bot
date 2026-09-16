package instant

import (
	"errors"
	"sync"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
)

var ErrInvalidLink = errors.New("invalid link")

type Player struct {
	sync.Mutex

	playing bool

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

	f, err := fsutil.GetFromCache(link)
	if err != nil {
		return "", err
	}
	defer f.Close()

	p.Stop()

	p.Lock()
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
