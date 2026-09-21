package bot

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
)

func voiceChannel(t *testing.T, id, guildID snowflake.ID, position int, name string) discord.GuildVoiceChannel {
	t.Helper()

	var channel discord.GuildVoiceChannel
	raw := fmt.Sprintf(`{"id":"%d","guild_id":"%d","type":2,"name":%q,"position":%d}`, id, guildID, name, position)
	if err := json.Unmarshal([]byte(raw), &channel); err != nil {
		t.Fatalf("building voice channel: %v", err)
	}
	return channel
}

func stageChannel(t *testing.T, id, guildID snowflake.ID, position int, name string) discord.GuildStageVoiceChannel {
	t.Helper()

	var channel discord.GuildStageVoiceChannel
	raw := fmt.Sprintf(`{"id":"%d","guild_id":"%d","type":13,"name":%q,"position":%d}`, id, guildID, name, position)
	if err := json.Unmarshal([]byte(raw), &channel); err != nil {
		t.Fatalf("building stage channel: %v", err)
	}
	return channel
}

func textChannel(t *testing.T, id, guildID snowflake.ID, position int, name string) discord.GuildTextChannel {
	t.Helper()

	var channel discord.GuildTextChannel
	raw := fmt.Sprintf(`{"id":"%d","guild_id":"%d","type":0,"name":%q,"position":%d}`, id, guildID, name, position)
	if err := json.Unmarshal([]byte(raw), &channel); err != nil {
		t.Fatalf("building text channel: %v", err)
	}
	return channel
}

func newTestCaches() cache.Caches {
	return cache.New(cache.WithCaches(cache.FlagGuilds, cache.FlagChannels, cache.FlagVoiceStates))
}

func TestParseChannelArg(t *testing.T) {
	tests := []struct {
		arg      string
		wantID   snowflake.ID
		wantName string
	}{
		{"#instants", 0, "instants"},
		{"instants", 0, "instants"},
		{"#Team Room", 0, "Team Room"},
		{"  #instants  ", 0, "instants"},
		{"<#234567890123456789>", 234567890123456789, ""},
		{"<#0>", 0, "<#0>"},
		{"<#abc>", 0, "<#abc>"},
		{"<#99999999999999999999999>", 0, "<#99999999999999999999999>"},
		{"#", 0, ""},
		{"", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			id, name := parseChannelArg(tt.arg)
			if id != tt.wantID || name != tt.wantName {
				t.Errorf("parseChannelArg(%q) = (%d, %q), want (%d, %q)", tt.arg, id, name, tt.wantID, tt.wantName)
			}
		})
	}
}

func TestFindVoiceChannel(t *testing.T) {
	channels := []discord.GuildChannel{
		textChannel(t, 1, 100, 0, "instants"),
		voiceChannel(t, 2, 100, 5, "Instants"),
		voiceChannel(t, 3, 100, 2, "instants"),
		voiceChannel(t, 4, 100, 2, "INSTANTS"),
		stageChannel(t, 5, 100, 1, "Stage"),
		voiceChannel(t, 6, 100, 0, "General"),
	}

	tests := []struct {
		name   string
		want   snowflake.ID
		wantOk bool
	}{
		{"instants", 3, true},
		{"INSTANTS", 3, true},
		{"stage", 5, true},
		{"general", 6, true},
		{"insta", 0, false},
		{"", 0, false},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findVoiceChannel(slices.Values(channels), tt.name)
			if ok != tt.wantOk {
				t.Fatalf("findVoiceChannel(%q) ok = %v, want %v", tt.name, ok, tt.wantOk)
			}
			if ok && got.ID() != tt.want {
				t.Errorf("findVoiceChannel(%q) = %d, want %d", tt.name, got.ID(), tt.want)
			}
		})
	}
}

func TestFindVoiceChannelNeverMatchesATextChannel(t *testing.T) {
	channels := []discord.GuildChannel{textChannel(t, 1, 100, 0, "instants")}

	if got, ok := findVoiceChannel(slices.Values(channels), "instants"); ok {
		t.Errorf("findVoiceChannel matched text channel %d", got.ID())
	}
}

func TestChatTarget(t *testing.T) {
	caches := newTestCaches()
	caches.AddChannel(voiceChannel(t, 10, 1, 0, "instants"))
	caches.AddChannel(voiceChannel(t, 11, 1, 1, "General"))
	caches.AddChannel(textChannel(t, 12, 1, 2, "chat"))
	caches.AddChannel(voiceChannel(t, 20, 2, 0, "elsewhere"))
	general := snowflake.ID(11)
	caches.AddVoiceState(discord.VoiceState{GuildID: 1, UserID: 5, ChannelID: &general})
	caches.AddVoiceState(discord.VoiceState{GuildID: 1, UserID: 6})
	client := &bot.Client{Caches: caches}

	tests := []struct {
		name    string
		author  snowflake.ID
		arg     string
		want    snowflake.ID
		wantErr error
	}{
		{"no argument follows the sender", 5, "", 11, nil},
		{"blank argument follows the sender", 5, "   ", 11, nil},
		{"sender in no channel", 6, "", 0, ErrOwnerNotInVoice},
		{"sender with no voice state", 7, "", 0, ErrOwnerNotInVoice},
		{"channel by name", 7, "#instants", 10, nil},
		{"channel by name, other case", 7, "#INSTANTS", 10, nil},
		{"channel by name without hash", 7, "instants", 10, nil},
		{"name wins over where the sender is", 5, "#instants", 10, nil},
		{"channel mention", 7, "<#20>", 20, nil},
		{"unknown name", 7, "#nope", 0, ErrChannelNotFound},
		{"text channel name", 7, "#chat", 0, ErrChannelNotFound},
		{"name from another server", 7, "#elsewhere", 0, ErrChannelNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chatTarget(client, 1, tt.author, tt.arg)
			if err != tt.wantErr {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("channel = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEveryJoinErrorHasATranslatedMessage(t *testing.T) {
	errs := []error{
		ErrNotReady, ErrChannelNotFound, ErrNotVoiceChannel, ErrOwnerUnknown, ErrOwnerNotInVoice,
		fmt.Errorf("%w: timed out", ErrJoinFailed), fmt.Errorf("other"),
	}

	seen := map[string]bool{}
	for _, err := range errs {
		key := joinErrorKey(err)
		seen[key] = true

		for _, tag := range i18n.Supported {
			if got := i18n.Text(tag, key); got == key {
				t.Errorf("%v: key %q has no %v message", err, key, tag)
			}
		}
	}

	if len(seen) != len(errs) {
		t.Errorf("errors map to %d distinct keys, want %d", len(seen), len(errs))
	}
}
