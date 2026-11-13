# Format a JSONC file

`github.com/ubgo/textdata/jsonc/format` re-lays out a JSONC document in one consistent style and keeps every comment, or minifies it to plain JSON. Use it for a `fmt` command, a pre-commit or CI check that config files are tidy, or to hand a commented config to a program that only reads JSON. It never changes what the file means: key order, duplicate keys, number spelling and string escapes all survive.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

`format` is a package inside the `jsonc` module (standard library plus the stdlib-only textdata base). You need a parsed `*jsonc.Doc`; see the jsonc guides for `jsonc.Parse` and `jsonc.Open`. The runnable program for this page is [`examples/jsonc-tools/format`](../../../examples/jsonc-tools/format).

The input used below is a config as people leave it:

```jsonc
{
  // editor settings
    "tabSize":4, "insertSpaces" : true,


  "files": { "exclude": ["node_modules",".git"], },
  "theme": /* dark by default */ "dark"
}
```

## Step 1 — Format in the default style

`format.DefaultStyle()` is the layout common editors write: two spaces per level, no trailing commas, a final newline. It is the zero `Style` spelled out. `Format` returns new bytes and leaves the document as it was; parse the result with `jsonc.Parse` if you want to keep editing it, or write it out.

<!-- ds:block id=jt-fmt-default-qrdpm99e -->
**formatDefault** · [`examples/jsonc-tools/format/main.go:85-87`](../../../examples/jsonc-tools/format/main.go#L85-L87)

```go
func formatDefault(d *jsonc.Doc) ([]byte, error) {
	return format.Format(d, format.DefaultStyle())
}
```
<!-- /ds:block hash=07ef47 -->

Every member and element gets a line of its own, `"key": value` with one space, and the two blank lines collapse to one. The block comment that sat between `"theme":` and its value moves above the member, the nearest line it can own (see Pitfalls).

## Step 2 — Choose a style

`Style` has four fields. All of them are optional; the zero value is the default style.

| Field | Zero value | Meaning |
|---|---|---|
| `Indent` | 2 | spaces per level; below 1 means 2; ignored with `Tabs` |
| `Tabs` | false | one tab per level |
| `TrailingCommas` | false | a comma after the last member or element of every non-empty container; ignored for a `jsonc.Strict()` document |
| `NoFinalNewline` | false | leave out the newline after the last line |

<!-- ds:block id=jt-fmt-custom-kpa5fn3a -->
**formatCustom** · [`examples/jsonc-tools/format/main.go:93-96`](../../../examples/jsonc-tools/format/main.go#L93-L96)

```go
func formatCustom(d *jsonc.Doc) ([]byte, error) {
	style := format.Style{Indent: indentWide, TrailingCommas: true}
	return format.Format(d, style)
}
```
<!-- /ds:block hash=6ffdf5 -->

The line ending (LF or CRLF) and a UTF-8 byte order mark are taken from the document, not from the style.

## Step 3 — Check that a file is formatted

A file is formatted when `Format` returns its own bytes. `Format` is idempotent (`Format(Format(x)) == Format(x)`, fuzzed), so its own output always passes. This is the rule behind `textdatactl jsonc fmt --check`, which prints the file name and exits 1 when the file would change.

<!-- ds:block id=jt-fmt-check-bptawujv -->
**isFormatted** · [`examples/jsonc-tools/format/main.go:103-113`](../../../examples/jsonc-tools/format/main.go#L103-L113)

```go
func isFormatted(src []byte) (bool, error) {
	d, err := jsonc.Parse(src, jsonc.WithFileName(fileName))
	if err != nil {
		return false, err
	}
	out, err := format.Format(d, format.DefaultStyle())
	if err != nil {
		return false, err
	}
	return bytes.Equal(out, src), nil
}
```
<!-- /ds:block hash=f8e252 -->

## Step 4 — Minify to plain JSON

`Minify` gives compact JSON with no insignificant whitespace, byte for byte `json.Compact` of `d.JSON()`. Comments and trailing commas are dropped, so the output is for programs that read JSON, never for writing back over the source.

<!-- ds:block id=jt-fmt-minify-8fgnr54y -->
**minify** · [`examples/jsonc-tools/format/main.go:119-121`](../../../examples/jsonc-tools/format/main.go#L119-L121)

```go
func minify(d *jsonc.Doc) ([]byte, error) {
	return format.Minify(d)
}
```
<!-- /ds:block hash=799c90 -->

## Step 5 — Handle a file with syntax errors

`Format` and `Minify` refuse a document with syntax errors instead of guessing at its layout. The error wraps `jsonc.ErrInvalid` (test with `errors.Is`) and names `file:line:col`.

<!-- ds:block id=jt-fmt-invalid-mkh66wj7 -->
**formatInvalid** · [`examples/jsonc-tools/format/main.go:127-133`](../../../examples/jsonc-tools/format/main.go#L127-L133)

```go
func formatInvalid() error {
	// Parse always returns a Doc; its syntax error is the input under test,
	// so it is dropped here and Format is left to refuse the Doc.
	d, _ := jsonc.Parse([]byte(`{"port": 80 "host": "a"}`), jsonc.WithFileName(fileName))
	_, err := format.Format(d, format.DefaultStyle())
	return err
}
```
<!-- /ds:block hash=45ade7 -->

## Try it

```sh
cd examples/jsonc-tools/format && go run .
```

```text
== DefaultStyle
{
  // editor settings
  "tabSize": 4,
  "insertSpaces": true,

  "files": {
    "exclude": [
      "node_modules",
      ".git"
    ]
  },
  /* dark by default */
  "theme": "dark"
}
== Indent 4, TrailingCommas
{
    // editor settings
    "tabSize": 4,
    "insertSpaces": true,

    "files": {
        "exclude": [
            "node_modules",
            ".git",
        ],
    },
    /* dark by default */
    "theme": "dark",
}
== check
messy formatted: false
tidy formatted: true
== Minify
{"tabSize":4,"insertSpaces":true,"files":{"exclude":["node_modules",".git"]},"theme":"dark"}
== invalid input
jsonc: document has syntax errors: settings.jsonc:1:13: missing comma before '"'
```

The same from the command line:

```sh
textdatactl jsonc fmt settings.jsonc            # rewrite in place, atomically, only when it changes
textdatactl jsonc fmt settings.jsonc --check    # CI: print the name and exit 1 when unformatted
textdatactl jsonc fmt settings.jsonc --indent 4 --trailing-commas --dry-run
```

## Pitfalls

- **`Format` does not change the document.** It returns bytes; `d.Render()` still gives the old text and a later `d.Save()` writes the old layout. Parse the output (`jsonc.Parse(out)`) or write the bytes yourself.
- **Comments in odd spots move.** A comment between a key and its value moves above the member; a comment before a comma moves after it. The text is kept, the position is not. `jsonc.Lint` reports these spots as `odd-comment` if you want to fix them by hand first.
- **Blank lines are normalised.** One blank line is kept where the source had one or more between members; none is kept at the top of a container. Vertical grouping beyond that is lost.
- **`TrailingCommas` is ignored for a strict document.** A document parsed with `jsonc.Strict()` never gains a comment or a trailing comma, so formatting plain JSON keeps it plain JSON (pinned by the package's strict-format test).
- **Minify is lossy.** Comments and trailing commas are gone; never save minified output over a hand-edited file.
- **Invalid input is refused, not repaired.** Both functions return an error wrapping `jsonc.ErrInvalid`; fix the syntax error at the reported position first.
- **Line endings come from the file.** The first line break decides LF or CRLF; there is no style field to convert between them.

## Related

- [Diff two JSONC files](diff.md), [Generate a JSONC file](generate.md), [Validate with a JSON Schema](schema.md)
- Package reference: [jsonc/format README](../../../jsonc/format/README.md) (layout rules and guarantees)
- CLI: [jsonc/cli README § verbs](../../../jsonc/cli/README.md#verbs) (`fmt`)
- Example: [`examples/jsonc-tools/format`](../../../examples/jsonc-tools/format)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc/format](https://pkg.go.dev/github.com/ubgo/textdata/jsonc/format)
