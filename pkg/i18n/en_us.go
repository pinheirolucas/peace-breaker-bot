package i18n

var enUS = map[string]string{
	"invalid_body":            "The bot couldn't understand the request",
	"invalid_url":             "That URL isn't valid",
	"instant_not_found":       "That instant couldn't be found",
	"unsuported_audio_format": "That instant isn't an audio format we can play",
	"invalid_region":          "That region code isn't valid",
	"provider_not_found":      "That site isn't supported",
	"http_request":            "Couldn't reach that site",
	"bad_http_status":         "That site answered with an error",
	"name_link_not_matched":   "Couldn't read that site's results. Its page may have changed.",
	"unknown_error":           "Something went wrong. Try again in a moment.",
	"bot_not_connected":       "The bot isn't in a voice channel yet. Join one and type !join, or use !join #channel.",
	"channel_not_found":       "Couldn't find that voice channel. Check the name and try again.",
	"owner_not_in_voice":      "The bot's owner isn't in a voice channel the bot can see",
	"author_not_in_voice":     "You're not in a voice channel. Join one, or name it: !join #channel",
	"owner_unknown":           "The bot hasn't seen its owner yet. Send it any message on Discord, then try again.",
	"not_voice_channel":       "That channel isn't a voice channel",
	"voice_join_failed":       "The bot couldn't connect to that voice channel",
	"bot_not_ready":           "The bot is still starting up. Try again in a moment.",

	"bot.ping.help":   "Checks whether the bot is online",
	"bot.join.help":   "Brings the bot into your voice channel, or the one you name (!join #channel)",
	"bot.leave.help":  "Disconnects the bot from its current voice channel",
	"bot.help.help":   "Lists the available commands",
	"bot.help.header": "Available commands:",
}
