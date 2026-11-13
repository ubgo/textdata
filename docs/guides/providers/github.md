# Push env values to GitHub Actions secrets

The GitHub provider (`github.com/ubgo/textdata/dotenv/cli/providers/github`, package `githubplugin`) pushes values from a `.env` file to GitHub Actions secrets, lists what is there and prunes what your file no longer defines, for repository, environment and organization secrets. It drives the [`gh` CLI](https://cli.github.com), which supplies authentication and the secret encryption; the provider supplies the selection, the guards and the confirm gate. Use it when one env file (or one prefix in it) is the source of truth for a repository's CI secrets and you want to stop pasting values into the web UI. It is mounted in `dotenvctl` and `textdatactl dotenv`, so `dotenvctl github …` and `textdatactl dotenv github …` are the same command, and you can mount it in your own cobra binary.

## Before you start

```sh
go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest   # or: go install github.com/ubgo/textdata/cmd/textdatactl@latest
go get github.com/ubgo/textdata/dotenv/cli/providers/github@latest     # only to mount it in your own binary
```

You need `gh` on your `PATH`, logged in (`gh auth login`) or given a token in `GH_TOKEN` (Step 6), and an account that can write the repository's secrets. Environment secrets need the environment to exist (Step 4).

The runnable program for this page is [`examples/providers/github`](../../../examples/providers/github). It mounts the real plugin under a root named `dotenvctl`, exactly as `dotenvctl` does, and replaces `gh` with a fake runner that prints each call it receives (the indented `[gh …] stdin="…"` lines) and answers from memory: the current directory's repository is `acme/api`, the account is `octocat`, and the remote holds `API_KEY`, `DB_URL`, `GHCR_PAT`, `OLD_TOKEN` and `VPS_DEPLOY_KEY`. No `gh` binary, network or GitHub account is used. Apart from those indented lines, its output is what `dotenvctl` prints with a stand-in `gh` on `PATH`. The `.env`:

```sh
GITHUB_SECRET_DB_URL=postgres://${DB_HOST}/app
GITHUB_SECRET_GHCR_PAT=ghp_example_only
GITHUB_SECRET_API_KEY=__YOU__
DB_HOST=db.internal
LOG_LEVEL=debug
app.name=api
```

These are the command lines the steps below run:

<!-- ds:block id=scenarios-vmqccpt9 -->
**scenarios** · [`examples/providers/github/main.go:164-182`](../../../examples/providers/github/main.go#L164-L182)

```go
func scenarios() [][]string {
	const sel = "--prefix=GITHUB_SECRET_"
	return [][]string{
		// 1. preview: target from the cwd's git remote, nothing written
		{"github", "push", sel, "--strip-prefix", "--dry-run"},
		// 2. CI shape: explicit repo and environment, --yes, one envelope
		{"--json", "github", "push", sel, "--strip-prefix", "--repo", "acme/api", "--env", "prod", "--yes"},
		// 3. environment secrets need the environment to exist first
		{"github", "env-create", "prod", "--repo", "acme/api", "--yes"},
		// 4. names only; each scope is listed on its own
		{"github", "list", "--repo", "acme/api", "--env", "prod"},
		// 5. prune preview, sparing a secret another pipeline manages
		{"github", "prune", sel, "--strip-prefix", "--repo", "acme/api", "--keep", "VPS_DEPLOY_KEY", "--dry-run"},
		// 6. --org cannot be combined with --repo: usage error before any write
		{"github", "push", "--org", "acme", "--repo", "acme/api", "--yes"},
		// 7. LOG_LEVEL is a legal secret name, app.name is not: nothing is written
		{"github", "push", "LOG_LEVEL", "app.name", "--repo", "acme/api", "--yes"},
	}
}
```
<!-- /ds:block hash=dca580 -->

### Commands and flags

| Command | What it does | `gh` calls |
|---|---|---|
| `github push [KEY...]` | upsert secrets | `gh secret set NAME [scope]`, value on stdin |
| `github list` | remote secret names (GitHub secrets are write-only) | `gh secret list --json name [scope]` |
| `github prune` | delete remote names absent from your keep-set | `gh secret list`, then `gh secret delete NAME [scope]` per stale name |
| `github env-create NAME` | create a deployment environment, idempotently | `gh api --method PUT repos/OWNER/REPO/environments/NAME` |
| every mutating verb | resolve and print the target and account first | `gh repo view` (no `--repo`), `gh api user` |

| Flag | Verbs | Target |
|---|---|---|
| *(none)* | `push`, `list`, `prune`, `env-create` | the repository of the current directory's git remote, as `gh` resolves it |
| `--repo owner/name` (`-R`) | `push`, `list`, `prune`, `env-create` | that repository |
| `--environment NAME` (alias `--env`) | `push`, `list`, `prune` | a deployment environment's secrets; combinable with `--repo` |
| `--org NAME` | `push`, `list`, `prune` | organization secrets; not combinable with `--repo` or `--environment` |

`push` and `prune` also take the [shared selection flags](selection.md) (`--prefix`, `--exclude-prefix`, `--include`, `--exclude`, `--strip-prefix`, `--expand` on by default, `--include-placeholders`, `--include-empty`) and `--dry-run`, `--yes`; `prune` takes `--keep`. `--json` is the host's global flag.

## Step 1 — Preview a push

Start every sync with `--dry-run`: it resolves the selection, prints where the values would go and as whom, and the per-key actions, and writes nothing. With no `--repo`, the target is the current directory's repository, and the banner says so.

```text
$ dotenvctl github push --prefix=GITHUB_SECRET_ --strip-prefix --dry-run
    [gh repo view --json nameWithOwner --jq .nameWithOwner] stdin=""
    [gh api user --jq .login] stdin=""
target : acme/api (from cwd git remote)
account: octocat (stored gh auth)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: would-push
GHCR_PAT: would-push
exit 0
```

`GITHUB_SECRET_DB_URL` becomes the secret `DB_URL` and its `${DB_HOST}` is resolved; the `__YOU__` placeholder is held back; `DB_HOST` and `LOG_LEVEL` are never considered because they lack the prefix. Without `--dry-run`, a terminal session asks `proceed? [y/N]` after the banner.

## Step 2 — Push from CI

CI has no terminal, so a write needs `--yes`; without it the run stops after the banner with `refusing to write without --yes in non-interactive mode` and exit 2. Name the repository and environment explicitly, and add `--json` for one machine-readable envelope. Each value reaches `gh` on stdin, never in the argument list.

```text
$ dotenvctl --json github push --prefix=GITHUB_SECRET_ --strip-prefix --repo acme/api --env prod --yes
    [gh api user --jq .login] stdin=""
    [gh secret set DB_URL --repo acme/api --env prod] stdin="postgres://db.internal/app"
    [gh secret set GHCR_PAT --repo acme/api --env prod] stdin="ghp_example_only"
{"ok":true,"data":{"meta":{"account":"octocat (stored gh auth)","target":"acme/api (environment prod)"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"DB_URL","action":"pushed"},{"name":"GHCR_PAT","action":"pushed"}],"dry_run":false}}
exit 0
```

As a workflow step:

```yaml
- name: Sync Actions secrets from the env file
  run: |
    dotenvctl -f .env.prod github push \
      --prefix GITHUB_SECRET_ --strip-prefix \
      --repo ${{ github.repository }} --env prod --yes --json
  env:
    GH_TOKEN: ${{ secrets.ADMIN_PAT }}
```

The default `GITHUB_TOKEN` of a workflow cannot write Actions secrets; use a token that can (a fine-grained PAT with the Secrets permission, or an app token).

## Step 3 — Read what is there

`list` prints names only: GitHub never returns secret values. Each scope is a separate store, so list with the same scope flags you push with.

```text
$ dotenvctl github list --repo acme/api --env prod
    [gh secret list --json name --repo acme/api --env prod] stdin=""
API_KEY
DB_URL
GHCR_PAT
OLD_TOKEN
VPS_DEPLOY_KEY
exit 0
```

On an empty scope, `list` says `no secrets found for this scope (repo/environment/org secrets are listed separately — try --environment or --org)`.

## Step 4 — Create the environment first

Environment secrets need the deployment environment to exist. `env-create` creates it, or confirms it when it exists (the underlying call is a `PUT`), so it belongs at the top of any setup script. It takes `--repo` and `--yes`, and is gated like every write.

```text
$ dotenvctl github env-create prod --repo acme/api --yes
target : acme/api
environment: prod (create or confirm)
    [gh api --method PUT repos/acme/api/environments/prod] stdin=""
prod: environment ready
exit 0
```

A bootstrap for a fresh repository is two lines:

```sh
dotenvctl github env-create prod --yes
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --yes
```

Under `--json`, `env-create` prints `{"ok":true,"data":{"repo":"…","environment":"…"}}`.

## Step 5 — Prune what your file no longer defines

`prune` deletes remote names that are not in the local keep-set: the names your selection would push, plus keys held back as placeholders or empty (so a `__YOU__` in your file never deletes the real value someone set), plus every `--keep` name (secrets another pipeline manages). Preview it, then run it with `--yes`.

```text
$ dotenvctl github prune --prefix=GITHUB_SECRET_ --strip-prefix --repo acme/api --keep VPS_DEPLOY_KEY --dry-run
    [gh secret list --json name --repo acme/api] stdin=""
    [gh api user --jq .login] stdin=""
target : acme/api
account: octocat (stored gh auth)
keys   : 1
API_KEY: kept-skipped
VPS_DEPLOY_KEY: kept-keep-flag
OLD_TOKEN: would-delete
exit 0
```

`DB_URL` and `GHCR_PAT` are in the selection and are kept silently; `keys` counts the names that would be deleted.

## Step 6 — Act as another identity

The provider inherits `gh`'s own authentication: `GH_TOKEN`, then `GITHUB_TOKEN`, then your stored `gh auth login` (plus `GH_HOST` and `GH_ENTERPRISE_TOKEN` for GitHub Enterprise). To act with a token instead of your login, set it for one command:

```sh
GH_TOKEN="$(gopass show -o ci/github-admin-pat)" dotenvctl github push --repo someorg/app --dry-run
```

The banner labels the source, so a token exported yesterday cannot act unnoticed today. The provider only checks whether the variable is set; it never reads the token, and `gh` takes it from the environment.

<!-- ds:block id=tokenoverride-2yhnvfrs -->
**tokenOverride** · [`examples/providers/github/main.go:189-204`](../../../examples/providers/github/main.go#L189-L204)

```go
func tokenOverride(w io.Writer, root *cobra.Command) error {
	prev, had := os.LookupEnv(tokenVar)
	if err := os.Setenv(tokenVar, "example-token-not-real"); err != nil {
		return err
	}
	defer func() {
		if had {
			_ = os.Setenv(tokenVar, prev)
		} else {
			_ = os.Unsetenv(tokenVar)
		}
	}()
	fmt.Fprintf(w, "# with %s set in the environment\n", tokenVar)
	fmt.Fprintf(w, "exit %d\n", execute(w, root, "github", "push", "DB_HOST", "--repo", "acme/api", "--dry-run"))
	return nil
}
```
<!-- /ds:block hash=b7aecf -->

```text
# with GH_TOKEN set in the environment
$ dotenvctl github push DB_HOST --repo acme/api --dry-run
    [gh api user --jq .login] stdin=""
target : acme/api
account: octocat (via GH_TOKEN)
keys   : 1
DB_HOST: would-push
exit 0
```

There is no `--token` flag on purpose: argv is visible to every process through `ps` and stays in shell history.

## Step 7 — Mount it in your own binary

`githubplugin.New()` returns a `providerkit.Plugin`; `Command(deps)` builds the `github` namespace, which you add under any cobra root. The host supplies everything through `providerkit.Deps` (see [Write a push provider](write-a-provider.md#step-4--wire-the-host-side)). In production `Runner` is `providerkit.ExecRunner{}`; the example passes the fake.

<!-- ds:block id=mount-z5xybc2g -->
**mount** · [`examples/providers/github/main.go:104-124`](../../../examples/providers/github/main.go#L104-L124)

```go
func mount(w io.Writer, envPath string, gh providerkit.Runner) *cobra.Command {
	printer := &outfmt.Printer{Out: w, Err: w, Program: programName}
	root := &cobra.Command{Use: programName, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&printer.JSON, flagJSON, false, "emit the {ok,data|error} envelope")
	root.SetErr(io.Discard)
	root.AddCommand(githubplugin.New().Command(providerkit.Deps{
		Printer: printer,
		File:    envPath,
		Source: func(opts providerkit.SelectOpts) (providerkit.Source, error) {
			sel, err := envkit.SelectFile(envPath, opts)
			if err != nil {
				return nil, providerkit.Fail(printer, outfmt.CodeIO, "%v", err)
			}
			return sel.Flat(), nil
		},
		Runner:      gh, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}))
	return root
}
```
<!-- /ds:block hash=3d4064 -->

The fake `gh` the example uses, the same shape as the plugin's own tests:

<!-- ds:block id=fakegh-pkpuy26f -->
**fakeGh** · [`examples/providers/github/main.go:73-76`](../../../examples/providers/github/main.go#L73-L76)

```go
type fakeGh struct {
	w      io.Writer
	remote []string // names `gh secret list` returns
}
```
<!-- /ds:block hash=5fae04 -->

<!-- ds:block id=fakegh-run-ccjrvarj -->
**fakeGh.Run** · [`examples/providers/github/main.go:81-97`](../../../examples/providers/github/main.go#L81-L97)

```go
func (f *fakeGh) Run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	fmt.Fprintf(f.w, "    [%s %s] stdin=%q\n", name, joined, stdin)
	switch {
	case joined == "api user --jq .login":
		return []byte("octocat\n"), nil
	case strings.HasPrefix(joined, "repo view"):
		return []byte("acme/api\n"), nil
	case strings.HasPrefix(joined, "secret list"):
		rows := make([]string, 0, len(f.remote))
		for _, n := range f.remote {
			rows = append(rows, fmt.Sprintf(`{"name":%q}`, n))
		}
		return []byte("[" + strings.Join(rows, ",") + "]"), nil
	}
	return nil, nil
}
```
<!-- /ds:block hash=feb2c0 -->

## Try it

```sh
cd examples/providers/github && go run .
```

```text
$ dotenvctl github push --prefix=GITHUB_SECRET_ --strip-prefix --dry-run
    [gh repo view --json nameWithOwner --jq .nameWithOwner] stdin=""
    [gh api user --jq .login] stdin=""
target : acme/api (from cwd git remote)
account: octocat (stored gh auth)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: would-push
GHCR_PAT: would-push
exit 0

$ dotenvctl --json github push --prefix=GITHUB_SECRET_ --strip-prefix --repo acme/api --env prod --yes
    [gh api user --jq .login] stdin=""
    [gh secret set DB_URL --repo acme/api --env prod] stdin="postgres://db.internal/app"
    [gh secret set GHCR_PAT --repo acme/api --env prod] stdin="ghp_example_only"
{"ok":true,"data":{"meta":{"account":"octocat (stored gh auth)","target":"acme/api (environment prod)"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"DB_URL","action":"pushed"},{"name":"GHCR_PAT","action":"pushed"}],"dry_run":false}}
exit 0

$ dotenvctl github env-create prod --repo acme/api --yes
target : acme/api
environment: prod (create or confirm)
    [gh api --method PUT repos/acme/api/environments/prod] stdin=""
prod: environment ready
exit 0

$ dotenvctl github list --repo acme/api --env prod
    [gh secret list --json name --repo acme/api --env prod] stdin=""
API_KEY
DB_URL
GHCR_PAT
OLD_TOKEN
VPS_DEPLOY_KEY
exit 0

$ dotenvctl github prune --prefix=GITHUB_SECRET_ --strip-prefix --repo acme/api --keep VPS_DEPLOY_KEY --dry-run
    [gh secret list --json name --repo acme/api] stdin=""
    [gh api user --jq .login] stdin=""
target : acme/api
account: octocat (stored gh auth)
keys   : 1
API_KEY: kept-skipped
VPS_DEPLOY_KEY: kept-keep-flag
OLD_TOKEN: would-delete
exit 0

$ dotenvctl github push --org acme --repo acme/api --yes
dotenvctl: --org is mutually exclusive with --repo/--environment
exit 2

$ dotenvctl github push LOG_LEVEL app.name --repo acme/api --yes
dotenvctl: key(s) [app.name] are not valid GitHub secret names ([A-Za-z_][A-Za-z0-9_]*) — rename them or select others; refusing a partial push
exit 2

# with GH_TOKEN set in the environment
$ dotenvctl github push DB_HOST --repo acme/api --dry-run
    [gh api user --jq .login] stdin=""
target : acme/api
account: octocat (via GH_TOKEN)
keys   : 1
DB_HOST: would-push
exit 0
```

The `account:` lines read `(stored gh auth)` only when neither `GH_TOKEN` nor `GITHUB_TOKEN` is set in your shell; the example's test clears both.

## Pitfalls

- **Each scope is listed and pruned on its own.** Repository, environment and organization secrets are separate stores. `github list` without `--env` shows repository secrets only, and `github prune` without `--env` compares your file with repository secrets only. Run `prune` with the same `--env` or `--org` you pushed with.
- **`--org` cannot be combined** with `--repo` or `--environment`; `push` and `prune` stop with a usage error (exit 2) before anything is written.
- **Names are checked before any write, after renaming.** GitHub accepts `[A-Za-z_][A-Za-z0-9_]*`; one bad name (a dot, a hyphen, a leading digit) refuses the whole push with exit 2 and nothing written, never a silent rename. The check sees the final names, so `GITHUB_SECRET_1PASSWORD` fails as `1PASSWORD` under `--strip-prefix`.
- **Trailing newlines are dropped.** Values reach `gh` on stdin, and `gh` trims a trailing `\r\n` from what it reads, so a multi-line value that ends in a newline is stored without it.
- **Without `--repo`, the target comes from the current directory.** Run from the wrong checkout and the banner shows another repository; read the `target:` line before you answer `y`, and pass `--repo` in scripts.
- **`env-create` takes no `--environment`.** The environment is the positional `NAME`; only `--repo` and `--yes` apply.
- **A mid-way failure is reported, not rolled back.** Keys pushed before the failing one are listed as `pushed`, the command exits 1, and under `--json` one `ok:false` envelope carries both `.data.results` and `.error`. Pushes are upserts, so a re-run completes the rest.
- **`gh` missing or logged out** fails the target or account lookup first, with a pointer to `https://cli.github.com` or `gh auth login`; nothing is written. A failing `gh secret list` fails `list` and `prune` instead of reading as an empty scope.
- **No `--token` flag.** Use `GH_TOKEN` for a one-off identity, and check the `account:` line.

## Related

- [Vercel provider](vercel.md): the same verbs for Vercel environment variables
- [Select which keys a command acts on](selection.md): the selection flags in full
- [Write a push provider](write-a-provider.md): the kit this provider is built on
- [Output and reports](output-and-reports.md): the `--json` envelope and exit codes
- Module reference: [dotenv/cli/providers/github README](../../../dotenv/cli/providers/github/README.md)
- Example: [`examples/providers/github`](../../../examples/providers/github); the plugin's own examples: [`ExampleNew`, `ExamplePlugin_Command`](../../../dotenv/cli/providers/github/example_test.go)
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/providers/github](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/providers/github)
- [Feature index](../../reference/features.md)
