# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go application that runs a Discord bot capable of joining a voice channel and playing "instants" (short mp3 clips, originally from myinstants.com), plus an HTTP API used to control that bot. It is the backend for a separate Electron/React desktop UI that lives in a sibling repository, `../peace-breaker-bot-desktop` (see `CLAUDE.md` there).

## Commands

```bash
make build   # go build -o ./bin/peace-breaker-bot <module>
make run     # build then run the binary
make test    # go test ./...
make cover   # go test -coverprofile cp.out ./... && go tool cover -html=cp.out
make clean   # go clean; remove ./bin, cp.out, nohup.out
```

Run a single test package/test directly with the standard Go toolchain, e.g. `go test ./pkg/instant/... -run TestName -v`.

The Go toolchain is pinned in `.tool-versions` (the asdf format, which mise and asdf both read, and which
`actions/setup-go` accepts via `go-version-file`). The `go` directive in `go.mod` states the minimum
language version the module requires and is a separate knob — bumping one does not bump the other.

Note the key there has to be `golang`, not `go`: mise accepts either, but `actions/setup-go` matches only
`golang`.

## Configuration

Config is loaded via Viper from (in order of precedence) CLI flags, environment variables, then a YAML file (`.peace-breaker-bot.yaml` in `$HOME` or the cwd; see `.peace-breaker-bot.sample.yaml` for the schema). Required settings:

- `bot.owner` / `--bot-owner` / `BOT_OWNER` — the only Discord username the bot will respond to.
- `bot.token` / `--bot-token` / `BOT_TOKEN` — Discord bot OAuth token.
- `server.address` / `--server-address` / `SERVER_ADDRESS` — address the HTTP API binds to (e.g. `0.0.0.0:9001`).

`cmd/root.go` fails fast (before starting anything) if any of these three are missing. `bot.locale` / `--bot-locale` / `BOT_LOCALE` is optional — see "Internationalization" below.

## Architecture

Entry point `main.go` → `cmd.Execute()` (Cobra root command in `cmd/root.go`) parses config/flags and then starts two long-running goroutines against a single shared `*instant.Player`:

- **`pkg/bot`** — the Discord client (`disgoorg/disgo`, migrated off `bwmarrin/discordgo` because it has no support for Discord's mandatory DAVE/E2EE voice protocol). `bot.New` registers chat commands (`!ping`, `!join`, `!help`) against a `command.DiscordDispatcher` (see `pkg/command/discord.go`), which does simple whitespace-tokenized string matching on `!command` prefixes to route messages — there's no argument parsing beyond splitting on spaces. `Register`'s second argument is a `pkg/i18n` key, not rendered text (see "Internationalization" below); `GetHelp` resolves it per call, since the response locale is only known at dispatch time. Only messages from `bot.owner` are dispatched (`handleMessages` in `pkg/bot/bot.go`). DMs never reach the `command.DiscordDispatcher` at all — instead, `handleMessages` checks whether the DM's content is a Discord invite link, since that's what the client's "invite to voice" quick-invite feature (the `+` icon next to a voice channel) actually sends the target — there's no dedicated API or gateway event for it, confirmed by capturing the actual DM payload. `handleInviteDM` (`pkg/bot/invite.go`) matches that link pattern, resolves it via `client.Rest.GetInvite`, and joins the invite's channel directly via the shared `joinVoiceChannel` helper (`pkg/bot/join.go`) — bypassing the guild member voice-state lookup `!join` relies on, since a DM has no guild member state to look up. Any other DM is silently ignored, with no reply at all — the bot used to answer every DM with a refusal message, but that was dropped once DMs gained an actual purpose (the invite-link handling) rather than being uniformly out of scope. Guilds/channels/voice-states are uncached by disgo unless explicitly enabled (`bot.WithCacheConfigOpts` in `bot.Start()`) — `!join` depends on all three. Voice DAVE/E2EE encryption is handled by `thomas-vilte/dave-go` (a pure-Go DAVE implementation, wired in via `voice.WithDaveSessionCreateFunc` in `bot.Start()`) — without it, voice defaults to an unencrypted no-op session that Discord's gateway now rejects outright with close code `4017`. `Bot.Start()` runs a loop that blocks on `player.GetNextPlay()` and streams the resulting file into the current voice connection via `pkg/opusaudio`, which decodes mp3 with the pure-Go `github.com/hajimehoshi/go-mp3` (chosen because the app only ever plays cached mp3 clips — see "Architecture" for `pkg/fsutil`'s format sniffing — so a full multi-format transcoding subprocess is more than the job needs), resamples to 48kHz with a small hand-rolled linear-interpolation `Resampler` (`pkg/opusaudio/resample.go`, since go-mp3 always decodes at the source's own rate, commonly 44.1kHz, and doesn't resample itself), and encodes to Opus with `github.com/pion/opus` (also pure Go). The whole pipeline is cgo-free and has no external runtime dependency, handing frames to disgo's voice package as a pull-based `OpusFrameProvider` (disgo pulls frames on its own 20ms clock, rather than accepting a push channel the way discordgo's voice API did).
- **`pkg/server`** — the HTTP API (stdlib `net/http.ServeMux` with Go 1.22 method patterns like `"POST /api/v1/bot/play"`; a small custom `corsMiddleware` in `pkg/server/cors.go` for CORS-with-`*`, replacing an earlier `gorilla/handlers` dependency). Routes are version-scoped under `/api/v1` (except `/api/docs`, see below): `POST /api/v1/bot/play` (play a URL through the bot, blocks until playback ends/stops and returns the exit reason; answers `409 bot_not_connected` before ever touching the player if the bot has no open voice connection — see below), `POST /api/v1/bot/stop`, `GET /api/v1/bot/status` (reports the bot's current voice connection — `{connected, guildId, guildName, channelId, channelName}`, names omitted while disconnected — so the UI can poll and gate its own "send to Discord" action before a click, rather than only reacting to a failed one), `GET /api/v1/instants/{url}/content` (fetch/cache a clip, keyed by its percent-encoded myinstants.com URL as the `{url}` path segment — Go's pattern-based `ServeMux` hands `r.PathValue("url")` back already decoded — and return it as a base64 data URI, without touching the bot), `GET /api/v1/instants?page=&search=&region=` (scrapes a `myinstants.com` listing page with `goquery` — this and the clip download in `pkg/fsutil` are the only integration points with myinstants.com, and the scrape is fragile to markup changes; see "Talking to myinstants.com" below), `GET /api/v1/openapi.yaml` and `GET /api/docs` (the API spec and its Redoc-rendered page; see "API spec" below — `/api/docs` is deliberately not version-scoped, since one docs page covers every version). The API adopts real HTTP status codes (400/404/409/422/500/502 as appropriate) rather than shipping everything as 200; see each handler in `pkg/server/server.go` for the exact mapping. `Server` depends on a narrow `BotStatus` interface (`Status() bot.VoiceStatus`), not the full `*bot.Bot`, so `pkg/server` doesn't need to import the bot package's whole surface for one status call — `*bot.Bot` satisfies it via `Bot.Status()`, which reads the connection behind a mutex (`Bot.vcMu`) since it's now read from the HTTP server's own goroutine as well as the gateway's. The server also registers a zeroconf/mDNS advertisement (`_myinstants._tcp`, see `pkg/server/autodiscovery.go`) on the same port so the UI can auto-discover the backend on the LAN instead of hardcoding an address. The advertisement carries two TXT records, `path=/api` (the base path a discovering client should hit — versioning is deliberately not part of it, since which version to use is the client's own concern) and `api=1` (lets a client refuse a bot it cannot talk to); the instance name stays `<hostname>-<port>`, which clients match on, so don't change it. The service is visible on the wire — a Node `bonjour-service` client discovered the running bot at `http://10.0.0.133:9001` and got HTTP 200 from `/instant/list` (pre-`/api/v1` route). It is *not* visible to macOS's own `dns-sd -B`, because `libp2p/zeroconf` answers multicast directly instead of registering with `mDNSResponder`, so the system tool has nothing to list — debug with a client that reads the wire, not with `dns-sd`. The UI browses for the advertisement; with neither a discovered nor a user-picked server, it talks to nothing rather than falling back to any default address (there is no `localhost:9001` fallback anymore).
- **`pkg/instant`** — the shared state machine. `Player` (`pkg/instant/player.go`) is a single-slot, mutex-guarded player: `Play()` pushes a file path onto `playChan`, blocks until either `endChan` or `internalStop` fires, and returns which one ("end"/"stop"); it only supports one playback at a time and calling `Play` while something is already playing stops the current one first. `GetPlayable`/`GetFromCache` (`pkg/fsutil/fsutil.go`) resolve a myinstants URL to a local cached file, downloading+validating (must sniff as mp3 via `h2non/filetype`) into `~/.instants/<md5(url)>.mp3` on first access.
- Both the bot loop and any HTTP handler that calls `player.Play` share the *same* player — there's no queueing beyond the single in-flight slot, so `/api/v1/bot/play` requests serialize through it.
- **`Bot.vc` doesn't clear itself on an external disconnect.** `!leave` and a normal shutdown both nil it out, but if the bot gets kicked from its channel or Discord drops the voice socket some other way, nothing notices — `Bot.Status()` (and `GET /api/v1/bot/status`) keeps reporting the last-known channel as connected until the process restarts. A voice-state-update listener that nils `vc` on the bot's own disconnect would close this; it hasn't been built yet.
- Errors surfaced from `pkg/instant`/`pkg/fsutil` (`ErrInvalidLink`, `fsutil.ErrNotFound`, `fsutil.ErrUnsuportedAudioFormat`) are mapped to specific HTTP status codes/labels in `pkg/server/server.go` — follow that pattern when adding new error cases rather than falling through to the generic 500. `writeErrorMessage` writes the status it is given, and the UI's HTTP client (in the sibling `peace-breaker-bot-desktop` repo) is expected to branch on it; `label` remains the stable machine-readable identifier within a given status. See "Internationalization" below for where the `message` text that goes with each `label` actually lives.

## Internationalization

`pkg/i18n` is a small, hand-rolled catalog (`map[language.Tag]map[string]string`,
plus `x/text/language.NewMatcher` for negotiation) covering the API's error
messages and the bot's command descriptions — not a framework like
`nicksnyder/go-i18n`, since there are no plural forms and only two locales to
justify one. `en_us.go` and `pt_br.go` hold the two catalogs; `Text(tag, key)`
resolves a key, falling back to English and finally to the key itself, so a
key this catalog doesn't recognize surfaces as an odd string rather than a
blank response.

**API errors default to English**, negotiated per request via an optional
`Accept-Language` header (`languageFor` in `pkg/server/server.go`) — the
companion UI never sends this header, since it already translates by `label`
on its own and only reads `message` as a fallback for a label it doesn't
recognize. `writeErrorMessage` takes a `label` and a `language.Tag`, not a
literal string, so a message can no longer drift from the catalog the way
`unknown_error` once shipped two different Portuguese texts under one label.

**The bot's response language follows the invoking guild.** `Bot.localeFor`
(`pkg/bot/bot.go`) resolves it per message: the `bot.locale` config value
always wins when set (a single-owner bot that wants a fixed language
regardless of server); otherwise the guild's own `PreferredLocale`, read from
disgo's guild cache (`client.Caches.Guild(guildID)` — the same cache `!join`
already depends on, so `bot.WithCacheConfigOpts(cache.FlagGuilds)` in
`bot.Start()` has to stay enabled); a DM carries no guild at all and falls
straight through to `pkg/i18n`'s own English default. Command triggers
(`!ping`, `!join`, `!help`) are exact map keys in the dispatcher and stay
English — only their descriptions translate. DMs never get a translated (or
any) reply — see "Architecture" above.

## Talking to myinstants.com

- **The User-Agent is load-bearing.** myinstants.com is behind Cloudflare, which answers 403 to Go's default `Go-http-client/2.0` (and to curl's, and to a bare `Mozilla/5.0`). Every request goes through `pkg/httpclient`, which sets `User-Agent: peace-breaker-bot/1.0`; both `Server.httpClient()` and `fsutil.Cache.client()` fall back to it. Never swap either back to `http.DefaultClient` — the listing and the clip download both break, silently, as 403s. The transport is not the problem: Go's HTTP/2 is not flagged (curl's is, so don't draw transport conclusions from curl) — confirmed when this UA was renamed from `discord_instants_player/1.0`: curl 403s on *both* strings (a transport-level Cloudflare fingerprint, not a UA check), while Go's own `http.Client` gets 200 on the new string, same as it did on the old one. Whatever the UA string says, it just has to be a real, non-default value — myinstants.com doesn't care which app name it names.
- **Endpoint split.** A search goes to `/search/?page=N&name=TERM`, which ignores the region (and redirects to `/en/search/`). Browsing with no search term goes to `/en/index/<region>/?page=N` — `/search/` with no name answers 404. `region` is a two-letter code the client stores; it is lowercased, defaults to `us`, and anything not matching `^[a-z]{2}$` is refused with the `invalid_region` label before any upstream request. An unknown region answers 200 with no instants. The language prefix is fixed at `en`; it does not change the results.
- **The page count is inferred.** Listing pages no longer carry a pager, so `parseInstantList` infers `pages` from how many instants came back: a full page (`pageSize`, 36) means `page+1`, a short page means `page`, an empty page means `max(1, page-1)`. If myinstants changes its page size, `pageSize` has to follow. A search paged past its end answers 404, which the handler normalizes into the same collection shape a successful listing uses (`data: {"instants": [], "pages": <last page>}`), not a bare empty array.
- **Clip URLs** come from the first quoted argument of the play button's `onclick="play('/media/sounds/x.mp3', 'loader-…', '…')"`. The `len(names) != len(links)` guard is what catches the next markup change — keep it loud.
- **Fixtures in `pkg/server/testdata` are trimmed captures of live responses**, fetched with the app's User-Agent. When the markup changes, re-capture them; don't hand-edit them into a shape the site no longer serves, or the suite passes against a fiction.

## API spec

`pkg/server/v1/openapi.yaml` is a hand-written OpenAPI 3.1 document, not generated — chosen over an annotation- or reflection-based generator (swaggo/swag, swaggest, huma) specifically because it adds zero dependencies and can say things a generator can't produce automatically. It lives under a version-named directory (`v1/`) so a future `v2/openapi.yaml` can sit alongside it. It's embedded into the binary via `//go:embed v1/openapi.yaml` in `pkg/server/spec.go` and served as-is at `GET /api/v1/openapi.yaml`; `pkg/server/docs.html` (also embedded) renders it at `GET /api/docs` via Redoc loaded from a CDN, so there's no npm/build step. `docs.html` is not version-scoped — it carries a small hardcoded list of `{label, specUrl}` entries (currently just `v1`) rendered as tabs that swap Redoc's spec source; adding v2 later is one more list entry. **There is nothing that keeps the spec in sync with the handlers** — when a route's request/response shape, status code, or error labels change, update `openapi.yaml` by hand in the same PR; `pkg/server/spec_test.go` only checks that it's valid YAML and that every route in `server.go` has an entry, not that the shapes match.

## Distribution

`peace-breaker-bot.iss` is an Inno Setup script used to build a Windows installer that bundles just the built binary — no external runtime dependency to bundle anymore, now that `pkg/opusaudio` decodes and resamples in pure Go (see "Architecture" above). `.github/workflows/ci.yaml`'s `build-windows` job compiles `peace-breaker-bot.exe` on a `windows-latest` runner on every push and pull request; since the whole module has been cgo-free for a while, this could become a cross-compiled step in the main job instead, but that's a CI-architecture change, not something this doc should assume happened.
