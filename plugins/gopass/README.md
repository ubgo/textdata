# gopass plugin for textdata

`github.com/ubgo/textdata/plugins/gopass` resolves `${gopass:path}` references from a [gopass](https://github.com/gopasspw/gopass) password store. It is written once against the base `plugin` interfaces, so the same plugin works in every textdata format (dotenv, jsonc, and later ones).

```sh
go get github.com/ubgo/textdata/plugins/gopass
```

## Example

```go
import (
	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	gopassplugin "github.com/ubgo/textdata/plugins/gopass"
)

p := gopassplugin.New(gopassplugin.WithPrefix("personal/app/"))

// .env:  DB_PASSWORD=${gopass:personal/db}
f, _ := dotenv.Open(".env", dotenv.WithPlugin(p))
pw, _, err := f.GetExpanded("DB_PASSWORD")

// app.jsonc:  {"db": {"password": "${gopass:personal/db}"}}
doc, _ := jsonc.Open("app.jsonc", jsonc.WithPlugin(p))
v, err := doc.GetExpanded("db.password")
```

The resolved value lives in memory only: `Render` and every save write the reference back, never the secret.

## What it does

- **Resolver** for the scheme `gopass`: `${gopass:personal/db}` runs `gopass show -o personal/db` and uses the first line of the entry.
- **Lookuper** (off until `WithPrefix` or `WithMapping` is given): a plain `${NAME}` the file itself does not define is looked up in the store. The file's own values always win. A name with no entry reads as unset (the next Lookuper is tried); a store failure is an error.

Running the `gopass` binary is this plugin's documented job, and it happens only when a format asks it to resolve a reference it owns; installing it runs nothing.

## Guarantees

- The secret is read from stdout only, with one trailing newline (`\n` or `\r\n`) trimmed; every other byte is kept.
- The secret never appears in an error. Errors name the reference and, for a store failure, the first line of gopass's stderr (capped at 200 bytes, with any stdout text redacted).
- argv is exactly `show -o <ref>`. A ref that is empty, starts with `-` (could read as a flag) or holds a control character is refused without running anything. Prefix lookups never map names containing `/`, `\`, `..` or a leading `-`.
- Stdin is not connected, so gopass cannot stop to ask a question.
- A missing entry wraps `plugin.ErrNotFound` (gopass exit code 11, or its "not in the password store" message). A missing binary, a timeout, a cancelled context or any other failure wraps `plugin.ErrUnavailable`, so an unreachable store never looks like an unset variable.

## Options

| Option | Default | What |
|---|---|---|
| `WithRunner(r Runner)` | `ExecRunner` (os/exec) | how gopass is run; inject a fake in tests |
| `WithBinary(path)` | `gopass` on PATH | the program to run |
| `WithContext(ctx)` | `context.Background()` | parent context of every call, for cancellation |
| `WithTimeout(d)` | 30s | bound on each call; `0` means none beyond the context |
| `WithCache(bool)` | on | fetch each ref once per plugin instance; only successes are cached; `ClearCache()` forgets |
| `WithPrefix(prefix)` | off | Lookuper maps `${NAME}` to `prefix+NAME` (used exactly as given) |
| `WithMapping(map)` | off | Lookuper maps exactly these names to store paths; wins over the prefix |

`Show(ref)` returns a value without a document, through the same rules and cache.
