# GitHub plugin

Module `github.com/ubgo/textdata/dotenv/cli/providers/github`, package `githubplugin`. A provider plugin for the textdata CLIs: mounted by default in `dotenvctl` and `textdatactl dotenv`, so `dotenvctl github …` and `textdatactl dotenv github …` are the same command. It is built on [`clikit/providerkit`](../../../../clikit/README.md#providerkit), imports no file format, and receives its values as a resolved `clikit/selection.Selection` from the host, so it never parses a file itself.

> **Moved from `github.com/ubgo/dotenv/cli/plugins/githubplugin`.** Same code, same verbs and flags; the package name `githubplugin` is kept. Only the import path changed, and the selection type is now `selection.Selection` from `clikit`. Status: released; the current tag is `dotenv/cli/providers/github/v0.1.4`.

```sh
go get github.com/ubgo/textdata/dotenv/cli/providers/github   # to mount it in your own cobra binary
```

Pushes env values to **GitHub Actions secrets** through the [`gh` CLI](https://cli.github.com), which supplies authentication and secret encryption. Requires `gh` on your PATH, logged in (`gh auth login`) or overridden by a token (below).

On this page: [Commands](#commands) · [Choosing the target](#choosing-the-target) · [Authentication and token override](#authentication-and-token-override) · [Creating the environment first](#creating-the-environment-first) · [Secret names](#secret-names) · [The prefix workflow](#the-prefix-workflow) · [Keeping remote secrets in sync](#keeping-remote-secrets-in-sync) · [CI usage](#ci-usage) · [Failure behavior](#failure-behavior) · [Pitfalls](#pitfalls) · [Use it from Go](#use-it-from-go) · [Related](#related)

## Commands

```sh
dotenvctl github push [KEY...]      # upsert secrets
dotenvctl github list               # remote secret names (GitHub secrets are write-only)
dotenvctl github prune --yes        # delete remote secrets absent from your local selection
dotenvctl github env-create NAME    # create a deployment environment, idempotently
```

All [shared selection and safety flags](../../../../docs/guides/providers/selection.md) apply to `push` and `prune`: `--prefix`, `--exclude-prefix`, `--strip-prefix`, `--include`, `--exclude` (both multi-value: repeat or comma-separate), `--expand` (default on), `--include-placeholders`/`--include-empty`, `--dry-run`, `--yes`, `--json`.

```sh
# push the prefixed keys except two act-only specials — both spellings equivalent
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix   --exclude GITHUB_SECRET_GITHUB_TOKEN,GITHUB_SECRET_GITHUB_ACTOR --env prod --yes
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix   --exclude GITHUB_SECRET_GITHUB_TOKEN --exclude GITHUB_SECRET_GITHUB_ACTOR --env prod --yes
```

## Choosing the target

| Flag | Target |
|---|---|
| *(none)* | [the repository of your current directory's git remote](ds:block?id=ghresolvetarget-4a2bqvwq), exactly as `gh` resolves it |
| `--repo owner/name` | that repository |
| `--environment production` (alias `--env`) | a deployment environment's secrets (combinable with `--repo`) |
| `--org acme` | organization secrets — mutually exclusive with `--repo`/`--environment` |

Before anything writes, the plugin prints where and as whom, then asks:

```
$ dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix
target : acme/api (from cwd git remote)
account: you (stored gh auth)
keys   : 10
proceed? [y/N]
```

`--dry-run` prints the banner and the would-push list without prompting or writing. Non-interactive runs (CI, pipes, `--json`) [require `--yes`](ds:block?id=confirm-wvppa3w3). The banner exists because "wrong directory + inferred repo + irreversible write" must never combine silently.

## Authentication and token override

The plugin inherits `gh`'s own auth chain: **`GH_TOKEN` → `GITHUB_TOKEN` → your stored `gh auth login`** (plus `GH_HOST` / `GH_ENTERPRISE_TOKEN` for GitHub Enterprise). To act with a different identity than your login — say your account lacks access but you hold a PAT that doesn't:

```sh
GH_TOKEN=ghp_yourtoken dotenvctl github push --repo someorg/private-repo --yes
```

[The banner labels the source](ds:block?id=tokenenvvars-egswkuty) — `account: you (via GH_TOKEN)` — so a token exported yesterday can't act invisibly today. There is deliberately **no `--token` flag**: argv is visible to every process via `ps` and persists in shell history. For the same reason, secret values reach `gh` via stdin, never as arguments.

## Creating the environment first

Environment-scoped secrets require the deployment environment to exist — the step everyone forgets and then does by hand in the web UI:

```sh
dotenvctl github env-create prod
dotenvctl github env-create staging --repo owner/name --yes
```

Idempotent: an existing environment is success, so it belongs at the top of any setup task. The usual confirm gate applies (`--yes` for scripts). A typical bootstrap for a fresh repo:

```sh
dotenvctl github env-create prod --yes
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --yes
```

## Secret names

[GitHub requires](ds:block?id=secretnamere-844cw9m5) `[A-Za-z_][A-Za-z0-9_]*`. Env keys that are legal in `.env` files but not as secret names (dots, hyphens, digit-first) fail the **whole push before any write**, with the offending keys listed — no silent renaming, no partial pushes.

## The prefix workflow

If your env files mark deploy secrets with a prefix, one command syncs exactly those:

```sh
dotenvctl -f .env.staging github push --prefix GITHUB_SECRET_ --strip-prefix
```

`GITHUB_SECRET_GHCR_PAT=…` becomes repo secret `GHCR_PAT`; unprefixed keys are never considered; placeholder values (`__YOU__`) are skipped with a per-key warning.

## Keeping remote secrets in sync

```sh
# What exists remotely? (repo-level secrets)
dotenvctl github list

# Environment-scoped secrets are listed separately — if `list` reports
# "no secrets found for this scope", yours probably live in an environment:
dotenvctl github list --env prod

# Preview a full sync: what would push, what would be skipped
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --dry-run

# Delete remote secrets your file no longer defines (after the same selection)
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix --dry-run
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix --yes
```

`prune` computes *remote names minus the local keep-set* and deletes only those; `--dry-run` shows the would-delete list first. The keep-set is wider than the pushable selection, on purpose:

- keys skipped as placeholders/empty locally are **spared** (`GITHUB_ACTOR: kept-skipped`) — a `__YOU__` in your file never deletes the real secret someone set upstream;
- `--keep NAME` (repeat or comma-separate) spares secrets managed outside this selection — e.g. a bundle or key-file secret another pipeline maintains:

```sh
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix \
  --keep ENV_B64,VPS_DEPLOY_KEY_B64 --env prod --dry-run
```

Every spared name is reported with its reason, so the decision is auditable.

## CI usage

```yaml
- name: Sync Actions secrets from the env template
  run: |
    dotenvctl -f .env.staging github push \
      --prefix GITHUB_SECRET_ --strip-prefix \
      --repo ${{ github.repository }} --yes --json
  env:
    GH_TOKEN: ${{ secrets.ADMIN_PAT }}
```

`--json` emits `{"ok":true,"data":{"meta":{"target":…,"account":…},"results":[{"name":"GHCR_PAT","action":"pushed"}]}}` — names and actions only; values never appear in any output.

## Failure behavior

- `gh` missing or unauthenticated: a clear error pointing at `https://cli.github.com` / `gh auth login`; nothing runs.
- A push failing mid-way: everything already pushed is reported as `pushed`, the command exits `1`, and re-running completes the rest — pushes are idempotent upserts. The shape is the same as in the [Vercel example](../vercel/README.md#failure-behavior).
- `gh secret list` failing: `list` and `prune` fail too, rather than treating the scope as empty.

## Pitfalls

- **Each scope is listed and pruned on its own.** Repository, environment and organization secrets are separate stores. `github list` without `--env` shows repository secrets only, and `github prune` without `--env` compares your file against repository secrets only. Run `prune` with the same `--env`/`--org` you pushed with.
- **`--org` cannot be combined** with `--repo` or `--environment`; `push` and `prune` stop with a usage error (exit `2`) before anything is written.
- **Trailing newlines are dropped.** Values reach `gh` on stdin, and `gh` trims a trailing `\r\n` from what it reads, so a multi-line value that ends in a newline is stored without it.
- **Name checks see the final names.** Validation runs after `--strip-prefix`, so `GITHUB_SECRET_1PASSWORD` fails (it becomes `1PASSWORD`, digit-first) even though the original key would be legal.
- **A mid-way failure under `--json` is one `ok:false` envelope that still has `data`**: `.data.results` lists what was written, `.error` names the key that failed. In `github.com/ubgo/dotenv/cli` v0.1.2 and earlier, it printed two envelopes.
- **No `--token` flag, on purpose.** Use `GH_TOKEN` for a one-off identity, and read the `account:` line in the banner before you answer `y`.

## Use it from Go

`githubplugin.New()` returns a `providerkit.Plugin`; `Command(deps)` builds its cobra namespace, which you mount under any root. The host supplies everything through `providerkit.Deps`: the output `Printer`, a `Source` that returns the resolved selection, the `Runner` that executes the backend CLI (`providerkit.ExecRunner{}` in production, a fake in tests), and the interactive/confirm hooks. The example uses a fake runner, so no `gh` binary or account is needed:

<!-- snippet: dotenv/cli/providers/github/example_test.go#api_githubplugin_New -->
```go
// ExampleNew mounts the github plugin under your own cobra root. All gh calls
// go through Deps.Runner — here a fake — and the secret value reaches gh on
// stdin, never in argv. The account is looked up once per run.
func ExampleNew() {
	var out bytes.Buffer
	runner := &apiGhRunner{}
	src := &selection.Selection{Pairs: []selection.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	deps := providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out, Err: &out},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}

	root := &cobra.Command{Use: "mytool", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	root.AddCommand(githubplugin.New().Command(deps))

	apiGhRun(root, &out, "github", "push", "--repo", "acme/api", "--yes")
	for _, c := range runner.calls {
		fmt.Println(c)
	}
	// Output:
	// target : acme/api
	// account: octocat
	// keys   : 1
	// DB_URL: pushed
	// exit: 0
	// gh api user --jq .login  stdin=""
	// gh secret set DB_URL --repo acme/api  stdin="postgres://db/app"
}
```

[`ExamplePlugin_Command`](example_test.go) shows the gate refusing a push with an illegal secret name.

## Related

- [Write a push provider](../../../../docs/guides/providers/write-a-provider.md) — the shared flags, guards and confirm gate
- [Vercel plugin](../vercel/README.md) — the same verbs for Vercel environment variables
- [Selecting keys](../../../../docs/guides/providers/selection.md) — the selection grammar in full
- [GitHub guide](../../../../docs/guides/providers/github.md) — the end-to-end sync workflow, step by step with real output
- [Mount it in your own binary](../../../../docs/guides/providers/github.md#step-7--mount-it-in-your-own-binary) — `githubplugin.New()` and `providerkit`
- [Feature index](../../../../docs/reference/features.md#push-providers)
- Tests that pin this behavior: [`github_test.go`](github_test.go)
