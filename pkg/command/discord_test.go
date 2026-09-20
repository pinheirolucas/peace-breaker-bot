package command

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"golang.org/x/text/language"
)

func messageWith(content string) *events.MessageCreate {
	return &events.MessageCreate{
		GenericMessage: &events.GenericMessage{
			GenericEvent: events.NewGenericEvent(nil, 0, 0),
			Message:      discord.Message{Content: content},
		},
	}
}

func TestDispatchRoutesToRegisteredCommand(t *testing.T) {
	d := NewDiscordDispatcher()

	var got *DiscordContext
	d.Register("!ping", "responde pong", func(ctx *DiscordContext) { got = ctx })

	d.Dispatch(messageWith("!ping"))

	if got == nil {
		t.Fatal("handler was not called for !ping")
	}
	if got.Event.Message.Content != "!ping" {
		t.Errorf("Message.Content = %q, want %q", got.Event.Message.Content, "!ping")
	}
	if len(got.Args) != 0 {
		t.Errorf("Args = %v, want empty", got.Args)
	}
	if got.Dispatcher != d {
		t.Error("Dispatcher was not threaded through to the context")
	}
}

func TestDispatchSplitsArgsOnWhitespace(t *testing.T) {
	d := NewDiscordDispatcher()

	var args []string
	d.Register("!play", "toca", func(ctx *DiscordContext) { args = ctx.Args })

	d.Dispatch(messageWith("!play https://example.com/a.mp3 loud"))

	want := []string{"https://example.com/a.mp3", "loud"}
	if len(args) != len(want) {
		t.Fatalf("Args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("Args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestDispatchIgnoresUnknownAndNonCommandMessages(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"unregistered command", "!nope"},
		{"plain chat message", "hello there"},
		{"empty message", ""},
		{"command not at the start", "say !ping"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDiscordDispatcher()
			called := false
			d.Register("!ping", "responde pong", func(ctx *DiscordContext) { called = true })

			d.Dispatch(messageWith(tt.content))

			if called {
				t.Errorf("handler ran for %q, expected it to be ignored", tt.content)
			}
		})
	}
}

func TestRegisterOverwritesSameCommand(t *testing.T) {
	d := NewDiscordDispatcher()

	which := ""
	d.Register("!ping", "first", func(ctx *DiscordContext) { which = "first" })
	d.Register("!ping", "second", func(ctx *DiscordContext) { which = "second" })

	d.Dispatch(messageWith("!ping"))

	if which != "second" {
		t.Errorf("dispatched to %q handler, want the most recently registered one", which)
	}
	if strings.Contains(d.GetHelp(language.BrazilianPortuguese), "first") {
		t.Error("GetHelp still lists the replaced help text")
	}
}

// "responde pong" and "entra no canal" are not real pkg/i18n keys; GetHelp
// falls back to a key it doesn't recognize by printing the key itself, which
// is exactly what lets this test assert on them without touching the catalog.
func TestGetHelpListsEveryRegisteredCommand(t *testing.T) {
	d := NewDiscordDispatcher()
	d.Register("!ping", "responde pong", func(ctx *DiscordContext) {})
	d.Register("!join", "entra no canal", func(ctx *DiscordContext) {})

	help := d.GetHelp(language.BrazilianPortuguese)

	for _, want := range []string{"Comandos disponíveis:", "!ping", "responde pong", "!join", "entra no canal"} {
		if !strings.Contains(help, want) {
			t.Errorf("GetHelp() missing %q\ngot:\n%s", want, help)
		}
	}
}

func TestGetHelpListsCommandsInRegistrationOrder(t *testing.T) {
	d := NewDiscordDispatcher()
	d.Register("!ping", "responde pong", func(ctx *DiscordContext) {})
	d.Register("!join", "entra no canal", func(ctx *DiscordContext) {})
	d.Register("!leave", "sai do canal", func(ctx *DiscordContext) {})

	help := d.GetHelp(language.BrazilianPortuguese)

	pingIdx := strings.Index(help, "!ping")
	joinIdx := strings.Index(help, "!join")
	leaveIdx := strings.Index(help, "!leave")
	if pingIdx == -1 || joinIdx == -1 || leaveIdx == -1 {
		t.Fatalf("GetHelp() missing a registered command:\n%s", help)
	}
	if !(pingIdx < joinIdx && joinIdx < leaveIdx) {
		t.Errorf("GetHelp() order = ping@%d, join@%d, leave@%d; want registration order", pingIdx, joinIdx, leaveIdx)
	}
}

func TestGetHelpResolvesRealKeysPerLocale(t *testing.T) {
	d := NewDiscordDispatcher()
	d.Register("!ping", "bot.ping.help", func(ctx *DiscordContext) {})

	ptBR := d.GetHelp(language.BrazilianPortuguese)
	if !strings.Contains(ptBR, "Teste para verificar se o bot está online") {
		t.Errorf("GetHelp(pt-BR) missing the Portuguese help text:\n%s", ptBR)
	}

	enUS := d.GetHelp(language.AmericanEnglish)
	if !strings.Contains(enUS, "Checks whether the bot is online") {
		t.Errorf("GetHelp(en-US) missing the English help text:\n%s", enUS)
	}
	if !strings.Contains(enUS, "Available commands:") {
		t.Errorf("GetHelp(en-US) missing the English header:\n%s", enUS)
	}
}

func TestDispatchLogsTheCommandButNeverAnOrdinaryMessage(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	d := NewDiscordDispatcher()
	d.Register("!ping", "bot.ping.help", func(ctx *DiscordContext) {})

	d.Dispatch(messageWith("!ping now"))
	d.Dispatch(messageWith("my-private-chat with a friend"))

	logs := buf.String()
	if !strings.Contains(logs, "dispatching command") || !strings.Contains(logs, "command=!ping") || !strings.Contains(logs, "argCount=1") {
		t.Errorf("log %q does not trace the matched command", logs)
	}
	if !strings.Contains(logs, "message is not a command") {
		t.Errorf("log %q does not note the ignored message", logs)
	}
	if strings.Contains(logs, "my-private-chat") {
		t.Errorf("log %q leaks message content", logs)
	}
}
