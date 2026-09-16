package i18n

// enUS is authored directly, not translated from ptBR — a couple of these
// read clearer in English than a literal rendering of the Portuguese would.
var enUS = map[string]string{
	"invalid_body":            "Invalid request",
	"invalid_url":             "That URL isn't valid",
	"instant_not_found":       "That instant couldn't be found",
	"unsuported_audio_format": "That instant isn't an audio format we can play",
	"invalid_region":          "That region code isn't valid",
	"http_request":            "Couldn't reach myinstants.com",
	"bad_http_status":         "myinstants.com answered with an error",
	"name_link_not_matched":   "myinstants.com's results couldn't be read — their page layout may have changed",
	"unknown_error":           "Something went wrong. Try again in a moment",
	"bot_not_connected":       "The bot isn't in a voice channel yet — send !join from the channel you want it in.",

	"bot.ping.help":   "Checks whether the bot is online",
	"bot.join.help":   "Calls the bot into the voice channel you're in",
	"bot.leave.help":  "Disconnects the bot from its current voice channel",
	"bot.help.help":   "Shows how to use the bot",
	"bot.help.header": "Available commands:",
}
