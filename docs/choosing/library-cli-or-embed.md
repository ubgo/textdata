# Library, CLI or embedding the CLI

Every textdata feature is reachable three ways: call the Go library, run the `textdatactl` (or `dotenvctl`) binary, or run the CLI's own code inside your Go program. They share one implementation, so the choice is about who calls it and what you need back, not about features.

## Decide

| You are | Use | Entry point | You get back | Guide | Example |
|---|---|---|---|---|---|
| A Go program that reads or edits config | the library | `dotenv`, `jsonc` and their subpackages | typed values, errors with `file:line:col` | [getting started](../guides/getting-started/install-and-first-read.md) | [`examples/getting-started/first-read`](../../examples/getting-started/first-read) |
| A Go program that needs what a `dotenvctl` verb does across files (select, diff, matrix, run) | the operations API | `envkit` in `github.com/ubgo/textdata/dotenv/cli` | `*Selection`, `*EditResult`, `*DiffResult`, `*Matrix`, an exit code | [envkit](../guides/cli/envkit.md) | [`examples/cli/envkit-ops`](../../examples/cli/envkit-ops) |
| A person at a shell, or a CI script | the binary | `textdatactl dotenv …`, `textdatactl jsonc …`, or `dotenvctl …` | text, or the `--json` envelope, and exit codes | [textdatactl](../guides/getting-started/textdatactl.md), [dotenvctl](../guides/cli/dotenvctl.md), [jsonc CLI](../guides/cli/jsonc-cli.md) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| A Go tool that wants to offer the same verbs as subcommands (`myapp env set …`) | embedding | `dotenvcmd.Execute`, `jsonccmd.Execute` / `jsonccmd.Command`, `ctl.Execute` | the CLI's exact output in your writers, and the exit code as an `int` | [embedding](../guides/cli/embedding.md) | [`examples/cli/embed-dotenvctl`](../../examples/cli/embed-dotenvctl), [`examples/cli/embed-jsonc`](../../examples/cli/embed-jsonc) |
| A Go tool that wants to push to a new secret store from `dotenvctl` | a push provider | `providerkit` in `github.com/ubgo/textdata/clikit` | a cobra command tree you mount | [write a provider](../guides/providers/write-a-provider.md) | [`examples/providers/fake-provider`](../../examples/providers/fake-provider) |
| A team that wants a `dotenvctl <name>` command without Go | an exec plugin | any executable named `dotenvctl-<name>` on `PATH` | `DOTENVCTL_*` context variables | [exec plugins](../guides/cli/exec-plugins.md) | [`examples/cli/exec-plugin`](../../examples/cli/exec-plugin) |

- If you are writing Go and the result feeds more Go code, use the library or `envkit`: you get typed results and errors you can branch on with `errors.Is`, instead of parsing text.
- If you are writing a shell script or a CI step, use the binary with `--json`: the envelope is `{"ok":…,"data":…,"error":…}` and the exit code is `0` ok, `1` found something or failed, `2` usage.
- If your tool should feel like it has the verbs built in, embed: arguments are what a user types after the program name, `WithProgramName` puts your name in help and errors, and nothing calls `os.Exit`.
- If you want to test your own scripts against the real CLI without building a binary, embed it in a Go test.

## dotenvctl or textdatactl

| | `dotenvctl` | `textdatactl` |
|---|---|---|
| Install | `go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest` | `go install github.com/ubgo/textdata/cmd/textdatactl@latest`, or a release archive |
| Formats | dotenv | dotenv (`textdatactl dotenv …`) and jsonc (`textdatactl jsonc …`) |
| Verbs | the same dotenv verbs, flags and output | the same, plus `version` |

- If you only handle `.env` files and want short commands, `dotenvctl`.
- If you handle both formats, or want one binary in CI images, `textdatactl`.

## Pitfalls

- **The CLIs install no secret backends and never run `$(cmd)`.** `get --expand` resolves references inside the file only. For `${vault:…}` and friends, use the library with a plugin.
- **The CLI modules need Go 1.26.4 and bring cobra.** Embedding raises your `go` line; the libraries stay at 1.22 and stdlib only ([modules](format-and-modules.md)).
- **Exec plugins under `textdatactl dotenv`:** `DOTENVCTL_BIN` points at `textdatactl` and `DOTENVCTL_BIN_ARGS` is `dotenv`, so plugins call `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f FILE get KEY`; an embedding host declares its namespace with `dotenvcmd.WithCallbackArgs`.
- **Help text examples under `textdatactl dotenv <verb> --help` still say `dotenvctl …`;** the arguments are the same.

## Related

- [Learning path](../README.md), [Which format and modules](format-and-modules.md), [Options for your kind of app](app-setups.md)
- [Output and reports](../guides/providers/output-and-reports.md): the `--json` envelope and HTML reports
