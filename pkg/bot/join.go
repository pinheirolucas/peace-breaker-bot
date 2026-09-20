package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/command"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
)

func (b *Bot) join(ctx *command.DiscordContext) {
	e := ctx.Event
	client := e.Client()

	channelID, err := chatTarget(client, *e.GuildID, e.Message.Author.ID, strings.Join(ctx.Args, " "))
	if err == nil {
		err = b.connect(context.Background(), client, channelID)
	}
	if err != nil {
		slog.Debug("join command failed", "guildId", e.GuildID, "err", err)
		b.replyJoinError(e, err)
	}
}

func chatTarget(client *bot.Client, guildID, authorID snowflake.ID, arg string) (snowflake.ID, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		state, ok := client.Caches.VoiceState(guildID, authorID)
		if !ok || state.ChannelID == nil {
			return 0, ErrOwnerNotInVoice
		}

		return *state.ChannelID, nil
	}

	id, name := parseChannelArg(arg)
	if id != 0 {
		return id, nil
	}

	channel, ok := findVoiceChannel(client.Caches.ChannelsForGuild(guildID), name)
	if !ok {
		return 0, ErrChannelNotFound
	}

	return channel.ID(), nil
}

func (b *Bot) replyJoinError(e *events.MessageCreate, err error) {
	text := i18n.Text(b.localeFor(e), joinErrorKey(err))
	if _, err := e.Client().Rest.CreateMessage(e.ChannelID, discord.MessageCreate{Content: text}); err != nil {
		slog.Error("failed to send join error message", "err", err)
	}
}

func joinErrorKey(err error) string {
	switch {
	case errors.Is(err, ErrNotReady):
		return "bot_not_ready"
	case errors.Is(err, ErrChannelNotFound):
		return "channel_not_found"
	case errors.Is(err, ErrNotVoiceChannel):
		return "not_voice_channel"
	case errors.Is(err, ErrOwnerUnknown):
		return "owner_unknown"
	case errors.Is(err, ErrOwnerNotInVoice):
		return "owner_not_in_voice"
	case errors.Is(err, ErrJoinFailed):
		return "voice_join_failed"
	default:
		return "unknown_error"
	}
}
