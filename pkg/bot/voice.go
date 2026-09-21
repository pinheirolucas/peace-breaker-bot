package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

const joinTimeout = 10 * time.Second

var (
	// ErrNotReady means the gateway hasn't sent READY since startup.
	ErrNotReady = errors.New("bot is not ready")
	// ErrChannelNotFound means no cached channel has the requested ID.
	ErrChannelNotFound = errors.New("channel not found")
	// ErrNotVoiceChannel means the channel exists but is not a voice or stage channel.
	ErrNotVoiceChannel = errors.New("not a voice channel")
	// ErrOwnerUnknown means the bot hasn't seen a message from its owner yet.
	ErrOwnerUnknown = errors.New("owner not seen yet")
	// ErrOwnerNotInVoice means the owner is not in a voice channel the bot can see.
	ErrOwnerNotInVoice = errors.New("owner is not in a voice channel")
	// ErrJoinFailed means Discord refused the voice connection or the handshake timed out.
	ErrJoinFailed = errors.New("failed to join voice channel")
)

// JoinOwner moves the bot into the voice channel its owner is in.
func (b *Bot) JoinOwner(ctx context.Context) error {
	client, err := b.readyClient()
	if err != nil {
		return err
	}

	channelID, err := b.ownerChannel(client)
	if err != nil {
		return err
	}

	return b.connect(ctx, client, channelID)
}

// JoinChannel moves the bot into the given voice channel.
func (b *Bot) JoinChannel(ctx context.Context, channelID snowflake.ID) error {
	client, err := b.readyClient()
	if err != nil {
		return err
	}

	return b.connect(ctx, client, channelID)
}

// Leave stops playback and disconnects from voice. It succeeds when the bot is not connected.
func (b *Bot) Leave() error {
	if _, err := b.readyClient(); err != nil {
		return err
	}

	b.joinMu.Lock()
	defer b.joinMu.Unlock()

	b.leaveLocked()
	return nil
}

func (b *Bot) readyClient() (*bot.Client, error) {
	client := b.discordClient()
	if client == nil {
		return nil, ErrNotReady
	}
	if _, ok := client.Caches.SelfUser(); !ok {
		return nil, ErrNotReady
	}

	return client, nil
}

func (b *Bot) rememberOwner(id snowflake.ID) {
	if b.owner == "" {
		return
	}

	b.ownerID.Store(uint64(id))
}

func (b *Bot) ownerChannel(client *bot.Client) (snowflake.ID, error) {
	ownerID := snowflake.ID(b.ownerID.Load())
	if ownerID == 0 {
		return 0, ErrOwnerUnknown
	}

	for guild := range client.Caches.Guilds() {
		state, ok := client.Caches.VoiceState(guild.ID, ownerID)
		if ok && state.ChannelID != nil {
			return *state.ChannelID, nil
		}
	}

	return 0, ErrOwnerNotInVoice
}

func (b *Bot) connect(ctx context.Context, client *bot.Client, channelID snowflake.ID) error {
	channel, ok := client.Caches.Channel(channelID)
	if !ok {
		return ErrChannelNotFound
	}

	audio, ok := channel.(discord.GuildAudioChannel)
	if !ok {
		return ErrNotVoiceChannel
	}

	b.joinMu.Lock()
	defer b.joinMu.Unlock()

	if current := b.voiceConn(); current != nil {
		if id := current.ChannelID(); id != nil && *id == channelID {
			slog.Debug("already in the requested channel", "channelId", channelID)
			return nil
		}

		slog.Debug("leaving current channel to join another", "requestedChannelId", channelID)
		b.leaveLocked()
	}

	ctx, cancel := context.WithTimeout(ctx, joinTimeout)
	defer cancel()

	start := time.Now()
	conn := client.VoiceManager.CreateConn(audio.GuildID())
	if err := conn.Open(ctx, channelID, false, true); err != nil {
		conn.Close(context.Background())
		slog.Error("failed to join voice channel",
			"guildId", audio.GuildID(),
			"channelId", channelID,
			"channelName", channel.Name(),
			"err", err,
		)
		return fmt.Errorf("%w: %v", ErrJoinFailed, err)
	}
	b.setVoiceConn(conn)

	slog.Debug("voice connection opened", "guildId", audio.GuildID(), "channelId", channelID, "durationMs", time.Since(start).Milliseconds())
	slog.Info("joined voice channel", "guildId", audio.GuildID(), "channelId", channelID, "channelName", channel.Name())
	return nil
}

func (b *Bot) leaveLocked() {
	conn := b.voiceConn()
	if conn == nil {
		return
	}

	guildID := conn.GuildID()
	var channelID snowflake.ID
	if id := conn.ChannelID(); id != nil {
		channelID = *id
	}

	b.player.Stop()
	conn.Close(context.Background())
	b.setVoiceConn(nil)

	slog.Info("left voice channel", "guildId", guildID, "channelId", channelID)
}
