---
name: add-bot-command
description: Use when adding or changing a Discord chat command (!name) in pkg/bot.
---

# Add a chat command

1. Design first. Publish the command's behaviour and replies as an artifact and wait for the user's
   sign-off.
2. Write the handler as a method in its own file, `pkg/bot/<name>.go` (see `leave.go`, `join.go`).
   Voice joins go through `Bot.connect`.
3. Register it in `New` in `pkg/bot/bot.go`: `b.disp.Register("!<name>", "bot.<name>.help", b.<name>)`.
4. Add `bot.<name>.help` and any reply texts to both `pkg/i18n/en_us.go` and `pt_br.go`. The trigger
   stays English.
5. Add a row to the README usage table.
6. Log only the command name, never the message content.
7. `make test`. `pkg/bot/docs_test.go` fails on missing help text or a missing README row.
