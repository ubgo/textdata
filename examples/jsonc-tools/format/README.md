# jsonc format

Re-lays out a hand-edited JSONC config in one style without losing a comment, checks whether a file is already formatted, and minifies it to plain JSON, with `github.com/ubgo/textdata/jsonc/format`. Nothing is read from or written to disk.

It shows:

- `format.Format` with `format.DefaultStyle()`: two spaces, no trailing commas, a final newline; the comment that sat between a key and its value moves above the member.
- A custom `format.Style{Indent: 4, TrailingCommas: true}`.
- The `fmt --check` rule: a file is formatted when `Format` returns its own bytes.
- `format.Minify`: compact plain JSON with comments and trailing commas dropped.
- `Format` refusing a document with syntax errors, with `file:line:col`.

## Run

```sh
cd examples/jsonc-tools/format && go run .
```

Expected output (pinned by `main_test.go` against `testdata/expected.txt`):

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

Guide: [Format a JSONC file](../../../docs/guides/jsonc-tools/format.md).
