# Push providers and clikit

Push a selection of a `.env` file's keys to GitHub Actions secrets or Vercel environment variables, choose exactly which keys go, and build your own provider. The shared plumbing lives in the `clikit` module. To decide what a deploy step needs, see [Options for your kind of app](../../choosing/app-setups.md#a-deploy-step-pushing-secrets-to-github-or-vercel).

| Guide | Package | What you learn | Example |
|---|---|---|---|
| [github.md](github.md) | `dotenv/cli/providers/github` | `push`, `list`, `prune` for repo, environment and org secrets, `env-create`, tokens, mounting | [`github`](../../../examples/providers/github/) |
| [vercel.md](vercel.md) | `dotenv/cli/providers/vercel` | `push`, `list`, `prune` with `--target` and `--sensitive`, CI, mounting | [`vercel`](../../../examples/providers/vercel/) |
| [selection.md](selection.md) | `clikit/selection` | `--prefix`, `--strip-prefix`, `--include`, `--exclude`, the placeholder and empty guards | [`selection`](../../../examples/providers/selection/) |
| [write-a-provider.md](write-a-provider.md) | `clikit/providerkit` | capabilities, the command tree, the confirmation gate, wiring and exit codes | [`fake-provider`](../../../examples/providers/fake-provider/) |
| [output-and-reports.md](output-and-reports.md) | `clikit/outfmt`, `report`, `textdiff` | the `--json` envelope and error codes, line diffs, HTML reports | [`output-and-reports`](../../../examples/providers/output-and-reports/) |

Every example drives the provider with a fake runner; none touches a real GitHub or Vercel account.
