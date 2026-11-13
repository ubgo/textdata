# dotenvctl recipes

Complete workflows built from the dotenvctl verbs: a CI gate, deriving one environment from another, one file serving several audiences, syncing to GitHub and Vercel, running tests against a chosen file, a drift report, loading a file into your shell, and safe edits in automation. Each recipe is copy-paste-able. The flags themselves are documented in the [command reference](dotenvctl.md) and [multi-environment tools](multi-env.md); this page is about composing them.

## Before you start

```sh
go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest
```

Outputs on this page come from real runs over the family used in [multi-environment tools](multi-env.md#before-you-start) (`.env`, `.env.staging`, `.env.prod`, `.env.example`). The GitHub and Vercel recipes need an authenticated `gh` or `vercel` CLI and write to real accounts, so they show commands only; their guides show the output. Example programs for the Go side live in [`examples/cli`](../../../examples/cli).

## Step 1 — gate the build on env completeness

### CI: gate the build on env completeness

The problem: someone adds `SENTRY_DSN` to the code and to `.env.example`, and three weeks later staging crashes because nobody added it to `.env.staging`. Make `.env.example` a contract and let CI enforce it:

```yaml
# .github/workflows/ci.yml
- name: env files satisfy the contract
  run: dotenvctl matrix .env.staging .env.prod --contract .env.example
```

`--contract` exits `1` when any environment misses a contract key, with the gaps named, after printing the whole table:

```console
$ dotenvctl matrix .env.staging .env.prod --contract .env.example; echo "exit=$?"
KEY         stag  prod
DB_HOST     ✓     ✓
DB_PASS     ✓     !
DOMAIN      ✓     ✓
FEATURE_X   ✓     #
SENTRY_DSN  —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
contract: stag is missing 1 key(s): [SENTRY_DSN]
exit=1
```

Name the deployed files, as above, unless your local `.env` must satisfy the contract too. Add `--json > /dev/null` when you want only the exit code in the log.

Add a second gate for placeholders: a `__YOU__` that reaches prod is a value nobody filled in. The contract gate does not catch it (a placeholder counts as present), and `matrix` needs two files, so read the PLACEHOLDERS column of `envs`:

```yaml
- name: no placeholder values left
  run: dotenvctl envs | awk 'NR > 1 && $NF > 0 { print "placeholders left:", $0; bad = 1 } END { exit bad }'
```

```console
$ dotenvctl envs | awk 'NR > 1 && $NF > 0 { print "placeholders left:", $0; bad = 1 } END { exit bad }'; echo "exit=$?"
placeholders left: prod                .env.prod     4     1         0          1
exit=1
```

Do not gate with `matrix … | grep '!'`: the legend line under every matrix contains `!`, so that check always fires.

## Step 2 — derive and split files

### Derive .env.prod from .env.staging

Copy the file and change only what differs. Every comment, banner and `${reference}` survives, so derived values (`API_URL=https://api.${DOMAIN}`) keep tracking:

```console
$ cp .env.staging .env.prod
$ dotenvctl -f .env.prod set DOMAIN=acme.io DB_HOST=prod.db.internal
$ dotenvctl -f .env.prod set DB_PASS=__YOU__
$ dotenvctl diff .env.staging .env.prod; echo "exit=$?"
~ DB_HOST: stag.db.internal -> prod.db.internal
~ DB_PASS: stag-pass -> __YOU__
~ DOMAIN: staging.acme.io -> acme.io
exit=1
```

Three changed lines and nothing else: that is the whole review. The placeholder shows as `!` in `matrix` and is never pushed by a provider. From Go, the same is `Clone` and `SaveAs` in the dotenv library ([editing guide](../dotenv/editing.md)).

### One file, several audiences

A single `.env.prod` holds app config and deploy credentials, separated by a prefix your repo chose:

```env
# mixed.env
A=1
B=2
A=3
# OFF=yes
HOME
GITHUB_SECRET_GHCR_PAT=ghp_xxxx
GITHUB_SECRET_DEPLOY_PATH=/srv/app
WHO=__YOU__
```

Read each audience without hand-rolled line parsing, which breaks on multiline values and inline comments:

```console
$ dotenvctl -f mixed.env list --prefix GITHUB_SECRET_ --strip-prefix --json
{"ok":true,"data":{"pairs":[{"key":"GHCR_PAT","value":"ghp_xxxx"},{"key":"DEPLOY_PATH","value":"/srv/app"}],"disabled":[{"key":"OFF","value":"yes"}],"inherited":["HOME"],"expanded":false}}
$ dotenvctl -f mixed.env list --exclude-prefix GITHUB_SECRET_
A    3
B    2
WHO  __YOU__
```

The selection flags filter `pairs` only; the JSON payload always carries the disabled and inherited sections, and with a selection flag `A` appears once with its last value. The same selection flags drive the providers, so previewing with `list` and pushing with `github push` can never disagree about which keys match.

## Step 3 — sync outward

### Sync deploy secrets to GitHub Actions

Requires an authenticated [`gh`](https://cli.github.com). The provider resolves the target repo from the current directory's git remote and prints it, with the acting account, before writing:

```sh
dotenvctl github env-create prod --yes                                                            # idempotent
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --dry-run   # preview first
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --yes
dotenvctl -f .env.prod github prune --prefix GITHUB_SECRET_ --strip-prefix --env prod --dry-run
```

Keys without the prefix are never considered, and a `__YOU__` placeholder is skipped with a warning instead of becoming a production secret. `prune` deletes remote names absent from your local keep-set, which is deliberately wider than what `push` pushes. Every flag and real output: [GitHub provider](../providers/github.md).

### Sync env vars to Vercel

Requires an authenticated [`vercel`](https://vercel.com/docs/cli) CLI and a linked directory (`vercel link`). The default target is `development`:

```sh
cd apps/web     # the linked directory: the banner reads .vercel/project.json here
dotenvctl -f .env.prod vercel push --prefix VERCEL_ --strip-prefix --target production --dry-run
dotenvctl -f .env.prod vercel push --prefix VERCEL_ --strip-prefix --target production --sensitive --yes
dotenvctl -f .env.prod vercel prune --prefix VERCEL_ --strip-prefix --target production --dry-run
```

Every flag and real output: [Vercel provider](../providers/vercel.md).

## Step 4 — run and report

### Run tests or services against a specific env file

```sh
dotenvctl -f .env.test run -- go test ./...
dotenvctl run -- npm start
```

The child's environment is your shell's overlaid with the file's values (the file wins); your own shell is never modified; the child's exit code passes through verbatim, so CI wrappers behave transparently. A failing `${VAR:?msg}` aborts before the child starts, so a missing required variable fails the run instead of starting a half-configured service. Real runs: [dotenvctl § run](dotenvctl.md#run--execute-a-command-with-the-files-values) and [`examples/cli/run-child`](../../../examples/cli/run-child).

### A weekly drift report for the team

```sh
dotenvctl matrix --only-drift --format html -o reports/env-drift.html
```

Self-contained HTML, no JavaScript, light and dark by the reader's preference, written `0600`, and masked by construction: without `--reveal` the values are never in the document, so the report is safe to attach to a ticket. Details: [HTML reports](multi-env.md#step-6--write-an-html-report).

### Load a file into your shell

`dotenvctl run` covers most cases, because the child gets the values and your shell stays clean. When you want the values in the current shell (an interactive debugging session, say), install the [`dotenvctl-export` exec plugin](exec-plugins.md#a-complete-plugin-dotenvctl-export) and `eval` its output:

```console
$ eval "$(dotenvctl export --prefix DB_URL)"; echo "$DB_URL"
postgres://db.internal/app
```

Values arrive expanded, placeholders are skipped with a note on stderr, and a failing `${VAR:?msg}` stops the plugin with exit `1`. Lines already printed for earlier keys still reach `eval`, so in scripts capture the output first and `eval` it only when the plugin succeeded.

## Step 5 — script safely

### jq cookbook

Every verb prints one `{"ok":…}` envelope under `--json`; the error `code` values are stable API (`not_found`, `io`, `required`, `usage`, `exec`). The payload shapes, with real output, are in the [command reference](dotenvctl.md):

```sh
dotenvctl get DB_HOST --json | jq -r .data.value
dotenvctl keys --json | jq -r '.data.keys[]'
dotenvctl list --expand --json | jq -r '.data.pairs[] | "\(.key)=\(.value)"'
dotenvctl diff .env.staging .env.prod --json | jq -r '.data.removed[].key'    # keys prod lacks
dotenvctl envs --json | jq -r '.data.files[].file'
dotenvctl matrix --json | jq '.data'                                          # cell states only; values need --reveal
```

### Safe edits in automation

```sh
dotenvctl set DEBUG=true --dry-run          # the exact -old/+new line diff, nothing written
dotenvctl set DB_PASSWORD=x --after DB_USER # a missing anchor exits 1 without writing
dotenvctl unset OLD_KEY                     # comment-out, not delete: reversible
dotenvctl restore OLD_KEY                   # byte-identical undo
```

Re-setting a key to its current value writes nothing (same bytes, same mtime), and `set` on a missing file creates it `0600`. One trap when scripting reads: a missing key and an empty value both leave the variable empty. A failing verb prints its message on stderr, so check the exit status together with the capture:

```console
$ if ! DB_HOST=$(dotenvctl -f .env.prod get NOPE); then echo "failed, DB_HOST=[$DB_HOST]"; fi
dotenvctl: key "NOPE" not found in .env.prod
failed, DB_HOST=[]
```

## Try it

Build the binary from a clone, write the four files from [multi-environment tools](multi-env.md#before-you-start) into a scratch directory, and run the recipes there:

```sh
cd dotenv/cli && go build -o /usr/local/bin/dotenvctl ./cmd/dotenvctl
```

The Go programs behind the run and plugin recipes run with `go run .` in [`examples/cli/run-child`](../../../examples/cli/run-child) and [`examples/cli/exec-plugin`](../../../examples/cli/exec-plugin).

## Pitfalls

- **The contract gate passes placeholders.** A key set to `__YOU__` is present; pair the contract gate with the placeholder gate.
- **`matrix` needs two files.** A one-file check exits `1`; use the `envs` PLACEHOLDERS column for one file.
- **`grep '!'` on a matrix always matches.** The legend line contains `!`.
- **`-f` typos read as an empty file.** `get` reports `not found`, `list` and `keys` print nothing with exit `0`. Check that the file exists when an empty answer would be wrong.
- **Provider pushes refuse to run unattended without `--yes`.** In CI, pass `--yes` after a reviewed `--dry-run`.
- **`eval` of a failing plugin still runs the lines printed before the failure.** Capture first, check the exit code, then `eval`.

## Related

- [dotenvctl command reference](dotenvctl.md) and [multi-environment tools](multi-env.md): every flag used above
- [Exec plugins](exec-plugins.md): `dotenvctl-export` in full
- [GitHub provider](../providers/github.md), [Vercel provider](../providers/vercel.md), [key selection](../providers/selection.md)
- [`dotenv/cli` README](../../../dotenv/cli/README.md)
