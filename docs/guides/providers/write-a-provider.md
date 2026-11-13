# Write a push provider

`github.com/ubgo/textdata/clikit/providerkit` is the seam between a textdata CLI and its push providers. A provider is any type with one method, `Command(Deps) *cobra.Command`; the kit then generates the standard `push`, `list` and `prune` verbs over your backend, with the shared selection flags, the placeholder and empty guards, `--dry-run`, `--yes`, the confirm gate, the `--json` envelope and the 0/1/2 exit codes. Use it when you want to sync env values to a secret store that textdata does not cover yet (a vault, a cloud secret manager, an internal API) and have it behave exactly like `dotenvctl github` and `dotenvctl vercel`, which are built the same way.

## Before you start

```sh
go get github.com/ubgo/textdata/clikit@latest
go get github.com/ubgo/textdata/dotenv/cli@latest   # only if your host reads .env files through envkit
```

You need a backend you can write a secret to by name, and ideally list and delete by name. The kit never asks a backend for values. The runnable program for this page is [`examples/providers/fake-provider`](../../../examples/providers/fake-provider): a provider over an in-memory map, mounted under a root called `memctl`, fed from this `.env`:

```sh
DB_URL=postgres://db/app
REDIS_URL=redis://cache:6379
API_KEY=__YOU__
```

The remote store starts with `OLD_FLAG` (stale) and a real `API_KEY` someone set by hand.

### What the kit owns and what you own

| The kit (`providerkit`) | Your provider |
|---|---|
| selection flags and their resolution, via `Deps.Source` | `Set`, and optionally `Names` and `Delete`, over your backend |
| the placeholder and empty guards | a `Gate` that prints where values will land, then calls `Confirm` |
| `--dry-run`, `--yes`, the confirm contract | optional `Meta` for the `--json` payload |
| per-key actions (`pushed`, `would-push`, `skipped-placeholder`, …) | optional `EmptyListHint` |
| the `{ok,data,error}` envelope and exit codes | any extra verbs or backend flags you add to the returned commands |

## Step 1 — Implement the capabilities your backend has

There are three capability interfaces, and a backend implements only what it truly supports: `SecretWriter` (`Set`, needed by `push`), `SecretLister` (`Names`, needed by `list` and `prune`) and `SecretDeleter` (`Delete`, needed by `prune`). `Set` is an upsert. When your backend is a CLI, `Set` must pass the value on stdin, never in argv: argv is visible to every user through `ps`.

<!-- ds:block id=memstore-mt7cwnnm -->
**memStore** · [`examples/providers/fake-provider/main.go:71-74`](../../../examples/providers/fake-provider/main.go#L71-L74)

```go
type memStore struct {
	values map[string]string
	failOn string
}
```
<!-- /ds:block hash=7f91b0 -->

<!-- ds:block id=memstore-set-dgg9twz8 -->
**memStore.Set** · [`examples/providers/fake-provider/main.go:79-85`](../../../examples/providers/fake-provider/main.go#L79-L85)

```go
func (s *memStore) Set(_ context.Context, name, value string) error {
	if name == s.failOn {
		return errBackendDown
	}
	s.values[name] = value
	return nil
}
```
<!-- /ds:block hash=ea2e8b -->

<!-- ds:block id=memstore-names-fp2jt9ym -->
**memStore.Names** · [`examples/providers/fake-provider/main.go:90-97`](../../../examples/providers/fake-provider/main.go#L90-L97)

```go
func (s *memStore) Names(context.Context) ([]string, error) {
	names := make([]string, 0, len(s.values))
	for n := range s.values {
		names = append(names, n)
	}
	slices.Sort(names)
	return names, nil
}
```
<!-- /ds:block hash=465b61 -->

<!-- ds:block id=memstore-delete-b4uvm46z -->
**memStore.Delete** · [`examples/providers/fake-provider/main.go:102-105`](../../../examples/providers/fake-provider/main.go#L102-L105)

```go
func (s *memStore) Delete(_ context.Context, name string) error {
	delete(s.values, name)
	return nil
}
```
<!-- /ds:block hash=7d790f -->

Optionally, implement `EmptyListHinter` so that `list` on an empty store names the scope flag the user probably forgot; without it `list` prints `DefaultEmptyListHint` (`no secrets found for this scope`).

<!-- ds:block id=memstore-emptylisthint-nnk3xbfk -->
**memStore.EmptyListHint** · [`examples/providers/fake-provider/main.go:112-114`](../../../examples/providers/fake-provider/main.go#L112-L114)

```go
func (s *memStore) EmptyListHint() string {
	return "no secrets in the memory store (did you push yet?)"
}
```
<!-- /ds:block hash=ed58a5 -->

## Step 2 — Return a command tree from `Command`

`Plugin.Command(Deps)` is the whole mandatory contract. Build a fresh tree each call (no `init()`, no package state), so a host can mount it under any root, even twice. `NewPushCmd`, `NewListCmd` and `NewPruneCmd` generate the verbs; pass the same `VerbConfig` to `push` and `prune`.

<!-- ds:block id=memplugin-k74n5sdy -->
**memPlugin** · [`examples/providers/fake-provider/main.go:121-123`](../../../examples/providers/fake-provider/main.go#L121-L123)

```go
type memPlugin struct {
	store *memStore
}
```
<!-- /ds:block hash=f2d6f2 -->

<!-- ds:block id=memplugin-command-edwkzky2 -->
**memPlugin.Command** · [`examples/providers/fake-provider/main.go:128-137`](../../../examples/providers/fake-provider/main.go#L128-L137)

```go
func (p *memPlugin) Command(deps providerkit.Deps) *cobra.Command {
	root := &cobra.Command{Use: providerName, Short: "Sync values to an in-memory store"}
	cfg := providerkit.VerbConfig{Gate: p.gate, Meta: p.meta}
	root.AddCommand(
		providerkit.NewPushCmd(deps, p.store, cfg),
		providerkit.NewListCmd(deps, p.store),
		providerkit.NewPruneCmd(deps, p.store, p.store, cfg),
	)
	return root
}
```
<!-- /ds:block hash=5d030b -->

The returned commands are ordinary cobra commands: add your own backend flags to them (GitHub adds `--repo`, `--environment`, `--org`; Vercel adds `--target` and, on `push` only, `--sensitive`) and add bespoke verbs beside them (GitHub's `env-create`).

## Step 3 — Gate every write

The gate runs after selection and before any write, dry runs included. Print where the values will land and as whom, then end with `Confirm`, which enforces the one proceed contract: `--dry-run` never prompts, `--yes` proceeds, a human on a terminal is asked, and anything else (CI, a pipe, every `--json` run) is refused with a usage error demanding `--yes`. If you pass no gate, the kit installs bare `Confirm`, so a mutating verb can never run ungated.

<!-- ds:block id=memplugin-gate-6ybxkq7p -->
**memPlugin.gate** · [`examples/providers/fake-provider/main.go:144-148`](../../../examples/providers/fake-provider/main.go#L144-L148)

```go
func (p *memPlugin) gate(_ context.Context, deps providerkit.Deps, info providerkit.GateInfo) error {
	deps.Printer.Humanf("target : memory (%d stored)", len(p.store.values))
	deps.Printer.Humanf("keys   : %d", len(info.Names))
	return providerkit.Confirm(deps, info)
}
```
<!-- /ds:block hash=ff6b11 -->

`Meta` adds resolution context to the `--json` payload as `data.meta`. It runs only under `--json`, once per `push` or `prune`; if it fails, the meta is omitted and the actions are still reported.

<!-- ds:block id=memplugin-meta-h2jyhftr -->
**memPlugin.meta** · [`examples/providers/fake-provider/main.go:154-156`](../../../examples/providers/fake-provider/main.go#L154-L156)

```go
func (p *memPlugin) meta(context.Context) (map[string]string, error) {
	return map[string]string{"target": "memory"}, nil
}
```
<!-- /ds:block hash=019c74 -->

## Step 4 — Wire the host side

The host gives every plugin a `Deps`: the shared `Printer`, the `File` path (display only), a `Source` that resolves the selection the verb parsed (plugins never parse files), the `Runner` that executes backend CLIs, and the `Interactive` and `Ask` hooks. Report a `Source` failure through `providerkit.Fail` so it prints once and carries an exit code. This is what `dotenvctl` does, with `envkit.SelectFile` behind `Source`.

<!-- ds:block id=newdeps-gcrdbt82 -->
**newDeps** · [`examples/providers/fake-provider/main.go:164-179`](../../../examples/providers/fake-provider/main.go#L164-L179)

```go
func newDeps(printer *outfmt.Printer, envPath string) providerkit.Deps {
	return providerkit.Deps{
		Printer: printer,
		File:    envPath,
		Source: func(opts providerkit.SelectOpts) (providerkit.Source, error) {
			sel, err := envkit.SelectFile(envPath, opts)
			if err != nil {
				return nil, providerkit.Fail(printer, outfmt.CodeIO, "%v", err)
			}
			return sel.Flat(), nil
		},
		Runner:      providerkit.ExecRunner{},
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
}
```
<!-- /ds:block hash=721469 -->

`Runner` is the exec seam: production passes `providerkit.ExecRunner{}` (real subprocesses, stderr folded into the error), tests pass a fake that records argv and stdin and answers from memory. The [GitHub](github.md) and [Vercel](vercel.md) examples show a fake runner standing in for `gh` and `vercel`.

## Step 5 — Mount it and map the exit code

Mount the plugin under your root and turn the returned error into an exit code. Kit failures come back as `*providerkit.CmdError` with `Code` 1 (failed) or 2 (usage) and `Quiet` true, meaning the message was already printed; silence cobra's own error printing so it is not printed twice.

<!-- ds:block id=execute-kwuqrdz4 -->
**execute** · [`examples/providers/fake-provider/main.go:187-208`](../../../examples/providers/fake-provider/main.go#L187-L208)

```go
func execute(w io.Writer, envPath string, store *memStore, args ...string) int {
	printer := &outfmt.Printer{Out: w, Err: w, Program: programName}
	root := &cobra.Command{Use: programName, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&printer.JSON, flagJSON, false, "emit the {ok,data|error} envelope")
	root.SetErr(io.Discard)
	root.AddCommand((&memPlugin{store: store}).Command(newDeps(printer, envPath)))
	fmt.Fprintln(w, "$", programName, strings.Join(args, " "))
	root.SetArgs(args)
	err := root.Execute()
	var ce *providerkit.CmdError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ce):
		return ce.Code
	default:
		// Only cobra's own parse errors (unknown flag, wrong arguments)
		// arrive without a CmdError; a host reports them as usage.
		fmt.Fprintf(w, "%s: %v\n", programName, err)
		return exitUsage
	}
}
```
<!-- /ds:block hash=f6a608 -->

For your own failures inside a bespoke verb, call `providerkit.Fail(printer, code, format, args...)`: it prints the failure in the current mode and returns the `CmdError` (exit 2 for `outfmt.CodeUsage`, 1 for every other code).

## Step 6 — Report a failure part-way

If `Set` fails, `push` stops at that key. What was already written is printed in human mode, and under `--json` the partial record and the error travel in one `ok:false` envelope, so a script never parses two objects. Pushes are upserts, so re-running after the fix completes the rest.

<!-- ds:block id=partialfailure-fttx3dk7 -->
**partialFailure** · [`examples/providers/fake-provider/main.go:241-250`](../../../examples/providers/fake-provider/main.go#L241-L250)

```go
func partialFailure(w io.Writer, envPath string) error {
	for _, args := range [][]string{
		{providerName, "push", "--yes"},
		{"--json", providerName, "push", "--yes"},
	} {
		store := &memStore{values: map[string]string{}, failOn: "REDIS_URL"}
		fmt.Fprintf(w, "exit %d\n\n", execute(w, envPath, store, args...))
	}
	return nil
}
```
<!-- /ds:block hash=78071d -->

## Try it

```sh
cd examples/providers/fake-provider && go run .
```

```text
$ memctl mem push
target : memory (2 stored)
keys   : 2
memctl: refusing to write without --yes in non-interactive mode
exit 2

$ memctl mem push --dry-run
target : memory (2 stored)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: would-push
REDIS_URL: would-push
exit 0

$ memctl mem push --yes
target : memory (2 stored)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: pushed
REDIS_URL: pushed
exit 0

$ memctl mem list
API_KEY
DB_URL
OLD_FLAG
REDIS_URL
exit 0

$ memctl mem prune --dry-run
target : memory (4 stored)
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: would-delete
exit 0

$ memctl mem prune --keep OLD_FLAG --dry-run
nothing to prune
API_KEY: kept-skipped
OLD_FLAG: kept-keep-flag
exit 0

$ memctl mem prune --yes
target : memory (4 stored)
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: deleted
exit 0

$ memctl --json mem push --prefix DB_ --yes
{"ok":true,"data":{"meta":{"target":"memory"},"results":[{"name":"DB_URL","action":"pushed"}],"dry_run":false}}
exit 0

$ memctl mem list
no secrets in the memory store (did you push yet?)
exit 0

$ memctl mem push --yes
target : memory (0 stored)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: pushed
memctl: push REDIS_URL: backend unavailable
exit 1

$ memctl --json mem push --yes
{"ok":false,"data":{"meta":{"target":"memory"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"DB_URL","action":"pushed"}],"dry_run":false},"error":{"code":"exec","message":"push REDIS_URL: backend unavailable"}}
exit 1
```

Read it top to bottom: the first `push` is refused because the session is not interactive; `prune` spares `API_KEY` because the local value is a placeholder (a `__YOU__` in your file never deletes a real remote value) and spares `OLD_FLAG` when `--keep` names it; `REDIS_URL` was never written in the last two runs, and `.data.results` says so.

### The per-key actions

| Action | Verb | Meaning |
|---|---|---|
| `pushed` | push | written to the backend |
| `would-push` | push `--dry-run` | would be written |
| `skipped-placeholder` | push | held back: the value matches the placeholder pattern |
| `skipped-empty` | push | held back: the value is empty |
| `deleted` | prune | the remote name was removed |
| `would-delete` | prune `--dry-run` | would be removed |
| `kept-skipped` | prune | spared: the local key exists but was guard-skipped |
| `kept-keep-flag` | prune | spared: `--keep` named it |

Names in the selection are kept by `prune` silently; every other spared name is reported with its reason. Values never appear in any output, human or `--json`.

## Pitfalls

- **Prune deletes what is not in the keep-set.** The keep-set is the selection plus guard-skipped names plus `--keep`. Run `prune` with the same selection flags (and scope flags) you push with, or names outside that selection look stale. Always look at `--dry-run` first: delete has no undo.
- **`prune` lists before it gates.** `Names` runs first; when nothing is stale it prints `nothing to prune` and the gate (and its banner) never runs. A `Names` failure fails `list` and `prune` with the `exec` code rather than reading as an empty store.
- **`--json` never prompts.** `Confirm` refuses a write under `--json` without `--yes` even on a terminal, because a prompt would corrupt the one-envelope output.
- **Leave `Interactive` or `Ask` nil and every write needs `--yes`.** `Confirm` treats a nil hook as "no human to ask" and refuses with `refusing to write without --yes in non-interactive mode` (exit `2`), so a host without a terminal can omit them; set both to let a person confirm at the prompt.
- **Your gate must call `Confirm`.** A `VerbConfig.Gate` replaces the default; one that prints a banner and returns nil writes without asking.
- **The gate also runs on `--dry-run`.** Keep it free of side effects; `GateInfo.DryRun` tells you not to prompt.
- **A Source error is returned as is.** The kit does not print it; report it through `providerkit.Fail` in your `Source` (Step 4), or the user sees nothing.
- **Cobra keeps flag values on the command.** Executing the same tree twice carries `--dry-run` or `--prefix` into the second run; build a fresh tree per run (the examples do), and reset any per-run cache in `PreRun` as the GitHub plugin does.
- **`Meta` costs a backend call per run** in the worst case; cache what the gate already resolved for the run instead of asking twice.
- **Values on stdin are your responsibility.** The kit hands `Set` the value; only your implementation decides whether it reaches a child process through stdin (safe) or argv (visible in `ps`). `ExecRunner` feeds stdin only when the value is non-empty.
- **`push` stops at the first failing key.** Keys after it are not attempted and not listed; design `Set` as an upsert so a re-run is safe.

## Related

- [Select which keys a command acts on](selection.md): what `Deps.Source` resolves
- [Output and reports](output-and-reports.md): the `Printer`, envelope and error codes the kit prints through
- [GitHub provider](github.md) and [Vercel provider](vercel.md): two complete providers built on the kit, driven by a fake runner
- Package reference: [clikit README § providerkit](../../../clikit/README.md#providerkit)
- Example: [`examples/providers/fake-provider`](../../../examples/providers/fake-provider); the kit's own examples: [`ExampleNewPushCmd`, `ExampleNewPruneCmd`, `ExampleNewListCmd`, `ExampleVerbConfig`](../../../clikit/providerkit/example_test.go)
- API: [pkg.go.dev/github.com/ubgo/textdata/clikit/providerkit](https://pkg.go.dev/github.com/ubgo/textdata/clikit/providerkit)
- [Feature index](../../reference/features.md)
