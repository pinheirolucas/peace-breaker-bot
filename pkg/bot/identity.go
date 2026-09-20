package bot

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// invitePermissions is what the bot needs in a server: reading and answering
// the owner's commands in text channels, and joining and speaking in voice.
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

// Identity returns the bot's own Discord account. It reports false until the
// gateway has sent READY, since that is what fills the client's self-user cache.
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

// ProfileURL links to the bot's profile in the Discord client.
func (i Identity) ProfileURL() string {
	return fmt.Sprintf("https://discord.com/users/%s", i.ID)
}

// InviteURL is the add-to-server link. A bot's application ID is its user ID.
func (i Identity) InviteURL() string {
	return fmt.Sprintf("https://discord.com/oauth2/authorize?client_id=%s&scope=bot&permissions=%d", i.ID, uint64(invitePermissions))
}

// ChannelURL links to a channel in the Discord client.
func ChannelURL(guildID, channelID snowflake.ID) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s", guildID, channelID)
}
