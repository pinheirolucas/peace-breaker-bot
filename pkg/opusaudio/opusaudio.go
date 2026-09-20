// Package opusaudio decodes cached mp3 clips to PCM with a pure-Go decoder
// and encodes them to Opus frames for disgo's voice package.
package opusaudio

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

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
	ctx     context.Context

	// frames is atomic because finish can run from PlayAudioFile's goroutine
	// (via Close) while disgo is still pulling frames on its own.
	frames atomic.Int64

	closeOnce sync.Once
	done      chan struct{}
}

func newMp3OpusProvider(ctx context.Context, filename string) (*mp3OpusProvider, <-chan struct{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	slog.Debug("decoder opened", "path", filename, "sampleRate", decoder.SampleRate(), "resampling", decoder.SampleRate() != frameRate)

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
		ctx:     ctx,
		done:    make(chan struct{}),
	}

	return p, p.done, nil
}

func (p *mp3OpusProvider) ProvideOpusFrame() ([]byte, error) {
	if p.ctx.Err() != nil {
		p.finish("cancelled")
		return nil, io.EOF
	}

	pcm := make([]byte, frameSize*channels*2)
	if _, err := io.ReadFull(p.pcm, pcm); err != nil {
		reason := "eof"
		if err != io.EOF && err != io.ErrUnexpectedEOF && p.ctx.Err() == nil {
			OnError("error reading decoded mp3 PCM", err)
			reason = "error"
		}
		p.finish(reason)
		return nil, io.EOF
	}

	out := make([]byte, maxBytes)
	n, err := p.encoder.Encode(pcm, out)
	if err != nil {
		OnError("encoding error", err)
		p.finish("error")
		return nil, io.EOF
	}

	p.frames.Add(1)

	return out[:n], nil
}

func (p *mp3OpusProvider) Close() {
	p.finish("closed")
}

// finish releases the file once. reason is only logged for the call that
// wins, and the log is one line per stream, never per frame.
func (p *mp3OpusProvider) finish(reason string) {
	p.closeOnce.Do(func() {
		_ = p.file.Close()
		close(p.done)

		slog.Debug("audio stream finished", "frames", p.frames.Load(), "reason", reason)
	})
}

// PlayAudioFile plays filename over conn and blocks until playback ends or ctx
// is cancelled.
func PlayAudioFile(ctx context.Context, conn voice.Conn, filename string) {
	provider, done, err := newMp3OpusProvider(ctx, filename)
	if err != nil {
		OnError("failed to decode mp3", err)
		return
	}

	conn.SetOpusFrameProvider(provider)

	select {
	case <-done:
	case <-ctx.Done():
		provider.Close()
	}
}
