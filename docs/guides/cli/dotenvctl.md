# dotenvctl command reference

`dotenvctl` reads and edits `.env` files from the shell without disturbing a byte you did not touch: comments, blank lines, ordering and quoting survive every edit. This page walks through every verb and flag, each with the output of a real run. Use it when you script `.env` files in CI, a Makefile or a deploy step, or when you want to look at a family of env files side by side.

The same verbs are reachable two ways: `dotenvctl <verb>` (the standalone binary) and `textdatactl dotenv <verb>` (the umbrella binary, see [textdatactl](../getting-started/textdatactl.md)). The umbrella hands everything after `dotenv` to the same command package, so flags, exit codes, the `--json` envelope and exec plugins behave the same; only the program name in failure lines differs (`dotenvctl: …` or `textdatactl dotenv: …`).

| Verb | Job |
|---|---|
| [`get`](#get--print-one-value) | print one value, optionally `${expanded}` |
| [`list`](#list--the-files-contents) | the file's pairs as a table or JSON |
| [`keys`](#keys--names-only-pipe-friendly) | names only, for piping |
| [`set`](#set--upsert-values-byte-preserving) | upsert `KEY=VALUE` pairs, byte-preserving, anchored placement |
| [`unset`](#unset--deactivate-a-key) | comment keys out (default) or delete them |
| [`restore`](#restore--the-inverse-of-unset) | re-enable a commented-out key |
| [`run`](#run--execute-a-command-with-the-files-values) | run a child process with the file's values |
| [`diff`](#diff--compare-two-files-effective-configuration) | compare two files' effective configuration |
| [`envs`](#envs--discover-the-env-family) | discover the directory's `.env` family |
| [`matrix`](#matrix--the-keys--environments-drift-table) | keys × environments drift table, contract gate, HTML reports |
| [`github`](#github-and-vercel--push-providers) | push, list and prune GitHub Actions secrets |
| [`vercel`](#github-and-vercel--push-providers) | push, list and prune Vercel environment variables |
| [`plugins`](#plugins--discover-exec-plugins) | list third-party exec plugins on PATH |
| [`completion`](#completion--shell-completions) | shell completion scripts |

## Before you start

Install the binary (or the umbrella, which carries the same verbs):

```sh
go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest
go install github.com/ubgo/textdata/cmd/textdatactl@latest   # optional: textdatactl dotenv …
```

Every output on this page comes from a real run in a directory holding this family of files:

```env
# .env
# local development
DB_HOST=localhost
DB_PORT=5432
API_URL=https://api.${DOMAIN}
DOMAIN=acme.dev

# .env.staging
DB_HOST=stag.db.internal
DB_PASS=stag-pass
DOMAIN=staging.acme.io
FEATURE_X=on

# .env.prod
DB_HOST=prod.db.internal
DB_PASS=__YOU__
DOMAIN=acme.io
# FEATURE_X=on
SENTRY_DSN=https://key@sentry.example/1

# .env.example
DB_HOST=
DB_PASS=
DOMAIN=
SENTRY_DSN=
```

Runnable Go versions of every verb live in [`dotenv/cli/dotenvcmd/example_test.go`](../../../dotenv/cli/dotenvcmd/example_test.go) (`ExampleExecute_get`, `_set`, `_unset`, `_list`, `_keys`, `_envs`, `_matrix`, `_matrix_placeholder`, `_diff`, `_json`), and `run` has its own program in [`examples/cli/run-child`](../../../examples/cli/run-child).

One program runs the read, edit and compare verbs in a row through the umbrella and checks every output: [`examples/cli/verb-tour`](../../../examples/cli/verb-tour). `textdatactl dotenv <verb>` and `dotenvctl <verb>` take the same arguments:

<!-- ds:block id=dotenvtour-np6rj397 -->
**dotenvTour** · [`examples/cli/verb-tour/main.go:105-114`](../../../examples/cli/verb-tour/main.go#L105-L114)

```go
func dotenvTour(w io.Writer) {
	textdatactl(w, "dotenv", "list", "--expand")
	textdatactl(w, "dotenv", "keys")
	textdatactl(w, "dotenv", "unset", "DEBUG", "--dry-run")
	textdatactl(w, "dotenv", "unset", "DEBUG")
	textdatactl(w, "dotenv", "restore", "DEBUG")
	textdatactl(w, "dotenv", "envs")
	textdatactl(w, "dotenv", "matrix", "--contract", contract)
	textdatactl(w, "dotenv", "diff", devFile, prodFile, "--expand")
}
```
<!-- /ds:block hash=b2a179 -->

## Step 1 — learn the global flags, the envelope and the exit codes

Three flags work on every built-in verb, before or after the verb name (`dotenvctl -f .env.prod get X` and `dotenvctl get X -f .env.prod` are the same). For an exec plugin, put `-f` and `--json` before the plugin name: everything after it goes to the plugin untouched (see [exec plugins](exec-plugins.md)).

| Flag | Meaning |
|---|---|
| `-f`, `--file PATH` | the `.env` file to read or edit; default `.env` in the current directory |
| `--json` | print exactly one `{"ok":…}` envelope on stdout instead of human output |
| `-v`, `--version` | print the version and exit |

```console
$ dotenvctl get DB_HOST --json
{"ok":true,"data":{"key":"DB_HOST","value":"localhost","expanded":false}}
$ dotenvctl get NOPE --json; echo "exit=$?"
{"ok":false,"error":{"code":"not_found","message":"key \"NOPE\" not found in .env"}}
exit=1
$ dotenvctl get NOPE; echo "exit=$?"
dotenvctl: key "NOPE" not found in .env
exit=1
$ dotenvctl nosuch; echo "exit=$?"
dotenvctl: unknown command "nosuch" for "dotenvctl"
Run 'dotenvctl --help' for usage.
exit=2
$ dotenvctl get --bogus X --json; echo "exit=$?"
{"ok":false,"error":{"code":"usage","message":"unknown flag: --bogus"}}
exit=2
```

Exit codes are the contract with shells and CI: `0` ok, `1` the operation ran and failed (a missing key, a required variable, `diff` found differences, a `matrix --contract` gap), `2` the command line was malformed (unknown verb or flag, wrong number of arguments). `run` and exec plugins are the exceptions: they pass the child's code through. The constants are exported from `dotenvcmd`:

<!-- ds:block id=exitcodes-msf66qpb -->
**ExitOK** · [`dotenv/cli/dotenvcmd/root.go:38-50`](../../../dotenv/cli/dotenvcmd/root.go#L38-L50)

```go
const (
	// ExitOK: the operation succeeded (for diff: the files are identical).
	ExitOK = 0

	// ExitFailure: the operation ran and failed — missing key, required-var
	// error, diff found differences. Mirrors diff(1)'s "1 = differences".
	ExitFailure = 1

	// ExitUsage: the invocation itself was malformed (unknown flag, missing
	// operand). Mirrors the BSD EX_USAGE convention of 2-for-usage that
	// grep/diff follow.
	ExitUsage = 2
)
```
<!-- /ds:block hash=87cbec -->

Error `code` values in the envelope are stable API: `not_found`, `io`, `required`, `usage`, `exec`. The envelope itself comes from [`clikit/outfmt`](../../../clikit/README.md#outfmt).

`--version` reads the version from the build itself, so there is no constant to forget at release time. A `go install …@<tag>` binary prints that module's tag; a binary built from a checkout prints a pseudo-version; a workspace build prints `devel`. `--version` ignores `--json`:

```console
$ dotenvctl --version --json
dotenvctl version v0.1.5-0.20261113071213-e24c1e3cc8bb+dirty
```

## Step 2 — read values

### get — print one value

```sh
dotenvctl get DATABASE_URL
dotenvctl get DATABASE_URL --expand        # resolve ${references} against the file
dotenvctl -f .env.prod get PORT --json
```

```console
$ dotenvctl get API_URL
https://api.${DOMAIN}
$ dotenvctl get API_URL --expand
https://api.acme.dev
$ dotenvctl -f .env.prod get DB_HOST
prod.db.internal
```

`get` exits `1` when the key has no active entry. A commented-out setting (`# KEY=…`) and a name-only declaration (`KEY` alone) do not count: they are inactive, and `list --disabled` / `list --inherited` show them. A failing `${VAR:?message}` under `--expand` also exits `1`, with the message and the error code `required`. The `--json` payload is `{key, value, expanded}`.

### list — the file's contents

```sh
dotenvctl list                       # active pairs, aligned table, file order
dotenvctl list --expand              # values with ${references} resolved
dotenvctl list --disabled            # add the commented-out settings
dotenvctl list --inherited           # add name-only declarations
dotenvctl list --prefix GITHUB_SECRET_ --strip-prefix --json   # a selected, renamed view
```

```console
$ dotenvctl list
DB_HOST  localhost
DB_PORT  5432
API_URL  https://api.${DOMAIN}
DOMAIN   acme.dev
$ dotenvctl list --expand
DB_HOST  localhost
DB_PORT  5432
API_URL  https://api.acme.dev
DOMAIN   acme.dev
```

With a file that holds a duplicate, a disabled line, a name-only line and two prefixed keys (`mixed.env`):

```console
$ dotenvctl -f mixed.env list --disabled --inherited
A                          1
B                          2
A                          3
GITHUB_SECRET_GHCR_PAT     ghp_xxxx
GITHUB_SECRET_DEPLOY_PATH  /srv/app
WHO                        __YOU__

# disabled
# OFF=yes

# inherited (values come from the environment, not this file)
HOME
$ dotenvctl -f mixed.env list --json
{"ok":true,"data":{"pairs":[{"key":"A","value":"1"},{"key":"B","value":"2"},{"key":"A","value":"3"},{"key":"GITHUB_SECRET_GHCR_PAT","value":"ghp_xxxx"},{"key":"GITHUB_SECRET_DEPLOY_PATH","value":"/srv/app"},{"key":"WHO","value":"__YOU__"}],"disabled":[{"key":"OFF","value":"yes"}],"inherited":["HOME"],"expanded":false}}
$ dotenvctl -f mixed.env list --prefix GITHUB_SECRET_ --strip-prefix --json
{"ok":true,"data":{"pairs":[{"key":"GHCR_PAT","value":"ghp_xxxx"},{"key":"DEPLOY_PATH","value":"/srv/app"}],"disabled":[{"key":"OFF","value":"yes"}],"inherited":["HOME"],"expanded":false}}
$ dotenvctl -f mixed.env list --exclude-prefix GITHUB_SECRET_
A    3
B    2
WHO  __YOU__
```

Without selection flags, `list` shows the file as a linear read sees it: file order, duplicates included. With any selection flag it switches to the effective (last-wins) view: each key once, at its first position, with its last value. `--disabled` and `--inherited` shape the human view only; the JSON payload always carries all three sections (`pairs`, `disabled`, `inherited`), and selection flags filter `pairs` only.

### keys — names only, pipe-friendly

`keys` prints the active key names, one per line, with no decoration, so its output feeds `grep`, `xargs` and `while read` loops directly. It takes the same selection flags as `list`, so a filter you preview with `keys` is the filter a provider push uses.

```console
$ dotenvctl keys
DB_HOST
DB_PORT
API_URL
DOMAIN
$ dotenvctl keys --prefix DB_ --strip-prefix
HOST
PORT
$ dotenvctl keys --exclude-prefix DB_ --json
{"ok":true,"data":{"keys":["API_URL","DOMAIN"]}}
$ dotenvctl keys --include DOMAIN --prefix DB_
DB_HOST
DB_PORT
DOMAIN
$ dotenvctl keys --exclude DB_HOST,DB_PORT
API_URL
DOMAIN
$ dotenvctl keys --include NOPE; echo "exit=$?"
dotenvctl: envkit: --include key "NOPE" not found
exit=1
```

### Selecting keys: the shared flags

`list`, `keys` and every provider store verb (`github push`, `vercel prune`, …) take the same selection flags, resolved by one pipeline, so `list --prefix X` and `github push --prefix X` can never disagree about which keys match:

| Flag | Meaning |
|---|---|
| `--prefix P` | keep only keys starting with `P` |
| `--exclude-prefix P` | drop keys starting with `P` |
| `--strip-prefix` | remove the matched `--prefix` from the printed names |
| `--include KEY` | force-include a key past the prefix filters (repeatable, or comma-separated) |
| `--exclude KEY` | drop a key (repeatable, or comma-separated) |

Resolution order: all keys, then `--prefix` keeps, `--exclude-prefix` drops, `--include` adds, `--exclude` drops. A key you name with `--include` must exist, or the command fails with exit `1`. `--include` and `--exclude` match the name as written in the file, never the stripped one, and `--exclude` beats `--include`. On reads the placeholder and empty-value guards are off: `list` shows `WHO=__YOU__`, while `github push` would skip it. The grammar in full, with placeholders and the guards: [key selection](../providers/selection.md).

## Step 3 — edit values

### set — upsert values, byte-preserving

```sh
dotenvctl set DB_HOST=localhost DB_PORT=5432        # several KEY=VALUE arguments
dotenvctl set CONN="postgres://h/db?sslmode=require"        # only the first '=' splits
dotenvctl set DB_USER=admin --after DB_PORT          # place a NEW key after an anchor
dotenvctl set DB_DRIVER=postgres --before DB_HOST    # …or before one
dotenvctl set DEBUG=true --dry-run                   # print the line diff, write nothing
```

`--dry-run` prints a minimal `-old`/`+new` line diff, the entire change the write would make:

```console
$ dotenvctl set DEBUG=true --dry-run
-DEBUG=false
+DEBUG=true
$ dotenvctl set DEBUG=false; echo "exit=$?"
no changes (values already current)
exit=0
$ dotenvctl set DEBUG=false --json
{"ok":true,"data":{"results":[{"key":"DEBUG","created":false}],"changed":false,"dry_run":false}}
$ dotenvctl set X=1 --after NOPE; echo "exit=$?"
dotenvctl: envkit: anchor key not found: "NOPE"
exit=1
$ dotenvctl set A=1 --after DB_HOST --before DB_PORT --json; echo "exit=$?"
{"ok":false,"error":{"code":"usage","message":"--after and --before are mutually exclusive"}}
exit=2
$ dotenvctl set NOEQ; echo "exit=$?"
dotenvctl: argument "NOEQ" is not KEY=VALUE
exit=2
```

Rules worth knowing:

- An existing key updates in place and never moves; only a new key is placed by `--after` or `--before`. A missing anchor exits `1` without writing anything, so there is no silent append.
- Setting a key to its current value writes nothing: same bytes, same mtime, file watchers see nothing. `changed` in the JSON payload says so.
- What the author wrote around the value survives: `export` prefixes, indentation, spacing around the delimiter, inline comments, and the `=` or `:` delimiter. The value's quoting is chosen for the new value.
- A value is stored literally: `dotenvctl set 'P=a$b'` writes `P='a$b'`, so `get --expand` gives `a$b` back. To store a reference such as `${HOST}`, edit the file by hand.
- A missing file is created with mode `0600`, because env files hold credentials.

### unset — deactivate a key

```console
$ dotenvctl unset DB_HOST DB_PORT --dry-run
-DB_HOST=localhost
-DB_PORT=5432
+# DB_HOST=localhost
+# DB_PORT=5432
$ dotenvctl unset DB_HOST --delete --dry-run
-DB_HOST=localhost
$ dotenvctl unset NOPE; echo "exit=$?"
dotenvctl: envkit: key not found: "NOPE"
exit=1
```

Comment-out is the default on purpose: it is reversible, diff-friendly, and the documentation above the setting stays attached to something. `--delete` removes the line. The JSON payload is `{keys, deleted, dry_run}`.

### restore — the inverse of unset

```console
$ dotenvctl unset DEBUG && grep DEBUG .env
# DEBUG=false
$ dotenvctl restore DEBUG && grep DEBUG .env
DEBUG=false
$ dotenvctl restore NOPE; echo "exit=$?"
dotenvctl: envkit: key not found: no disabled entry for "NOPE"
exit=1
```

`restore` removes exactly the comment marker that was recorded, so `#KEY=v` comes back without a space being invented; `unset` followed by `restore` returns the file to its original bytes. `list --disabled` shows what can be restored. `restore` has no `--dry-run`.

## Step 4 — run a command with the file's values

### run — execute a command with the file's values

```sh
dotenvctl run -- npm start
dotenvctl -f .env.test run -- go test ./...
dotenvctl run -- sh -c 'echo $DATABASE_URL'
```

```console
$ dotenvctl run -- sh -c 'echo "child sees: $DB_HOST"'
child sees: localhost
$ DB_HOST=from-shell dotenvctl run -- sh -c 'echo "file wins: $DB_HOST"'
file wins: localhost
$ dotenvctl run -- sh -c 'echo $API_URL'
https://api.acme.dev
$ dotenvctl run -- sh -c 'exit 3'; echo "exit=$?"
exit=3
$ dotenvctl -f ../req.env run -- echo hi; echo "exit=$?"     # req.env holds NEED=${MISSING:?fill me}
dotenvctl: envkit: expand: dotenv: required variable MISSING is not set or is empty: fill me
exit=1
$ dotenvctl run -- nosuchbinary; echo "exit=$?"
dotenvctl: envkit: run nosuchbinary: exec: "nosuchbinary": executable file not found in $PATH
exit=1
```

Semantics:

- The child's environment is your shell's environment overlaid with the file's values, and the file wins: that is why you ran it.
- Values arrive expanded; a failing `${VAR:?msg}` stops the run before the child starts, with exit `1` and the error code `exec`.
- dotenvctl's own environment is never modified; the merge exists only in the child.
- The child's exit code passes through verbatim, so CI wrappers behave transparently.
- Name-only lines (`HOME`) need no handling: the child inherits the parent environment anyway.

The same from Go, with `dotenvcmd.Execute` and the child's output captured in a writer ([`examples/cli/run-child`](../../../examples/cli/run-child)):

<!-- ds:block id=runchild-7dpqrucc -->
**runChild** · [`examples/cli/run-child/main.go:97-100`](../../../examples/cli/run-child/main.go#L97-L100)

```go
func runChild(w io.Writer, envFile string, command ...string) int {
	args := append([]string{"-f", envFile, "run", "--"}, command...)
	return dotenvcmd.Execute(args, w, w)
}
```
<!-- /ds:block hash=1b61db -->

```console
$ cd examples/cli/run-child && go run .
child: from the file, postgres://db.internal/app
exit: 0
parent still has: from the shell
exit: 3
dotenvctl: envkit: expand: dotenv: required variable MISSING is not set or is empty: set MISSING first
exit: 1
```

For typed access without the CLI, call `envkit.Run`, `ChildEnv` and `MergeEnv` directly; the [`envkit` example functions](../../../dotenv/cli/envkit/example_test.go) show each.

## Step 5 — compare files

### diff — compare two files' effective configuration

```console
$ dotenvctl diff .env.staging .env.prod; echo "exit=$?"
+ SENTRY_DSN=https://key@sentry.example/1
- FEATURE_X=on
~ DB_HOST: stag.db.internal -> prod.db.internal
~ DB_PASS: stag-pass -> __YOU__
~ DOMAIN: staging.acme.io -> acme.io
exit=1
$ dotenvctl diff .env.staging .env.staging; echo "exit=$?"
exit=0
$ dotenvctl diff .env.staging .env.prod --json
{"ok":true,"data":{"added":[{"key":"SENTRY_DSN","value":"https://key@sentry.example/1"}],"removed":[{"key":"FEATURE_X","value":"on"}],"changed":[{"key":"DB_HOST","from":"stag.db.internal","to":"prod.db.internal"},{"key":"DB_PASS","from":"stag-pass","to":"__YOU__"},{"key":"DOMAIN","from":"staging.acme.io","to":"acme.io"}],"expanded":false,"file_a":".env.staging","file_b":".env.prod"}}
$ dotenvctl diff .env.staging nope.env; echo "exit=$?"
dotenvctl: envkit: nope.env does not exist
exit=1
```

`+ KEY=…` is only in the second file, `- KEY=…` only in the first, `~ KEY: a -> b` changed. Exit `0` identical, `1` different (like `diff(1)`). The comparison is over the effective last-wins view, so formatting, comments and shadowed duplicates are invisible on purpose. Flags: `--expand` compares resolved values, `--format human|json|html` and `-o/--output PATH` choose the output, and `--reveal` puts real values into an HTML report. A missing file is an error here, where the other verbs read it as empty. The human and JSON output carry real values; treat them like the files themselves.

### envs — discover the .env family

```console
$ dotenvctl envs
ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default             .env          4     0         0          0
stag                .env.staging  4     0         0          0
prod                .env.prod     4     1         0          1
example (contract)  .env.example  4     0         0          0
```

`--dir PATH` scans another directory (default `.`). Detection is filename-based and non-recursive; `.env.example`, `.sample`, `.template` and `.dist` are contracts. Full rules and the JSON shape: [multi-environment tools](multi-env.md#step-1--envs-discover-the-family).

### matrix — the keys × environments drift table

```console
$ dotenvctl matrix .env.staging .env.prod --only-drift
KEY         stag  prod
DB_PASS     ✓     !
FEATURE_X   ✓     #
SENTRY_DSN  —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

| Flag | Meaning |
|---|---|
| `FILE...` | compare exactly these files (at least two); default: every file `envs` finds in `--dir` |
| `--dir PATH` | directory to scan when no files are named (default `.`) |
| `--only-drift` | hide rows that are `✓` in every column |
| `--values` | show values instead of symbols, masked as `••••••` |
| `--reveal` | with `--values` or HTML, show real values |
| `--contract FILE` | add a contract's keys and exit `1` when an environment misses one |
| `--placeholder REGEX` | what counts as a placeholder (default `^__[A-Z0-9_]+__$`) |
| `--format human\|json\|html`, `-o PATH` | output format and destination |

Every flag with real output, the HTML reports and the sharp edges: [multi-environment tools](multi-env.md#step-2--matrix-the-drift-table).

## Step 6 — sync outward and extend

### github and vercel — push providers

`github` pushes, lists and prunes GitHub Actions secrets through an authenticated `gh` (repo, environment and organization secrets; `env-create` makes a deployment environment). `vercel` does the same for Vercel environment variables through the `vercel` CLI (`--target`, `--sensitive`). Both share the selection flags above, skip placeholder and empty values, pass values on stdin rather than in arguments, print the target before writing, and refuse to write without `--yes` when nobody can answer the prompt.

```sh
dotenvctl -f .env.staging github push --prefix GITHUB_SECRET_ --strip-prefix --dry-run
dotenvctl vercel push --target production --yes
```

Full guides with every verb and flag: [GitHub provider](../providers/github.md) and [Vercel provider](../providers/vercel.md).

### plugins — discover exec plugins

Any executable named `dotenvctl-<name>` on PATH becomes a subcommand, git-style: `dotenvctl acme push` runs `dotenvctl-acme push`. `plugins` lists what was found, so you can see which third-party code a `dotenvctl <name>` would run. Built-ins always win; a PATH binary with a built-in's name is listed as `shadowed by built-in` and never run.

```console
$ dotenvctl plugins
no exec plugins found on PATH (binaries named dotenvctl-<name>)
$ dotenvctl plugins --json
{"ok":true,"data":{"plugins":[]}}
```

Writing one, the `DOTENVCTL_*` variables it receives, and calling the host back: [exec plugins](exec-plugins.md).

### completion — shell completions

`completion` prints a tab-completion script, so `dotenvctl <TAB>` offers verbs and `dotenvctl list --<TAB>` offers flags with their help text.

```sh
dotenvctl completion bash          # also: zsh, fish, powershell
dotenvctl completion zsh --help    # the exact install lines for that shell
```

| Shell | Try in this session | Install for every session |
|---|---|---|
| bash (needs the `bash-completion` package) | `source <(dotenvctl completion bash)` | `dotenvctl completion bash > /etc/bash_completion.d/dotenvctl` (Linux) or `dotenvctl completion bash > $(brew --prefix)/etc/bash_completion.d/dotenvctl` (macOS) |
| zsh | `source <(dotenvctl completion zsh)` | `dotenvctl completion zsh > "${fpath[1]}/_dotenvctl"`, then start a new shell |
| fish | `dotenvctl completion fish \| source` | `dotenvctl completion fish > ~/.config/fish/completions/dotenvctl.fish` |
| powershell | `dotenvctl completion powershell \| Out-String \| Invoke-Expression` | add the output of that command to your PowerShell profile |

```console
$ dotenvctl completion zsh | head -2
#compdef dotenvctl
compdef _dotenvctl dotenvctl
```

Every shell subcommand takes `--no-descriptions` to complete names only. The script completes verbs and flags, not key names from your `.env`; use `dotenvctl keys` for those.

Under the umbrella, use `textdatactl completion bash` (or `textdatactl dotenv completion bash`, which prints the same script): it is a script for the `textdatactl` binary that completes the formats, then each format's verbs and flags, so `textdatactl dotenv li<TAB>` offers `list`.

## Try it

The fastest way to see every verb is the example functions, which run each verb through `dotenvcmd.Execute` on temp files:

```console
$ cd dotenv/cli && go test -run '^ExampleExecute' ./dotenvcmd/
ok  	github.com/ubgo/textdata/dotenv/cli/dotenvcmd	0.029s
```

Or build the binary from a clone and run the commands on this page in a scratch directory:

```sh
cd dotenv/cli && go build -o /usr/local/bin/dotenvctl ./cmd/dotenvctl
```

## Pitfalls

- **Failure messages go to stderr in human mode.** When a verb fails, the `dotenvctl: …` line goes to stderr and stdout stays empty, so `VALUE=$(dotenvctl get NOPE)` leaves `VALUE` empty. An empty value is also a valid value, so check the exit code: `if VALUE=$(dotenvctl get KEY); then …; fi`.
- **A missing file reads as empty.** `get` reports `not found`, `keys` and `list` print nothing and exit `0` (`dotenvctl -f .env.typo keys --json` gives `{"ok":true,"data":{"keys":[]}}`), and `set` creates the file. A typo in `-f` is not reported as a missing file. `diff` is the exception: a missing file is an error there.
- **Flags after `--` belong to the child.** `dotenvctl run -- cmd --json` passes `--json` to `cmd`.
- **Verbs that take no arguments reject them as unknown commands.** `dotenvctl keys DB_HOST` prints `unknown command "DB_HOST" for "dotenvctl keys"` and exits `2`; filter with `--prefix` or `--include` instead.
- **Every usage mistake exits 2.** Besides unknown verbs and flags, that covers what a verb detects itself: `set` with both `--after` and `--before`, a `set` argument without `=`, an unknown `--format` value and an invalid `matrix --placeholder` regexp. Each reports the error code `usage`. An unknown `--format` is rejected before `-o` opens its file, so an existing report is left alone.
- **`keys` lists each name once; `list` prints every line.** A file with `A=1`, `B=2`, `A=3` gives `A`, `B` from `keys` and three rows from plain `list`.
- **`--json` is exactly one envelope per run**, on stdout, including usage errors. Not envelopes: `--version`, `help` and `--help`, `completion` (their output is the version line, help text or a script), `run` (the child's own output) and exec plugins (whatever the plugin prints).
- **`completion` under the umbrella still writes a dotenvctl script.** `textdatactl dotenv completion zsh` prints `#compdef dotenvctl`; install completions from the `dotenvctl` binary.

## Related

- [Multi-environment tools](multi-env.md): `envs`, `matrix`, contracts and HTML reports in depth
- [Recipes](recipes.md): complete workflows built from these verbs
- [Exec plugins](exec-plugins.md) and [embedding the CLI](embedding.md)
- [GitHub provider](../providers/github.md), [Vercel provider](../providers/vercel.md), [key selection](../providers/selection.md)
- [jsonc CLI](jsonc-cli.md): the same contract for JSON and JSONC files
- [`dotenv/cli` README](../../../dotenv/cli/README.md) and the API on [pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/dotenvcmd)
- Examples: [`examples/cli/run-child`](../../../examples/cli/run-child), [`dotenvcmd/example_test.go`](../../../dotenv/cli/dotenvcmd/example_test.go)
