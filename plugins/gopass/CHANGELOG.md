# Changelog

All notable changes to `github.com/ubgo/textdata/plugins/gopass` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`plugins/gopass/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `plugins/gopass/v0.1.1` tag was a pre-release; it is retracted in `go.mod`, and everything it contained is in this release.

### Added

- gopass `Resolver` (`${gopass:ref}`) and an optional prefix/mapping `Lookuper`; injectable runner, per-plugin cache, context and timeout; secrets never in errors or argv.
- Requires only the base module `github.com/ubgo/textdata`: importing the plugin pulls no format. Its tests in dotenv and jsonc live in the repo's test-only `integration/` module.
