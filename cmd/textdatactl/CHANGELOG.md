# Changelog

All notable changes to `github.com/ubgo/textdata/cmd/textdatactl` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`cmd/textdatactl/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `cmd/textdatactl/v0.1.1`–`cmd/textdatactl/v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- Umbrella CLI: `textdatactl dotenv …` and `textdatactl jsonc …` by delegation, `version` and `--version` from voltkit `versioncmd.Collect`.

### Fixed during the pre-releases

- Release builds are no longer stamped `(dirty)`: volt writes its archives into `dist/` while building, and the repo now git-ignores that folder. In `textdatactl` v0.1.1 every platform except darwin/arm64 was stamped dirty.
- `textdatactl completion <shell>` prints a completion script for `textdatactl`, and its `__complete` requests inside `dotenv` and `jsonc` reach those formats' verbs; `textdatactl dotenv completion` printed a `dotenvctl` script before. `textdatactl dotenv <verb> --help` examples now say `textdatactl dotenv`.
- Exec plugins run as `textdatactl dotenv <plugin>` can call the host back: `textdatactl` sets `DOTENVCTL_BIN_ARGS=dotenv`, so `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f FILE get KEY` reaches the dotenv verbs instead of failing with `unknown shorthand flag: 'f'`.
