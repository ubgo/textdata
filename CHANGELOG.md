# Changelog

All notable changes to `github.com/ubgo/textdata` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`vX.Y.Z`).

## [Unreleased]

## [0.1.4] - 2026-11-13

First public release. The earlier `v0.1.1`–`v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- First release of the shared base, stdlib only, additive-only from here on.
- `plugin`: `Plugin`, `Lookuper`, `Resolver`, `CommandRunner`, `ValueTransformer(path, value)`, `SaveGuard(Document)`, `ExpandObserver`, `Validator`, `Includer`, the `Document` view, capabilities, `Registry`, `*plugin.Error` and `ErrUnavailable`, `ErrUnsupportedScheme`, `ErrDenied`, `ErrNotFound`.
- `expand`: `${VAR}` engine with every operator, nesting, `$$`, `$(cmd)` only with a runner, `${scheme:ref}` through resolvers, cycle guard with positions.
- `atomicfile`: temp file, fsync, chmod, rename, folder fsync; keeps mode; writes through symlinks.
- `pos`: `Pos`, offset `Index`, `file:line:col` `Error` and `ErrorList`.
- `path`: dotted paths with quoted segments and indexes, JSON Pointer, `ParseAny`; a leading `/` key is quoted so `ParseAny(p.String())` round-trips.
- `texttest`: round-trip and untouched-bytes property harness and a shared awkward-input corpus.
- `expand.CommandError`: the error for a `$(command)` whose runner failed; `Cmd` names the command as handed to the runner and `Unwrap` returns the runner's error.

### Changed during the pre-releases

- `expand`: a `$(command)` whose `Run` returns an error (the command failed, exited non-zero or was refused) now stops `Expand` with a `*expand.CommandError` instead of expanding to `""`. Behaviour change: a failed command no longer reads as an empty value. dotenv and jsonc report it from `GetExpanded`/`Expanded` (jsonc at the value's `file:line:col`).

### Fixed during the pre-releases

- `pos.ErrorList.Error()` says "(and 1 more error)" for two errors; "(and N more errors)" is kept for more.

### Documentation

- Repository-wide docs under `docs/`: a 14-step learning path and a "start from your goal" table (`docs/README.md`), 49 step-by-step guides, with an index page per folder, covering every module (getting started, dotenv, jsonc, jsonc tools, value plugins, the command line, push providers and clikit, base packages), decision guides under `docs/choosing/` (format and modules, methods, value plugins, options per kind of app, library or CLI), a feature index of all 136 features with the guide step and example for each (`docs/reference/features.md`), and an architecture diagram (`docs/assets/architecture.svg`).
- `examples/`: a test-only module of 46 runnable programs, one per guide topic, each with a README and a test that checks its output; guide snippets are copied from them and kept in sync by docsync.
- Module READMEs name their current tag, and links that used to go to the archived `ubgo/dotenv` docs point at the new guides.
- Repository: README with install, quick start per format and FAQ; `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, and GitHub issue and pull request templates.
