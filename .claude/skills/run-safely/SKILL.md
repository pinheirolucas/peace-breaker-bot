---
name: run-safely
description: Use when you need to see the bot's real startup output or CLI behaviour without connecting to Discord.
---

# Run the bot without the live token

The repo root and `$HOME` hold the user's real `.peace-breaker-bot.yaml`. Running from either one
connects the live bot. `go run`, `make run` and `./bin/...` are denied in `.claude/settings.json`.

```sh
tmp=$(mktemp -d)
go build -o "$tmp/pbb" .
(cd "$tmp" && HOME="$tmp" ./pbb --log-level debug)
```

It exits at "bot token not provided". Pass other flags (`--log-format json`, `--log-color always`) to
check their output. Never pass `--bot-token`, and never copy the real config into `$tmp`.
