# Options for your kind of app

The same file can be read by a service at startup, edited by a CLI a person runs, checked in CI, and pushed to a deploy platform. Each of those wants different options turned on. This page lists the options that matter for safety and correctness, then gives a setup per kind of app.

## The options that change behaviour

| Option | Default | Turn it on when | Turn it off / leave it off when | Guide | Example |
|---|---|---|---|---|---|
| Command substitution `$(cmd)`: `WithCommandSubstitution(true)` in dotenv and jsonc, or a `CommandRunner` plugin | **off** | the file is yours, trusted, and a value really comes from a command | the file comes from a repository, an image, a teammate or a user: with it on, reading the file runs shell | [dotenv Step 7](../guides/dotenv/expansion.md#step-7--run-commands-opt-in), [jsonc Step 4](../guides/jsonc/expansion-and-plugins.md#step-4--turn-on-command-substitution-deliberately) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion), [`examples/jsonc/expand`](../../examples/jsonc/expand) |
| An allow-listed runner: `dotenv.WithCommandRunner(fn)` or a `CommandRunner` plugin | - | you need `$(cmd)` but only for known commands | - | [dotenv Step 7](../guides/dotenv/expansion.md#step-7--run-commands-opt-in) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion) |
| File confinement: `fileplugin.New(dir)`, `extends.New(extends.WithRoot(dir))` | **confined** to the base dir; absolute `$extends` refused | - | widen with `fileplugin.WithRoot` (a parent folder) before ever using `fileplugin.Unconfined()` | [file](../guides/plugins/file.md#step-3--widen-the-root-only-as-far-as-needed), [extends](../guides/plugins/extends.md#step-4--read-from-disk-confined) | [`examples/plugins/file-confined`](../../examples/plugins/file-confined), [`examples/plugins/extends-fs`](../../examples/plugins/extends-fs) |
| SaveGuards: `dotenv.WithSaveGuard(fn)`, or any plugin with `GuardSave(plugin.Document)` | none | a program writes files people commit: refuse private keys, real secrets in `.env.example`, a broken schema | - | [dotenv plugins](../guides/dotenv/plugins.md#step-1--install-one-function-hooks), [jsonc save Step 5](../guides/jsonc/save.md#step-5--veto-a-save-with-a-saveguard), [base plugin](../guides/base/plugin.md#step-1--write-a-plugin-with-the-abilities-it-needs) | [`examples/dotenv/write-a-plugin`](../../examples/dotenv/write-a-plugin), [`examples/base/registry`](../../examples/base/registry) |
| Schema validation: `schema.Validate`, or `schema.Plugin` plus `doc.Validate()` (jsonc) | none | users write the config by hand and you want `file:line:col` errors before the app starts | - | [schema](../guides/jsonc-tools/schema.md) | [`examples/jsonc-tools/schema-plugin`](../../examples/jsonc-tools/schema-plugin) |
| Lint: `jsonc.Lint`, `jsonc.ForbidTrailingCommas` | none | CI should reject duplicate keys, mixed indentation or trailing commas | - | [lint and validate](../guides/jsonc/lint-validate.md) | [`examples/jsonc/lint`](../../examples/jsonc/lint) |
| Strict JSON: `jsonc.Strict()` | off | the file must stay parseable by a plain JSON parser | the file is meant for people (comments welcome) | [parse and strict](../guides/jsonc/parse-and-strict.md#step-2--parse-plain-json-with-strict) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Required values `${NAME:?message}` | - | a value must be set before the app starts | - | [dotenv Step 6](../guides/dotenv/expansion.md#step-6--fail-loudly-with-requirederror), [jsonc Step 2](../guides/jsonc/expansion-and-plugins.md#step-2--fail-loudly-on-a-missing-value) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion) |

## Setups

### A service reading its config at startup (library inside an app)

- Call `dotenv.Read` or `jsonc.Read(path, jsonc.WithExpand())` once; decode into a struct ([typed config](../guides/dotenv/typed-config.md), [`Decode`](../guides/jsonc/read-and-paths.md#step-7--decode-into-a-struct)).
- Install the secret backend for the environment (a Lookuper keeps the file the same across environments). Leave command substitution off.
- Mark must-have values `${NAME:?…}` so a missing one fails at startup with the name, not later with an empty string.
- If users write the file by hand (jsonc), validate it with a schema and print the `file:line:col` errors.
- textdata never reads or sets `os.Environ`; if your app wants the values as environment variables, set them yourself, or start children with [`envkit.ChildEnv`](../guides/cli/envkit.md#step-6--build-a-child-process-environment).

Example: [`examples/getting-started/first-read`](../../examples/getting-started/first-read), [`examples/dotenv/typed-config`](../../examples/dotenv/typed-config).

### A CLI tool or generator that edits files people commit

- `Open`, edit with `Set`/`SetAfter`/`Unset`/`Disable`, `Save`. Every other byte stays as written, so the user's diff shows only your change.
- Offer a preview: diff `Render()` before and after ([`textdiff.Lines`](../guides/providers/output-and-reports.md#step-4--preview-an-edit)), or use `envkit.Set` with `DryRun`.
- Install SaveGuards for what must never be written (a PEM key, a real value in a template). They run on `Save` and `SaveAs`.
- Never write values you read through expansion.

Example: [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config), [`examples/cli/envkit-ops`](../../examples/cli/envkit-ops).

### A CI pipeline

- Gate on completeness: `textdatactl dotenv matrix --contract .env.example` exits 1 when an environment lacks a contract key ([multi-env Step 3](../guides/cli/multi-env.md#step-3--gate-ci-on-a-contract)).
- Gate on format and shape: `textdatactl jsonc fmt --check FILE` and `textdatactl jsonc validate FILE --schema schema.json` ([jsonc CLI Step 4](../guides/cli/jsonc-cli.md#step-4--format-and-validate-in-ci)).
- Script with `--json` and exit codes: `0` ok, `1` the check found something, `2` usage error.
- The CLIs never run `$(cmd)` and install no secret backends: `--expand` resolves only references inside the file.

Example: [`examples/cli/verb-tour`](../../examples/cli/verb-tour), recipes in [recipes](../guides/cli/recipes.md).

### A deploy step pushing secrets to GitHub or Vercel

- Keep the deploy values under a prefix in the file (`GITHUB_SECRET_DB_URL`) and push with `--prefix GITHUB_SECRET_ --strip-prefix`.
- Always `--dry-run` first; `push` expands `${refs}` by default and skips placeholders (`__LIKE_THIS__`) and empty values unless you lift the guards.
- `push` and `prune` ask before writing, and in a non-interactive run (CI) refuse with a usage error unless you pass `--yes`. `prune` removes remote secrets your file no longer defines.

Guides: [GitHub](../guides/providers/github.md), [Vercel](../guides/providers/vercel.md), [selection](../guides/providers/selection.md). Examples: [`examples/providers/github`](../../examples/providers/github), [`examples/providers/vercel`](../../examples/providers/vercel).

## Pitfalls

- **Installing `schema.Plugin` refuses invalid saves.** It is a Validator and a SaveGuard: `Save` refuses a document with schema errors; call `doc.Validate()` to check a file you only read.
- **A failing `$(cmd)` fails the read.** A non-zero exit or a runner error is an `*expand.CommandError` naming the command, returned by `GetExpanded`, `Expanded`, `ExpandedMap` and `Read`; make a command that may fail harmlessly succeed on its own (`$(cmd || true)`).
- **Installing a `CommandRunner` turns substitution on** in both formats; the last runner installed wins.
- **Placeholders are only skipped by the push and select paths.** A library `Read` returns `__FILL_ME__` as an ordinary value.

## Related

- [Which method](methods.md), [Which value plugin](value-plugins.md), [Library, CLI or embedding](library-cli-or-embed.md)
- [Learning path](../README.md)
