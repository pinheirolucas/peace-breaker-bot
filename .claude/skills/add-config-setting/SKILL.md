---
name: add-config-setting
description: Use when adding, renaming or removing a configuration setting (flag, env var or config file key).
---

# Add a config setting

1. In `cmd/root.go` `init()`, add the persistent flag `--<section>-<name>`. Bind it with
   `viper.BindPFlag("<section>.<name>", ...)`. The env var `PBB_<SECTION>_<NAME>` follows automatically.
2. Validate it in `runRootCmd` before anything starts, and return a clear error for bad values
   (see the `log.level` check and `root_test.go`).
3. Add a commented block in `.peace-breaker-bot.sample.yaml` under its section, ending with
   `Setting mapped to:` plus the CLI flag and environment lines.
4. Add a row to the README settings table with the config key, flag and env var in backticks, like
   the existing rows.
5. `make test`. `cmd/docs_test.go` fails if the sample config or the README row is missing.
6. Renaming or removing a setting is breaking: say so in the PR summary.
