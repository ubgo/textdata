# envkit: the operations API in Go

`github.com/ubgo/textdata/dotenv/cli/envkit` is the layer every `dotenvctl` verb calls: select keys, edit with a dry-run preview, diff two files, build the drift matrix, discover a `.env` family, and run a child process with a file's values. Each operation is a plain function on file paths that returns a typed result, with no cobra, no flags and no printing. Use it when your own Go program (a deploy tool, a test helper, a migration script) needs what `dotenvctl` does without shelling out to it or parsing its text output.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv/cli@latest
```

`envkit` lives in the `dotenv/cli` module, so it pulls in cobra through that module; if you only need to read and edit one file, the `dotenv` module alone is enough (see [which module to import](../../choosing/format-and-modules.md)). The runnable program for this page is [`examples/cli/envkit-ops`](../../../examples/cli/envkit-ops); it writes this family to a temp directory:

| File | Content |
|---|---|
| `.env.example` (the contract) | `DB_URL=`, `API_KEY=`, `SENTRY_DSN=` |
| `.env.stag` | `HOST=db.stag`, `DB_URL=postgres://${HOST}/app`, `API_KEY=__FILL_ME__`, `# SENTRY_DSN=x` |
| `.env.prod` | `HOST=db.prod`, `DB_URL=postgres://${HOST}/app`, `API_KEY=k-123`, `DEPLOY_TOKEN=t-1`, `DEPLOY_REGION=eu` |

| Function | Verb it backs | Returns |
|---|---|---|
| `Discover`, `Inventories` | `envs` | `[]EnvFile`, `[]Inventory` |
| `SelectFile`, `Select` | selection flags of `list`, `run` and the push providers | `*Selection` (`Pairs`, `Skipped`) |
| `Set`, `Unset`, `Restore` | `set`, `unset`, `restore` | `*EditResult` (`Changed`, `Written`, `Diff`, `Created`, `Affected`) |
| `Diff`, `DiffMaps` | `diff` | `*DiffResult` (`Added`, `Removed`, `Changed`) |
| `BuildMatrix`, `DriftRows`, `SparseColumn`, `DisambiguateColumns`, `EnvNameForPath` | `matrix` | `*Matrix` |
| `Run`, `ChildEnv`, `MergeEnv` | `run` | exit code, `[]string` environment |

## Step 1 — discover the family

`Inventories` scans a directory (`""` means `.`) with the same rules as `dotenvctl envs`: files ordered by distance from prod, long names collapsed (`.env.production` is `prod`), templates such as `.env.example` tagged `Contract`, editor leftovers such as `.env.bak` ignored. Each row counts active keys, disabled settings, name-only declarations and placeholder values (`^__[A-Z0-9_]+__$` unless you set `PlaceholderPattern`). `Discover` returns the same list without the counts.

The default is one exported constant, so a program that embeds envkit can use the same pattern the CLI uses, or see what it overrides when it sets its own:

<!-- ds:block id=placeholderpattern-up25s3m5 -->
**DefaultPlaceholderPattern** · [`dotenv/cli/envkit/envkit.go:38`](../../../dotenv/cli/envkit/envkit.go#L38-L38)

```go
const DefaultPlaceholderPattern = selection.DefaultPlaceholderPattern
```
<!-- /ds:block hash=2f72ac -->

<!-- ds:block id=discoverfamily-mb5kcafy -->
**discoverFamily** · [`examples/cli/envkit-ops/main.go:78-88`](../../../examples/cli/envkit-ops/main.go#L78-L88)

```go
func discoverFamily(w io.Writer, dir string) error {
	inv, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir})
	if err != nil {
		return err
	}
	for _, i := range inv {
		fmt.Fprintf(w, "%-12s env=%-7s contract=%-5t keys=%d placeholders=%d\n",
			i.File, i.Env, i.Contract, i.Keys, i.Placeholders)
	}
	return nil
}
```
<!-- /ds:block hash=676076 -->

## Step 2 — select the keys you will act on

`SelectFile` (or `Select` on a file you already parsed) applies the selection grammar shared by every textdata CLI and push provider: `Keys`, `Prefix`, `ExcludePrefix`, `IncludeKeys`, `ExcludeKeys`, `StripPrefix`, `Expand`, and the placeholder and empty guards. Guarded keys land in `Skipped` with a reason instead of disappearing, so a caller can tell "nothing matched" from "everything was a placeholder". The full grammar is in [Key selection](../providers/selection.md).

<!-- ds:block id=selectdeploykeys-6ztzpzuk -->
**selectDeployKeys** · [`examples/cli/envkit-ops/main.go:95-111`](../../../examples/cli/envkit-ops/main.go#L95-L111)

```go
func selectDeployKeys(w io.Writer, dir string) error {
	sel, err := envkit.SelectFile(filepath.Join(dir, prodFile), envkit.SelectOptions{
		Prefix:      "DEPLOY_",
		StripPrefix: true,
		IncludeKeys: []string{"DB_URL"},
		Expand:      true,
	})
	if err != nil {
		return err
	}
	for _, p := range sel.Pairs {
		fmt.Fprintf(w, "select %s=%s\n", p.Key, p.Value)
	}
	_, err = envkit.SelectFile(filepath.Join(dir, prodFile), envkit.SelectOptions{Keys: []string{"NOPE"}})
	fmt.Fprintln(w, "named key missing:", err)
	return nil
}
```
<!-- /ds:block hash=50f5b8 -->

## Step 3 — edit with a preview

`Set` upserts pairs, `Unset` comments keys out (or removes them with `Delete: true`), `Restore` brings commented-out keys back. Every one takes `EditOptions`; with `DryRun` it computes the same `Diff` and writes nothing, so a preview and the real run cannot disagree. `Anchor` (and `AnchorBefore`) places a new key next to an existing one.

<!-- ds:block id=editwithpreview-xu4xtfku -->
**editWithPreview** · [`examples/cli/envkit-ops/main.go:118-140`](../../../examples/cli/envkit-ops/main.go#L118-L140)

```go
func editWithPreview(w io.Writer, dir string) error {
	path := filepath.Join(dir, stagFile)
	pairs := []dotenv.Pair{{Key: "SENTRY_ENV", Value: "staging"}}
	preview, err := envkit.Set(path, pairs, envkit.EditOptions{Anchor: "API_KEY", DryRun: true})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "preview:", preview.Diff, "written:", preview.Written)
	if _, err := envkit.Set(path, pairs, envkit.EditOptions{Anchor: "API_KEY"}); err != nil {
		return err
	}
	if _, err := envkit.Unset(path, []string{"SENTRY_ENV"}, envkit.EditOptions{}); err != nil {
		return err
	}
	res, err := envkit.Restore(path, []string{"SENTRY_ENV"}, envkit.EditOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "restore:", res.Diff)
	_, err = envkit.Unset(path, []string{"NEVER"}, envkit.EditOptions{})
	fmt.Fprintln(w, "unset absent key is ErrKeyNotFound:", errors.Is(err, envkit.ErrKeyNotFound))
	return nil
}
```
<!-- /ds:block hash=c40d1c -->

## Step 4 — compare two files

`Diff` compares what a consumer of each file would see: last-wins maps, optionally expanded. Comments, formatting and shadowed duplicates do not count. `DiffMaps` does the same for two maps you already have, for example a file against the keys a remote store reports.

<!-- ds:block id=difffiles-d9y2f3t7 -->
**diffFiles** · [`examples/cli/envkit-ops/main.go:146-161`](../../../examples/cli/envkit-ops/main.go#L146-L161)

```go
func diffFiles(w io.Writer, dir string) error {
	d, err := envkit.Diff(filepath.Join(dir, stagFile), filepath.Join(dir, prodFile), envkit.DiffOptions{Expand: true})
	if err != nil {
		return err
	}
	for _, p := range d.Added {
		fmt.Fprintln(w, "diff +", p.Key)
	}
	for _, p := range d.Removed {
		fmt.Fprintln(w, "diff -", p.Key)
	}
	for _, c := range d.Changed {
		fmt.Fprintf(w, "diff ~ %s: %s -> %s\n", c.Key, c.From, c.To)
	}
	return nil
}
```
<!-- /ds:block hash=ef4c49 -->

## Step 5 — build the drift matrix

`BuildMatrix` puts every file of a directory (or the `Files` you list) in columns and every key in rows; each cell is `present`, `placeholder`, `empty`, `disabled`, `inherited` or `missing`. With `ContractPath`, `ContractMissing` lists per environment the contract keys it lacks. `DriftRows` keeps only rows whose cells differ. Values stay out of the cells unless you pass `Reveal`.

<!-- ds:block id=driftmatrix-anaguvzr -->
**driftMatrix** · [`examples/cli/envkit-ops/main.go:167-186`](../../../examples/cli/envkit-ops/main.go#L167-L186)

```go
func driftMatrix(w io.Writer, dir string) error {
	m, err := envkit.BuildMatrix(envkit.MatrixOptions{
		Dir:          dir,
		ContractPath: filepath.Join(dir, contractFile),
	})
	if err != nil {
		return err
	}
	m.Rows = envkit.DriftRows(m.Rows)
	fmt.Fprintln(w, "columns:", m.Envs)
	for _, row := range m.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, c := range row.Cells {
			cells = append(cells, c.Env+"="+string(c.State))
		}
		fmt.Fprintf(w, "drift %-13s %s\n", row.Key, strings.Join(cells, " "))
	}
	fmt.Fprintln(w, "contract gaps:", m.HasContractGaps(), m.ContractMissing["stag"])
	return nil
}
```
<!-- /ds:block hash=c3a142 -->

When a filtered matrix looks wrong because one nearly empty file makes every row drift, `SparseColumn(m, rowsBefore)` names that column; `DisambiguateColumns` and `EnvNameForPath` are what `matrix` uses to name columns when files from several directories share an environment name. Both are shown as runnable examples in [`envkit/example_test.go`](../../../dotenv/cli/envkit/example_test.go).

## Step 6 — build a child process environment

`ChildEnv(path, base, sel)` overlays the file's selected values on `base`, the file winning, and removes the duplicate names from the base. A nil selection takes every active key with expansion on, because a child cannot resolve `${refs}` itself. `Run(ctx, path, argv, RunOptions)` does the same and starts the command; it returns the child's exit code as a value and an error only when the file could not be read or expanded or the child could not start. `MergeEnv` is the overlay step on its own, for a map you built yourself.

<!-- ds:block id=childenvironment-5m6hvqgp -->
**childEnvironment** · [`examples/cli/envkit-ops/main.go:192-201`](../../../examples/cli/envkit-ops/main.go#L192-L201)

```go
func childEnvironment(w io.Writer, dir string) error {
	base := []string{"HOST=from-shell", "TERM=xterm"}
	env, err := envkit.ChildEnv(filepath.Join(dir, prodFile), base, nil)
	if err != nil {
		return err
	}
	slices.Sort(env)
	fmt.Fprintln(w, "child env:", env)
	return nil
}
```
<!-- /ds:block hash=c63214 -->

The CLI form, with a real child process, is [`examples/cli/run-child`](../../../examples/cli/run-child).

## Try it

```sh
cd examples/cli/envkit-ops && go run .
```

```text
.env.stag    env=stag    contract=false keys=3 placeholders=1
.env.prod    env=prod    contract=false keys=5 placeholders=0
.env.example env=example contract=true  keys=3 placeholders=0
select TOKEN=t-1
select REGION=eu
select DB_URL=postgres://db.prod/app
named key missing: envkit: key "NOPE" not found
preview: [+SENTRY_ENV=staging] written: false
restore: [-# SENTRY_ENV=staging +SENTRY_ENV=staging]
unset absent key is ErrKeyNotFound: true
diff + DEPLOY_REGION
diff + DEPLOY_TOKEN
diff - SENTRY_ENV
diff ~ API_KEY: __FILL_ME__ -> k-123
diff ~ DB_URL: postgres://db.stag/app -> postgres://db.prod/app
diff ~ HOST: db.stag -> db.prod
columns: [stag prod]
drift API_KEY       stag=placeholder prod=present
drift DEPLOY_REGION stag=missing prod=present
drift DEPLOY_TOKEN  stag=missing prod=present
drift SENTRY_DSN    stag=disabled prod=missing
drift SENTRY_ENV    stag=present prod=missing
contract gaps: true [SENTRY_DSN]
child env: [API_KEY=k-123 DB_URL=postgres://db.prod/app DEPLOY_REGION=eu DEPLOY_TOKEN=t-1 HOST=db.prod TERM=xterm]
```

## Pitfalls

- **A key you name must exist.** `Keys` (and `IncludeKeys`) naming an absent key is an error (`envkit: key "NOPE" not found`), never a silent skip; a prefix that matches nothing is not an error.
- **`StripPrefix` with `IncludeKeys` can produce the same name twice.** `P_DB_HOST` stripped to `DB_HOST` plus an included `DB_HOST` gives two pairs with one name; a push writes both and the last wins. Check `Names()` for duplicates when you combine the two.
- **`Anchor` only places new keys.** An existing key is updated where it is and never moved. An anchor that names no active key returns `ErrAnchorNotFound` rather than appending at the end.
- **No-op edits do not touch the file.** Setting a key to its current value returns `Changed: false` and `Written: false`; the mtime does not change. `Set` creates a missing file with mode 0600; `Unset` and `Restore` need the file to exist, and an absent key is `ErrKeyNotFound`.
- **`Diff` needs both files.** A missing file is an error, not "everything was removed".
- **`RunOptions.Base` nil means an empty base.** The child then sees only the file's values, with no `PATH` or `HOME`; pass `os.Environ()` for the usual inherit-and-override behaviour.
- **A non-zero child is not an error.** Check the returned code; `err` is only for failures before or while starting the child.

## Related

- [dotenvctl command reference](dotenvctl.md) and [Multi-environment](multi-env.md): the same operations as verbs
- [Key selection](../providers/selection.md): the grammar `SelectOptions` aliases
- [dotenv editing](../dotenv/editing.md): the single-file `Set`, `Unset`, `Restore` this layer wraps
- [`envkit` example functions](../../../dotenv/cli/envkit/example_test.go), [`discover` example functions](../../../dotenv/cli/discover/example_test.go)
- README: [`dotenv/cli`](../../../dotenv/cli/README.md)
- API: [`envkit`](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/envkit), [`discover`](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/discover)
