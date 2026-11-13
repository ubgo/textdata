# Push env values to Vercel environment variables

The Vercel provider (`github.com/ubgo/textdata/dotenv/cli/providers/vercel`, package `vercelplugin`) pushes values from a `.env` file to a Vercel project's environment variables, lists what is there and prunes what your file no longer defines, per Vercel environment (`production`, `preview`, `development`). It drives the [`vercel` CLI](https://vercel.com/docs/cli), which supplies authentication, project linking and the API calls; the provider supplies the selection, the guards and the confirm gate. Use it when one env file is the source of truth for a Vercel project and you want the dashboard to follow it without pasting values into a browser or onto a command line. It is mounted in `dotenvctl` and `textdatactl dotenv` as `vercel`, and you can mount it in your own cobra binary.

## Before you start

```sh
go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest   # or: go install github.com/ubgo/textdata/cmd/textdatactl@latest
go get github.com/ubgo/textdata/dotenv/cli/providers/vercel@latest     # only to mount it in your own binary
```

You need the `vercel` CLI on your `PATH`, logged in (`vercel login`) or given a token in `VERCEL_TOKEN`, and the directory you run from linked to the project (`vercel link`, which writes `.vercel/project.json`). The provider reads that file only to show the project in the banner; the `vercel` CLI decides what an unlinked run does.

The runnable program for this page is [`examples/providers/vercel`](../../../examples/providers/vercel). It mounts the real plugin under a root named `dotenvctl` and replaces `vercel` with a fake runner that prints each call (the indented `[vercel …] stdin="…"` lines) and answers from memory: `whoami` is `acme-bot`, the `production` target holds `API_KEY`, `DB_URL` and `OLD_FLAG`, and every other target is empty. No `vercel` binary, network or Vercel account is used, and the example's directory is not linked, so the banner says `unlinked directory`. The `.env`:

```sh
DB_URL=postgres://${DB_HOST}/app
DB_HOST=db.internal
API_KEY=__YOU__
VERCEL_ANALYTICS_ID=abc123
EMPTY=
```

These are the command lines the steps below run:

<!-- ds:block id=scenarios-safb27wv -->
**scenarios** · [`examples/providers/vercel/main.go:153-170`](../../../examples/providers/vercel/main.go#L153-L170)

```go
func scenarios() [][]string {
	return [][]string{
		// 1. preview: default target is development
		{"vercel", "push", "--dry-run"},
		// 2. two targets, stored as sensitive: one `env add` per key per target
		{"vercel", "push", "DB_URL", "--target", "production,preview", "--sensitive", "--yes"},
		// 3. only the Vercel-only keys, prefix stripped
		{"vercel", "push", "--prefix", "VERCEL_", "--strip-prefix", "--target", "preview", "--dry-run"},
		// 4. names only, parsed from the table
		{"vercel", "list", "--target", "production"},
		// 5. an empty target says which target it read
		{"vercel", "list", "--target", "preview"},
		// 6. prune preview: the local placeholder spares the real remote value
		{"vercel", "prune", "--target", "production", "--dry-run"},
		// 7. --sensitive belongs to push only
		{"vercel", "list", "--sensitive"},
	}
}
```
<!-- /ds:block hash=01eada -->

### Commands and flags

| Command | `vercel` call (once per target) |
|---|---|
| `vercel push [KEY...]` | `vercel env add NAME TARGET --force` (plus `--sensitive` when asked), value on stdin |
| `vercel list` | `vercel env ls TARGET`; names are the union across targets |
| `vercel prune` | `vercel env ls TARGET`, then `vercel env rm NAME TARGET --yes` per stale name |
| every mutating verb | `vercel whoami` first, for the banner |

| Flag | Verbs | Meaning |
|---|---|---|
| `--target production\|preview\|development` | `push`, `list`, `prune` | which Vercel environments; repeat or comma-separate; default `development` |
| `--sensitive` | `push` only | store as a sensitive variable: encrypted, unreadable in the dashboard |

`push` and `prune` also take the [shared selection flags](selection.md) and `--dry-run`, `--yes`; `prune` takes `--keep`. `--json` is the host's global flag.

## Step 1 — Preview a push

`--dry-run` resolves the selection, prints the banner (project, account, targets, how many keys would be written) and the per-key actions, and writes nothing. With no `--target`, the target is `development`, the least dangerous place for an accident.

```text
$ dotenvctl vercel push --dry-run
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: development
keys   : 3
API_KEY: skipped-placeholder
EMPTY: skipped-empty
DB_URL: would-push
DB_HOST: would-push
VERCEL_ANALYTICS_ID: would-push
exit 0
```

In a linked directory the first line reads `project: prj_… (from .vercel/project.json)`. `DB_URL` would be pushed expanded (`postgres://db.internal/app`), because `--expand` is on for pushes.

## Step 2 — Push to the environments you mean

Name the targets; each key is written to each target in turn, value on stdin. `--sensitive` makes the variables unreadable in the dashboard.

```text
$ dotenvctl vercel push DB_URL --target production,preview --sensitive --yes
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: production, preview
keys   : 1
    [vercel env add DB_URL production --force --sensitive] stdin="postgres://db.internal/app"
    [vercel env add DB_URL preview --force --sensitive] stdin="postgres://db.internal/app"
DB_URL: pushed
exit 0
```

`--force` makes every push an upsert; without it `vercel env add` fails on an existing name.

## Step 3 — Push one audience

When the file mixes app config with Vercel-only values, mark the Vercel ones with a prefix and strip it on the way out:

```text
$ dotenvctl vercel push --prefix VERCEL_ --strip-prefix --target preview --dry-run
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: preview
keys   : 1
ANALYTICS_ID: would-push
exit 0
```

## Step 4 — Read what is there

`list` prints names, sorted. Each target is listed separately, and an empty one says which target it read.

```text
$ dotenvctl vercel list --target production
    [vercel env ls production] stdin=""
API_KEY
DB_URL
OLD_FLAG
exit 0

$ dotenvctl vercel list --target preview
    [vercel env ls preview] stdin=""
no environment variables found for target(s) preview (each target is listed separately — try another --target: production, preview or development)
exit 0
```

The `vercel` CLI has no stable machine format for `env ls`, so names are parsed from its table: the first column of each row, skipping rows that start with `name`, `Vercel`, `>`, `Error` or `WARN`. A format change would show as an odd listing, never as wrong data for `push`, which does not read the listing.

## Step 5 — Prune what your file no longer defines

`prune` deletes remote names not in the local keep-set. The keep-set is wider than what `push` writes: `API_KEY` is `__YOU__` locally, so push skips it and prune spares the real remote value (`kept-skipped`); `--keep NAME` spares variables another tool manages (`kept-keep-flag`); names in the selection are kept silently.

```text
$ dotenvctl vercel prune --target production --dry-run
    [vercel env ls production] stdin=""
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: production
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: would-delete
exit 0
```

Run it again with `--yes` instead of `--dry-run` to delete.

## Step 6 — Run it from CI, and read a failure

CI has no terminal, so `--yes` is required; without it the run stops after the banner with `refusing to write without --yes in non-interactive mode` and exit 2. Authenticate with `VERCEL_TOKEN`; the banner labels it as `account: … (via VERCEL_TOKEN)`. There is no `--token` flag, because argv is visible in `ps` and shell history.

```yaml
- name: Install the vercel CLI
  run: npm install --global vercel

- name: Sync Vercel env vars from the env file
  run: |
    dotenvctl -f .env.prod vercel push \
      --prefix VERCEL_ --strip-prefix \
      --target production --yes --json
  env:
    VERCEL_TOKEN: ${{ secrets.VERCEL_TOKEN }}
```

When a push fails part-way, the keys already written are reported and the command exits 1. Here the fake refuses `DB_HOST`:

<!-- ds:block id=partialpush-dugzgphc -->
**partialPush** · [`examples/providers/vercel/main.go:192-197`](../../../examples/providers/vercel/main.go#L192-L197)

```go
func partialPush(w io.Writer, envPath string) error {
	cli := &fakeVercel{w: w, failOn: "DB_HOST"}
	args := []string{"--json", "vercel", "push", "--target", "production", "--yes"}
	fmt.Fprintf(w, "exit %d\n", execute(w, mount(w, envPath, cli), args...))
	return nil
}
```
<!-- /ds:block hash=9fc367 -->

```text
$ dotenvctl --json vercel push --target production --yes
    [vercel whoami] stdin=""
    [vercel env add DB_URL production --force] stdin="postgres://db.internal/app"
    [vercel env add DB_HOST production --force] stdin="db.internal"
{"ok":false,"data":{"meta":{"account":"acme-bot (stored vercel auth)","project":"unlinked directory (vercel CLI will resolve or prompt)","targets":"production"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"EMPTY","action":"skipped-empty"},{"name":"DB_URL","action":"pushed"}],"dry_run":false},"error":{"code":"exec","message":"push DB_HOST: target production: exit status 1: Error: simulated refusal"}}
exit 1
```

`.data.results` is what was written; `.error` names the key and the target that stopped the run. `VERCEL_ANALYTICS_ID` was never attempted. A real `vercel` failure also carries the CLI's own argv and stderr in the message. Re-running after the fix completes the rest.

## Step 7 — Mount it in your own binary

`vercelplugin.New()` returns a `providerkit.Plugin`; add `Command(deps)` under any cobra root. In production `Runner` is `providerkit.ExecRunner{}`; the example passes the fake.

<!-- ds:block id=mount-2wyy43pv -->
**mount** · [`examples/providers/vercel/main.go:110-130`](../../../examples/providers/vercel/main.go#L110-L130)

```go
func mount(w io.Writer, envPath string, cli providerkit.Runner) *cobra.Command {
	printer := &outfmt.Printer{Out: w, Err: w, Program: programName}
	root := &cobra.Command{Use: programName, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&printer.JSON, flagJSON, false, "emit the {ok,data|error} envelope")
	root.SetErr(io.Discard)
	root.AddCommand(vercelplugin.New().Command(providerkit.Deps{
		Printer: printer,
		File:    envPath,
		Source: func(opts providerkit.SelectOpts) (providerkit.Source, error) {
			sel, err := envkit.SelectFile(envPath, opts)
			if err != nil {
				return nil, providerkit.Fail(printer, outfmt.CodeIO, "%v", err)
			}
			return sel.Flat(), nil
		},
		Runner:      cli, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}))
	return root
}
```
<!-- /ds:block hash=b62fbc -->

<!-- ds:block id=fakevercel-tdjduh94 -->
**fakeVercel** · [`examples/providers/vercel/main.go:81-84`](../../../examples/providers/vercel/main.go#L81-L84)

```go
type fakeVercel struct {
	w      io.Writer
	failOn string
}
```
<!-- /ds:block hash=58a1f0 -->

<!-- ds:block id=fakevercel-run-jkptq7d2 -->
**fakeVercel.Run** · [`examples/providers/vercel/main.go:89-103`](../../../examples/providers/vercel/main.go#L89-L103)

```go
func (f *fakeVercel) Run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	fmt.Fprintf(f.w, "    [%s %s] stdin=%q\n", name, joined, stdin)
	switch {
	case joined == "whoami":
		return []byte("acme-bot\n"), nil
	case joined == "env ls production":
		return []byte(productionTable), nil
	case strings.HasPrefix(joined, "env ls"):
		return []byte("Vercel CLI 33.0.0\n> No Environment Variables found\n"), nil
	case f.failOn != "" && strings.HasPrefix(joined, "env add "+f.failOn+" "):
		return nil, errors.New("exit status 1: Error: simulated refusal")
	}
	return nil, nil
}
```
<!-- /ds:block hash=abcce6 -->

## Try it

```sh
cd examples/providers/vercel && go run .
```

```text
$ dotenvctl vercel push --dry-run
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: development
keys   : 3
API_KEY: skipped-placeholder
EMPTY: skipped-empty
DB_URL: would-push
DB_HOST: would-push
VERCEL_ANALYTICS_ID: would-push
exit 0

$ dotenvctl vercel push DB_URL --target production,preview --sensitive --yes
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: production, preview
keys   : 1
    [vercel env add DB_URL production --force --sensitive] stdin="postgres://db.internal/app"
    [vercel env add DB_URL preview --force --sensitive] stdin="postgres://db.internal/app"
DB_URL: pushed
exit 0

$ dotenvctl vercel push --prefix VERCEL_ --strip-prefix --target preview --dry-run
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: preview
keys   : 1
ANALYTICS_ID: would-push
exit 0

$ dotenvctl vercel list --target production
    [vercel env ls production] stdin=""
API_KEY
DB_URL
OLD_FLAG
exit 0

$ dotenvctl vercel list --target preview
    [vercel env ls preview] stdin=""
no environment variables found for target(s) preview (each target is listed separately — try another --target: production, preview or development)
exit 0

$ dotenvctl vercel prune --target production --dry-run
    [vercel env ls production] stdin=""
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: production
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: would-delete
exit 0

$ dotenvctl vercel list --sensitive
dotenvctl: unknown flag: --sensitive
exit 2

$ dotenvctl --json vercel push --target production --yes
    [vercel whoami] stdin=""
    [vercel env add DB_URL production --force] stdin="postgres://db.internal/app"
    [vercel env add DB_HOST production --force] stdin="db.internal"
{"ok":false,"data":{"meta":{"account":"acme-bot (stored vercel auth)","project":"unlinked directory (vercel CLI will resolve or prompt)","targets":"production"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"EMPTY","action":"skipped-empty"},{"name":"DB_URL","action":"pushed"}],"dry_run":false},"error":{"code":"exec","message":"push DB_HOST: target production: exit status 1: Error: simulated refusal"}}
exit 1
```

The `account:` lines read `(stored vercel auth)` only when `VERCEL_TOKEN` is unset in your shell; the example's test clears it.

## Pitfalls

- **The default target is `development`.** A forgotten `--target production` writes to development. That is deliberate, but read the `targets:` line in the banner.
- **`--sensitive` is a `push` flag only.** `list --sensitive` and `prune --sensitive` are usage errors (`unknown flag: --sensitive`, exit 2).
- **`--sensitive` goes to every target you name, including the default**, unchanged; Vercel decides whether a target accepts sensitive variables, and a refusal stops the push with the target named.
- **The provider does not remember how a variable was created.** Each push sends exactly the flags given this time; keep `--sensitive` in the script that pushes.
- **Sensitive values cannot be read back.** Your `.env` (or the secret store behind it) stays the source of truth; `dotenvctl` never reads values from Vercel.
- **With several targets, one key can be half-written.** `push` writes a key to each target in order; if the second target fails, the first already has the new value and the key is not reported as `pushed`. Re-run after fixing the cause.
- **The project line comes from the current directory, not from `-f`.** `dotenvctl -f app/.env vercel push` from the repository root reads the root's `.vercel/project.json` (or says `unlinked directory`). `cd app` first, so the banner and the `vercel` CLI agree.
- **Names are not checked locally.** Unlike the [GitHub provider](github.md#pitfalls), a name Vercel rejects fails at that key, after earlier keys were pushed.
- **Values are expanded before pushing.** `--expand` is on by default, so `${DB_HOST}` arrives resolved; pass `--expand=false` only if you mean to store the literal.
- **`list` and `prune` depend on the table parser.** Run `prune --dry-run` before every real prune. A failing `vercel env ls` fails `list` and `prune` rather than reading as an empty store.

## Related

- [GitHub provider](github.md): the same verbs for GitHub Actions secrets
- [Select which keys a command acts on](selection.md): the selection flags in full
- [Write a push provider](write-a-provider.md): the kit this provider is built on
- [Output and reports](output-and-reports.md): the `--json` envelope and exit codes
- Module reference: [dotenv/cli/providers/vercel README](../../../dotenv/cli/providers/vercel/README.md)
- Example: [`examples/providers/vercel`](../../../examples/providers/vercel); the plugin's own examples: [`ExampleNew`, `ExamplePlugin_Command`](../../../dotenv/cli/providers/vercel/example_test.go)
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/providers/vercel](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/providers/vercel)
- [Feature index](../../reference/features.md)
