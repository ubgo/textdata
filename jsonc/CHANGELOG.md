# Changelog

All notable changes to `github.com/ubgo/textdata/jsonc` are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are this module's own git tags (`jsonc/vX.Y.Z`).

## [Unreleased]

### Added

- `WithStrictExpand()`: `Expanded`, `GetExpanded` and `Read` with `WithExpand` refuse an unset name (`*expand.UnsetError`), a `${scheme:ref}` no installed Resolver claims (`*expand.UnknownSchemeError`) and a `$(cmd)` with substitution off (`*expand.CommandOffError`), at the string's `file:line:col`; all match `expand.ErrStrict`. Named apart from `Strict()`, which parses plain JSON. Needs the base's next release.
- `Update(ctx, path, fn, opts...)`: a read-modify-write under the cross-process `atomicfile.Lock`: open (a file with syntax errors is refused before `fn`), `fn`, save only when the bytes changed, unlock. Needs the base's next release.
- `schema.StrictKeys()`: a key rejected by `additionalProperties: false` is an error instead of a warning, still at the key, so `Result.OK`, `Err` and a `Plugin`'s `Validate` and save guard refuse a typo'd key. Without it nothing changes.

### Tests

- `jsonc/schema`: an agx-shaped config (maps of structs, a `MarshalText` string ID as key and value, `enum:"plan,api"`, `[]string`, pointers, `omitempty`) checked through `FromStruct`, `Validate`, `StrictKeys`, the plugin's save guard, and agreement with `generate.FromStruct`.


## [0.1.6] - 2026-11-14

### Added

- `WithCommandContext(ctx)` bounds `$(command)` substitution: when the context is cancelled or its deadline passes, the running command is killed and `Expanded`, `GetExpanded` and `Read` with expansion fail with an error unwrapping to `*expand.CommandError` and to `shell.ErrTimeout` (a deadline) or `context.Canceled`. Clones keep it. Without it, commands are unbounded, as before.

### Changed

- `$(command)` runs through the base `shell.Run` instead of a private `sh -c` copy (needs base `v0.1.5`). On macOS and Linux nothing changes except that a trailing `\r` before the final newline is now trimmed too, as dotenv already did. On Windows substitution now works, through `pwsh` when it is on `PATH`, else `powershell.exe`. A failing command's error now unwraps to `*shell.ExitError` instead of `*exec.ExitError`.

## [0.1.5] - 2026-11-13

### Fixed

- Text inserted into a file that mixes LF and CRLF took its line ending from a line break inside a block comment. In `{/*\r\n*/\n}`, `Set` wrote a CRLF that `Delete` then left behind, so Set then Delete no longer gave back the input. Inserted text now uses the break where it goes, or the line break just before the new line, or the document's first line break outside a comment, never one inside a block comment. This covers `Set` and `SetAfter`/`SetBefore` on objects, `Append`/`Insert` on arrays, `SetComment` and the comments `Merge` copies. Found by `FuzzSetDelete`; the input is kept as a fuzz seed, and a table test covers each mixed LF/CRLF layout.
- `format.Format` took the line ending from the first line break anywhere, so a CRLF inside a block comment turned an LF file into a CRLF one. It now uses the first break outside a block comment or string.

## [0.1.4] - 2026-11-13

First public release. The earlier `jsonc/v0.1.1`–`jsonc/v0.1.3` tags were pre-releases; they are retracted in `go.mod`, and everything they contained is in this release.

### Added

- Lossless JSONC parser (tolerant by default, `Strict()` for plain JSON matching `encoding/json.Valid`), comments attached to members, `file:line:col` errors.
- Read: `JSON`, `Decode`, `Get` and typed getters, `Has`, `Keys`, `Len`, `Kind`, `Comment`, `Pos`, `KeyPos`, dotted paths and JSON Pointer; `Read` one-call.
- Edit: `Set`, `SetAfter`, `SetBefore`, `Delete`, `Append`, `Insert`, `Remove`, `SetComment`, `RemoveComment`, `Disable`, `Restore`, `Merge`, `Clone`; `Save`/`SaveAs` through `atomicfile` with save guards.
- Dynamic values: expansion through the base engine with Lookupers, Resolvers, CommandRunner and ValueTransformers (off by default; positioned errors); `$extends` through `Extended` when an Includer is installed.
- `Lint`, and packages `format` (`Format`, `Minify`), `generate` (builders, `FromStruct`), `schema` (`FromStruct`, `Parse`/`Load`, `Validate`, `Plugin`), `diff` (`Diff`).
- Tests: JSONTestSuite conformance, a JSONC case set, property tests through `texttest`, fuzz targets for parse, edits, merge, format, lint, extends, schema and diff.

### Changed during the pre-releases

- `schema.Plugin` is now also a `plugin.SaveGuard`: installing it makes `Save` and `SaveAs` refuse a document with schema errors, leaving the file untouched. Warnings never refuse a save. It used to be a `Validator` only, so `Save` never ran it.
- A failing or refused `$(command)` is now an error from `GetExpanded`, `Expanded` and `Read` with expansion, carrying the value's `file:line:col` and unwrapping to `*expand.CommandError`; it used to expand to `""` silently.
- `Read` on a missing file now returns an error that names the path and wraps `fs.ErrNotExist`, matching `dotenv.Read`; it used to return an empty map and no error. `Open` is unchanged: a missing file is still an empty document that `Save` creates.

### Fixed during the pre-releases

- `Doc.Pos`, `Doc.KeyPos` and every error position raced when several goroutines read one unedited `Doc`: the lazily built offset index is now an atomic pointer, reset the same way by every edit. `Clone` builds a new `Doc` instead of copying one. Pinned by a race-detector test.
- `schema.Parse` reported a `type` that is neither a string nor an array as `must be an array of strings`; a single type name is valid too, so it now says `must be a string or an array of strings`.
