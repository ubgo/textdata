# file plugin for textdata

`github.com/ubgo/textdata/plugins/file` resolves `${file:path}` references to the contents of a file, for example `${file:./key.pem}`. It is written once against the base `plugin` interfaces, so the same plugin works in every textdata format (dotenv, jsonc, and later ones).

```sh
go get github.com/ubgo/textdata/plugins/file
```

Requires Go 1.24 or newer (it reads through `os.Root`).

## Example

```go
import (
	"path/filepath"

	"github.com/ubgo/textdata/jsonc"
	fileplugin "github.com/ubgo/textdata/plugins/file"
)

path := "config/app.jsonc" // {"tls": {"key": "${file:./key.pem}"}}
dir := filepath.Dir(path)

p := fileplugin.New(dir) // refs are confined to dir by default
doc, _ := jsonc.Open(path, jsonc.WithPlugin(p))
key, err := doc.GetExpanded("tls.key")
```

The same instance can be installed in a dotenv file with `dotenv.WithPlugin(p)`. The value lives in memory only: `Render` and every save write the reference back.

## Guarantees

- Read-only. It reads only when a format asks it to resolve a reference it owns.
- Relative refs resolve against the base directory given to `New` (normally the document's folder), made absolute once at construction: a later `os.Chdir` changes nothing. `New("")` means the working directory at construction.
- Confined by default: the root is the base directory. A ref that leaves the root is refused with `plugin.ErrDenied` before any byte is read: absolute refs (even ones inside the root), `..` climbing out, and symlinks resolving outside. The read itself goes through an `os.Root`, so a symlink swapped in after the check cannot escape either. Symlinks that stay inside the root are followed. `WithRoot` moves the root; `Unconfined()` turns confinement off.
- Only regular files are read: a directory, FIFO, device or socket is refused with `ErrNotRegular` (a FIFO could block forever).
- A file larger than the size limit is an error wrapping `ErrTooLarge`, never a truncated value.
- A missing file wraps `plugin.ErrNotFound`; a permission error wraps `plugin.ErrDenied`. Errors name the ref as written in the document.

## Options

| Option | Default | What |
|---|---|---|
| `WithRoot(dir)` | the base directory | confine refs to this tree instead; a relative root resolves against the base directory (`WithRoot("..")` lets a document reach its parent folder) |
| `Unconfined()` | off | no confinement: any readable path, absolute refs and symlinks included. For trusted documents only |
| `WithMaxSize(n)` | 1 MiB | largest file read, in bytes |
| `WithTrimNewline(bool)` | on | remove one trailing `\n` or `\r\n`; turn off for the exact bytes |

The last of `WithRoot` and `Unconfined()` wins. A trusted file that needs `${file:../shared/key.pem}` gets `WithRoot("..")`, which is narrower than `Unconfined()`. With confinement on (the default) the base directory must exist, or every Resolve reports it.

`Read(ref)` returns a value without a document, through the same rules.
