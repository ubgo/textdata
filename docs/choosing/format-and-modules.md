# Which format and which modules

textdata is one repository of many Go modules. You pick a format by the file you have, and you import only the modules your code calls, so a program that reads one `.env` file pulls in nothing but the format and the stdlib-only base. This page answers "which format package?" and "which `go get` lines?".

## Decide the format

| Your file | Package | Open it with | Guide | Example |
|---|---|---|---|---|
| `.env`, `.env.prod`, Compose-style `KEY=value` | `github.com/ubgo/textdata/dotenv` | `dotenv.Read` / `dotenv.Open` | [dotenv guides](../guides/dotenv/README.md) | [`examples/dotenv/reading`](../../examples/dotenv/reading) |
| JSON with comments or trailing commas (`tsconfig.json`, `.vscode/settings.json`, `config.jsonc`) | `github.com/ubgo/textdata/jsonc` | `jsonc.Read` / `jsonc.Open` | [jsonc: parse and strict](../guides/jsonc/parse-and-strict.md) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Plain JSON that must stay plain JSON (`package.json`, an API payload on disk) | `github.com/ubgo/textdata/jsonc` with `jsonc.Strict()` | `jsonc.Open(path, jsonc.Strict())` | [Step 2 — parse plain JSON with Strict](../guides/jsonc/parse-and-strict.md#step-2--parse-plain-json-with-strict) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| YAML, TOML | not available yet | - | - | - |

- If the file is JSON and a person edits it, use `jsonc` without `Strict`: comments and trailing commas are read, kept on save, and every edit touches only its own bytes.
- If the file is JSON that another tool parses with a strict JSON parser, add `jsonc.Strict()`: comments and trailing commas become errors, and an edit that would need a comment fails with `jsonc.ErrStrict` instead of writing something the other tool rejects.
- If you only need the values and do not care about keeping bytes, `encoding/json` is enough for plain JSON; reach for `jsonc` when you need comments, positions (`file:line:col`), or byte-exact edits.

## Decide the modules

Every library module except the CLI ones is stdlib only. The CLI modules bring cobra.

| You want to | `go get` | Brings in | Go |
|---|---|---|---|
| Read or edit `.env` files | `github.com/ubgo/textdata/dotenv` | base `github.com/ubgo/textdata` | 1.22 |
| Read or edit JSONC / JSON | `github.com/ubgo/textdata/jsonc` | base | 1.22 |
| Format, diff, generate or schema-check JSONC | same `jsonc` module: packages `jsonc/format`, `jsonc/diff`, `jsonc/generate`, `jsonc/schema` | nothing more | 1.22 |
| A secret backend inside a file | one of `github.com/ubgo/textdata/plugins/{gopass,file,extends,vault,aws}` | base only | 1.22 (`file`, `extends`: 1.24) |
| Write your own plugin | base `github.com/ubgo/textdata` (package `plugin`) | nothing | 1.22 |
| Multi-file operations in Go (select, diff, matrix, run) | `github.com/ubgo/textdata/dotenv/cli` (package `envkit`) | cobra, `clikit`, both push providers | 1.26.4 |
| Embed the dotenv CLI | `github.com/ubgo/textdata/dotenv/cli` (package `dotenvcmd`) | same as above | 1.26.4 |
| Embed the jsonc CLI | `github.com/ubgo/textdata/jsonc/cli` (package `jsonccmd`) | cobra, `clikit`, `jsonc` | 1.26.4 |
| Embed both, as `textdatactl` | `github.com/ubgo/textdata/cmd/textdatactl` (package `ctl`) | every CLI module | 1.26.4 |
| Build a push provider | `github.com/ubgo/textdata/clikit` (package `providerkit`) | cobra | 1.26.4 |

- If your program only reads config at startup, import the format module and stop there: `go.mod` gains two lines (the format and the base) and no third-party code.
- If you need one secret backend, add only that plugin module; each one requires only the base, never the other plugins and never a format, so the same plugin instance works in `dotenv` and `jsonc`.
- If you need `envkit`, accept that it lives in the `dotenv/cli` module: that module requires cobra, `clikit` and both push providers, because `dotenvctl` mounts them. Go's module graph prunes what you never import from the build, but `go.mod` lists them.
- If you only want the `textdatactl` binary, `go install github.com/ubgo/textdata/cmd/textdatactl@latest`; nothing goes into your own `go.mod`.

## Pitfalls

- **The format modules require Go 1.22; the CLI modules require Go 1.26.4.** Importing `dotenvcmd`, `jsonccmd`, `envkit`, `ctl` or `providerkit` raises your module's `go` line.
- **Module versions are tagged per module** (`dotenv/v0.1.4`, `jsonc/v0.1.5`, `plugins/vault/v0.1.4`). `go get github.com/ubgo/textdata@latest` gets the base only; name the module path you want.
- **`dotenv` does not know about `$extends` or `Validator` plugins.** `dotenv.WithPlugin` refuses a plugin whose only abilities are ones dotenv never uses, such as the `extends` Includer: `Open`, `ParseReader` and `Read` return `dotenv: plugin "extends": no ability dotenv supports (Includer)` (`errors.Is(err, dotenv.ErrUnsupportedPlugin)`), and on a file from `Parse` the error comes from `GetExpanded`, `ExpandedMap` and `Save`. A plugin with at least one ability dotenv supports still installs; `schema.Plugin` installs on a dotenv file as a SaveGuard only (see [value plugins](value-plugins.md)).
- **`jsonc.Strict()` is per document.** A document parsed without it and saved with `SaveAs` keeps whatever comments it had; `Strict` does not strip them. Use [`format.Minify`](../guides/jsonc-tools/format.md#step-4--minify-to-plain-json) or `Doc.JSON()` to emit plain JSON from a commented file.

## Related

- [Learning path](../README.md)
- [Install and first read](../guides/getting-started/install-and-first-read.md), [`examples/getting-started/first-read`](../../examples/getting-started/first-read)
- [Library, CLI or embedding](library-cli-or-embed.md)
- [Feature index](../reference/features.md)
