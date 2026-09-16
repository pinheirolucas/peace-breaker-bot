package bot

import (
	"context"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/command"
)

func (b *Bot) leave(ctx *command.DiscordContext) {
	conn := b.voiceConn()
	if conn == nil {
		return
	}

	b.player.Stop()
	conn.Close(context.Background())
	b.setVoiceConn(nil)
}
