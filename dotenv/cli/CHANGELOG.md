# Changelog

All notable changes to `github.com/ubgo/textdata/dotenv/cli` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`dotenv/cli/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `dotenv/cli/v0.1.1`–`dotenv/cli/v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- The dotenv CLI moved from `ubgo/dotenv/cli`: command package `dotenvcmd` (`Execute`, `Command`, `WithProgramName`), `envkit`, `discover`, and the `dotenvctl` binary, unchanged in behaviour; also mounted as `textdatactl dotenv`.
- `dotenvcmd.WithCallbackArgs` and the exec-plugin variable `DOTENVCTL_BIN_ARGS`: a host that mounts the verbs under a namespace declares the words that reach them, so a plugin's callback `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get KEY` works under `textdatactl dotenv` and embedding hosts as well as `dotenvctl` (where the variable is empty).
- `envkit.ErrExpand` wraps every expansion failure and `envkit.SelectKeyError` (matching `ErrKeyNotFound`) reports a key the selection named that the file lacks, so callers classify selection errors with `errors.Is`/`errors.As`; messages are unchanged.

### Fixed during the pre-releases

- Under a host name (`WithProgramName`, e.g. `textdatactl dotenv`) the `--help` examples of every verb, the provider namespaces included, now show that name instead of `dotenvctl`, and `completion` emits a script for the host binary (the name's first word) rather than for `dotenvctl`. The standalone binary is unchanged.
- `--strip-prefix` with `--include` (or explicit keys) could select two keys under the same final name (`P_DB_HOST` and `DB_HOST` both as `DB_HOST`), and a push wrote that name twice with the last value winning. Selection now fails with `envkit.NameCollisionError` (matching `envkit.ErrNameCollision`) naming both source keys; the CLI reports it as a usage error, exit `2`.
- Usage mistakes a verb detects itself now exit `2`, like cobra's: `set --after` with `--before`, a `set` argument without `=`, an unknown `--format` (`diff`, `matrix`) and an invalid `matrix --placeholder` regexp (whose envelope code changes from `io` to `usage`). They exited `1` before. An unknown `--format` is also rejected before `-o` truncates the report file. New sentinel `envkit.ErrPlaceholderPattern`.
- Provider verbs chose the JSON error code by searching the message for "not found" or "expand", so a `${VAR:?not found in the vault}` failure was reported as `not_found` and a missing file under a directory named `expand` as `required`. The code now comes from the error's identity.
