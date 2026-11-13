# Edit and save a config file

Most config libraries read a file into a map and, if they write at all, write the map back: comments, blank lines, key order and quoting are gone. textdata edits the file in place instead. You open it, change a value, and save; only the bytes of what you changed differ, and the write is atomic. Use this whenever a program updates a file a person also edits: a setup wizard, a migration, a `config set` command, a deploy script.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
go get github.com/ubgo/textdata/jsonc@latest
```

Read [Install and read your first file](install-and-first-read.md) first; this page uses the same two files. The runnable program is [`examples/getting-started/edit-save`](../../../examples/getting-started/edit-save). It writes the same two files into a temp directory before editing, so running it changes nothing in the repository.

## Step 1 — Edit a .env file and save it

Open the file, then edit it:

- `Set(key, value)` changes a value, or appends `KEY=value` when the key is new. Only the value's bytes change: the `export` prefix, the spacing around `=`, and the inline comment after the value stay.
- `SetAfter(anchor, key, value)` and `SetBefore` place a new key next to a related one instead of at the end.
- `Unset(key, false)` comments the entry out (`# DB_HOST=localhost`), so it can come back with `Restore(key)`; `Unset(key, true)` deletes it.
- `Save()` writes back to the path it was opened from; `SaveAs(path)` writes a copy elsewhere.

<!-- ds:block id=editenv-8qgna9mt -->
**editEnv** · [`examples/getting-started/edit-save/main.go:77-89`](../../../examples/getting-started/edit-save/main.go#L77-L89)

```go
func editEnv(path string) error {
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	f.Set("DB_PORT", "6543")   // only the value's bytes change
	f.Set("LOG_LEVEL", "info") // the export prefix and comment stay
	if _, err := f.SetAfter("DB_PORT", "DB_NAME", "app"); err != nil {
		return err
	}
	f.Unset("DB_HOST", false) // commented out, not deleted
	return f.Save()
}
```
<!-- /ds:block hash=6fe38c -->

## Step 2 — Edit a JSONC file and save it

The JSONC document works the same way, by path. `Set` replaces a value or creates a missing member (and any missing parent objects) at the end of its object, in that object's own layout. `Append` adds an array element; `Delete` removes a member or element with its comments. Values are encoded with `encoding/json`, so pass Go values (`9090`, `true`, a struct), or a `jsonc.Raw` to write text verbatim.

<!-- ds:block id=editconfig-3sybqwqh -->
**editConfig** · [`examples/getting-started/edit-save/main.go:95-110`](../../../examples/getting-started/edit-save/main.go#L95-L110)

```go
func editConfig(path string) error {
	doc, err := jsonc.Open(path)
	if err != nil {
		return err
	}
	if err := doc.Set("server.port", 9090); err != nil {
		return err
	}
	if err := doc.Set("server.tls", true); err != nil {
		return err
	}
	if err := doc.Append("features", "audit"); err != nil {
		return err
	}
	return doc.Save()
}
```
<!-- /ds:block hash=74dadd -->

## Try it

```sh
cd examples/getting-started/edit-save && go run .
```

```text
--- .env
# Database settings for local development
# DB_HOST=localhost
DB_PORT=6543
DB_NAME=app

export LOG_LEVEL=info # quoted, with an inline comment
--- config.jsonc
{
  // Where the API listens.
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
    "tls": true,
  },
  /* Features switched on in this environment. */
  "features": ["search", "billing", "audit"],
}
```

Compare with the input on [the previous page](install-and-first-read.md#before-you-start): both comments, the blank line, the `export` prefix, the inline comment, the trailing commas and the one-line array all survive.

## Pitfalls

- **A changed value is re-quoted canonically.** dotenv writes a new value bare when that reads back the same, and quotes it otherwise; `LOG_LEVEL="debug"` became `LOG_LEVEL=info` above. The inline comment is kept verbatim, even when (as here) it no longer describes the value. Setting the value a key already has is a byte-for-byte no-op.
- **A value containing `$` is stored literally.** `f.Set("PRICE", "cost $5")` writes `PRICE='cost $5'`, single-quoted, so it is never expanded when read back. To store a reference on purpose, edit the file or build the entry yourself; the dotenv README covers this under writing.
- **Only the last occurrence is edited.** With a duplicated key, `Set` and `Unset` act on the last one, because that is the one a reader sees.
- **`jsonc` `Set` does not create array elements.** Setting `features[5]` on a two-element array is an error; use `Append` or `Insert`.
- **`Disable` needs the member on lines of its own.** jsonc's `Disable` (comment a member out, the counterpart of dotenv's `Unset(key, false)`) returns an error for a member that shares a line with another, as `"server": {"host": "a", "port": 1}` does; `Delete` works on any layout.
- **Save keeps the mode, and new files are private.** An existing file keeps its permissions and a symlink keeps pointing at the same file; a new file is created `0600`, because these files often hold secrets. See [Atomic writes](../base/atomicfile.md) for what happens on a crash.
- **A save can be refused.** If a `SaveGuard` plugin is installed, `Save` returns its error and the file on disk is untouched. See [Plugins](../base/plugin.md).

## Related

- [Install and read your first file](install-and-first-read.md)
- [textdatactl](textdatactl.md): `set`, `unset`, `restore` and `--dry-run` from the command line.
- [dotenv README § Writing](../../../dotenv/README.md#writing), [§ Disabled settings](../../../dotenv/README.md#disabled-settings), [§ Saving safely](../../../dotenv/README.md#saving-safely).
- [jsonc README § Editing](../../../jsonc/README.md#editing), [§ Disable and Restore](../../../jsonc/README.md#disable-and-restore), [§ Saving](../../../jsonc/README.md#saving).
- [Atomic writes](../base/atomicfile.md): what `Save` does underneath.
- Example: [`examples/getting-started/edit-save`](../../../examples/getting-started/edit-save).
- API: [dotenv on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/dotenv), [jsonc on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/jsonc).
