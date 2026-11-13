# Changelog

All notable changes to `github.com/ubgo/textdata/dotenv/cli/providers/vercel` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`dotenv/cli/providers/vercel/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `dotenv/cli/providers/vercel/v0.1.1` tag was a pre-release; it is retracted in `go.mod`, and everything it contained is in this release.

### Added

- Moved from `ubgo/dotenv/cli/plugins/vercelplugin`: push, list and prune Vercel env vars through the `vercel` CLI, on `clikit/providerkit`. Lives at `dotenv/cli/providers/vercel` (its own module, needing only clikit and cobra).
