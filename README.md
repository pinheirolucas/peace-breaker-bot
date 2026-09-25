# Peace Breaker Bot

A Discord bot that joins a voice channel and plays short audio clips ("instants", like the ones on [myinstants.com](https://www.myinstants.com)) on command, paired with a local HTTP API for controlling the bot and browsing clips from myinstants.com, SoundboardGuy and Sound Buttons. This is the backend service; the desktop UI that drives it lives in the sibling repo [`peace-breaker-bot-desktop`](https://github.com/pinheirolucas/peace-breaker-bot-desktop).

## Features

- Discord bot that streams mp3 clips into a voice channel.
- HTTP API to play and stop clips, join and leave voice channels, report the bot's status, fetch a clip for local preview, and browse or search each provider. The full reference is served at `/api/docs`.
- Downloaded clips are cached locally (`~/.instants`) so repeat plays don't re-fetch.
- Advertises itself on the local network via mDNS/zeroconf (`_myinstants._tcp`) so clients can auto-discover it.
- Only responds to a single configured Discord username, to avoid the bot being hijacked in shared servers.

## Requirements

- Go 1.27+ (the exact version is pinned in `.tool-versions`)
- A Discord bot application/token — see the [Discord developer docs](https://discord.com/developers/docs/intro) to create one

## Installation

### From source

```bash
git clone https://github.com/pinheirolucas/peace-breaker-bot.git
cd peace-breaker-bot
make build
```

The binary is built to `./bin/peace-breaker-bot`.

### Prebuilt binaries

Every tagged release publishes plain binaries for Linux, macOS, and Windows (amd64 and arm64, where applicable) on the [Releases page](https://github.com/pinheirolucas/peace-breaker-bot/releases) — no build toolchain and, as of the pure-Go audio pipeline, no runtime dependency either. Download and run.

A Windows installer (built from `peace-breaker-bot.iss` with [Inno Setup](https://jrsoftware.org/isinfo.php)) is published alongside them for anyone who'd rather have Start Menu shortcuts, an uninstaller, and a settings wizard that writes `.peace-breaker-bot.yaml` for you.

### Docker

```bash
docker run -d --name instants \
  -e PBB_BOT_OWNER=yourname -e PBB_BOT_TOKEN=... -e PBB_SERVER_ADDRESS=0.0.0.0:9001 \
  -p 9001:9001 -v instants-cache:/home/app/.instants \
  ghcr.io/pinheirolucas/peace-breaker-bot:latest
```

Images are published to [GitHub Container Registry](https://github.com/pinheirolucas/peace-breaker-bot/pkgs/container/peace-breaker-bot) on every tagged release, for `linux/amd64` and `linux/arm64`, tagged by exact version (`:1.4.0`), minor track (`:1.4`), and `:latest`. The `-v` mount persists the downloaded-clip cache (`~/.instants` inside the container) across restarts.

## Configuration

Settings can be provided via config file, environment variable, or CLI flag (in that order of precedence, flags winning):

| Setting | Config key | CLI flag | Environment variable | Description |
| --- | --- | --- | --- | --- |
| Bot owner | `bot.owner` | `--bot-owner` | `PBB_BOT_OWNER` | Discord username allowed to command the bot |
| Bot token | `bot.token` | `--bot-token` | `PBB_BOT_TOKEN` | Discord application OAuth token |
| Server address | `server.address` | `--server-address` | `PBB_SERVER_ADDRESS` | Address the HTTP API binds to, e.g. `0.0.0.0:9001` |
| Bot locale | `bot.locale` | `--bot-locale` | `PBB_BOT_LOCALE` | Optional. Fixes the bot's response language (e.g. `en-US`, `pt-BR`) instead of following each Discord server's own locale |
| Log level | `log.level` | `--log-level` | `PBB_LOG_LEVEL` | Optional. `debug`, `info` (default), `warn` or `error`. An unknown value stops the app at startup |
| Log format | `log.format` | `--log-format` | `PBB_LOG_FORMAT` | Optional. `text` (default, key=value lines) or `json` (one object per line, no banner). An unknown value stops the app at startup |
| Log color | `log.color` | `--log-color` | `PBB_LOG_COLOR` | Optional. `auto` (default), `always` or `never`. `auto` colors only on a terminal and follows `NO_COLOR`, `FORCE_COLOR` and `CLICOLOR_FORCE`. An unknown value stops the app at startup |

Every environment variable carries a `PBB_` prefix. The exceptions are `PUID` and `PGID` (Docker only) and the standard `NO_COLOR`, `FORCE_COLOR` and `CLICOLOR_FORCE`, which are not prefixed.

The first three are required; the app exits immediately if any are missing. Bot locale and the log settings are optional.

The config file is YAML, named `.peace-breaker-bot.yaml`, and is looked up in your home directory or the current working directory. See [`.peace-breaker-bot.sample.yaml`](./.peace-breaker-bot.sample.yaml) for a template:

```bash
cp .peace-breaker-bot.sample.yaml ~/.peace-breaker-bot.yaml
# edit ~/.peace-breaker-bot.yaml with your bot owner/token
```

## Usage

```bash
make run
```

Once running, invite the bot to your server and, from a text channel, use:

| Command | Description |
| --- | --- |
| `!join` | Bot joins the voice channel you're currently in |
| `!join #name` | Bot joins the named voice channel in this server |
| `!leave` | Bot stops playing and leaves the voice channel |
| `!ping` | Health check |
| `!help` | Lists available commands |

## Development

```bash
make build   # compile the binary
make run     # build and run
make test    # go test -race -timeout 90s ./...
make lint    # golangci-lint (version pinned in .tool-versions)
make cover   # run tests with coverage and open an HTML report
make clean   # remove build artifacts
```

## License

[Apache License 2.0](./LICENSE)
