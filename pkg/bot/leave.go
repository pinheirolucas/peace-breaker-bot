package bot

import (
	"context"
	"log/slog"

	"github.com/disgoorg/snowflake/v2"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/command"
)

func (b *Bot) leave(ctx *command.DiscordContext) {
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
