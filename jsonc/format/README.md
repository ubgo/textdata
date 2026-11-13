# jsonc/format

Re-indent and tidy a JSONC document in one consistent style without losing a comment, or minify it to plain JSON. Part of the `github.com/ubgo/textdata/jsonc` module; stdlib only.

## Install

```sh
go get github.com/ubgo/textdata/jsonc
```

## Example

```go
d, err := jsonc.Open("settings.jsonc")
if err != nil {
	return err
}
out, err := format.Format(d, format.Style{Indent: 4, TrailingCommas: true})
if err != nil {
	return err // jsonc.ErrInvalid on syntax errors
}
min, _ := format.Minify(d) // {"a":1,...}
```

Format returns bytes and never changes the document; parse the result (`jsonc.Parse(out)`) to keep editing it, or write it out.

## Style

| Field | Zero value | Meaning |
|---|---|---|
| `Indent` | 2 | spaces per level (below 1 means 2); ignored with `Tabs` |
| `Tabs` | false | one tab per level |
| `TrailingCommas` | false | comma after the last member or element of every non-empty container; ignored for a `Strict()` document |
| `NoFinalNewline` | false | leave out the newline after the last line |

`DefaultStyle()` is the zero style spelled out: two spaces, no trailing commas, a final newline, which is what common editors write.

## Layout

- Every member and element on a line of its own, `"key": value`; empty containers stay `{}` / `[]`.
- Comment lines above a member stay above it; a comment after a member on its line stays there, after the comma.
- One blank line is kept where the source had one or more between members; none at the top of a container.
- The document's line ending (LF or CRLF) and BOM are kept.
- Comments in odd spots move to the nearest line they can own: one between a key and its value moves above the member, one before a comma moves after it. `jsonc.Lint` reports these spots (`odd-comment`).
- Strings and numbers are written exactly as found (escapes and spelling kept).

## Guarantees (pinned by tests and fuzzing)

- Every comment is kept with its text (a line comment loses only trailing blanks).
- The meaning is unchanged: `json.Compact` of `JSON()` is byte-equal before and after, so key order and duplicate keys survive too.
- The output re-parses with no errors under the document's own options.
- Idempotent: `Format(Format(x)) == Format(x)`.
- A `Strict()` document never gains a comment or a trailing comma.
- `Minify(d)` is byte-equal to `json.Compact(d.JSON())`.
- A document with syntax errors is refused with an error wrapping `jsonc.ErrInvalid`.
