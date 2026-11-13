# jsonc/cli

The command line for [jsonc](../README.md): read and edit JSON and JSONC files from the shell without disturbing a byte you did not touch. It is mounted in the umbrella binary as `textdatactl jsonc`; this module ships the command package, `jsonccmd`.

Module `github.com/ubgo/textdata/jsonc/cli` (cobra, the textdata `clikit` output seam, and the jsonc module).

```sh
go install github.com/ubgo/textdata/cmd/textdatactl@latest
textdatactl jsonc --help
```

Status: released; the current tag is `jsonc/cli/v0.1.5`.

## Verbs

| Command | Does |
|---|---|
| `get <file> <path>` | print a value: a string bare, anything else as indented plain JSON (`--json`: `{path, kind, value}` with the value as raw JSON, number literals exact) |
| `set <file> <path> <value>` | replace or create a value, comments kept; the value is a string unless `--json-value` (then it is parsed as one JSON value and written exactly as typed); creates missing parent objects and a missing file (mode 0600) |
| `unset <file> <path>` | comment an object member out (`// "debug": true,`), reversible |
| `restore <file> <path>` | bring an unset member back byte for byte |
| `delete <file> <path>` | remove a member or array element with its attached comments and comma |
| `diff <a> <b>` | changes by path, ignoring comments, layout and key order ([jsonc/diff](../diff/README.md)) |
| `to-json <file>` | the file as plain JSON: comments, trailing commas and BOM dropped, everything else as written |
| `fmt <file>` | re-lay the file out in one style through [jsonc/format](../format/README.md), every comment and the meaning kept; silent like gofmt, written atomically and only when it changes. `--indent N` (default 2), `--tabs`, `--trailing-commas` (ignored with `--strict`), `--dry-run` (print the diff), `--check` (write nothing; print the file name and exit 1 when it would change, for CI) |
| `validate <file>` | syntax errors, `jsonc.Lint` findings (duplicate keys, mixed indentation, comments tools lose; trailing commas with `--forbid-trailing-commas`) and, with `--schema <file>`, JSON Schema violations from `jsonc/schema`. Every issue prints as `file:line:col: message (rule)`, sorted by position; any error exits 1. Schema warnings (unknown keys under `additionalProperties`, unsupported keywords) print with `warning:` and do not fail. `--json`: `{file, valid, errors[], warnings[]}`, each issue `{file, line, col, source (syntax/lint/schema), rule, path, message}` |

`set`, `unset`, `restore` and `delete` take `--dry-run`, which prints the would-be line diff and writes nothing. Paths are dotted (`server.port`, `a."b.c"[2]`) or JSON Pointers (`/server/port`); `""` is the whole document.

```sh
textdatactl jsonc get config.jsonc server.port
textdatactl jsonc set config.jsonc server.port 9090 --json-value --dry-run
textdatactl jsonc unset config.jsonc server.debug
textdatactl jsonc diff base.jsonc prod.jsonc --json | jq '.data.changes[].path'
textdatactl jsonc to-json config.jsonc > config.json
textdatactl jsonc fmt config.jsonc --check
textdatactl jsonc validate config.jsonc --schema config.schema.json
```

Global flags: `--json` (one `{ok,data|error}` envelope per run, from `clikit/outfmt`), `--strict` (read files as plain JSON: comments and trailing commas are errors, `unset` is refused), `--version`.

## Contract

- Exit codes: `0` ok, `1` the operation failed, `diff` found differences (as `diff(1)`), `fmt --check` found an unformatted file or `validate` found an error, `2` the command line was malformed (unknown verb or flag, wrong operand count, a path or `--json-value` that does not parse).
- Human mode: results on stdout, failures on stderr as `textdatactl jsonc: <message>`, with `file:line:col` when the library has a position. `validate` issues and the `fmt --check` file list are results, so they go to stdout. Under `--json` both success and failure are one envelope line on stdout.
- Error codes in the envelope: `not_found`, `io`, `usage` (from `clikit/outfmt`), plus `syntax` (file is not valid JSONC/JSON), `type` (wrong kind for the verb, e.g. unset of an array element or delete of the root), `exists` (restore of a live key), `layout` (unset of a member that does not occupy its own lines), `strict` (unset under `--strict`), `schema` (the `validate --schema` file is not a valid schema), exported as `jsonccmd.Code*`.
- Edits go through `jsonc.Doc` and `Doc.Save` (atomic: temp file, fsync, rename; mode kept; writes through a symlink). An edit that leaves the bytes identical writes nothing, so the file's mtime is untouched. A refused edit leaves the file untouched.

## Embedding

<!-- snippet: exempt — call shapes only; TestCommandGraft and TestHelpAndProgramName in jsonccmd/cmd_test.go exercise both -->
```go
// Delegation: the host passes the args after its own namespace.
code := jsonccmd.Execute(args, os.Stdout, os.Stderr, jsonccmd.WithProgramName("myapp config"))

// Grafting: a cobra command named "jsonc"; failures come back as *jsonccmd.ExitError.
root.AddCommand(jsonccmd.Command(jsonccmd.WithProgramName("myapp jsonc")))
```

`WithProgramName` changes the name in failure lines, the usage hint, the help Usage line and `--version`; the `--help` examples keep naming `textdatactl jsonc`. `--version` reports this module's own version.
