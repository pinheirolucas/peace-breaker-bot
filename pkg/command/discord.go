package command

import (
	"fmt"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/events"
	"golang.org/x/text/language"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
)

type DiscordDispatcher struct {
	sync.Mutex
	handlers map[string]*discordHandlerInfo
	order    []string
}

type discordHandlerInfo struct {
	helpKey     string
	handlerFunc DiscordHandler
}

func NewDiscordDispatcher() *DiscordDispatcher {
	return &DiscordDispatcher{
		handlers: make(map[string]*discordHandlerInfo),
	}
}

type DiscordContext struct {
	Dispatcher *DiscordDispatcher
	Event      *events.MessageCreate
	Args       []string
}

type DiscordHandler func(ctx *DiscordContext)

// Register maps cmd to h, described by helpKey — a pkg/i18n key, not
// rendered text, since GetHelp resolves it into text per call.
func (d *DiscordDispatcher) Register(cmd string, helpKey string, h DiscordHandler) {
	d.Lock()
	if _, exists := d.handlers[cmd]; !exists {
		d.order = append(d.order, cmd)
	}
	d.handlers[cmd] = &discordHandlerInfo{
		helpKey:     helpKey,
		handlerFunc: h,
	}
	d.Unlock()
}

func (d *DiscordDispatcher) Dispatch(e *events.MessageCreate) {
	c := strings.Split(e.Message.Content, " ")

	cmd := c[0]
	args := c[1:]

	d.Lock()
	info, ok := d.handlers[cmd]
	d.Unlock()
	if !ok {
		return
	}

	info.handlerFunc(&DiscordContext{
		Dispatcher: d,
		Event:      e,
		Args:       args,
	})
}

func (d *DiscordDispatcher) GetHelp(lang language.Tag) string {
	help := i18n.Text(lang, "bot.help.header") + "\n"

	d.Lock()
	for _, cmd := range d.order {
		help += fmt.Sprintf("`%s`: %s\n", cmd, i18n.Text(lang, d.handlers[cmd].helpKey))
	}
	d.Unlock()

	return help
}
