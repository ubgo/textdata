# Multi-environment tools

A project usually holds a family of env files: `.env`, `.env.local`, `.env.staging`, `.env.prod`, `.env.example`. Three dotenvctl verbs answer the cross-file questions: `envs` (what exists), `matrix` (what drifts, what is missing, what is still a placeholder) and `diff` (what exactly differs between two files), plus a self-contained HTML report you can hand to someone. Use them in review, in CI as a gate, and whenever a deploy fails because one environment lacks a key.

## Before you start

Install `dotenvctl` (or use `textdatactl dotenv`, which runs the same verbs):

```sh
go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest
```

You need a directory holding at least two env files. Every output on this page comes from a real run over this family, the same files the [`dotenvcmd` example functions](../../../dotenv/cli/dotenvcmd/example_test.go) (`ExampleExecute_envs`, `ExampleExecute_matrix`, `ExampleExecute_diff`, `ExampleExecute_json`) use:

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

## Step 1 — envs: discover the family

```sh
dotenvctl envs
dotenvctl envs --dir ./deploy
dotenvctl envs --json | jq -r '.data.files[].file'
```

```console
$ dotenvctl envs
ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default             .env          4     0         0          0
stag                .env.staging  4     0         0          0
prod                .env.prod     4     1         0          1
example (contract)  .env.example  4     0         0          0
$ dotenvctl envs --dir ../empty; echo "exit=$?"
no env files found
exit=0
$ dotenvctl envs --dir ../nodir; echo "exit=$?"
dotenvctl: discover: read ../nodir: open ../nodir: no such file or directory
exit=1
```

`--json` gives one object per file, with the path relative to `--dir` and a `contract` flag:

```console
$ dotenvctl envs --json
{"ok":true,"data":{"files":[{"env":"default","file":".env","path":"./.env","contract":false,"keys":4,"disabled":0,"inherited":0,"placeholders":0},{"env":"stag","file":".env.staging","path":"./.env.staging","contract":false,"keys":4,"disabled":0,"inherited":0,"placeholders":0},{"env":"prod","file":".env.prod","path":"./.env.prod","contract":false,"keys":4,"disabled":1,"inherited":0,"placeholders":1},{"env":"example","file":".env.example","path":"./.env.example","contract":true,"keys":4,"disabled":0,"inherited":0,"placeholders":0}]}}
```

Detection rules (non-recursive, by file name):

| File name | Classified as |
|---|---|
| `.env` | `default` |
| `.env.local`, `.env.dev`/`.env.development`, `.env.test`/`.env.testing`, `.env.ci`, `.env.stag`/`.env.staging`, `.env.prod`/`.env.production` | the short name; long forms collapse, so `.env.production` and `.env.prod` are the same environment |
| `.env.example`, `.env.sample`, `.env.template`, `.env.dist` | a contract: listed here, never a matrix column, usable with `--contract` |
| `.env.bak`, `.tmp`, `.old`, `.orig`, `.swp`, `.swo`, `.lock` | ignored (editor and backup leftovers) |
| any other `.env.something` | kept, named `something` |

Ordering is distance from prod: `default`, `local`, `dev`, `test`, `ci`, `stag`, `prod`, then unknown names alphabetically, contracts last. `foo.env` is not detected. The PLACEHOLDERS column counts values matching `^__[A-Z0-9_]+__$` (the `__YOU__` convention): your "still needs a real value" signal. The same discovery is a Go call: `envkit.Inventories`, built on `discover.Scan` and `discover.Classify` ([examples](../../../dotenv/cli/discover/example_test.go)).

## Step 2 — matrix: the drift table

```sh
dotenvctl matrix                                   # every detected file as a column
dotenvctl matrix .env.staging .env.prod            # exactly these files
dotenvctl matrix --only-drift                      # hide rows present everywhere
dotenvctl matrix --values                          # values, masked
dotenvctl matrix --values --reveal                 # real values (treat the output as a secret)
dotenvctl matrix --contract .env.example           # CI gate: exit 1 when an env misses a contract key
```

```console
$ dotenvctl matrix
KEY         default  stag  prod
API_URL     ✓        —     —
DB_HOST     ✓        ✓     ✓
DB_PASS     —        ✓     !
DB_PORT     ✓        —     —
DOMAIN      ✓        ✓     ✓
FEATURE_X   —        ✓     #
SENTRY_DSN  —        —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
$ dotenvctl matrix .env.staging .env.prod --only-drift
KEY         stag  prod
DB_PASS     ✓     !
FEATURE_X   ✓     #
SENTRY_DSN  —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

Cell legend: `✓` an active pair with a real value · `!` set, but to a placeholder · `∅` set to empty · `#` only a commented-out setting exists · `→` a name-only declaration (the value comes from the environment) · `—` no entry at all. The distinctions matter: disabled, missing and placeholder are different problems with different fixes, and a parse-to-map tool sees all three as "absent". `.env.example` is never a column: it is the contract.

### Values, masked and revealed

```console
$ dotenvctl matrix .env.staging .env.prod --values
KEY         stag    prod
DB_HOST     ••••••  ••••••
DB_PASS     ••••••  ••••••
DOMAIN      ••••••  ••••••
FEATURE_X   ••••••  #
SENTRY_DSN  —       ••••••

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
$ dotenvctl matrix .env.staging .env.prod --values --reveal
⚠ values revealed — treat this output as a secret
KEY         stag              prod
DB_HOST     stag.db.internal  prod.db.internal
DB_PASS     stag-pass         __YOU__
DOMAIN      staging.acme.io   acme.io
FEATURE_X   on                #
SENTRY_DSN  —                 https://key@sentry.example/1

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

`--json` carries cell states only; value fields appear only with `--reveal`:

```console
$ dotenvctl matrix .env.staging .env.prod --json
{"ok":true,"data":{"envs":["stag","prod"],"rows":[{"key":"DB_HOST","cells":[{"env":"stag","state":"present"},{"env":"prod","state":"present"}]},{"key":"DB_PASS","cells":[{"env":"stag","state":"present"},{"env":"prod","state":"placeholder"}]},{"key":"DOMAIN","cells":[{"env":"stag","state":"present"},{"env":"prod","state":"present"}]},{"key":"FEATURE_X","cells":[{"env":"stag","state":"present"},{"env":"prod","state":"disabled"}]},{"key":"SENTRY_DSN","cells":[{"env":"stag","state":"missing"},{"env":"prod","state":"present"}]}],"revealed":false,"sources":[".env.staging",".env.prod"]}}
```

## Step 3 — gate CI on a contract

`--contract FILE` adds the contract's keys to the table, reports which environment misses which, and exits `1` when there is any gap, after printing the whole table:

```console
$ dotenvctl matrix --contract .env.example; echo "exit=$?"
KEY         default  stag  prod
API_URL     ✓        —     —
DB_HOST     ✓        ✓     ✓
DB_PASS     —        ✓     !
DB_PORT     ✓        —     —
DOMAIN      ✓        ✓     ✓
FEATURE_X   —        ✓     #
SENTRY_DSN  —        —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
contract: default is missing 2 key(s): [DB_PASS SENTRY_DSN]
contract: stag is missing 1 key(s): [SENTRY_DSN]
exit=1
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

If your local `.env` is not meant to satisfy the contract, name the deployed files, as in the second run. The [CI recipe](recipes.md#ci-gate-the-build-on-env-completeness) adds a placeholder gate beside it.

## Step 4 — change what counts as a placeholder

By default a value is a placeholder when the whole value matches `^__[A-Z0-9_]+__$` (`__YOU__`, `__FILL_ME__`). `--placeholder REGEX` replaces that pattern, so keep the default in an alternation when you want both:

```console
$ dotenvctl matrix .env.staging .env.prod --placeholder '^https://key@'
KEY         stag  prod
DB_HOST     ✓     ✓
DB_PASS     ✓     ✓
DOMAIN      ✓     ✓
FEATURE_X   ✓     #
SENTRY_DSN  —     !

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
$ dotenvctl matrix .env.staging .env.prod --placeholder '^(__[A-Z0-9_]+__|https://key@.*)$' --only-drift
KEY         stag  prod
DB_PASS     ✓     !
FEATURE_X   ✓     #
SENTRY_DSN  —     !

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
$ dotenvctl matrix --placeholder '('; echo "exit=$?"
dotenvctl: envkit: placeholder pattern "(": error parsing regexp: missing closing ): `(`
exit=2
```

In the first run `DB_PASS=__YOU__` turned into `✓`, because the new pattern no longer matches it. `--placeholder` exists only on `matrix`: the `envs` PLACEHOLDERS column, the provider push guards and the `DOTENVCTL_PLACEHOLDER` value given to exec plugins always use the default.

## Step 5 — diff two files

`diff` compares the effective (last-wins) view of two files and lists keys added, removed and changed; the exit code is `1` when they differ, like `diff(1)`:

```console
$ dotenvctl diff .env.staging .env.prod; echo "exit=$?"
+ SENTRY_DSN=https://key@sentry.example/1
- FEATURE_X=on
~ DB_HOST: stag.db.internal -> prod.db.internal
~ DB_PASS: stag-pass -> __YOU__
~ DOMAIN: staging.acme.io -> acme.io
exit=1
```

Every flag and the JSON shape: [dotenvctl § diff](dotenvctl.md#diff--compare-two-files-effective-configuration).

## Step 6 — write an HTML report

`matrix` and `diff` take `--format human|json|html` and `-o/--output PATH`:

```sh
dotenvctl matrix --format html -o envs.html
dotenvctl diff .env.staging .env.prod --format html -o diff.html
```

The page is self-contained: inline CSS, no JavaScript, no external requests, light and dark through the reader's `prefers-color-scheme`. The file is written with mode `0600`. It is masked by construction: without `--reveal` the values are never placed in the document at all (not hidden by styling), so the default report is safe to attach to a ticket. `--reveal` embeds real values and stamps a "CONTAINS SECRETS" banner:

```console
$ dotenvctl matrix --format html -o envs.html && ls -l envs.html | cut -c1-10
-rw-------
$ dotenvctl diff .env.staging .env.prod --format html -o diff.html; grep -c stag-pass diff.html
0
$ dotenvctl diff .env.staging .env.prod --format html --reveal -o diff2.html; grep -c stag-pass diff2.html
1
```

The HTML is rendered by [`clikit/report`](../../../clikit/README.md#report) (`MatrixHTML`, `DiffHTML`); the [output and reports guide](../providers/output-and-reports.md) shows it from Go.

## Try it

From a clone, build the binary and run the steps on the family above:

```sh
cd dotenv/cli && go build -o /usr/local/bin/dotenvctl ./cmd/dotenvctl
```

Or run the same verbs through `dotenvcmd.Execute` on temp files:

```console
$ cd dotenv/cli && go test -run 'ExampleExecute_(envs|matrix|diff|json)' -v ./dotenvcmd/ | grep -E '^(---|ok)'
--- PASS: ExampleExecute_envs (0.00s)
--- PASS: ExampleExecute_matrix (0.00s)
--- PASS: ExampleExecute_matrix_placeholder (0.00s)
--- PASS: ExampleExecute_diff (0.00s)
--- PASS: ExampleExecute_json (0.00s)
ok  	github.com/ubgo/textdata/dotenv/cli/dotenvcmd	0.014s
```

## Pitfalls

- **`--only-drift` counts a missing cell as drift.** A column much sparser than the rest (a one-key `.env` beside full staging and prod files) marks nearly every row as drift, and the filter seems to do nothing. dotenvctl prints a hint naming the sparse column; compare just the files you mean:

  ```console
  $ dotenvctl matrix --only-drift
  KEY  default  stag  prod
  K02  —        ✓     ✓
  …
  K12  —        ✓     ✓

  ✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing

  hint: --only-drift hid 1 row(s) — "default" is missing 11 of 11 keys, so almost every row counts as drift. Compare just the files you mean: dotenvctl matrix FILE_A FILE_B --only-drift
  ```

- **`matrix` needs two files.** `dotenvctl matrix .env.prod` exits `1` with `envkit: matrix needs at least two env files, found 1`. To check one file for placeholders, read the `envs` PLACEHOLDERS column.
- **`grep '!'` always matches.** The legend line under every human matrix contains `! placeholder`. Count placeholders with `envs`, or select `"state":"placeholder"` cells from `matrix --json`.
- **`--values` hides the placeholder marker.** Masked values are all `••••••`, placeholders included; `!` appears only in the default symbol view.
- **The contract checks presence, not quality.** A contract key set to `__YOU__` passes `--contract`; a key that exists only commented out (`#`) fails it.
- **Two files with the same environment are relabeled, not merged.** `.env.stag` and `.env.staging` both classify as `stag`; `envs` lists both, and `matrix .env.stag .env.staging` labels the columns by file name. Two directories' `.env` files become `a/.env` and `b/.env`. No file's data is dropped.
- **`diff --json` carries real values, `matrix --json` does not.** Treat a `diff` envelope like the env files themselves.
- **An unknown `--format` is a usage error.** `matrix --format xml` prints `--format must be human, json, or html` and exits `2` before `-o` creates or truncates its file.

## Related

- [dotenvctl command reference](dotenvctl.md): every verb and flag
- [Recipes](recipes.md): the CI gate and the weekly drift report in full
- `envkit.Inventories`, `BuildMatrix`, `DriftRows`, `SparseColumn` and `Diff`, the functions behind these verbs: [envkit examples](../../../dotenv/cli/envkit/example_test.go) and [pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/envkit)
- [`dotenv/cli` README](../../../dotenv/cli/README.md#multi-environment-see-the-whole-family-at-once)
