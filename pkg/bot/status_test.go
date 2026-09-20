package bot

import (
	"context"
	"sync"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	botgateway "github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

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
			_, _ = b.Identity()
		}
	}()

	wg.Wait()
}

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

func TestIdentityIsUnavailableWithoutAClient(t *testing.T) {
	b := &Bot{}

	if _, ok := b.Identity(); ok {
		t.Fatalf("Identity() ok = true with no client, want false")
	}
}

func TestIdentityIsUnavailableBeforeReady(t *testing.T) {
	b := &Bot{}
	b.setClient(&bot.Client{Caches: cache.New()})

	if _, ok := b.Identity(); ok {
		t.Fatalf("Identity() ok = true with an empty self-user cache, want false")
	}
}

func TestIdentityReadsTheSelfUserCache(t *testing.T) {
	globalName := "Peace Breaker"
	caches := cache.New()
	caches.SetSelfUser(discord.OAuth2User{User: discord.User{
		ID:            345,
		Username:      "peace-breaker",
		GlobalName:    &globalName,
		Discriminator: "0",
	}})
	b := &Bot{}
	b.setClient(&bot.Client{Caches: caches})

	identity, ok := b.Identity()
	if !ok {
		t.Fatalf("Identity() ok = false, want true")
	}
	if identity.ID != 345 || identity.Username != "peace-breaker" || identity.DisplayName != "Peace Breaker" {
		t.Errorf("Identity() = %+v, want id 345, username peace-breaker, display name Peace Breaker", identity)
	}
}

func TestIdentityLinks(t *testing.T) {
	identity := Identity{ID: 345}

	if got, want := identity.ProfileURL(), "https://discord.com/users/345"; got != want {
		t.Errorf("ProfileURL() = %q, want %q", got, want)
	}
	if got, want := identity.InviteURL(), "https://discord.com/oauth2/authorize?client_id=345&scope=bot&permissions=3214336"; got != want {
		t.Errorf("InviteURL() = %q, want %q", got, want)
	}
	if got, want := ChannelURL(123, 456), "https://discord.com/channels/123/456"; got != want {
		t.Errorf("ChannelURL() = %q, want %q", got, want)
	}
}
