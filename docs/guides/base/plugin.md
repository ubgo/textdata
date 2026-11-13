# Plugins: one interface set for every format

Package `github.com/ubgo/textdata/plugin` defines the abilities a plugin can have: supply missing values, resolve `${scheme:ref}`, run `$(cmd)`, transform values after reading, veto a save, observe expansions, validate a document, and load `$extends` targets. A plugin written against this package installs unchanged in dotenv and in jsonc, because formats hand plugins plain strings and a small read-only `Document`, never their own types. Read this page when you write a plugin (a secret store, a policy check, an audit hook), or when a plugin you installed does not seem to run.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

The `plugin` package is in the base module (standard library only); a plugin module needs nothing else. The program for this page installs its plugins in both formats, so it also uses `github.com/ubgo/textdata/dotenv` and `github.com/ubgo/textdata/jsonc`: [`examples/base/registry`](../../../examples/base/registry). Ready-made plugins live under [`plugins/`](../../../plugins) (gopass, file, extends, vault, aws).

The interfaces, each a single method beside `Name() string`:

| Interface | Method | What the format does with it |
|---|---|---|
| `Lookuper` | `Lookup(name) (value, ok, err)` | asks it for a `${NAME}` the file itself does not define; the file always wins; the first `ok=true` wins |
| `Resolver` | `Schemes() []string`, `Resolve(scheme, ref)` | answers `${scheme:ref}` for the schemes it lists |
| `CommandRunner` | `RunCommand(cmd) (string, error)` | runs `$(cmd)`; installing one turns command substitution on; the last installed wins |
| `ValueTransformer` | `TransformValue(path, value) (string, error)` | rewrites a value after expansion (decrypt, trim); chained in install order |
| `SaveGuard` | `GuardSave(doc Document) error` | may refuse a save; runs before anything is written |
| `ExpandObserver` | `ObserveExpand(path, name, resolved)` | sees every reference resolution; cannot fail |
| `Validator` | `Validate(doc Document) error` | checks a document when the caller asks (`Validate()`), never on read |
| `Includer` | `Include(from, ref) (data, location, err)` | loads an `$extends` target from somewhere other than the local disk |

## Step 1 — Write a plugin with the abilities it needs

A plugin is any type with `Name()`. Add the methods for the abilities you want; the format detects them by type assertion when you install the plugin. One type may hold several: this store is a `Lookuper` for undefined names and a `Resolver` for `${store:ref}`, so both share one client and one cache. Failures wrap the named errors (`ErrUnavailable`, `ErrNotFound`, `ErrUnsupportedScheme`, `ErrDenied`) so callers can branch with `errors.Is`.

<!-- ds:block id=store-wgesjcub -->
**store** · [`examples/base/registry/main.go:64`](../../../examples/base/registry/main.go#L64-L64)

```go
type store struct{ values map[string]string }
```
<!-- /ds:block hash=70bce4 -->

Its `Lookup` answers names the file leaves undefined; a store that cannot be reached returns an error wrapping `ErrUnavailable`, never `ok=false`:

<!-- ds:block id=store-lookup-2fkdgvx4 -->
**store.Lookup** · [`examples/base/registry/main.go:69-77`](../../../examples/base/registry/main.go#L69-L77)

```go
func (s store) Lookup(name string) (string, bool, error) {
	if name == downKey {
		// Down is an error, never "not found": an unreachable store must not
		// look like an unset variable.
		return "", false, fmt.Errorf("dial store: %w", plugin.ErrUnavailable)
	}
	v, ok := s.values[name]
	return v, ok, nil
}
```
<!-- /ds:block hash=c969a5 -->

Its `Resolve` answers `${store:ref}` for the schemes `Schemes()` lists; an unknown scheme is `ErrUnsupportedScheme` and an absent reference `ErrNotFound`:

<!-- ds:block id=store-resolve-b55eetqv -->
**store.Resolve** · [`examples/base/registry/main.go:82-91`](../../../examples/base/registry/main.go#L82-L91)

```go
func (s store) Resolve(scheme, ref string) (string, error) {
	if scheme != storeScheme {
		return "", plugin.ErrUnsupportedScheme
	}
	v, ok := s.values[ref]
	if !ok {
		return "", fmt.Errorf("%s: %w", ref, plugin.ErrNotFound)
	}
	return v, nil
}
```
<!-- /ds:block hash=7c07b3 -->

A `SaveGuard` and a `Validator` receive a `plugin.Document`: `FormatName()`, `Location()`, `ValuePaths()` (keys in dotenv, canonical dotted paths in jsonc), `ValueAt(path)` (the raw, unexpanded value as text) and `Bytes()` (exactly what would be written). Written against that view, one guard protects every format:

<!-- ds:block id=noprivatekeys-cx6t7r74 -->
**noPrivateKeys** · [`examples/base/registry/main.go:97`](../../../examples/base/registry/main.go#L97-L97)

```go
type noPrivateKeys struct{}
```
<!-- /ds:block hash=c49310 -->

<!-- ds:block id=noprivatekeys-guardsave-786rpfc6 -->
**noPrivateKeys.GuardSave** · [`examples/base/registry/main.go:102-109`](../../../examples/base/registry/main.go#L102-L109)

```go
func (noPrivateKeys) GuardSave(doc plugin.Document) error {
	for _, p := range doc.ValuePaths() {
		if v, _ := doc.ValueAt(p); strings.Contains(v, pemMarker) {
			return fmt.Errorf("%s %s: %s holds a private key: %w", doc.FormatName(), filepath.Base(doc.Location()), p, plugin.ErrDenied)
		}
	}
	return nil
}
```
<!-- /ds:block hash=5ca14d -->

## Step 2 — Check what was detected

A method with a slightly wrong signature still compiles and installs, and then never runs. This `upper` plugin's `TransformValue` takes `[]byte` instead of `string`:

<!-- ds:block id=upper-eumvk3jg -->
**upper** · [`examples/base/registry/main.go:115`](../../../examples/base/registry/main.go#L115-L115)

```go
type upper struct{}
```
<!-- /ds:block hash=35111a -->

`plugin.Has(p, cap)` reports whether a plugin implements a capability; `plugin.Capabilities` lists every capability in a fixed order. Every format also reports what it detected at install (`File.Plugins()`, `Doc.Plugins()`, rendered as `store: Lookuper, Resolver`), which turns an hour of "why does my hook not run" into a one-line check.

<!-- ds:block id=capabilities-c6u7z394 -->
**capabilities** · [`examples/base/registry/main.go:124-134`](../../../examples/base/registry/main.go#L124-L134)

```go
func capabilities(w io.Writer) {
	for _, p := range []plugin.Plugin{store{}, noPrivateKeys{}, upper{}} {
		var got []string
		for _, c := range plugin.Capabilities {
			if plugin.Has(p, c) {
				got = append(got, string(c))
			}
		}
		fmt.Fprintf(w, "%-16s %v\n", p.Name()+":", got)
	}
}
```
<!-- /ds:block hash=3d1744 -->

## Step 3 — Keep plugins in a Registry

`plugin.Registry` is what a format uses inside: plugins in install order, a nil plugin ignored, `Info()` for reporting, `Filter[T]` for every installed plugin implementing `T`, and `NameFor(cap, i)` to name the i-th holder of a capability in an error. `NewRegistry` takes extra `Check`s for abilities this package does not define (dotenv adds its own `EntryObserver` this way); a `Check` with a known `Cap` replaces that detection.

<!-- ds:block id=registry-v5n983ak -->
**registry** · [`examples/base/registry/main.go:140-152`](../../../examples/base/registry/main.go#L140-L152)

```go
func registry(w io.Writer) {
	r := plugin.NewRegistry(plugin.Check{Cap: capCloser, Has: func(p plugin.Plugin) bool {
		_, ok := p.(io.Closer)
		return ok
	}})
	r.Add(store{}, noPrivateKeys{}, upper{}, nil) // a nil plugin is ignored
	for _, info := range r.Info() {
		fmt.Fprintln(w, "info:", info)
	}
	guards := plugin.Filter[plugin.SaveGuard](r)
	fmt.Fprintf(w, "save guards: %d, first resolver: %s, second resolver: %s\n",
		len(guards), r.NameFor(plugin.CapResolver, 0), r.NameFor(plugin.CapResolver, 1))
}
```
<!-- /ds:block hash=01ae62 -->

## Step 4 — Name the plugin in errors

`plugin.Errorf(prefix, pluginName, op, err)` builds a `*plugin.Error` whose message says which plugin failed while doing what, and which unwraps to the plugin's own error. Formats wrap every plugin failure this way.

<!-- ds:block id=pluginerrors-874f9ryb -->
**pluginErrors** · [`examples/base/registry/main.go:158-164`](../../../examples/base/registry/main.go#L158-L164)

```go
func pluginErrors(w io.Writer) {
	_, _, cause := store{}.Lookup(downKey)
	err := plugin.Errorf(errorPrefix, storeName, "lookup "+downKey, cause)
	var pe *plugin.Error
	fmt.Fprintf(w, "error: %v\nunavailable=%v plugin=%v\n",
		err, errors.Is(err, plugin.ErrUnavailable), errors.As(err, &pe) && pe.Plugin == storeName)
}
```
<!-- /ds:block hash=cc1052 -->

## Step 5 — Install the same plugins in dotenv and jsonc

`dotenv.WithPlugin` and `jsonc.WithPlugin` take the same values. The store answers `${DB_PASSWORD}` (which neither file defines) and `${store:api/token}` in both; the guard refuses to save a private key in both, and the file on disk keeps its old bytes.

<!-- ds:block id=lookupinbothformats-cyx5gztt -->
**lookupInBothFormats** · [`examples/base/registry/main.go:170-198`](../../../examples/base/registry/main.go#L170-L198)

```go
func lookupInBothFormats(w io.Writer) error {
	s := store{values: map[string]string{"DB_PASSWORD": sampleSecret, "api/token": "t-123"}}

	env := dotenv.Parse("DSN=postgres://app@db/app?password=${DB_PASSWORD}\nTOKEN=${store:api/token}\n", dotenv.WithPlugin(s))
	dsn, _, err := env.GetExpanded("DSN")
	if err != nil {
		return err
	}
	token, _, err := env.GetExpanded("TOKEN")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv: DSN=%s TOKEN=%s\n", dsn, token)

	doc, err := jsonc.Parse([]byte(`{"db": {"password": "${DB_PASSWORD}"}, "token": "${store:api/token}"}`), jsonc.WithPlugin(s))
	if err != nil {
		return err
	}
	pw, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	tok, err := doc.GetExpanded("token")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc:  db.password=%v token=%v\n", pw, tok)
	return nil
}
```
<!-- /ds:block hash=fcfa16 -->

<!-- ds:block id=guardinbothformats-jr8cm64t -->
**guardInBothFormats** · [`examples/base/registry/main.go:208-226`](../../../examples/base/registry/main.go#L208-L226)

```go
func guardInBothFormats(w io.Writer, dir string) error {
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TLS_KEY=\n"), 0o600); err != nil {
		return err
	}
	env, err := dotenv.Open(envPath, dotenv.WithPlugin(noPrivateKeys{}))
	if err != nil {
		return err
	}
	env.Set("TLS_KEY", privateKey)
	envErr := env.Save()
	fmt.Fprintf(w, "dotenv save: %s (denied=%v)\n", hideDir(envErr, dir), errors.Is(envErr, plugin.ErrDenied))
	onDisk, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, ".env on disk: %q\n", onDisk)
	return guardJSONC(w, dir)
}
```
<!-- /ds:block hash=c90c51 -->

<!-- ds:block id=guardjsonc-hzz8h899 -->
**guardJSONC** · [`examples/base/registry/main.go:231-247`](../../../examples/base/registry/main.go#L231-L247)

```go
func guardJSONC(w io.Writer, dir string) error {
	jsonPath := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(jsonPath, []byte(`{"tls": {"key": ""}}`+"\n"), 0o600); err != nil {
		return err
	}
	doc, err := jsonc.Open(jsonPath, jsonc.WithPlugin(noPrivateKeys{}))
	if err != nil {
		return err
	}
	if err := doc.Set("tls.key", privateKey); err != nil {
		return err
	}
	jsonErr := doc.Save()
	fmt.Fprintf(w, "jsonc save:  %s (denied=%v)\n", hideDir(jsonErr, dir), errors.Is(jsonErr, plugin.ErrDenied))
	fmt.Fprintf(w, "plugins seen by jsonc: %v\n", doc.Plugins())
	return nil
}
```
<!-- /ds:block hash=05cf33 -->

## Try it

```sh
cd examples/base/registry && go run .
```

```text
store:           [Lookuper Resolver]
no-private-keys: [SaveGuard]
upper:           []
info: store: Lookuper, Resolver
info: no-private-keys: SaveGuard
info: upper: (none)
save guards: 1, first resolver: store, second resolver: unknown
error: demo: plugin "store": lookup DOWN: dial store: plugin unavailable
unavailable=true plugin=true
dotenv: DSN=postgres://app@db/app?password=hush TOKEN=t-123
jsonc:  db.password=hush token=t-123
dotenv save: dotenv: plugin "no-private-keys": guard save $DIR/.env: dotenv .env: TLS_KEY holds a private key: access denied (denied=true)
.env on disk: "TLS_KEY=\n"
jsonc save:  jsonc: plugin "no-private-keys": guard save $DIR/config.jsonc: jsonc config.jsonc: tls.key holds a private key: access denied (denied=true)
plugins seen by jsonc: [no-private-keys: SaveGuard]
```

`$DIR` stands for the temp directory the program creates; the program replaces it so the output is the same on every run.

## Pitfalls

- **A wrong signature fails silently.** `upper` above installs with no capabilities and never runs. Check `Plugins()` (or `plugin.Has`) in a test for every plugin you write.
- **Down is an error, not "missing".** A `Lookuper` returns `ok=false` only for "I do not have this name"; an unreachable backend must return an error (wrapping `ErrUnavailable`), or a deploy quietly proceeds with an empty password. A `Resolver` has no `ok`: it returns `ErrNotFound` for a missing reference.
- **The file wins over a Lookuper.** Lookupers run only for names the file does not define, so adding an environment or secret-store fallback never overrides a value someone wrote down.
- **Hooks change reads, never writes.** Resolved, transformed and looked-up values are used in memory; `Save` writes the references back as written. A `SaveGuard` may only veto; it cannot rewrite what is saved.
- **Transformer errors abort the read.** A `ValueTransformer` error is returned instead of the untransformed value, so a caller never receives ciphertext where it expected a secret. Its output is not expanded again.
- **Command substitution is off until you install a `CommandRunner`** (or turn it on with the format's `WithCommandSubstitution` option). With it on, merely reading a file runs commands; only enable it for files you trust.
- **An unowned scheme is not an error.** `${other:thing}` with no resolver for `other` reads as an ordinary name (usually unset, so `""`). A typo in a scheme name therefore reads as an empty value; check in a test that `Plugins()` lists the `Resolver` you expect.
- **`Validator` does not run on read.** Call the format's `Validate()` when you want it.

## Related

- [The expand engine](expand.md): where `Lookuper`, `Resolver` and `CommandRunner` plug in.
- [dotenv README § Plugins](../../../dotenv/README.md#plugins) (the seven hooks, the firewall, writing a plugin) and [jsonc README § Expansion and plugins](../../../jsonc/README.md#expansion-and-plugins).
- Value plugins: [gopass](../../../plugins/gopass/README.md), [file](../../../plugins/file/README.md), [extends](../../../plugins/extends/README.md), [vault](../../../plugins/vault/README.md), [aws](../../../plugins/aws/README.md).
- [Root README § plugin](../../../README.md#plugin).
- Example: [`examples/base/registry`](../../../examples/base/registry).
- API: [plugin on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/plugin).
