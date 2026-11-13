# Parse JSONC and strict JSON

`jsonc.Parse` turns the bytes of a commented config into a `*jsonc.Doc` that you can read, edit and save without losing a single comment, blank line or trailing comma. Use it for any JSON a person edits by hand: `tsconfig.json`, VS Code settings, your own tool's config. The same package reads plain JSON when you add `jsonc.Strict()`, so one API covers both; there is no separate json module.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

You need Go 1.22 or newer. The code on this page comes from the runnable program [`examples/jsonc/read-config`](../../../examples/jsonc/read-config), which the test suite runs on every change.

What JSONC means here: all of JSON (RFC 8259), plus `// line` and `/* block */` comments anywhere whitespace is allowed, trailing commas in objects and arrays, and an optional UTF-8 BOM. LF and CRLF line endings are kept as found. JSON5 is not JSONC: unquoted keys, single quotes, hex numbers, `NaN` and `Infinity` are errors whose message says they are JSON5.

## Step 1 — Parse a commented file

The example file uses every JSONC extension:

<!-- ds:block id=configsrc-38thfgz2 -->
**configSrc** · [`examples/jsonc/read-config/main.go:26-39`](../../../examples/jsonc/read-config/main.go#L26-L39)

```go
`{
  // Public name of the service
  "name": "billing",
  "port": 8080,
  "debug": false,
  "ratio": 0.75,
  /* upstream servers, tried in order */
  "servers": [
    {"host": "a.internal", "tags": {"x.y": true}},
    {"host": "b.internal"},
  ],
  "a/b": "slash key",
}
`
```
<!-- /ds:block hash=ccc9ea -->

`Parse` copies the bytes, so you may reuse your buffer. `WithFileName` gives the bytes a name for error messages and positions; `Open` sets it from the path for you. The round trip is the core promise: `Render` returns the input byte for byte, for every input, valid or not.

<!-- ds:block id=parseconfig-673uqvd2 -->
**parseConfig** · [`examples/jsonc/read-config/main.go:86-93`](../../../examples/jsonc/read-config/main.go#L86-L93)

```go
func parseConfig(w io.Writer) (*jsonc.Doc, error) {
	doc, err := jsonc.Parse([]byte(configSrc), jsonc.WithFileName(configName))
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(w, "round trip exact:", bytes.Equal(doc.Render(), []byte(configSrc)))
	return doc, nil
}
```
<!-- /ds:block hash=63729d -->

Three entry points build a `*Doc`:

| Call | Use it for |
|---|---|
| `Parse(src, opts...)` | bytes you already hold (an embedded file, a test fixture) |
| `ParseReader(r, opts...)` | any `io.Reader`: a request body, an `fs.File` |
| `Open(path, opts...)` | a file on disk that you will `Save` later; a missing file is an empty document, not an error (see [save](save.md)) |

`ParseReader` reads to the end and parses. `Bytes()` is what `Save` would write, the same bytes as `Render()`; it exists so plugins can see the document through the shared `plugin.Document` view.

<!-- ds:block id=parsefromreader-2uj8g78s -->
**parseFromReader** · [`examples/jsonc/read-config/main.go:272-279`](../../../examples/jsonc/read-config/main.go#L272-L279)

```go
func parseFromReader(w io.Writer) error {
	doc, err := jsonc.ParseReader(strings.NewReader(configSrc), jsonc.WithFileName(configName))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "ParseReader, Bytes equals Render:", bytes.Equal(doc.Bytes(), doc.Render()))
	return nil
}
```
<!-- /ds:block hash=6a1d84 -->

## Step 2 — Parse plain JSON with Strict

For a file that must stay plain JSON (a `package.json`, an API payload), add `jsonc.Strict()`. Comments, trailing commas and a BOM become errors, and the parser then accepts exactly what `encoding/json.Valid` accepts (a differential fuzz test pins this). `IsStrict` tells a tool which mode a document was parsed in.

<!-- ds:block id=parsestrict-p6sb4csj -->
**parseStrict** · [`examples/jsonc/read-config/main.go:257-266`](../../../examples/jsonc/read-config/main.go#L257-L266)

```go
func parseStrict(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(plainJSON), jsonc.Strict())
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "strict:", doc.IsStrict())
	_, err = jsonc.Parse([]byte(`{"a": [1, 2,]}`), jsonc.Strict(), jsonc.WithFileName("data.json"))
	fmt.Fprintln(w, "strict error:", err)
	return nil
}
```
<!-- /ds:block hash=008126 -->

## Step 3 — Handle syntax errors

Parsing is tolerant: `Parse` always returns a `*Doc`, even when it also returns an error. The error is a `pos.ErrorList` whose entries read `file:line:col: message`, and `doc.Err()` returns it again later. The document keeps the tree up to the first error, still renders its input unchanged, and refuses edits, `Decode` and `JSON` with `ErrInvalid`.

<!-- ds:block id=showsyntaxerror-gsq5n4wh -->
**showSyntaxError** · [`examples/jsonc/read-config/main.go:285-292`](../../../examples/jsonc/read-config/main.go#L285-L292)

```go
func showSyntaxError(w io.Writer) error {
	src := []byte("{\n  port: 8080\n}")
	doc, err := jsonc.Parse(src, jsonc.WithFileName(configName))
	fmt.Fprintln(w, "syntax error:", err)
	fmt.Fprintln(w, "still renders input:", bytes.Equal(doc.Render(), src))
	fmt.Fprintln(w, "Set is ErrInvalid:", errors.Is(doc.Set("port", 1), jsonc.ErrInvalid))
	return nil
}
```
<!-- /ds:block hash=64c7e6 -->

So a tool can show the error and still hand the file back untouched: `Save` on a document with syntax errors writes it byte for byte.

## Try it

```sh
cd examples/jsonc/read-config && go run .
```

```text
round trip exact: true
name: billing port: 8080 debug: false ratio: 0.75
GetInt(ratio) is ErrType: true
Get(servers[1]): map[host:b.internal]
keys: [name port debug ratio servers a/b]
servers: array of 2 | has timeout: false
GetString(timeout) is ErrNotFound: true
comment of name: "Public name of the service"
comment of servers: "upstream servers, tried in order"
comment of port: ""
paths: a.internal a.internal true slash key
port value at app.jsonc:4:11, key at 4:3
JSON() parses under Strict(): 2 servers
decoded: billing on 8080 via b.internal
strict: true
strict error: data.json:1:12: trailing comma is not allowed in strict JSON
ParseReader, Bytes equals Render: true
syntax error: app.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)
still renders input: true
Set is ErrInvalid: true
Read: billing 8080
```

The lines for this page are `round trip exact`, `strict`, `strict error`, `ParseReader`, `syntax error`, `still renders input` and `Set is ErrInvalid`; the rest belong to [read and paths](read-and-paths.md).

## Pitfalls

- **Check the error, but keep the document.** `Parse` returns a usable `*Doc` next to a non-nil error. Code that does `if err != nil { return }` is fine; code that ignores `err` and edits will get `ErrInvalid` from every edit instead.
- **JSON5 habits are errors.** `{port: 8080}` fails with `object keys must be double-quoted (unquoted keys are JSON5)` at the key's `line:col`; single quotes, hex numbers, `NaN` and `Infinity` fail the same way.
- **Strict forbids anything that needs a comment.** Under `Strict()`, `SetComment` and `Disable` return `ErrStrict`; ordinary `Set`, `Delete` and array edits still work.
- **Strict rejects a BOM** (`1:1: byte order mark is not allowed in strict JSON`); JSONC mode accepts one at the start and keeps it.
- **Duplicate keys are kept, not rejected.** Both members stay in the file; reads and edits use the last one, like `encoding/json`. `Lint` reports them (see [lint and validate](lint-validate.md)).
- **An empty input is a valid, empty document.** `Parse(nil)` returns no error, `JSON()` returns `null`, and the first `Set` creates the root object.
- **`Open` on a misspelt path does not fail.** A missing file is an empty document so first-run code works; check `doc.Existed()` when the file must already be there, or use `jsonc.Read`, which returns an error wrapping `fs.ErrNotExist` for a missing file.

## Related

- [Read values and paths](read-and-paths.md) · [Edit a document](edit.md) · [Save](save.md) · [Lint and validate](lint-validate.md)
- Module README: [jsonc § Format](../../../jsonc/README.md#format), [§ Parsing](../../../jsonc/README.md#parsing), [§ Guarantees](../../../jsonc/README.md#guarantees)
- Example: [`examples/jsonc/read-config`](../../../examples/jsonc/read-config)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
