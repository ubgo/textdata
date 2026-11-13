# Changelog

All notable changes to `github.com/ubgo/textdata/jsonc/cli` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`jsonc/cli/vX.Y.Z`).

## [Unreleased]

## [0.1.5] - 2026-11-13

### Changed

- Requires `github.com/ubgo/textdata/jsonc` v0.1.5: `set` and the other edit verbs no longer take a new line's ending from a line break inside a block comment in a file that mixes LF and CRLF, and `fmt` keeps such a file's own line ending.

## [0.1.4] - 2026-11-13

First public release. The earlier `jsonc/cli/v0.1.1`–`jsonc/cli/v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- Command package `jsonccmd` mounted as `textdatactl jsonc`: `get`, `set`, `unset`, `restore`, `delete`, `fmt` (`--check`), `validate` (lint + JSON Schema), `diff`, `to-json`; `--json` envelope, `--strict`, `--dry-run`, exit codes 0/1/2.

### Fixed during the pre-releases

- The `textdatactl jsonc --help` example `set config.jsonc server.port 9090 --dry-run` wrote the string `"9090"`; it now carries `--json-value`. Every help example was run against a fixture; the others behave as described.
