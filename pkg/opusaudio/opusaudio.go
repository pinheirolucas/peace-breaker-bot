// Package opusaudio decodes cached mp3 clips to PCM with a pure-Go decoder
// and encodes them to Opus frames for disgo's voice package.
package opusaudio

import (
	"bufio"
	"io"
	"os"
	"sync"

	"github.com/disgoorg/disgo/voice"
	"github.com/hajimehoshi/go-mp3"
	"github.com/pion/opus"
)

const (
	channels  int = 2
	frameRate int = 48000
	frameSize int = 960
	maxBytes  int = (frameSize * 2) * 2
)

// OnError is called on decode/encode errors. It logs to stderr by default.
var OnError = func(str string, err error) {
	prefix := "opusaudio: " + str

	if err != nil {
		os.Stderr.WriteString(prefix + ": " + err.Error() + "\n")
	} else {
		os.Stderr.WriteString(prefix + "\n")
	}
}

type mp3OpusProvider struct {
	file    *os.File
	pcm     *bufio.Reader
	encoder *opus.Encoder
	stop    <-chan bool

	closeOnce sync.Once
	done      chan struct{}
}

func newMp3OpusProvider(filename string, stop <-chan bool) (*mp3OpusProvider, <-chan struct{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	resampled := NewResampler(decoder, decoder.SampleRate(), frameRate, channels)

	encoder, err := opus.NewEncoder(
		opus.WithChannels(channels),
		opus.WithApplication(opus.ApplicationAudio),
		opus.WithBitrate(96000),
		opus.WithComplexity(10),
		opus.WithVBR(true),
	)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	p := &mp3OpusProvider{
		file:    file,
		pcm:     bufio.NewReaderSize(resampled, 16384),
		encoder: encoder,
		stop:    stop,
		done:    make(chan struct{}),
	}

	return p, p.done, nil
}

func (p *mp3OpusProvider) ProvideOpusFrame() ([]byte, error) {
	select {
	case <-p.stop:
		p.finish()
		return nil, io.EOF
	default:
	}

	pcm := make([]byte, frameSize*channels*2)
	if _, err := io.ReadFull(p.pcm, pcm); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			OnError("error reading decoded mp3 PCM", err)
		}
		p.finish()
		return nil, io.EOF
	}

	out := make([]byte, maxBytes)
	n, err := p.encoder.Encode(pcm, out)
	if err != nil {
		OnError("encoding error", err)
		p.finish()
		return nil, io.EOF
	}

	return out[:n], nil
}

func (p *mp3OpusProvider) Close() {
	p.finish()
}

func (p *mp3OpusProvider) finish() {
	p.closeOnce.Do(func() {
		_ = p.file.Close()
		close(p.done)
	})
}

// PlayAudioFile plays filename over the given voice.Conn and blocks until
// playback ends or stop is signalled.
func PlayAudioFile(conn voice.Conn, filename string, stop <-chan bool) {
	provider, done, err := newMp3OpusProvider(filename, stop)
	if err != nil {
		OnError("failed to decode mp3", err)
		return
	}

	conn.SetOpusFrameProvider(provider)
	<-done
}
