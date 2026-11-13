# Edit a JSONC document

The jsonc edit API changes a config the way a careful person would by hand: it touches only the bytes of the value you set, add or remove, and keeps every comment, blank line, key order, indentation, trailing comma and line ending elsewhere. Use it when a tool writes to a file people also edit: a CLI `config set`, a migration that renames a key, an installer that adds a section.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from two runnable programs the test suite runs: [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config) (object members) and [`examples/jsonc/arrays-comments`](../../../examples/jsonc/arrays-comments) (array elements and comments). How edits work: each one splices the source bytes and re-parses, so "only the touched bytes change" holds by construction, and a fuzz test (`Set` then `Delete` returns the original bytes) pins it.

## The promise, concretely

This is the file a person wrote:

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

After `Set`, `SetAfter`, `SetBefore`, `Delete` and `Disable` from the steps below, `doc.Render()` is:

```jsonc
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
```

Both comments, the blank line between the sections, the trailing commas and the key order are still there. The only changed bytes are the new `branch`, `alpha` and `log` members, the disabled `interval` line, and the removed `beta` line.

## Step 1 — Set values

`Set(path, value)` replaces an existing value in place, or creates it at the end of its object in that object's indentation and comma style, creating any missing parent objects on the way (`log.level` creates `log`). Values are encoded with `encoding/json`.

<!-- ds:block id=setvalues-fuvbnztb -->
**setValues** · [`examples/jsonc/edit-config/main.go:93-98`](../../../examples/jsonc/edit-config/main.go#L93-L98)

```go
func setValues(doc *jsonc.Doc) error {
	if err := doc.Set("sync.interval", 60); err != nil {
		return err
	}
	return doc.Set("log.level", "info")
}
```
<!-- /ds:block hash=70f6ac -->

## Step 2 — Place a new key next to a sibling

`SetAfter(anchor, path, value)` and `SetBefore` put a new key right after or before the sibling named `anchor` (a bare key in the same object), so related settings stay together. For a key that already exists they behave like `Set` and update it where it is. The parent object must exist.

<!-- ds:block id=placekeys-95kby248 -->
**placeKeys** · [`examples/jsonc/edit-config/main.go:104-109`](../../../examples/jsonc/edit-config/main.go#L104-L109)

```go
func placeKeys(doc *jsonc.Doc) error {
	if err := doc.SetAfter("remote", "sync.branch", "main"); err != nil {
		return err
	}
	return doc.SetBefore("beta", "features.alpha", true)
}
```
<!-- /ds:block hash=66d0df -->

## Step 3 — Delete a member

`Delete(path)` removes the value with its key, its attached comments and its comma. It works on array elements too (`Delete("hosts[1]")`). Deleting a key that `Set` just created gives back the original bytes.

<!-- ds:block id=deletekey-tq94ac46 -->
**deleteKey** · [`examples/jsonc/edit-config/main.go:114-116`](../../../examples/jsonc/edit-config/main.go#L114-L116)

```go
func deleteKey(doc *jsonc.Doc) error {
	return doc.Delete("features.beta")
}
```
<!-- /ds:block hash=743af0 -->

To keep a member in the file but switch it off, use `Disable` instead; see [disable and restore](disable-restore.md).

## Step 4 — Edit arrays

`Set` never creates array elements; `Append(path, v)` adds one at the end, `Insert(path, i, v)` at index `i` (`0 <= i <= length`), and `Remove(path, i)` deletes element `i` with its comma. `Set("hosts[0]", v)` replaces an existing element. The example starts from this file:

<!-- ds:block id=src-v5375csv -->
**src** · [`examples/jsonc/arrays-comments/main.go:18-26`](../../../examples/jsonc/arrays-comments/main.go#L18-L26)

```go
`{
  // Hosts tried in order
  "hosts": [
    "a.internal", // primary
    "b.internal",
  ],
  "retries": 3,
}
`
```
<!-- /ds:block hash=6bff20 -->

<!-- ds:block id=growarray-wk9rj7jt -->
**growArray** · [`examples/jsonc/arrays-comments/main.go:62-67`](../../../examples/jsonc/arrays-comments/main.go#L62-L67)

```go
func growArray(doc *jsonc.Doc) error {
	if err := doc.Append("hosts", "c.internal"); err != nil {
		return err
	}
	return doc.Insert("hosts", 0, "local")
}
```
<!-- /ds:block hash=cc1cae -->

<!-- ds:block id=shrinkarray-pwtb9zs9 -->
**shrinkArray** · [`examples/jsonc/arrays-comments/main.go:72-74`](../../../examples/jsonc/arrays-comments/main.go#L72-L74)

```go
func shrinkArray(doc *jsonc.Doc) error {
	return doc.Remove("hosts", 2)
}
```
<!-- /ds:block hash=0d2730 -->

## Step 5 — Insert a value verbatim with Raw

A `jsonc.Raw` value is JSONC text written as is, so comments and layout inside it survive (an ordinary Go value is re-encoded by `encoding/json`). It must parse as exactly one value. `Set`, `Append` and `Insert` all accept it.

<!-- ds:block id=appendraw-x7zj7axp -->
**appendRaw** · [`examples/jsonc/arrays-comments/main.go:80-82`](../../../examples/jsonc/arrays-comments/main.go#L80-L82)

```go
func appendRaw(doc *jsonc.Doc) error {
	return doc.Append("hosts", jsonc.Raw(`{"host": "d.internal" /* canary */}`))
}
```
<!-- /ds:block hash=e57726 -->

## Step 6 — Set and remove comments

`SetComment(path, text)` sets the comment above a value, replacing what was there; each line of `text` becomes a `// ` line. For a value that shares its line with others it writes a `/* */` block in front instead. `RemoveComment(path)` removes the leading and trailing comments of a value. `Comment(path)` reads them back without the markers.

<!-- ds:block id=editcomments-zqx5vg3x -->
**editComments** · [`examples/jsonc/arrays-comments/main.go:88-96`](../../../examples/jsonc/arrays-comments/main.go#L88-L96)

```go
func editComments(doc *jsonc.Doc) error {
	if err := doc.SetComment("retries", "How often to retry\nbefore giving up"); err != nil {
		return err
	}
	if err := doc.SetComment("hosts", "Hosts tried in order, local first"); err != nil {
		return err
	}
	return doc.RemoveComment("hosts[1]")
}
```
<!-- /ds:block hash=723352 -->

<!-- ds:block id=readcomment-t7x4xa8a -->
**readComment** · [`examples/jsonc/arrays-comments/main.go:101-108`](../../../examples/jsonc/arrays-comments/main.go#L101-L108)

```go
func readComment(w io.Writer, doc *jsonc.Doc) error {
	c, err := doc.Comment("retries")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "comment of retries: %q\n", c)
	return nil
}
```
<!-- /ds:block hash=3799f8 -->

## Try it

```sh
cd examples/jsonc/arrays-comments && go run .
```

```text
--- append and insert ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "b.internal",
    "c.internal",
  ],
  "retries": 3,
}
--- remove ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "c.internal",
  ],
  "retries": 3,
}
--- raw value ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "c.internal",
    {"host": "d.internal" /* canary */},
  ],
  "retries": 3,
}
--- comments ---
{
  // Hosts tried in order, local first
  "hosts": [
    "local",
    "a.internal",
    "c.internal",
    {"host": "d.internal" /* canary */},
  ],
  // How often to retry
  // before giving up
  "retries": 3,
}
comment of retries: "How often to retry\nbefore giving up"
```

The object edits and their output are in [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config) (`cd examples/jsonc/edit-config && go run .`); the before and after above are that program's file and output.

## Pitfalls

- **Every edit refuses on a document with syntax errors** (`ErrInvalid`) and leaves it unchanged; check `Parse`'s error first.
- **`Set` does not grow arrays.** `Set("hosts[3]", v)` past the end is `ErrNotFound` with the hint `Set does not create array elements; use Append or Insert`.
- **`Set` cannot create through a scalar.** `Set("name.first", v)` when `name` is a string is `ErrType` (`cannot create "name.first" inside a string`); `Delete` or replace `name` first.
- **A Go map is written with sorted keys and no comments**, because `encoding/json` encodes it. For a nested value whose key order or comments matter, pass a `jsonc.Raw` string or `Set` the members one by one.
- **`SetAfter` needs both the parent and the anchor.** A missing parent is `ErrNotFound` naming the parent; a missing anchor is `ErrNotFound` naming the anchor.
- **A `Raw` value must be one value.** `Raw("1 2")` is refused with `Raw value is not one valid value`.
- **Under `Strict()` anything that writes a comment is refused:** `SetComment` and `Disable` return `ErrStrict`.
- **Duplicate keys: edits go to the last member** with that key, the one readers see; the earlier ones stay as written.
- **A `Doc` is not safe for concurrent edits.** Only `Pos` and `KeyPos` are documented as safe from several goroutines, and only on a document nobody is editing; guard everything else with your own lock.

## Related

- [Disable and restore](disable-restore.md) · [Merge and clone](merge-clone.md) · [Save](save.md) · [Read and paths](read-and-paths.md)
- Module README: [jsonc § Editing](../../../jsonc/README.md#editing)
- Examples: [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config), [`examples/jsonc/arrays-comments`](../../../examples/jsonc/arrays-comments)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
