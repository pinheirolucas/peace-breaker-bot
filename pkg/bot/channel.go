package bot

import (
	"iter"
	"regexp"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

var channelMentionPattern = regexp.MustCompile(`^<#(\d+)>$`)

func parseChannelArg(arg string) (snowflake.ID, string) {
	if match := channelMentionPattern.FindStringSubmatch(strings.TrimSpace(arg)); match != nil {
		if id, err := snowflake.Parse(match[1]); err == nil && id != 0 {
			return id, ""
		}
	}

	return 0, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(arg), "#"))
}

func findVoiceChannel(channels iter.Seq[discord.GuildChannel], name string) (discord.GuildAudioChannel, bool) {
	if name == "" {
		return nil, false
	}

	var found discord.GuildAudioChannel
	for channel := range channels {
		audio, ok := channel.(discord.GuildAudioChannel)
		if !ok || !strings.EqualFold(audio.Name(), name) {
			continue
		}

		if found == nil || audio.Position() < found.Position() ||
			(audio.Position() == found.Position() && audio.ID() < found.ID()) {
			found = audio
		}
	}

	return found, found != nil
}
