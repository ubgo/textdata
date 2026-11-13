# Disable and restore a member

`Disable(path)` comments an object member out, turning each of its lines into a `// ` line, so the setting no longer counts but stays in the file where a person can see it and switch it back on. `Restore(path)` brings it back byte for byte. Use the pair for "turn this feature off for now" from a tool, the way people do it by hand, instead of deleting a value and losing what it was.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config), which edits this file:

<!-- ds:block id=before-hqbcqwjn -->
**before** · [`examples/jsonc/edit-config/main.go:26-38`](../../../examples/jsonc/edit-config/main.go#L26-L38)

```go
`{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "interval": 30,
  },

  /* feature switches */
  "features": {
    "beta": false,
  },
}
`
```
<!-- /ds:block hash=226c25 -->

## Step 1 — Disable a member

The member must sit on lines of its own, with its comma (if any) on the value's last line. Everything else in the file is left alone.

<!-- ds:block id=disablekey-xhvfs3up -->
**disableKey** · [`examples/jsonc/edit-config/main.go:122-124`](../../../examples/jsonc/edit-config/main.go#L122-L124)

```go
func disableKey(doc *jsonc.Doc) error {
	return doc.Disable("sync.interval")
}
```
<!-- /ds:block hash=216866 -->

In the example `sync.interval` was first set to `60`, so after `Disable` the `sync` object reads:

```jsonc
  "sync": {
    "remote": "origin", // git remote name
    "branch": "main",
    // "interval": 60,
  },
```

From now on `Has("sync.interval")` is `false`, `Get` returns `ErrNotFound`, and `JSON()` and `Decode` leave it out, as with any comment. A member that spans several lines gets `// ` on each line, after its indentation.

## Step 2 — Restore it

`Restore(path)` looks for a disabled member with that key in the parent object and removes the `// ` marks it added, so the bytes are exactly what they were before `Disable`. A fuzz test pins that `Disable` then `Restore` is byte-identical. A second `Restore` of a live key is `ErrExists`.

<!-- ds:block id=restorekey-j3679hwq -->
**restoreKey** · [`examples/jsonc/edit-config/main.go:129-140`](../../../examples/jsonc/edit-config/main.go#L129-L140)

```go
func restoreKey(w io.Writer, doc *jsonc.Doc) error {
	if err := doc.Restore("sync.interval"); err != nil {
		return err
	}
	interval, err := doc.GetInt("sync.interval")
	if err != nil {
		return err
	}
	again := doc.Restore("sync.interval")
	fmt.Fprintln(w, "restored interval:", interval, "| second Restore is ErrExists:", errors.Is(again, jsonc.ErrExists))
	return nil
}
```
<!-- /ds:block hash=cdc678 -->

## Try it

```sh
cd examples/jsonc/edit-config && go run .
```

```text
--- after edits ---
{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "branch": "main",
    // "interval": 60,
  },

  /* feature switches */
  "features": {
    "alpha": true,
  },
  "log": {
    "level": "info"
  },
}
restored interval: 60 | second Restore is ErrExists: true
saved app.jsonc, mode -rw-r-----
first run: existed=false, created new.jsonc mode -rw-------
{
  "port": 8080
}
link still a symlink: true | Path after SaveAs: link.jsonc
no path: jsonc: no path set; use Open, or SetPath before Save
after SetPath, saved from-bytes.jsonc
refused: jsonc: plugin "no-plaintext-secret": guard save guarded.jsonc: plaintext secret; write a ${scheme:ref} reference instead
is errPlaintextSecret: true | file written: false
```

The `// "interval": 60,` line in the first block is the disabled member, and `restored interval: 60 | second Restore is ErrExists: true` is Step 2. The lines after it belong to the [save](save.md) guide.

## Pitfalls

- **Only object members can be disabled.** An array element is `ErrType` (`is an array element; Disable works on object members`); use `Remove` instead.
- **The member must own its lines.** `{"a": 1, "b": 2}` on one line is `ErrLayout` (`value does not occupy its own lines`): commenting out part of a line would comment out its neighbours too. A comma alone on a later line is also `ErrLayout`, because it would read back as a separate `// ,` line that `Restore` could not tell from one a person wrote.
- **`Restore` with nothing to restore is `ErrNotFound`** (`no disabled "zz"`), and with the key live again it is `ErrExists`; neither changes the file.
- **Strict JSON has no comments**, so under `Strict()` `Disable` returns `ErrStrict`.
- **A disabled member is invisible to every read**, `Lint` and `Merge` included. To change its value, `Restore`, `Set`, then `Disable` again.

## Related

- [Edit](edit.md) · [Save](save.md) · [Lint and validate](lint-validate.md)
- Module README: [jsonc § Disable and Restore](../../../jsonc/README.md#disable-and-restore)
- CLI: `textdatactl jsonc unset` and `restore` wrap these two calls ([jsonc CLI guide](../cli/jsonc-cli.md))
- Example: [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
