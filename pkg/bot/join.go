package bot

import (
	"context"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/command"
)

func (b *Bot) join(ctx *command.DiscordContext) {
	e := ctx.Event
	client := e.Client()

	guild, ok := client.Caches.Guild(*e.GuildID)
	if !ok {
		slog.Error("failed to fetch guild info", "GuildID", e.GuildID)
		return
	}

	voiceState, ok := client.Caches.VoiceState(guild.ID, e.Message.Author.ID)
	if !ok || voiceState.ChannelID == nil {
		slog.Info("voice channel not found", "AuthorUsername", e.Message.Author.Username)
		return
	}

	channel, ok := client.Caches.Channel(*voiceState.ChannelID)
	if !ok {
		slog.Error("failed to fetch voice channel info", "ChannelID", *voiceState.ChannelID)
		return
	}

	b.joinVoiceChannel(client, guild.ID, *voiceState.ChannelID, channel.Name())
}

func (b *Bot) joinVoiceChannel(client *bot.Client, guildID, channelID snowflake.ID, channelName string) {
	if b.voiceConn() != nil {
		return
	}

	conn := client.VoiceManager.CreateConn(guildID)
	if err := conn.Open(context.Background(), channelID, false, true); err != nil {
		slog.Error("failed to join voice channel",
			"GuildID", guildID,
			"ChannelID", channelID,
			"ChannelName", channelName,
			"err", err,
		)
		return
	}
	b.setVoiceConn(conn)
}
