# Changelog

All notable changes to `github.com/ubgo/textdata/plugins/extends` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`plugins/extends/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `plugins/extends/v0.1.1` tag was a pre-release; it is retracted in `go.mod`, and everything it contained is in this release.

### Added

- `plugin.Includer` for `$extends`: `New` (disk; `WithRoot` confinement through the real path and `os.Root`, `AllowAbsolute`, `WithMaxSize`) and `NewFS` (`fs.FS`).
- Requires only the base module `github.com/ubgo/textdata`: importing the plugin pulls no format. Its tests in dotenv and jsonc live in the repo's test-only `integration/` module.
