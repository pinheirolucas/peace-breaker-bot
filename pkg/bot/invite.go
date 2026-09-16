package bot

import (
	"log/slog"
	"regexp"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

var inviteLinkPattern = regexp.MustCompile(`discord(?:app)?\.(?:gg|com/invite)/([A-Za-z0-9-]+)`)

func parseInviteCode(content string) (string, bool) {
	match := inviteLinkPattern.FindStringSubmatch(content)
	if match == nil {
		return "", false
	}
	return match[1], true
}

func (b *Bot) handleInviteDM(e *events.MessageCreate) {
	code, ok := parseInviteCode(e.Message.Content)
	if !ok {
		return
	}

	client := e.Client()
	invite, err := client.Rest.GetInvite(code)
	if err != nil {
		slog.Error("failed to resolve invite", "Code", code, "err", err)
		return
	}

	if invite.Guild == nil || invite.Channel == nil {
		slog.Info("invite has no voice channel target", "Code", code)
		return
	}

	if invite.Channel.Type != discord.ChannelTypeGuildVoice && invite.Channel.Type != discord.ChannelTypeGuildStageVoice {
		slog.Info("invite does not target a voice channel",
			"Code", code,
			"ChannelType", invite.Channel.Type,
		)
		return
	}

	b.joinVoiceChannel(client, invite.Guild.ID, invite.Channel.ID, invite.Channel.Name)
}
