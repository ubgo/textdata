# Changelog

All notable changes to `github.com/ubgo/textdata/dotenv` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`dotenv/vX.Y.Z`).

## [Unreleased]

### Added

- `WithStrictExpand()`: `GetExpanded`, `ExpandedMap` and `Read` refuse an unset name (`*expand.UnsetError`), a `${scheme:ref}` no installed Resolver claims (`*expand.UnknownSchemeError`) and a `$(cmd)` with substitution off (`*expand.CommandOffError`), wrapped in a `*pos.Error` at the value's `file:line:col`; all match `expand.ErrStrict`. Defaults and `*RequiredError` are unchanged, and nothing changes without the option. Needs the base's next release.
- `Update(ctx, path, fn, opts...)`: a read-modify-write under the cross-process `atomicfile.Lock`: open, `fn`, save only when the rendered bytes changed, unlock. `fn`'s error is returned untouched and nothing is written; a no-op does not create or touch the file. Needs the base's next release.


## [0.1.5] - 2026-11-14

### Added

- `WithCommandContext(ctx)` bounds `$(command)` substitution: when the context is cancelled or its deadline passes, the running command is killed and `GetExpanded`, `ExpandedMap` and `Read` return an `*expand.CommandError` wrapping `shell.ErrTimeout` (a deadline) or `context.Canceled`. Without it, commands are unbounded, as before.

### Changed

- `$(command)` runs through the base `shell.Run` instead of a private `sh -c` copy (needs base `v0.1.5`). On macOS and Linux nothing changes: the same `sh -c`, the same trailing CR/LF trimming, stdin already at the null device. On Windows substitution now works, through `pwsh` when it is on `PATH`, else `powershell.exe`; a `.env` written for `sh` may need `WithCommandRunner(shell.Runner{Shell: ...}.RunCommand)` there. A failing command's error now unwraps to `*shell.ExitError` (exit code and the head of stderr) instead of `*exec.ExitError`.

## [0.1.4] - 2026-11-13

First public release. The earlier `dotenv/v0.1.1`–`dotenv/v0.1.2` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- Moved from `github.com/ubgo/dotenv` (source commit cf531f4); every original test and fuzz seed passes unchanged; the old repo is deprecated and archived after this release (no forwarder); its tags stay downloadable.
- Expansion runs on the base `expand` engine; plugin interfaces are aliases of the base `plugin` package; saving goes through `atomicfile` and now writes through symlinks.
- `Resolver` plugins for `${scheme:ref}` and the cross-format `plugin.SaveGuard` (a `*File` is a `plugin.Document`).

### Changed during the pre-releases

- A `$(command)` that fails, exits non-zero or is refused by a `CommandRunner` now makes `GetExpanded`, `ExpandedMap` and `Read` return an `*expand.CommandError` naming the command (it unwraps to the runner's error) instead of expanding to `""`. Behaviour change: code that relied on a failing command reading as empty must make the command succeed (`$(cmd || true)`) or return a fallback from its runner.

### Fixed during the pre-releases

- `SetAfter`, `SetBefore`, `InsertAfter` and `InsertBefore` name the missing anchor: the error is `dotenv: anchor key not found: "NOPE"`, wrapping `ErrAnchorNotFound` (match it with `errors.Is`; a direct `==` comparison no longer matches).
- `WithPlugin` no longer silently installs a plugin whose only abilities are ones dotenv never uses (the `extends` Includer, a `Validator`). It is refused with `dotenv: plugin "<name>": no ability dotenv supports (<abilities>)`, wrapping the new `ErrUnsupportedPlugin`; `Open`, `ParseReader` and `Read` return it, and on a file from `Parse` so do `GetExpanded`, `ExpandedMap`, `Save` and `SaveAs`. A plugin with at least one supported ability still installs.
