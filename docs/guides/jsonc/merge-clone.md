# Merge and clone documents

`Merge(other)` deep-merges one JSONC document into another for the classic layered config: a shared base file plus a local override. Objects merge by key, everything else in the override replaces the base's value whole, and the result keeps the base's comments and layout. `Clone()` gives you an independent copy first, so the base stays as it was. Use them when you build a config from several files in code; for files that name their own bases, `$extends` does the same merge for you ([extends](extends.md)).

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from [`examples/jsonc/merge-layers`](../../../examples/jsonc/merge-layers). Its two layers are a base with the team's comments:

<!-- ds:block id=basesrc-4jzfdp6r -->
**baseSrc** · [`examples/jsonc/merge-layers/main.go:20-28`](../../../examples/jsonc/merge-layers/main.go#L20-L28)

```go
`{
  // HTTP listener
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
  },
  "features": ["auth", "audit"],
}
`
```
<!-- /ds:block hash=945652 -->

and one machine's override, which changes a nested value, replaces an array and adds a key with a comment:

<!-- ds:block id=overridesrc-cnk5hmx8 -->
**overrideSrc** · [`examples/jsonc/merge-layers/main.go:34-40`](../../../examples/jsonc/merge-layers/main.go#L34-L40)

```go
`{
  "server": {"port": 9090},
  "features": ["auth"],
  // Local tracing collector
  "tracing": "http://localhost:4318",
}
`
```
<!-- /ds:block hash=f2124c -->

## Step 1 — Clone the base and merge the override

`Clone()` is deep: its own bytes, tree, errors, path and options. Merging into the clone leaves `base` usable for the next machine, or for saving unchanged.

<!-- ds:block id=mergelayers-nrqw7asf -->
**mergeLayers** · [`examples/jsonc/merge-layers/main.go:74-80`](../../../examples/jsonc/merge-layers/main.go#L74-L80)

```go
func mergeLayers(base, override *jsonc.Doc) (*jsonc.Doc, error) {
	merged := base.Clone()
	if err := merged.Merge(override); err != nil {
		return nil, err
	}
	return merged, nil
}
```
<!-- /ds:block hash=31f74a -->

The merged document renders as:

```jsonc
{
  // HTTP listener
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
  },
  "features": [
    "auth"
  ],
  // Local tracing collector
  "tracing": "http://localhost:4318",
}
```

What happened, rule by rule:

| In the override | Result |
|---|---|
| an object (`server`) | merged key by key: `port` replaced, `host` kept, `// HTTP listener` kept |
| an array (`features`) | replaces the base array whole; arrays are never merged element by element |
| a scalar | replaces the base value |
| a key the base lacks (`tracing`) | added at the end of its object in the base's indentation and comma style, with the override's `//` comment lines above it when it lands on a line of its own |
| a value equal to the base's (as JSON) | left alone, bytes untouched |

## Step 2 — Merge again safely

Because equal values are left alone, merging the same override twice changes nothing, so a merge can run on every start without the file drifting.

<!-- ds:block id=mergeagain-fmwspgqs -->
**mergeAgain** · [`examples/jsonc/merge-layers/main.go:86-93`](../../../examples/jsonc/merge-layers/main.go#L86-L93)

```go
func mergeAgain(w io.Writer, merged, override *jsonc.Doc) error {
	before := merged.Render()
	if err := merged.Merge(override); err != nil {
		return err
	}
	fmt.Fprintln(w, "second merge is a no-op:", bytes.Equal(before, merged.Render()))
	return nil
}
```
<!-- /ds:block hash=bb40e2 -->

## Step 3 — Know when Merge refuses

`Merge` works on a copy and only commits when every step succeeded, so a refusal leaves the target as it was. It refuses an override with syntax errors (`ErrInvalid`) and a root that is not an object (`ErrType`).

<!-- ds:block id=refusebroken-6qvehjs8 -->
**refuseBroken** · [`examples/jsonc/merge-layers/main.go:99-114`](../../../examples/jsonc/merge-layers/main.go#L99-L114)

```go
func refuseBroken(w io.Writer, merged *jsonc.Doc) error {
	// Parse always returns a Doc; broken's syntax error is the input under
	// test, so it is dropped here and Merge is left to refuse it.
	broken, _ := jsonc.Parse([]byte(`{"server": {port: 1}}`))
	array, err := jsonc.Parse([]byte(`[1, 2]`))
	if err != nil {
		return err
	}
	before := merged.Render()
	errBroken := merged.Merge(broken)
	errArray := merged.Merge(array)
	fmt.Fprintln(w, "broken is ErrInvalid:", errors.Is(errBroken, jsonc.ErrInvalid))
	fmt.Fprintln(w, "array root is ErrType:", errors.Is(errArray, jsonc.ErrType))
	fmt.Fprintln(w, "target unchanged:", bytes.Equal(before, merged.Render()))
	return nil
}
```
<!-- /ds:block hash=b976dc -->

## Try it

```sh
cd examples/jsonc/merge-layers && go run .
```

```text
--- merged ---
{
  // HTTP listener
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
  },
  "features": [
    "auth"
  ],
  // Local tracing collector
  "tracing": "http://localhost:4318",
}
base unchanged: true
second merge is a no-op: true
broken is ErrInvalid: true
array root is ErrType: true
target unchanged: true
```

## Pitfalls

- **Merge changes the receiver.** `base.Merge(override)` edits `base`; clone first when you still need the base.
- **A clone keeps the original's path.** A clone of a document from `Open` saves over that same file; call `SetPath` (or `SaveAs`) before saving a merged view, or you bake the override into the shared base.
- **Plugins are shared, not copied.** The clone's plugin lists are its own, but the plugin values are the ones you passed in; a plugin with internal state sees both documents.
- **Arrays replace, never append.** An override with `"features": ["auth"]` drops `"audit"`. To add to a list, read both and `Set` the combined array yourself.
- **Copied values are re-laid out.** A value taken from the override is re-indented in the base's style and loses comments inside it (the `features` array above became multi-line); key order inside it is kept. Only the leading `//` comment lines of a newly added key travel with it, and never under `Strict()`.
- **Duplicate keys follow `encoding/json`.** The override's last member with a key is the one merged, and it lands on the base's last member with that key.
- **An empty override is a no-op; an empty base becomes a copy of the override's root object.**
- **`Merge` cannot remove a key.** There is no "delete" marker in an override; call `Delete` on the merged document.

## Related

- [Extends](extends.md) (the same merge, driven by `$extends`) · [Edit](edit.md) · [Save](save.md)
- Module README: [jsonc § Merge and Clone](../../../jsonc/README.md#merge-and-clone)
- Example: [`examples/jsonc/merge-layers`](../../../examples/jsonc/merge-layers)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
