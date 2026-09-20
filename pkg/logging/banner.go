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

	fmt.Fprint(w, banner(ColorEnabled(), displayVersion(version)))
}

func displayVersion(version string) string {
	if version != "" && version[0] >= '0' && version[0] <= '9' {
		return "v" + version
	}

	return version
}

var glyphs = map[rune][3]string{
	'P': {"█▀█", "█▀▀", "▀  "},
	'E': {"█▀▀", "█▀▀", "▀▀▀"},
	'A': {"▄▀▄", "█▀█", "▀ ▀"},
	'C': {"█▀▀", "█  ", "▀▀▀"},
	'B': {"█▀▄", "█▀▄", "▀▀ "},
	'R': {"█▀▄", "█▀▄", "▀ ▀"},
	'K': {"█ █", "█▀▄", "▀ ▀"},
	'O': {"█▀█", "█ █", "▀▀▀"},
	'T': {"▀█▀", " █ ", " ▀ "},
}

var slotColors = []string{ansiBlue, ansiRed, ansiYellow, ansiGreen, ansiCyan, ansiMagenta}

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

	wordmark := wordmarkRows(p, "PEACE BREAKER BOT")

	rows := []string{
		edge("╭" + strings.Repeat("─", 14) + "╮"),
		edge("│") + " " + reel("╭─╮") + strings.Repeat(" ", 6) + reel("╭─╮") + " " + edge("│") + "  " + wordmark[0],
		edge("│") + " " + reel("│@│") + tape + reel("│@│") + " " + edge("│") + "  " + wordmark[1],
		edge("│") + " " + reel("╰─╯") + strings.Repeat(" ", 6) + reel("╰─╯") + " " + edge("│") + "  " + wordmark[2],
		edge("╰"+strings.Repeat("─", 14)+"╯") + "  " + p(ansiDim, version),
	}

	return strings.TrimRight(strings.Join(rows, "\n"), " ") + "\n\n"
}

func wordmarkRows(p func(style, text string) string, text string) [3]string {
	var rows [3]string

	letter := 0
	words := strings.Fields(text)
	for i, word := range words {
		for r := range rows {
			if i > 0 {
				rows[r] += "  "
			}
		}

		for j, ch := range word {
			for r := range rows {
				if j > 0 {
					rows[r] += " "
				}
				glyph := glyphs[ch][r]
				if i == len(words)-1 && j == len(word)-1 {
					glyph = strings.TrimRight(glyph, " ")
				}
				rows[r] += p(slotColors[letter%len(slotColors)], glyph)
			}
			letter++
		}
	}

	for r := range rows {
		rows[r] = strings.TrimRight(rows[r], " ")
	}

	return rows
}
