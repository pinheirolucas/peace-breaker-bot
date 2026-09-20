package bot

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

const invitePermissions = discord.PermissionViewChannel |
	discord.PermissionSendMessages |
	discord.PermissionReadMessageHistory |
	discord.PermissionConnect |
	discord.PermissionSpeak

// Identity is the Discord account the bot is logged in as.
type Identity struct {
	ID          snowflake.ID
	Username    string
	DisplayName string
	AvatarURL   string
}

// Identity returns the bot's Discord account, or false before the gateway is ready.
func (b *Bot) Identity() (Identity, bool) {
	client := b.discordClient()
	if client == nil {
		return Identity{}, false
	}

	user, ok := client.Caches.SelfUser()
	if !ok {
		return Identity{}, false
	}

	return Identity{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.EffectiveName(),
		AvatarURL:   user.EffectiveAvatarURL(),
	}, true
}

// ProfileURL is the link to the bot's Discord profile.
func (i Identity) ProfileURL() string {
	return fmt.Sprintf("https://discord.com/users/%s", i.ID)
}

// InviteURL is the link to add the bot to a server.
func (i Identity) InviteURL() string {
	return fmt.Sprintf("https://discord.com/oauth2/authorize?client_id=%s&scope=bot&permissions=%d", i.ID, uint64(invitePermissions))
}

// ChannelURL is the link to a channel.
func ChannelURL(guildID, channelID snowflake.ID) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s", guildID, channelID)
}
