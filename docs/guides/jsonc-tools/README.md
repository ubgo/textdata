# jsonc tools

Four packages inside the `jsonc` module that work on whole documents. They need no extra `go get`: `github.com/ubgo/textdata/jsonc` already contains them.

| Guide | Package | What you learn | Example |
|---|---|---|---|
| [format.md](format.md) | `jsonc/format` | `Format` with a `Style`, `DefaultStyle`, a formatted-or-not check, `Minify` to plain JSON | [`format`](../../../examples/jsonc-tools/format/) |
| [diff.md](diff.md) | `jsonc/diff` | `Diff` by path, `Change` and `Kind`, `Equal` | [`diff`](../../../examples/jsonc-tools/diff/) |
| [generate.md](generate.md) | `jsonc/generate` | `FromStruct` with comment tags, `New` with objects, arrays and comments, the generator options | [`generate-from-struct`](../../../examples/jsonc-tools/generate-from-struct/) |
| [schema.md](schema.md) | `jsonc/schema` | `FromStruct`, `Validate` with `file:line:col` issues, `Parse`/`Load`, `Plugin` as a Validator, refusing a save | [`schema-plugin`](../../../examples/jsonc-tools/schema-plugin/) |

From the shell, the same tools are `textdatactl jsonc fmt`, `diff`, `to-json` and `validate` ([jsonc CLI](../cli/jsonc-cli.md)).
