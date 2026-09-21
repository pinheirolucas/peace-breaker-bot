package bot

import "github.com/pinheirolucas/peace-breaker-bot/pkg/command"

func (b *Bot) leave(ctx *command.DiscordContext) {
	_ = b.Leave()
}
