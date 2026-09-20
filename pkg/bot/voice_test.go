package bot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
)

type recordingConn struct {
	voice.Conn

	guildID   snowflake.ID
	channelID snowflake.ID
	opened    bool
	closed    bool
	openErr   error
	block     bool
}

func (c *recordingConn) GuildID() snowflake.ID { return c.guildID }

func (c *recordingConn) ChannelID() *snowflake.ID {
	if !c.opened {
		return nil
	}

	id := c.channelID
	return &id
}

func (c *recordingConn) Open(ctx context.Context, channelID snowflake.ID, selfMute, selfDeaf bool) error {
	if c.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if c.openErr != nil {
		return c.openErr
	}

	c.channelID = channelID
	c.opened = true
	return nil
}

func (c *recordingConn) Close(ctx context.Context) { c.closed = true }

type recordingManager struct {
	voice.Manager

	mu      sync.Mutex
	conns   []*recordingConn
	openErr error
	block   bool
}

func (m *recordingManager) CreateConn(guildID snowflake.ID) voice.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn := &recordingConn{guildID: guildID, openErr: m.openErr, block: m.block}
	m.conns = append(m.conns, conn)
	return conn
}

func (m *recordingManager) created() []*recordingConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]*recordingConn(nil), m.conns...)
}

func (m *recordingManager) openCount() int {
	open := 0
	for _, conn := range m.created() {
		if conn.opened && !conn.closed {
			open++
		}
	}
	return open
}

type fixture struct {
	bot     *Bot
	manager *recordingManager
	caches  interface {
		AddVoiceState(discord.VoiceState)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	caches := newTestCaches()
	caches.SetSelfUser(discord.OAuth2User{User: discord.User{ID: 345, Username: "peace-breaker"}})
	caches.AddGuild(discord.Guild{ID: 1})
	caches.AddGuild(discord.Guild{ID: 2})
	caches.AddChannel(voiceChannel(t, 10, 1, 0, "General"))
	caches.AddChannel(voiceChannel(t, 11, 1, 1, "instants"))
	caches.AddChannel(stageChannel(t, 20, 2, 0, "Stage"))
	caches.AddChannel(textChannel(t, 12, 1, 2, "chat"))

	manager := &recordingManager{}
	b := &Bot{owner: "owner", player: instant.NewPlayer()}
	b.setClient(&bot.Client{Caches: caches, VoiceManager: manager})

	return &fixture{bot: b, manager: manager, caches: caches}
}

func (f *fixture) status(t *testing.T) VoiceStatus {
	t.Helper()
	return f.bot.Status()
}

func TestVoiceCallsReportNotReadyBeforeTheGateway(t *testing.T) {
	for name, b := range map[string]*Bot{
		"no client": {},
		"no self user": func() *Bot {
			b := &Bot{}
			b.setClient(&bot.Client{Caches: newTestCaches()})
			return b
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := b.JoinOwner(context.Background()); !errors.Is(err, ErrNotReady) {
				t.Errorf("JoinOwner err = %v, want ErrNotReady", err)
			}
			if err := b.JoinChannel(context.Background(), 10); !errors.Is(err, ErrNotReady) {
				t.Errorf("JoinChannel err = %v, want ErrNotReady", err)
			}
			if err := b.Leave(); !errors.Is(err, ErrNotReady) {
				t.Errorf("Leave err = %v, want ErrNotReady", err)
			}
		})
	}
}

func TestJoinChannelRejectsUnknownAndNonVoiceChannels(t *testing.T) {
	f := newFixture(t)

	if err := f.bot.JoinChannel(context.Background(), 999); !errors.Is(err, ErrChannelNotFound) {
		t.Errorf("unknown channel err = %v, want ErrChannelNotFound", err)
	}
	if err := f.bot.JoinChannel(context.Background(), 12); !errors.Is(err, ErrNotVoiceChannel) {
		t.Errorf("text channel err = %v, want ErrNotVoiceChannel", err)
	}
	if got := len(f.manager.created()); got != 0 {
		t.Errorf("%d voice connections created for rejected joins, want 0", got)
	}
}

func TestJoinChannelConnectsToTheChannel(t *testing.T) {
	f := newFixture(t)

	if err := f.bot.JoinChannel(context.Background(), 11); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}

	status := f.status(t)
	if !status.Connected || status.GuildID != 1 || status.ChannelID != 11 || status.ChannelName != "instants" {
		t.Errorf("status = %+v, want connected to instants (11) in guild 1", status)
	}
}

func TestJoinChannelIsANoOpForTheChannelTheBotIsAlreadyIn(t *testing.T) {
	f := newFixture(t)

	if err := f.bot.JoinChannel(context.Background(), 11); err != nil {
		t.Fatalf("first join: %v", err)
	}
	if err := f.bot.JoinChannel(context.Background(), 11); err != nil {
		t.Fatalf("second join: %v", err)
	}

	conns := f.manager.created()
	if len(conns) != 1 {
		t.Fatalf("%d connections created, want 1", len(conns))
	}
	if conns[0].closed {
		t.Error("the connection was closed by a join to its own channel")
	}
}

func TestJoinChannelMovesTheBotWhenItIsElsewhere(t *testing.T) {
	f := newFixture(t)

	for _, channelID := range []snowflake.ID{10, 11, 20} {
		if err := f.bot.JoinChannel(context.Background(), channelID); err != nil {
			t.Fatalf("join %d: %v", channelID, err)
		}
	}

	conns := f.manager.created()
	if len(conns) != 3 {
		t.Fatalf("%d connections created, want 3", len(conns))
	}
	if !conns[0].closed || !conns[1].closed || conns[2].closed {
		t.Errorf("closed = %v/%v/%v, want true/true/false", conns[0].closed, conns[1].closed, conns[2].closed)
	}
	if status := f.status(t); status.ChannelID != 20 || status.GuildID != 2 {
		t.Errorf("status = %+v, want the stage channel 20 in guild 2", status)
	}
}

func TestJoinChannelLeavesTheOldChannelWhenTheNewOneFails(t *testing.T) {
	f := newFixture(t)

	if err := f.bot.JoinChannel(context.Background(), 10); err != nil {
		t.Fatalf("first join: %v", err)
	}

	f.manager.openErr = errors.New("missing permission")
	err := f.bot.JoinChannel(context.Background(), 11)

	if !errors.Is(err, ErrJoinFailed) {
		t.Fatalf("err = %v, want ErrJoinFailed", err)
	}
	if status := f.status(t); status.Connected {
		t.Errorf("status = %+v, want disconnected", status)
	}
	for i, conn := range f.manager.created() {
		if !conn.closed {
			t.Errorf("connection %d left open", i)
		}
	}
}

func TestJoinChannelGivesUpWhenTheHandshakeNeverFinishes(t *testing.T) {
	f := newFixture(t)
	f.manager.block = true

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := f.bot.JoinChannel(ctx, 10); !errors.Is(err, ErrJoinFailed) {
		t.Fatalf("err = %v, want ErrJoinFailed", err)
	}
	if status := f.status(t); status.Connected {
		t.Errorf("status = %+v, want disconnected", status)
	}
}

func TestConcurrentJoinsLeaveExactlyOneConnectionOpen(t *testing.T) {
	f := newFixture(t)

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = f.bot.JoinChannel(context.Background(), []snowflake.ID{10, 11, 20}[i%3])
		}(i)
	}
	wg.Wait()

	if got := f.manager.openCount(); got != 1 {
		t.Errorf("%d connections open, want exactly 1", got)
	}
	if !f.status(t).Connected {
		t.Error("bot is disconnected after the joins")
	}
}

func TestLeaveDisconnectsAndIsIdempotent(t *testing.T) {
	f := newFixture(t)

	if err := f.bot.Leave(); err != nil {
		t.Fatalf("Leave while disconnected: %v", err)
	}

	if err := f.bot.JoinChannel(context.Background(), 10); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := f.bot.Leave(); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	if f.status(t).Connected {
		t.Error("bot is still connected after Leave")
	}
	if conns := f.manager.created(); len(conns) != 1 || !conns[0].closed {
		t.Errorf("connections = %+v, want the one connection closed", conns)
	}
}

func TestJoinOwnerNeedsTheOwnerToHaveBeenSeen(t *testing.T) {
	f := newFixture(t)

	if err := f.bot.JoinOwner(context.Background()); !errors.Is(err, ErrOwnerUnknown) {
		t.Fatalf("err = %v, want ErrOwnerUnknown", err)
	}
}

func TestJoinOwnerNeedsTheOwnerToBeInVoice(t *testing.T) {
	f := newFixture(t)
	f.bot.rememberOwner(77)

	if err := f.bot.JoinOwner(context.Background()); !errors.Is(err, ErrOwnerNotInVoice) {
		t.Fatalf("err = %v, want ErrOwnerNotInVoice", err)
	}

	f.caches.AddVoiceState(discord.VoiceState{GuildID: 1, UserID: 77})
	if err := f.bot.JoinOwner(context.Background()); !errors.Is(err, ErrOwnerNotInVoice) {
		t.Fatalf("owner with no channel: err = %v, want ErrOwnerNotInVoice", err)
	}
}

func TestJoinOwnerFollowsTheOwnerAcrossServers(t *testing.T) {
	f := newFixture(t)
	f.bot.rememberOwner(77)

	stage := snowflake.ID(20)
	f.caches.AddVoiceState(discord.VoiceState{GuildID: 2, UserID: 77, ChannelID: &stage})
	if err := f.bot.JoinOwner(context.Background()); err != nil {
		t.Fatalf("JoinOwner: %v", err)
	}
	if status := f.status(t); status.GuildID != 2 || status.ChannelID != 20 {
		t.Fatalf("status = %+v, want the stage channel 20 in guild 2", status)
	}

	general := snowflake.ID(10)
	f.caches.AddVoiceState(discord.VoiceState{GuildID: 2, UserID: 77})
	f.caches.AddVoiceState(discord.VoiceState{GuildID: 1, UserID: 77, ChannelID: &general})
	if err := f.bot.JoinOwner(context.Background()); err != nil {
		t.Fatalf("JoinOwner after the owner moved: %v", err)
	}
	if status := f.status(t); status.GuildID != 1 || status.ChannelID != 10 {
		t.Errorf("status = %+v, want General (10) in guild 1", status)
	}
}

func TestRememberOwnerIgnoresEveryoneWhenNoOwnerIsConfigured(t *testing.T) {
	b := &Bot{}
	b.rememberOwner(77)

	if got := b.ownerID.Load(); got != 0 {
		t.Errorf("ownerID = %d, want 0 with no configured owner", got)
	}
}
