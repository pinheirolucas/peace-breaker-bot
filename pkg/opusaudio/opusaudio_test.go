package opusaudio

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/pion/opus"
)

// TestEncodeDecodeRoundTrip encodes a synthetic tone with the same encoder
// settings newMp3OpusProvider uses and decodes it back with pion/opus's
// own Decoder, as a cheap guard against an encoder swap silently breaking
// the bitstream. It needs no mp3 fixture or a live voice connection.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	encoder, err := opus.NewEncoder(
		opus.WithChannels(channels),
		opus.WithApplication(opus.ApplicationAudio),
		opus.WithBitrate(96000),
		opus.WithComplexity(10),
		opus.WithVBR(true),
	)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}

	pcm := make([]byte, frameSize*channels*2)
	const freqHz = 440.0
	for i := 0; i < frameSize; i++ {
		sample := int16(math.Sin(2*math.Pi*freqHz*float64(i)/float64(frameRate)) * 0.5 * math.MaxInt16)
		for ch := 0; ch < channels; ch++ {
			offset := (i*channels + ch) * 2
			pcm[offset] = byte(sample)
			pcm[offset+1] = byte(sample >> 8)
		}
	}

	encoded := make([]byte, maxBytes)
	n, err := encoder.Encode(pcm, encoded)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if n == 0 {
		t.Fatal("Encode produced zero bytes")
	}
	encoded = encoded[:n]

	decoder := opus.NewDecoder()
	decoded := make([]byte, frameSize*channels*2)
	if _, _, err := decoder.Decode(encoded, decoded); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	nonZero := false
	for _, b := range decoded {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatal("decoded PCM is all zero")
	}
}

// TestMp3OpusProviderDecodesFixture decodes a real mp3 fixture end to end
// through go-mp3, the resampler, and the Opus encoder, as a guard against
// the decode pipeline silently producing empty or corrupt frames.
func TestMp3OpusProviderDecodesFixture(t *testing.T) {
	provider, done, err := newMp3OpusProvider(context.Background(), "testdata/valid.mp3")
	if err != nil {
		t.Fatalf("newMp3OpusProvider: %v", err)
	}

	frames := 0
	nonEmpty := false
	for {
		frame, err := provider.ProvideOpusFrame()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("ProvideOpusFrame: %v", err)
			}
			break
		}
		frames++
		if len(frame) > 0 {
			nonEmpty = true
		}
	}

	if frames == 0 {
		t.Fatal("decoded zero Opus frames from the fixture")
	}
	if !nonEmpty {
		t.Fatal("every decoded Opus frame was empty")
	}

	select {
	case <-done:
	default:
		t.Fatal("done channel should be closed once playback ends naturally")
	}
}

// TestMp3OpusProviderStopsEarly checks that cancelling the context before any
// frame is pulled ends playback immediately, without decoding the file.
func TestMp3OpusProviderStopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider, done, err := newMp3OpusProvider(ctx, "testdata/valid.mp3")
	if err != nil {
		t.Fatalf("newMp3OpusProvider: %v", err)
	}

	frame, err := provider.ProvideOpusFrame()
	if err != io.EOF {
		t.Fatalf("ProvideOpusFrame error = %v, want io.EOF", err)
	}
	if frame != nil {
		t.Fatalf("ProvideOpusFrame frame = %v, want nil", frame)
	}

	select {
	case <-done:
	default:
		t.Fatal("done channel should be closed once the context is cancelled")
	}
}

func TestMp3OpusProviderLogsOncePerStreamNotPerFrame(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	provider, done, err := newMp3OpusProvider(context.Background(), "testdata/valid.mp3")
	if err != nil {
		t.Fatalf("newMp3OpusProvider: %v", err)
	}

	for {
		if _, err := provider.ProvideOpusFrame(); err != nil {
			break
		}
	}
	<-done

	logs := buf.String()
	if strings.Count(logs, "\n") != 2 {
		t.Errorf("want 2 lines (decoder opened, stream finished), got %q", logs)
	}
	if !strings.Contains(logs, "reason=eof") || !strings.Contains(logs, "frames=") || strings.Contains(logs, "frames=0") {
		t.Errorf("log %q should report a nonzero frame count and reason=eof", logs)
	}
}
