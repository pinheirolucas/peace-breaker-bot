---
name: cut-release
description: Cut a bot release through the Cut Release workflow. Only when the user explicitly asks for a release.
disable-model-invocation: true
---

# Cut a release

Releases publish binaries, the Windows installer and GHCR images. Only run this when the user asks.
Never tag by hand: the tag is the version (`-X cmd.Version`).

1. Confirm `main` is green: `gh run list --workflow ci.yaml --branch main --limit 1`.
2. Dry run: `gh workflow run cut-release.yaml -f bump=<patch|minor|major> -f dry_run=true`, then read the
   computed version in the run log and confirm it with the user.
3. Real run: the same command without `dry_run`, plus `-f prerelease=true` for a `vX.Y.Z-rcN` candidate.
4. Watch the workflows the tag triggers: `release.yaml` (GHCR `:X.Y.Z`, `:X.Y`, `:latest`, `:nightly`
   plus cross-compiled binaries) and `release-windows.yaml` (Inno Setup installer).
5. Report the release URL and the image tags.

`gh workflow run` is denied in `.claude/settings.json`, so give the user the exact commands to run
themselves (for example with the `!` prefix).
