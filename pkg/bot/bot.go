package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	davesession "github.com/thomas-vilte/dave-go/session"
	"golang.org/x/text/language"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/command"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/logging"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/opusaudio"
)

type Bot struct {
	token string
	owner string

	locale  string
	version string

	vcMu   sync.RWMutex
	vc     voice.Conn
	client *bot.Client

	disp   *command.DiscordDispatcher
	player *instant.Player
}

func (b *Bot) voiceConn() voice.Conn {
	b.vcMu.RLock()
	defer b.vcMu.RUnlock()

	return b.vc
}

func (b *Bot) setVoiceConn(conn voice.Conn) {
	b.vcMu.Lock()
	defer b.vcMu.Unlock()

	b.vc = conn
}

func (b *Bot) setClient(client *bot.Client) {
	b.vcMu.Lock()
	defer b.vcMu.Unlock()

	b.client = client
}

func (b *Bot) discordClient() *bot.Client {
	b.vcMu.RLock()
	defer b.vcMu.RUnlock()

	return b.client
}

// VoiceStatus is the bot's current voice connection.
type VoiceStatus struct {
	Connected   bool
	GuildID     snowflake.ID
	GuildName   string
	ChannelID   snowflake.ID
	ChannelName string
}

// Status returns the bot's current voice connection.
func (b *Bot) Status() VoiceStatus {
	conn := b.voiceConn()
	if conn == nil {
		return VoiceStatus{}
	}

	client := b.discordClient()

	status := VoiceStatus{Connected: true, GuildID: conn.GuildID()}
	if guild, ok := client.Caches.Guild(status.GuildID); ok {
		status.GuildName = guild.Name
	}
	if channelID := conn.ChannelID(); channelID != nil {
		status.ChannelID = *channelID
		if channel, ok := client.Caches.Channel(*channelID); ok {
			status.ChannelName = channel.Name()
		}
	}
	return status
}

func New(token string, player *instant.Player, options ...Option) (*Bot, error) {
	b := &Bot{
		token:  token,
		disp:   command.NewDiscordDispatcher(),
		player: player,
	}

	b.disp.Register("!ping", "bot.ping.help", b.ping)
	b.disp.Register("!join", "bot.join.help", b.join)
	b.disp.Register("!leave", "bot.leave.help", b.leave)
	b.disp.Register("!help", "bot.help.help", b.help)

	for _, option := range options {
		option(b)
	}

	return b, nil
}

func (b *Bot) Start() error {
	client, err := disgo.New(b.token,
		// At DEBUG disgo logs REST bodies and voice-gateway payloads, which
		// carry the voice token. Cap it at INFO whatever level the app runs at.
		bot.WithLogger(slog.New(logging.Floor(slog.Default().Handler(), slog.LevelInfo))),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildVoiceStates,
				gateway.IntentGuildMessages,
				gateway.IntentDirectMessages,
				gateway.IntentMessageContent,
			),
		),
		// !join needs Guilds, Channels and VoiceStates cached; none are on by default.
		bot.WithCacheConfigOpts(
			cache.WithCaches(cache.FlagGuilds, cache.FlagChannels, cache.FlagVoiceStates),
		),
		bot.WithEventListenerFunc(b.handleReady),
		bot.WithEventListenerFunc(b.handleMessages),
		// Async listeners: !join blocks on the voice handshake, which would
		// otherwise stall the gateway read loop and miss heartbeat ACKs.
		bot.WithEventManagerConfigOpts(bot.WithAsyncEventsEnabled()),
		// Without a DAVE session factory, voice defaults to an unencrypted
		// noop session, which Discord's gateway rejects with close code 4017.
		bot.WithVoiceManagerConfigOpts(
			voice.WithDaveSessionCreateFunc(davesession.CreateFunc()),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create a client: %w", err)
	}
	defer client.Close(context.Background())

	// Set before any goroutine that could call Status() starts.
	b.setClient(client)

	opusaudio.OnError = func(str string, err error) {
		slog.Debug(str, "err", err)
	}

	if err = client.OpenGateway(context.Background()); err != nil {
		return fmt.Errorf("failed to open websocket connection: %w", err)
	}

	defer func() {
		conn := b.voiceConn()
		if conn == nil {
			return
		}

		conn.Close(context.Background())
	}()

	go func() {
		// TODO: create a bot client to manage all this complexity
		for {
			pb, ok := b.player.Next()
			if !ok {
				return
			}

			conn := b.voiceConn()
			if conn == nil {
				pb.End()
				continue
			}

			slog.Info("playing instant", "path", pb.Path())
			opusaudio.PlayAudioFile(pb.Context(), conn, pb.Path())
			pb.End()
		}
	}()

	slog.Info("bot is now running", "version", b.version)

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	return errors.New("application is shutting down")
}

func (b *Bot) handleReady(e *events.Ready) {
	slog.Info("bot is ready")
}

func (b *Bot) handleMessages(e *events.MessageCreate) {
	if b.owner != "" && b.owner != e.Message.Author.Username {
		return
	}

	if e.Message.Author.ID == e.Client().ID() {
		return
	}

	if e.GuildID == nil {
		b.handleInviteDM(e)
		return
	}

	b.disp.Dispatch(e)
}

func (b *Bot) localeFor(e *events.MessageCreate) language.Tag {
	if b.locale != "" {
		return i18n.Match(b.locale)
	}

	if e.GuildID == nil {
		return i18n.Supported[0]
	}

	guild, ok := e.Client().Caches.Guild(*e.GuildID)
	if !ok {
		return i18n.Supported[0]
	}

	return i18n.Match(guild.PreferredLocale)
}
