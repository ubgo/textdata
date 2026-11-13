# Changelog

All notable changes to `github.com/ubgo/textdata/plugins/vault` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`plugins/vault/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `plugins/vault/v0.1.1` tag was a pre-release; it is retracted in `go.mod`, and everything it contained is in this release.

### Added

- HashiCorp Vault `Resolver` + `Lookuper`: KV v1/v2 over HTTP, `path[?version=N][#field]`, namespace, opt-in `FromEnv`, per-plugin cache, `ErrDenied`/`ErrUnavailable`/`ErrNotFound` mapping; stdlib only.
- Requires only the base module `github.com/ubgo/textdata`: importing the plugin pulls no format. Its tests in dotenv and jsonc live in the repo's test-only `integration/` module.
