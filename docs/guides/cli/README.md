# Command line

The `dotenvctl` and `textdatactl` command lines, the Go packages behind them, and how to extend or embed them. To decide between the library, the binary and embedding, see [Library, CLI or embedding](../../choosing/library-cli-or-embed.md).

| Guide | What you learn | Example |
|---|---|---|
| [dotenvctl.md](dotenvctl.md) | every dotenv verb and flag: `get`, `set`, `unset`, `restore`, `list`, `keys`, `run`, the `--json` envelope, exit codes | [`verb-tour`](../../../examples/cli/verb-tour/), [`run-child`](../../../examples/cli/run-child/) |
| [jsonc-cli.md](jsonc-cli.md) | every `textdatactl jsonc` verb: `get`, `set`, `unset`, `restore`, `delete`, `diff`, `to-json`, `fmt`, `validate`; `--json`, `--strict` | [`verb-tour`](../../../examples/cli/verb-tour/), [`embed-jsonc`](../../../examples/cli/embed-jsonc/) |
| [multi-env.md](multi-env.md) | `envs`, `matrix` with a contract, `diff`, placeholders, HTML reports | [`verb-tour`](../../../examples/cli/verb-tour/) |
| [recipes.md](recipes.md) | complete workflows: CI gates, deriving files, syncing outward, scripting safely | - |
| [exec-plugins.md](exec-plugins.md) | `dotenvctl-<name>` commands in any language, the `DOTENVCTL_*` context | [`exec-plugin`](../../../examples/cli/exec-plugin/) |
| [embedding.md](embedding.md) | `dotenvcmd.Execute`, `jsonccmd.Execute` and `Command`, `ctl.Execute`, program names, exit codes | [`embed-dotenvctl`](../../../examples/cli/embed-dotenvctl/), [`embed-jsonc`](../../../examples/cli/embed-jsonc/) |
| [envkit.md](envkit.md) | the operations API behind the verbs: select, edit with preview, diff, matrix, discover, child environments | [`envkit-ops`](../../../examples/cli/envkit-ops/) |

Installing the binary: [textdatactl](../getting-started/textdatactl.md). Pushing to GitHub or Vercel: [providers](../providers/README.md).
