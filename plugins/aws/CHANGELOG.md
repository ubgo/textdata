# Changelog

All notable changes to `github.com/ubgo/textdata/plugins/aws` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`plugins/aws/vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `plugins/aws/v0.1.1` tag was a pre-release; it is retracted in `go.mod`, and everything it contained is in this release.

### Added

- AWS Secrets Manager and SSM Parameter Store `Resolver` + `Lookuper`: `sm/` and `ssm/` grammar, `#jsonKey`, stage/version, stdlib SigV4 checked against AWS's published suite, static/env/shared-file/source credentials, endpoint override, per-plugin cache.
- Requires only the base module `github.com/ubgo/textdata`: importing the plugin pulls no format. Its tests in dotenv and jsonc live in the repo's test-only `integration/` module.
