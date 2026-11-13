# textdatactl

The umbrella CLI for [ubgo/textdata](../../README.md): one binary for every format, each mounted from its own command package.

```sh
go install github.com/ubgo/textdata/cmd/textdatactl@latest
```

| Command | What | Docs |
|---|---|---|
| `textdatactl dotenv …` | every `dotenvctl` verb (get, set, unset, run, diff, matrix, push to GitHub or Vercel, …) | [dotenv/cli](../../dotenv/cli/README.md) |
| `textdatactl jsonc …` | get, set, unset, restore, delete, fmt, validate, diff, to-json on JSONC files, comments kept | [jsonc/cli](../../jsonc/cli/README.md) |
| `textdatactl version` | build facts (`--json` for the envelope) | |
| `textdatactl completion <shell>` | shell completion for every format's verbs and flags (bash, zsh, fish, powershell) | |

Mounting is by delegation: `textdatactl dotenv <args>` hands `<args>` to `dotenvcmd.Execute` with program name `textdatactl dotenv`, so exit codes (0 ok, 1 failure, 2 usage), the `{ok,data|error}` JSON envelope (`--json`) and plugins behave exactly as in the standalone `dotenvctl`. `textdatactl --version` and `textdatactl version` report the same version.
