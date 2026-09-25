package bot

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
)

var helpLine = regexp.MustCompile("(?m)^`(![a-z]+)`: (.*)$")

func TestEveryCommandHasHelpTextAndAReadmeRow(t *testing.T) {
	b, err := New("token", instant.NewPlayer())
	if err != nil {
		t.Fatal(err)
	}

	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}

	for _, lang := range i18n.Supported {
		lines := helpLine.FindAllStringSubmatch(b.disp.GetHelp(lang), -1)
		if len(lines) == 0 {
			t.Fatalf("GetHelp(%s) lists no commands", lang)
		}

		for _, line := range lines {
			if strings.HasPrefix(line[2], "bot.") {
				t.Errorf("%s has no %s help text (got key %q)", line[1], lang, line[2])
			}
			if !strings.Contains(string(readme), "| `"+line[1]) {
				t.Errorf("README usage table has no row for %s", line[1])
			}
		}
	}
}
