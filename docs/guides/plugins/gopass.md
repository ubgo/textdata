# gopass plugin

`github.com/ubgo/textdata/plugins/gopass` reads secrets from a [gopass](https://github.com/gopasspw/gopass) password store. A file says `DB_PASSWORD=${gopass:personal/app/db}`; the expanded read runs `gopass show -o personal/app/db` and returns the first line, while the file keeps the reference. Use it on developer machines and small teams where secrets already live in gopass, so `.env` and JSONC files can be committed without them.

## Before you start

```sh
go get github.com/ubgo/textdata/plugins/gopass@latest
```

You need the `gopass` binary on `PATH` (or a path given with `WithBinary`) with an unlocked store at run time; nothing runs at install time. The runnable program for this page is [`examples/plugins/gopass-fake-runner`](../../../examples/plugins/gopass-fake-runner). It injects a fake runner, so it runs without gopass or gpg.

## Step 1 — Build the plugin

`gopassplugin.New` returns a plugin that is a `plugin.Resolver` for the `gopass` scheme. `WithMapping` additionally turns on the `plugin.Lookuper` for the names you list, so a plain `${DB_USER}` the file does not define is read from `personal/app/db-user`:

<!-- ds:block id=gopass-newplugin-q3jcvte8 -->
**newPlugin** · [`examples/plugins/gopass-fake-runner/main.go:72-77`](../../../examples/plugins/gopass-fake-runner/main.go#L72-L77)

```go
func newPlugin(calls *int) *gopassplugin.Plugin {
	return gopassplugin.New(
		gopassplugin.WithRunner(newFakeRunner(calls)),
		gopassplugin.WithMapping(map[string]string{"DB_USER": "personal/app/db-user"}),
	)
}
```
<!-- /ds:block hash=adb1d8 -->

| Option | Default | Effect |
|---|---|---|
| `WithRunner(r)` | `ExecRunner` (`os/exec`, stdin not connected, environment inherited) | how gopass is run; inject a fake in tests |
| `WithBinary(name)` | `gopass` | program name looked up on `PATH`, or a path |
| `WithTimeout(d)` | 30s (gpg may wait for a pinentry dialog) | bound on one call; zero or less means none beyond the context |
| `WithContext(ctx)` | `context.Background()` | parent of every call, so the caller can cancel |
| `WithCache(on)` | on | each ref is fetched once per plugin; failures are never cached |
| `WithMapping(m)` | off | Lookuper for exactly the names in `m`, `${NAME}` → `m[NAME]` |
| `WithPrefix(p)` | off | Lookuper for every undefined name: `${NAME}` → `p+NAME` |

A mapped name wins over the prefix. Prefer `WithMapping` when you know the names: with `WithPrefix`, every undefined name the file references costs one gopass call.

## Step 2 — Inject a runner for tests

`Runner` is `func(ctx, binary, args) (stdout, stderr, err)`. The fake below serves a map, returns gopass's not-found exit status (11) for a missing entry, and counts calls so the cache is visible. The plugin only looks at an `ExitCode() int` method on the error, so a small type is enough:

<!-- ds:block id=newfakerunner-x8sc24ut -->
**newFakeRunner** · [`examples/plugins/gopass-fake-runner/main.go:53-66`](../../../examples/plugins/gopass-fake-runner/main.go#L53-L66)

```go
func newFakeRunner(calls *int) gopassplugin.Runner {
	return func(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
		*calls++
		if len(args) != 3 || args[0] != "show" || args[1] != "-o" {
			return nil, []byte("unexpected arguments"), exec.ErrNotFound
		}
		v, ok := fakeStore[args[2]]
		if !ok {
			return nil, []byte("entry is not in the password store"), exitError{code: exitNotFound}
		}
		// gopass ends its output with a newline; the plugin trims exactly one.
		return []byte(v + "\n"), nil, nil
	}
}
```
<!-- /ds:block hash=e0b08d -->

## Step 3 — Read through dotenv

`Get` returns the reference as written; `GetExpanded` asks the plugin. A missing entry is an error wrapping `plugin.ErrNotFound`, never an empty string:

<!-- ds:block id=gopass-readdotenv-2xz5yprn -->
**readDotenv** · [`examples/plugins/gopass-fake-runner/main.go:83-100`](../../../examples/plugins/gopass-fake-runner/main.go#L83-L100)

```go
func readDotenv(w io.Writer, p *gopassplugin.Plugin) error {
	f := dotenv.Parse(envSource, dotenv.WithPlugin(p))
	raw, _ := f.Get("DB_PASSWORD")
	pass, _, err := f.GetExpanded("DB_PASSWORD")
	if err != nil {
		return err
	}
	user, _, err := f.GetExpanded("DB_USER")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv raw:      %s\n", raw)
	fmt.Fprintf(w, "dotenv expanded: %s (user %s)\n", pass, user)
	// A missing entry is an error, never an empty value.
	_, _, err = f.GetExpanded("MISSING")
	fmt.Fprintf(w, "missing entry:   not found=%t\n", errors.Is(err, plugin.ErrNotFound))
	return nil
}
```
<!-- /ds:block hash=8b0fb8 -->

## Step 4 — Share the instance with JSONC

The same plugin installs in a JSONC document. The password was cached by the dotenv read, so gopass does not run again:

<!-- ds:block id=gopass-readjsonc-egyent23 -->
**readJSONC** · [`examples/plugins/gopass-fake-runner/main.go:106-118`](../../../examples/plugins/gopass-fake-runner/main.go#L106-L118)

```go
func readJSONC(w io.Writer, p *gopassplugin.Plugin) error {
	doc, err := jsonc.Parse([]byte(jsoncSource), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc expanded:  %v\n", v)
	fmt.Fprintf(w, "jsonc unchanged: %t\n", string(doc.Render()) == jsoncSource)
	return nil
}
```
<!-- /ds:block hash=56c2a2 -->

## Try it

```sh
cd examples/plugins/gopass-fake-runner && go run .
```

```text
dotenv raw:      ${gopass:personal/app/db}
dotenv expanded: hush-from-gopass (user app)
missing entry:   not found=true
jsonc expanded:  hush-from-gopass
jsonc unchanged: true
gopass calls:    3
```

Three calls: the password and the user once each, plus the missing entry. The JSONC read came from the cache.

## Pitfalls

- **Only the first line is returned.** The plugin runs `gopass show -o`, which prints the password line; one trailing newline is trimmed and every other byte is kept. Fields of a multi-line entry (`ref#key`, gopass's `show path key`) are **not supported yet**: store each value you need as its own entry (`personal/app/db-user`), or wrap the call in your own `Runner` if you must read a field.
- **Errors map to the base sentinels.** Exit status 11 or the text "not in the password store" is `plugin.ErrNotFound`; a missing binary, a timeout, a cancelled context or any other failure is `plugin.ErrUnavailable`. A Lookuper turns not-found into `ok=false` (next Lookuper), but an unavailable store is always an error, so a locked store never reads as "unset".
- **The secret never appears in an error.** Only stdout carries the value; the error repeats at most 200 bytes of the first stderr line, with any line of stdout redacted from it (`TestSecretNeverInError`).
- **Refs that could be read as a flag run nothing.** An empty ref, one starting with `-`, or one with a control character is refused before gopass starts (`TestBadRefRunsNothing`). With `WithPrefix`, names containing `/`, `\`, `..`, a leading `-` or control bytes are never mapped, so a variable name cannot walk the store.
- **Stdin is not connected.** gopass cannot stop to ask a question; if gpg needs a passphrase, the gpg agent and its pinentry must provide it, or the call times out (`WithTimeout`).
- **The cache lives as long as the plugin.** A long-running process that wants rotated secrets calls `ClearCache()` or builds a new plugin. `Show(ref)` reads through the same cache without a document.

## Related

- [Value plugins overview](overview.md) (Lookuper vs Resolver, ordering, caching), [write a value plugin](write-a-value-plugin.md)
- Package reference: [plugins/gopass README](../../../plugins/gopass/README.md)
- Example: [`examples/plugins/gopass-fake-runner`](../../../examples/plugins/gopass-fake-runner); cross-format test: [`integration/gopass`](../../../integration/gopass)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugins/gopass](https://pkg.go.dev/github.com/ubgo/textdata/plugins/gopass)
