package bot

import (
	"context"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/command"
)

// leave disconnects from the current voice channel, if any, mirroring !join's
// silent behavior (no chat reply either way). It stops any in-flight
// playback first so the play loop in Start() doesn't keep pulling frames
// into a connection that's about to close.
func (b *Bot) leave(ctx *command.DiscordContext) {
	conn := b.voiceConn()
	if conn == nil {
		return
	}

	b.player.Stop()
	conn.Close(context.Background())
	b.setVoiceConn(nil)
}
