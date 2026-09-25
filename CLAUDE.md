# CLAUDE.md

Go backend for Peace Breaker Bot: a Discord bot that plays short mp3 "instants" from myinstants.com,
SoundboardGuy and Sound Buttons in a voice channel, plus an HTTP API that controls it. The client is
the Electron/React app in `../peace-breaker-bot-desktop`.

## Commands

- `make build | test | lint | cover`. `make test` is `go test -race -timeout 90s ./...`.
- `.tool-versions` is the one place for the Go and golangci-lint versions; CI reads it. The Go key
  must be `golang`, not `go` (`actions/setup-go` ignores `go`). `go.mod`'s `go` line is separate.
- `misspell` is off in `.golangci.yml` because it flags the Portuguese catalog.

## Workflow

- Never run the app from this directory (`make run`, `go run .`, `./bin/...`): the repo root holds the
  real `.peace-breaker-bot.yaml` with a live token. To see output, build to a scratch path and run it
  in an empty directory with `HOME` pointed there; it exits at "bot token not provided".
- New endpoint or command: publish the interface as an artifact and wait for sign-off before coding.
- Comments: only on exported identifiers, one direct line. None in tests, none explaining why
  something is logged. Reasoning goes in the PR body.
- Startup output stays minimal: the one static banner, then normal slog lines. No taglines or animation.
- Branch `type/kebab-slug`, one concern per PR; a big change is a stack ("Stacked on #N").
  Merge commits, no squash. Commit subject: imperative, sentence case, no prefix; body says why.
- PR body: `## Summary`, `## Test plan` (commands run), `## Not verified`,
  `Other repo: none | peace-breaker-bot-desktop#N`. Backend PRs land before the desktop PRs using them.
- Release: Actions › Cut Release (dry run first). Never tag by hand; the tag is the version.

## Update together

- Route, status code, label or request shape → `pkg/server/v1/openapi.yaml` (hand-written; no test
  checks it against the handlers), both i18n catalogs, tests, and the desktop's `src/service.ts`.
- Config key → `cmd/root.go` (flag, validation in `runRootCmd`), `.peace-breaker-bot.sample.yaml`,
  README table.
- Chat command → `Register` in `pkg/bot/bot.go` with a help key in both catalogs, README usage table.
- New error case → map it in `pkg/server/server.go` to a real status and stable `label`, never the
  generic 500.

## Contract with the desktop app

- The desktop never launches the bot. It finds it via mDNS `_myinstants._tcp` with TXT `path=/api`
  and `api=1`, instance name `<hostname>-<port>` (clients match on it; don't change it), and appends
  `/v1` itself. macOS `dns-sd` can't see the advertisement; debug with a client that reads the wire.
- Errors are a real 4xx/5xx with `{label, message}`. The UI branches on status and `label`; `message`
  is a fallback, so labels are stable API.
- `GET /bot/status` is polled; `POST /bot/play` answers `409 bot_not_connected` with no voice connection.
- `provider` defaults to `myinstants`; `region` is accepted and ignored by providers without regions.

## Architecture (non-obvious parts only)

- `pkg/bot` uses disgo because discordgo has no DAVE/E2EE voice. dave-go is wired via
  `voice.WithDaveSessionCreateFunc`, or Discord closes voice with 4017. `WithCacheConfigOpts` in
  `Bot.Start` must keep guilds, channels and voice states cached (`!join`, locale).
- "Invite to voice" arrives as a DM with an invite link, so DMs are matched for that and otherwise
  ignored silently. Every join (chat, DM, HTTP) goes through `Bot.connect`.
- Audio is pure Go and cgo-free: go-mp3 → linear resampler to 48kHz → pion/opus, pulled by disgo.
- `pkg/instant.Player`: one shared player, newest request wins, ordered by ticket; `Stop` also
  cancels downloads in flight. There is no queue; a replaced play returns `exitReason: "stop"`.
- `pkg/provider`: one `Provider` per site. `List` returns sentinel errors that `server.go` maps to
  statuses; `InferPages` guesses the page count. Only `myinstants` uses `Region`. Site quirks live in
  each provider's package doc.
- `/instants/{url}/content` enforces the providers' `AllowedContentHosts`; `/bot/play` doesn't.
- Known gaps: `Bot.vc` isn't cleared on an external disconnect; following the owner answers
  `owner_unknown` after a restart until the owner speaks.

## Provider sites

- Every request goes through `pkg/httpclient` (UA `peace-breaker-bot/1.0`). Cloudflare 403s Go's
  default UA on myinstants.com, so never use `http.DefaultClient`. curl 403s regardless, so don't
  debug with it. If a site adds a bot wall, drop it; no CAPTCHA, proxies or headless browsers.
- `testdata` fixtures are trimmed live captures made with the app UA. Re-capture; never hand-edit.

## Logging

- `log/slog` package-level calls only; never derive a logger at package scope.
- disgo is capped at INFO (`logging.Floor`): its DEBUG output carries the voice token.
- Never log the token, message content (only the command name), or HTTP headers and bodies.
  Nothing per audio frame. Keys are camelCase; durations are integer ms.

## i18n

- Catalogs: `pkg/i18n/en_us.go`, `pt_br.go`. API errors are English unless `Accept-Language` says
  otherwise. The bot follows the guild's locale unless `bot.locale` is set. Triggers stay English.

## Platform & Docker

- Only `pkg/privdrop` and `pkg/logging` have build-tagged files; CI vets every release target.
- The image is `FROM scratch` and starts as root. `dropPrivileges` chowns the cache to `PUID:PGID`
  (default 65532) and drops before reading config. It's a no-op when not root.

## Keeping this file honest

- Change CLAUDE.md in the same commit as the behaviour it describes. Write it as the current state,
  never history.
- Add a line only if it is non-obvious from the code AND can't be enforced by a test, lint rule or
  hook. If it can be enforced, write the check instead.
- When the user corrects the same thing twice, promote the correction to a rule here (or to a check),
  not only to personal memory.
- Anything that changes the desktop contract (routes, status codes, labels, mDNS records, instance
  name) also updates openapi.yaml, and the PR says "Other repo: …".
- Keep the file under ~100 lines. When it grows past that, move procedures into .claude/skills.
