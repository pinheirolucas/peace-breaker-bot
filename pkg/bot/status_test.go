package bot

import (
	"context"
	"sync"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	botgateway "github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

// fakeVoiceConn is a minimal voice.Conn stand-in — only GuildID/ChannelID
// are ever read by Status(), everything else exists purely to satisfy the
// interface.
type fakeVoiceConn struct {
	guildID   snowflake.ID
	channelID snowflake.ID
}

func (f *fakeVoiceConn) Gateway() voice.Gateway { return nil }
func (f *fakeVoiceConn) UDP() voice.UDPConn     { return nil }
func (f *fakeVoiceConn) ChannelID() *snowflake.ID {
	id := f.channelID
	return &id
}
func (f *fakeVoiceConn) GuildID() snowflake.ID                                            { return f.guildID }
func (f *fakeVoiceConn) UserIDBySSRC(ssrc uint32) snowflake.ID                            { return 0 }
func (f *fakeVoiceConn) SetSpeaking(ctx context.Context, flags voice.SpeakingFlags) error { return nil }
func (f *fakeVoiceConn) SetOpusFrameProvider(handler voice.OpusFrameProvider)             {}
func (f *fakeVoiceConn) SetOpusFrameReceiver(handler voice.OpusFrameReceiver)             {}
func (f *fakeVoiceConn) SetEventHandlerFunc(eventHandlerFunc voice.EventHandlerFunc)      {}
func (f *fakeVoiceConn) Open(ctx context.Context, channelID snowflake.ID, selfMute, selfDeaf bool) error {
	return nil
}
func (f *fakeVoiceConn) Close(ctx context.Context)                                        {}
func (f *fakeVoiceConn) HandleVoiceStateUpdate(update botgateway.EventVoiceStateUpdate)   {}
func (f *fakeVoiceConn) HandleVoiceServerUpdate(update botgateway.EventVoiceServerUpdate) {}

// TestStatusIsRaceFreeUnderConcurrentSetVoiceConn is the regression guard
// for b.vc: before vcMu existed, Status() (via voiceConn()) read the field
// with no synchronization against join.go/leave.go's writes, which
// `go test -race` flags as a data race. It doesn't need a real Discord
// connection — just concurrent readers and writers of the guarded field.
func TestStatusIsRaceFreeUnderConcurrentSetVoiceConn(t *testing.T) {
	b := &Bot{}
	b.setClient(&bot.Client{Caches: cache.New()})
	conn := &fakeVoiceConn{guildID: 1, channelID: 2}

	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				b.setVoiceConn(conn)
			} else {
				b.setVoiceConn(nil)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = b.Status()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = b.voiceConn()
		}
	}()

	wg.Wait()
}

// TestStatusReportsDisconnectedByDefault guards the documented zero value:
// a Bot that never joined a voice channel reports Connected: false with no
// other fields set.
func TestStatusReportsDisconnectedByDefault(t *testing.T) {
	b := &Bot{}

	status := b.Status()
	if status.Connected {
		t.Fatalf("Connected = true, want false")
	}
	if status != (VoiceStatus{}) {
		t.Fatalf("Status() = %+v, want the zero value", status)
	}
}

// TestStatusReportsConnectedWithoutCache guards the "cache miss just leaves
// the name empty" behavior: with a client whose guild/channel caches hold
// nothing for this connection's IDs, Status still reports
// Connected/GuildID/ChannelID from the connection itself, just with empty
// names.
func TestStatusReportsConnectedWithoutCache(t *testing.T) {
	b := &Bot{}
	b.setClient(&bot.Client{Caches: cache.New()})
	conn := &fakeVoiceConn{guildID: 42, channelID: 99}
	b.setVoiceConn(conn)

	status := b.Status()
	if !status.Connected {
		t.Fatalf("Connected = false, want true")
	}
	if status.GuildID != 42 {
		t.Errorf("GuildID = %v, want 42", status.GuildID)
	}
	if status.ChannelID != 99 {
		t.Errorf("ChannelID = %v, want 99", status.ChannelID)
	}
	if status.GuildName != "" || status.ChannelName != "" {
		t.Errorf("GuildName/ChannelName = %q/%q, want empty with no cache", status.GuildName, status.ChannelName)
	}
}
