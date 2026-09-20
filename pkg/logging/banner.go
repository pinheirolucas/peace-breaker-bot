package logging

import (
	"fmt"
	"io"
	"strings"
)

// PrintBanner writes the startup banner to w, unless the logs are JSON.
func PrintBanner(w io.Writer, version string) {
	if CurrentFormat() == FormatJSON {
		return
	}

	fmt.Fprint(w, banner(ColorEnabled(), version))
}

func banner(colored bool, version string) string {
	p := func(style, text string) string {
		if !colored {
			return text
		}
		return style + text + ansiReset
	}

	tape := strings.Join([]string{
		p(ansiBlue, "═"), p(ansiRed, "═"), p(ansiYellow, "═"),
		p(ansiGreen, "═"), p(ansiCyan, "═"), p(ansiMagenta, "═"),
	}, "")

	edge := func(s string) string { return p(ansiBlue, s) }
	reel := func(s string) string { return p(ansiYellow, s) }

	rows := []string{
		edge("╭" + strings.Repeat("─", 18) + "╮"),
		edge("│") + " " + reel("╭───╮") + strings.Repeat(" ", 6) + reel("╭───╮") + " " + edge("│") + "   " + p(ansiBold, "Peace Breaker Bot"),
		edge("│") + " " + reel("│ @ │") + tape + reel("│ @ │") + " " + edge("│") + "   " + p(ansiDim, version),
		edge("│") + " " + reel("╰───╯") + strings.Repeat(" ", 6) + reel("╰───╯") + " " + edge("│") + "   " + p(ansiDim, "instants on demand"),
		edge("╰" + strings.Repeat("─", 18) + "╯"),
	}

	return strings.Join(rows, "\n") + "\n\n"
}
