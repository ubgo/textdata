# Changelog

All notable changes to `github.com/ubgo/textdata/clikit` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`clikit/vX.Y.Z`).

## [Unreleased]

### Changed

- `go.mod` is tidy for a standalone build: `github.com/spf13/pflag` is now an indirect requirement (only cobra imports it) and `github.com/inconshreveable/mousetrap` is listed as indirect. No code or behaviour change; this ships with the next patch release.

## [0.1.4] - 2026-11-13

First public release. The earlier `clikit/v0.1.1`–`clikit/v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- Shared CLI plumbing moved out of the dotenv CLI: `outfmt` (`{ok,data|error}` envelope, error codes; default program `textdatactl`), `report`, `textdiff`, `selection` (the selection grammar over flat pairs), `providerkit` (provider plugin seam).

### Changed during the pre-releases

- `selection.Options.StripPrefix` documents that two selected keys ending with the same name are an error, as dotenv's `envkit` now enforces.

### Fixed during the pre-releases

- `providerkit.Confirm` panicked when `Deps.Interactive` (or `Deps.Ask`) was nil. A nil hook now counts as non-interactive: the write is refused with the usual usage error asking for `--yes`.
