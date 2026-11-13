# Select which keys a command acts on

`github.com/ubgo/textdata/clikit/selection` is the one selection grammar every textdata CLI and push provider shares: which keys (`--prefix`, `--exclude-prefix`, `--include`, `--exclude` or explicit names), under which names (`--strip-prefix`), whether `${references}` are resolved (`--expand`), and which values are held back (placeholders such as `__YOU__`, and empty values). Use it when you push a slice of an env file to a secret store, when you build your own verb or provider and want `--prefix` to mean exactly what it means in `dotenvctl github push`, or when you call dotenv's `envkit.SelectFile` from Go.

## Before you start

```sh
go get github.com/ubgo/textdata/clikit@latest       # the grammar: Options, Selection, PlaceholderMatcher
go get github.com/ubgo/textdata/dotenv/cli@latest   # envkit.SelectFile: applies it to a .env file
```

`selection` itself knows no file format: it holds the options and the result types. A format's operations package applies them to a file; for dotenv that is `envkit.SelectFile(path, opts)` (and `envkit.Select(f, opts)` for a file you already parsed), which is the call `dotenvctl` makes before every `github` or `vercel` verb. The runnable program for this page is [`examples/providers/selection`](../../../examples/providers/selection). Every step resolves this file:

```sh
# deploy secrets (pushed to CI)
GITHUB_SECRET_DB_URL=postgres://${DB_HOST}/app
GITHUB_SECRET_API_KEY=__YOU__
GITHUB_SECRET_GITHUB_TOKEN=ghp_local_only
GITHUB_SECRET_SENTRY_DSN=
# app config
DB_HOST=db.internal
DEPLOY_REGION=eu-west-1
LOG_LEVEL=debug
```

The result is a `Selection{Pairs, Skipped}`: `Pairs` is what to act on (key and value, in the destination's naming), `Skipped` is what a guard held back, each with a `SkipReason` (`placeholder` or `empty`). Skipped keys are part of the result, not dropped, so a caller can tell "nothing matched" from "everything was a placeholder".

### The order things happen in

| # | Stage | Option | CLI flag |
|---|---|---|---|
| 1 | explicit names, as given (the prefix filters do not second-guess them); otherwise every active key | `Keys` | positional `KEY...` |
| 2 | keep keys with this prefix | `Prefix` | `--prefix` |
| 3 | drop keys with this prefix | `ExcludePrefix` | `--exclude-prefix` |
| 4 | force-add named keys, bypassing 2 and 3 | `IncludeKeys` | `--include` (repeat or comma-separate) |
| 5 | drop named keys, matched on the original name | `ExcludeKeys` | `--exclude` (repeat or comma-separate) |
| 6 | remove `Prefix` from the emitted names | `StripPrefix` | `--strip-prefix` |
| 7 | resolve `${VAR}` references against the file | `Expand` | `--expand` (on by default for `push` and `prune`) |
| 8 | hold back values matching the placeholder pattern | `IncludePlaceholders` lifts it | `--include-placeholders` |
| 9 | hold back empty values | `IncludeEmpty` lifts it | `--include-empty` |

The placeholder pattern is `DefaultPlaceholderPattern`, `^__[A-Z0-9_]+__$`, unless `PlaceholderPattern` replaces it.

## Step 1 — Take every key, with the push defaults

The zero `Options` selects every active key. A push wants references resolved (a literal `${DB_HOST}` stored in a remote secret is almost always wrong), so `push` and `prune` turn `--expand` on by default; from Go you set `Expand` yourself.

<!-- ds:block id=everykey-u4bhavam -->
**everyKey** · [`examples/providers/selection/main.go:111-113`](../../../examples/providers/selection/main.go#L111-L113)

```go
func everyKey() selection.Options {
	return selection.Options{Expand: true}
}
```
<!-- /ds:block hash=ed0f4f -->

`GITHUB_SECRET_API_KEY` (`__YOU__`) and `GITHUB_SECRET_SENTRY_DSN` (empty) come back in `Skipped`, not in `Pairs`.

## Step 2 — Keep one audience and rename it

Mark the keys for one destination with a prefix, select by it and strip it on the way out, so `GITHUB_SECRET_DB_URL` lands as `DB_URL`. `ExcludeKeys` is matched against the original name, before stripping.

<!-- ds:block id=deploysecrets-xdm9r9ay -->
**deploySecrets** · [`examples/providers/selection/main.go:120-127`](../../../examples/providers/selection/main.go#L120-L127)

```go
func deploySecrets() selection.Options {
	return selection.Options{
		Prefix:      deployPrefix,
		StripPrefix: true,
		ExcludeKeys: []string{deployPrefix + "GITHUB_TOKEN"},
		Expand:      true,
	}
}
```
<!-- /ds:block hash=a9285c -->

The same from the command line:

```sh
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --exclude GITHUB_SECRET_GITHUB_TOKEN --dry-run
```

## Step 3 — Force one more key in

`IncludeKeys` adds a key the prefix filters would drop. It is appended after the prefix matches and keeps its own name under `StripPrefix`, because it never carried the prefix.

<!-- ds:block id=forceinclude-4zqem3f9 -->
**forceInclude** · [`examples/providers/selection/main.go:133-141`](../../../examples/providers/selection/main.go#L133-L141)

```go
func forceInclude() selection.Options {
	return selection.Options{
		Prefix:      deployPrefix,
		StripPrefix: true,
		IncludeKeys: []string{"DEPLOY_REGION"},
		ExcludeKeys: []string{deployPrefix + "GITHUB_TOKEN"},
		Expand:      true,
	}
}
```
<!-- /ds:block hash=c9d92e -->

## Step 4 — Lift the guards

The guards exist so a stand-in never overwrites a real production value. When you mean to push them anyway, lift them one at a time.

<!-- ds:block id=liftguards-bt8rqkzf -->
**liftGuards** · [`examples/providers/selection/main.go:147-155`](../../../examples/providers/selection/main.go#L147-L155)

```go
func liftGuards() selection.Options {
	return selection.Options{
		Prefix:              deployPrefix,
		StripPrefix:         true,
		IncludePlaceholders: true,
		IncludeEmpty:        true,
		Expand:              true,
	}
}
```
<!-- /ds:block hash=832893 -->

## Step 5 — Keep references literal

Reading verbs want to show what is written; leave `Expand` off. Naming one key explicitly selects only that key.

<!-- ds:block id=noexpand-vqkqqcs7 -->
**noExpand** · [`examples/providers/selection/main.go:161-163`](../../../examples/providers/selection/main.go#L161-L163)

```go
func noExpand() selection.Options {
	return selection.Options{Keys: []string{deployPrefix + "DB_URL"}}
}
```
<!-- /ds:block hash=0e88e5 -->

## Step 6 — Use your own placeholder convention

`PlaceholderPattern` replaces the default pattern; it does not add to it.

<!-- ds:block id=customplaceholder-5ddbcpnr -->
**customPlaceholder** · [`examples/providers/selection/main.go:170-176`](../../../examples/providers/selection/main.go#L170-L176)

```go
func customPlaceholder() selection.Options {
	return selection.Options{
		Prefix:             deployPrefix,
		Expand:             true,
		PlaceholderPattern: `^ghp_local_only$`,
	}
}
```
<!-- /ds:block hash=5413dd -->

## Step 7 — A named key must exist

The one invariant every caller depends on: a key the user named (`Keys` or `IncludeKeys`) that is not in the file is an error, never a silent skip, because reporting success for work that never happened is the failure this grammar exists to prevent. Excluding an absent key is fine ("make sure X never ships" is valid when X is already gone).

<!-- ds:block id=namedkeymustexist-db8wseyh -->
**namedKeyMustExist** · [`examples/providers/selection/main.go:183-191`](../../../examples/providers/selection/main.go#L183-L191)

```go
func namedKeyMustExist(w io.Writer, path string) error {
	_, err := envkit.SelectFile(path, selection.Options{Keys: []string{"MISSING"}})
	fmt.Fprintln(w, "7. a named key that does not exist")
	fmt.Fprintf(w, "  error: %v\n", err)

	_, err = envkit.SelectFile(path, selection.Options{ExcludeKeys: []string{"MISSING"}})
	fmt.Fprintf(w, "  excluding it instead: err=%v\n", err)
	return nil
}
```
<!-- /ds:block hash=6090e4 -->

`dotenvctl` maps this error to the `not_found` code.

## Step 8 — Check a pattern before you use it

`PlaceholderMatcher` turns an optional pattern into a compiled regexp: `""` (or the default itself) returns the shared, precompiled default; a pattern that does not compile is an error, never a panic, because the pattern is user input in every caller.

<!-- ds:block id=badpattern-w3rkhehe -->
**badPattern** · [`examples/providers/selection/main.go:198-209`](../../../examples/providers/selection/main.go#L198-L209)

```go
func badPattern(w io.Writer) error {
	re, err := selection.PlaceholderMatcher("")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "8. checking a pattern before use")
	fmt.Fprintf(w, "  default %s matches __YOU__: %t\n", re, re.MatchString("__YOU__"))

	_, err = selection.PlaceholderMatcher("__(")
	fmt.Fprintf(w, "  bad pattern: %v\n", err)
	return nil
}
```
<!-- /ds:block hash=90a507 -->

## Try it

```sh
cd examples/providers/selection && go run .
```

```text
1. every key, expanded, guards on
  pair    GITHUB_SECRET_DB_URL=postgres://db.internal/app
  pair    GITHUB_SECRET_GITHUB_TOKEN=ghp_local_only
  pair    DB_HOST=db.internal
  pair    DEPLOY_REGION=eu-west-1
  pair    LOG_LEVEL=debug
  skipped GITHUB_SECRET_API_KEY (placeholder)
  skipped GITHUB_SECRET_SENTRY_DSN (empty)
2. only deploy secrets, prefix stripped
  pair    DB_URL=postgres://db.internal/app
  skipped API_KEY (placeholder)
  skipped SENTRY_DSN (empty)
3. force-include one app key
  pair    DB_URL=postgres://db.internal/app
  pair    DEPLOY_REGION=eu-west-1
  skipped API_KEY (placeholder)
  skipped SENTRY_DSN (empty)
4. lift both guards
  pair    DB_URL=postgres://db.internal/app
  pair    API_KEY=__YOU__
  pair    GITHUB_TOKEN=ghp_local_only
  pair    SENTRY_DSN=
5. no expansion
  pair    GITHUB_SECRET_DB_URL=postgres://${DB_HOST}/app
6. a custom placeholder pattern
  pair    GITHUB_SECRET_DB_URL=postgres://db.internal/app
  pair    GITHUB_SECRET_API_KEY=__YOU__
  skipped GITHUB_SECRET_GITHUB_TOKEN (placeholder)
  skipped GITHUB_SECRET_SENTRY_DSN (empty)
7. a named key that does not exist
  error: envkit: key "MISSING" not found
  excluding it instead: err=<nil>
8. checking a pattern before use
  default ^__[A-Z0-9_]+__$ matches __YOU__: true
  bad pattern: placeholder pattern "__(": error parsing regexp: missing closing ): `__(`
```

Pairs keep file order; the skipped list is printed after them by the example, and also keeps file order.

## Pitfalls

- **Explicit keys ignore the prefix filters.** `push DB_HOST --prefix GITHUB_SECRET_` selects `DB_HOST`: named keys are taken as given, and `--prefix`/`--exclude-prefix` apply only when no key is named. `--include` and `--exclude` still apply on top.
- **`--exclude` takes the original name.** With `--strip-prefix`, write `--exclude GITHUB_SECRET_GITHUB_TOKEN`, not `--exclude GITHUB_TOKEN`; the stripped name matches nothing and nothing is excluded.
- **Skipped names are the final names.** After `--strip-prefix`, a held-back key is reported as `API_KEY`, the way the destination would name it, so `prune` can match it against remote names.
- **Stripping can collide, and a collision is an error.** `GITHUB_SECRET_DB_HOST` stripped is `DB_HOST`; if you also `--include DB_HOST`, two keys would end up with the same name. `envkit.SelectFile` refuses with a `NameCollisionError` (`errors.Is(err, envkit.ErrNameCollision)`) naming both keys, and `dotenvctl` exits `2` with the code `usage` before any provider runs: `envkit: keys "GITHUB_SECRET_DB_HOST" and "DB_HOST" both select as "DB_HOST" after --strip-prefix; exclude one of them`. Drop the `--include` or `--exclude` one of the two.
- **`--strip-prefix` without `--prefix` does nothing.** There is no prefix to strip.
- **The guards look at the expanded value.** With `--expand` on (the push default), `KEY=${OTHER}` where `OTHER=__YOU__` is held back as a placeholder; a reference to a key the file does not define expands to `""` and is held back as empty (the process environment is not consulted).
- **A failing `${VAR:?msg}` stops the whole selection** when `--expand` is on, before any provider side effect and even when the key holding it is not selected: the whole file is expanded first. `dotenvctl` reports it as `required`.
- **A custom pattern replaces the default.** Step 6 shows `__YOU__` becoming an ordinary value once `PlaceholderPattern` is set. Include the default alternative in your pattern if you still want it.
- **`push` and `prune` have no flag for the pattern.** The CLI's store verbs always use the default; only Go callers (and `dotenvctl matrix --placeholder`) can change it.
- **Read verbs get only the five pure flags.** `dotenvctl list` and `keys` take `--prefix`, `--exclude-prefix`, `--include`, `--exclude` and `--strip-prefix` (registered by `providerkit.AddSelectionFlags`), with no guards and no expansion, because a reader wants to see placeholders, not have them hidden. A bare `list` keeps its full view; the selection view is used only when one of those flags is given (`providerkit.SelectionActive`).
- **The resolution lives in each format.** `clikit/selection` defines the grammar and result types; the code that applies them is `envkit.Select` for dotenv. A second format applies the same `Options` with the same order.

## Related

- [Write a push provider](write-a-provider.md): how `push` and `prune` consume a `Selection` (`Deps.Source`)
- [GitHub provider](github.md) and [Vercel provider](vercel.md): the grammar in real commands
- [Output and reports](output-and-reports.md): the error codes a selection failure maps to
- Package reference: [clikit README § selection](../../../clikit/README.md#selection)
- Example: [`examples/providers/selection`](../../../examples/providers/selection)
- API: [pkg.go.dev/github.com/ubgo/textdata/clikit/selection](https://pkg.go.dev/github.com/ubgo/textdata/clikit/selection), [envkit](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/envkit)
- [Feature index](../../reference/features.md)
