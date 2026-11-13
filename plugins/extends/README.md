# extends plugin for textdata

`github.com/ubgo/textdata/plugins/extends` loads the targets of include references such as `"$extends": "../base.jsonc"`. It is a `plugin.Includer` written once against the base `plugin` interfaces: it hands a format bytes and a location and never looks at what they contain, so one instance serves every textdata format.

```sh
go get github.com/ubgo/textdata/plugins/extends
```

Requires Go 1.24 or newer (confinement opens through `os.Root`). Imports only the textdata base.

## Example

```go
import (
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugins/extends"
)

// config/app.jsonc: { "$extends": "../defaults/base.jsonc", "port": 8080 }
doc, err := jsonc.Open("config/app.jsonc", jsonc.WithPlugin(extends.New(extends.WithRoot("."))))
cfg, err := doc.Extended() // base deep-merged first, this file on top, "$extends" removed
port, _ := cfg.GetInt("port")
```

`doc` itself is never modified: `doc.Render()` and `doc.Save()` still write `"$extends"` exactly as written. Without an Includer installed, `$extends` is an ordinary key and reading a file never opens another one.

For an embedded or in-memory tree use `extends.NewFS(fsys)` (an `embed.FS`, `os.DirFS`, `testing/fstest.MapFS`).

## Includers

| Constructor | Name | Reads from | Locations |
|---|---|---|---|
| `New(opts...)` | `extends` | the local disk | OS paths, cleaned (`config/../base.jsonc` becomes `base.jsonc`) |
| `NewFS(fsys, opts...)` | `extends-fs` | an `fs.FS` | slash-separated FS paths; the FS is its own root |

A reference is resolved against the folder of the including file (`from`); with `from == ""` (a document parsed from bytes with no file name) the disk includer uses the working directory and the FS includer the FS root. The returned location is the base for the target's own references, so nested `$extends` are relative to each file, and it is what a format compares to detect cycles.

## Options

| Option | Default | Effect |
|---|---|---|
| `WithRoot(dir)` | none | Every target must lie inside `dir`, checked on the real path (symlinks resolved) and opened through `os.Root`, so neither `../` nor a symlink leads out. An escape fails with `plugin.ErrDenied`. Ignored by `NewFS`. |
| `AllowAbsolute(bool)` | `true` | `false` refuses absolute references with `plugin.ErrDenied`. An allowed absolute reference is still subject to `WithRoot`. `NewFS` always refuses them. |
| `WithMaxSize(n)` | 10 MiB | A larger target fails with `ErrTooLarge`; `n <= 0` restores the default. |

## Errors

- A missing target wraps `plugin.ErrNotFound` and `fs.ErrNotExist`; a format tries the next installed Includer on `plugin.ErrNotFound` only.
- A refused target (absolute when not allowed, outside the root, or refused by `os.Root` under a root) wraps `plugin.ErrDenied`; a permission error without a root wraps `fs.ErrPermission`.
- A directory, device or named pipe is `ErrNotRegular`, checked before opening so a pipe cannot block the read.
- The format wraps every error with the plugin's name and the `file:line:col` of the `$extends` value: `app.jsonc:2:15: $extends "nope.jsonc": jsonc: plugin "extends": include nope.jsonc: extends: nope.jsonc: not found: ...`.

## Guarantees

- Never writes, never runs a command, never reads or changes the process environment.
- A confined includer never returns a location outside its root (fuzzed: `FuzzInclude`).
- One instance is safe to share between documents and formats; it holds no state beyond its options.
