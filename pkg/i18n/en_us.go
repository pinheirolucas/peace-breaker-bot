package i18n

var enUS = map[string]string{
	"invalid_body":            "Invalid request",
	"invalid_url":             "That URL isn't valid",
	"instant_not_found":       "That instant couldn't be found",
	"unsuported_audio_format": "That instant isn't an audio format we can play",
	"invalid_region":          "That region code isn't valid",
	"provider_not_found":      "That provider isn't recognized",
	"http_request":            "Couldn't reach that provider's site",
	"bad_http_status":         "That provider's site answered with an error",
	"name_link_not_matched":   "That provider's results couldn't be read — its page layout may have changed",
	"unknown_error":           "Something went wrong. Try again in a moment",
	"bot_not_connected":       "The bot isn't in a voice channel yet — make it join one first.",
	"channel_not_found":       "That voice channel couldn't be found",
	"owner_not_in_voice":      "The bot's owner isn't in a voice channel the bot can see",
	"owner_unknown":           "The bot hasn't seen its owner yet. Send it any message on Discord, then try again",
	"not_voice_channel":       "That channel isn't a voice channel",
	"voice_join_failed":       "The bot couldn't connect to that voice channel",
	"bot_not_ready":           "The bot is still starting up. Try again in a moment",

	"bot.ping.help":   "Checks whether the bot is online",
	"bot.join.help":   "Calls the bot into the voice channel you're in, or the one you name (!join #channel)",
	"bot.leave.help":  "Disconnects the bot from its current voice channel",
	"bot.help.help":   "Shows how to use the bot",
	"bot.help.header": "Available commands:",
}
