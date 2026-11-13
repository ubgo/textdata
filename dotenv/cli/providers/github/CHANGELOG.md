# Changelog

All notable changes to `github.com/ubgo/textdata/dotenv/cli/providers/github` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`dotenv/cli/providers/github/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `dotenv/cli/providers/github/v0.1.1`–`dotenv/cli/providers/github/v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- Moved from `ubgo/dotenv/cli/plugins/githubplugin`: push, list and prune GitHub Actions secrets through `gh`, on `clikit/providerkit`. Lives at `dotenv/cli/providers/github` (its own module, needing only clikit and cobra).

### Fixed during the pre-releases

- The tests no longer depend on an ambient `GH_TOKEN` or `GITHUB_TOKEN`: a TestMain clears both, so the suite passes under a release tool that exports a token. No library change.
