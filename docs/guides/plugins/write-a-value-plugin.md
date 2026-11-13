# Write a value plugin

A value plugin is a Go type with a `Name()` method plus the ability methods it needs, written against the shared package `github.com/ubgo/textdata/plugin`. Because every format accepts those interfaces, one plugin works in dotenv, JSONC and any later format. Write one when your values live somewhere the ready-made plugins do not reach: an internal secret service, a database of feature flags, a cloud the aws plugin does not cover.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

The base module is stdlib only, so your plugin's module requires nothing else; require a format only in the program or test that installs it. The runnable program for this page is [`examples/plugins/custom-resolver`](../../../examples/plugins/custom-resolver): a `Store` over a map that resolves `${mem:ref}` and looks up mapped plain names.

## Step 1 — Choose the abilities

A format detects each ability once, at install, by type assertion. Implement only what you need; one type may hold several, sharing a client and a cache.

| Interface | Method | Called for |
|---|---|---|
| `plugin.Plugin` (required) | `Name() string` | naming the plugin in errors and `Plugins()` |
| `plugin.Resolver` | `Schemes() []string`, `Resolve(scheme, ref string) (string, error)` | `${scheme:ref}` for a scheme it lists |
| `plugin.Lookuper` | `Lookup(name string) (string, bool, error)` | a `${NAME}` the file does not define |
| `plugin.ValueTransformer` | `TransformValue(path, value string) (string, error)` | every value after expansion, chained (decryption) |
| `plugin.CommandRunner` | `RunCommand(cmd string) (string, error)` | `$(cmd)`; installing one turns command substitution on; last installed wins |
| `plugin.ExpandObserver` | `ObserveExpand(path, name, resolved string)` | every reference resolution (audit, dead-key detection) |
| `plugin.SaveGuard` | `GuardSave(doc plugin.Document) error` | before a save; may veto, never rewrite |
| `plugin.Validator` | `Validate(doc plugin.Document) error` | only when the caller asks (`jsonc.Doc.Validate`) |
| `plugin.Includer` | `Include(from, ref string) ([]byte, string, error)` | `$extends` targets |

## Step 2 — Declare the type and prove its abilities

Hold whatever the backend needs (a client, a cache, a mapping of names). Then add compile-time assertions: an ability method with a wrong signature (`LookUp`, a missing `error` result) compiles, installs and is silently never called; the assertion turns that into a build failure.

<!-- ds:block id=custom-store-decl-bkfm37jz -->
**Store** · [`examples/plugins/custom-resolver/main.go:30-50`](../../../examples/plugins/custom-resolver/main.go#L30-L50)

```go
type Store struct {
	name   string
	values map[string]string
	// lookups maps plain ${NAME} references to store refs; a name not in it
	// is "not mine", so the next Lookuper is asked.
	lookups map[string]string
}

// Compile-time proof of the abilities Store claims. Without these lines a
// typo in a method signature compiles, installs, and is silently never called.
var (
	_ plugin.Plugin   = (*Store)(nil)
	_ plugin.Resolver = (*Store)(nil)
	_ plugin.Lookuper = (*Store)(nil)
)

// Name implements plugin.Plugin; errors name the plugin with it.
func (s *Store) Name() string { return s.name }

// Schemes implements plugin.Resolver.
func (s *Store) Schemes() []string { return []string{memScheme} }
```
<!-- /ds:block hash=f7123e -->

`Schemes` returns lower-case names without the colon; matching is case-insensitive.

## Step 3 — Resolve a scheme reference

`Resolve` refuses a scheme it does not own with `plugin.ErrUnsupportedScheme`, and wraps the base's named errors so callers can branch with `errors.Is`: `plugin.ErrNotFound` for a missing value, `plugin.ErrDenied` for a refusal, `plugin.ErrUnavailable` for a backend that is down. Never put the secret in an error message.

<!-- ds:block id=custom-resolve-d3rb33qv -->
**Store.Resolve** · [`examples/plugins/custom-resolver/main.go:56-68`](../../../examples/plugins/custom-resolver/main.go#L56-L68)

```go
func (s *Store) Resolve(scheme, ref string) (string, error) {
	if !strings.EqualFold(scheme, memScheme) {
		return "", fmt.Errorf("%s: scheme %q: %w", s.name, scheme, plugin.ErrUnsupportedScheme)
	}
	if ref == refDown {
		return "", fmt.Errorf("%s: %s: %w", s.name, ref, plugin.ErrUnavailable)
	}
	v, ok := s.values[ref]
	if !ok {
		return "", fmt.Errorf("%s: %s: %w", s.name, ref, plugin.ErrNotFound)
	}
	return v, nil
}
```
<!-- /ds:block hash=d9f90e -->

## Step 4 — Look up plain names

A Lookuper answers "not mine" with `ok=false`, never with `ErrNotFound`, so the next Lookuper is asked and finally the name reads as unset. A backend failure is still an error: an unreachable store must not look like an unset variable, or a deploy carries on with an empty password.

<!-- ds:block id=custom-lookup-pdggg4g9 -->
**Store.Lookup** · [`examples/plugins/custom-resolver/main.go:75-88`](../../../examples/plugins/custom-resolver/main.go#L75-L88)

```go
func (s *Store) Lookup(name string) (string, bool, error) {
	ref, ok := s.lookups[name]
	if !ok {
		return "", false, nil
	}
	v, err := s.Resolve(memScheme, ref)
	if errors.Is(err, plugin.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}
```
<!-- /ds:block hash=ed5180 -->

## Step 5 — Install it and check the order

Install with `dotenv.WithPlugin` or `jsonc.WithPlugin`. Install order decides who answers: the file's own values first, then Lookupers in order (first non-miss wins), and for a scheme the first Resolver that owns it. Here two stores are installed; `primary` answers `DB_HOST` and `mem:` references, and `fallback` only gets the names `primary` does not map:

<!-- ds:block id=custom-install-3yhpjcb5 -->
**install** · [`examples/plugins/custom-resolver/main.go:107-119`](../../../examples/plugins/custom-resolver/main.go#L107-L119)

```go
func install() []dotenv.Option {
	primary := &Store{
		name:    "primary",
		values:  map[string]string{"prod/db": "pw-primary", "hosts/db": "db.primary"},
		lookups: map[string]string{"DB_HOST": "hosts/db"},
	}
	fallback := &Store{
		name:    "fallback",
		values:  map[string]string{"prod/db": "pw-fallback", "hosts/db": "db.fallback", "hosts/cache": "cache.fallback"},
		lookups: map[string]string{"DB_HOST": "hosts/db", "CACHE_HOST": "hosts/cache", "REGION": "hosts/db"},
	}
	return []dotenv.Option{dotenv.WithPlugin(primary), dotenv.WithPlugin(fallback)}
}
```
<!-- /ds:block hash=f90184 -->

`Plugins()` reports what each plugin was detected as, and a plugin failure arrives as a `*plugin.Error` naming the plugin and wrapping your error:

<!-- ds:block id=custom-readdotenv-ryprryaz -->
**readDotenv** · [`examples/plugins/custom-resolver/main.go:124-138`](../../../examples/plugins/custom-resolver/main.go#L124-L138)

```go
func readDotenv(w io.Writer) {
	f := dotenv.Parse(envSource, install()...)
	for _, info := range f.Plugins() {
		fmt.Fprintf(w, "installed: %s\n", info)
	}
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		var perr *plugin.Error
		if errors.As(err, &perr) {
			fmt.Fprintf(w, "%-12s error from plugin %q, unavailable=%t\n", key+":", perr.Plugin, errors.Is(err, plugin.ErrUnavailable))
			continue
		}
		fmt.Fprintf(w, "%-12s %q\n", key+":", v)
	}
}
```
<!-- /ds:block hash=e98ace -->

The same type installs in JSONC unchanged:

<!-- ds:block id=custom-readjsonc-5ykgkc66 -->
**readJSONC** · [`examples/plugins/custom-resolver/main.go:143-155`](../../../examples/plugins/custom-resolver/main.go#L143-L155)

```go
func readJSONC(w io.Writer) error {
	s := &Store{name: "primary", values: map[string]string{"prod/db": "pw-primary"}}
	doc, err := jsonc.Parse([]byte(`{"db": {"password": "${mem:prod/db}"}}`), jsonc.WithPlugin(s))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-12s %q\n", "jsonc:", v)
	return nil
}
```
<!-- /ds:block hash=625274 -->

## Try it

```sh
cd examples/plugins/custom-resolver && go run .
```

```text
installed: primary: Lookuper, Resolver
installed: fallback: Lookuper, Resolver
REGION:      "eu-west-1"
ZONE:        "eu-west-1-a"
DB_HOST:     "db.primary"
CACHE_HOST:  "cache.fallback"
DB_PASSWORD: "pw-primary"
TYPO:        ""
BROKEN:      error from plugin "primary", unavailable=true
jsonc:       "pw-primary"
```

`ZONE=${REGION}-a` reads the file's own `REGION` even though `fallback` maps that name; `TYPO=${mme:prod/db}` uses a scheme nobody owns and reads as empty.

## Pitfalls

- **No context reaches `Resolve` or `Lookup`.** The interfaces carry none, so fix cancellation and timeouts at construction, as the gopass plugin does with `WithContext` and `WithTimeout` and the vault and aws plugins do with a per-request timeout.
- **Make it safe for concurrent use.** A document may be expanded from several goroutines and one instance may serve several documents; guard a cache with a mutex.
- **Cache successes only.** A cached failure would hide a recovered backend; the ready-made plugins retry failures on the next reference and expose `ClearCache` / `Purge` for rotation.
- **Read only, and only when asked.** Do no I/O in the constructor beyond reading your own options, and never read the process environment unless the caller passes an explicit option (the vault and aws plugins' `FromEnv`).
- **A resolved value is used as is.** It is not expanded again, and the format always writes the reference back, never your value. A `SaveGuard` may only veto; it receives a read-only `plugin.Document`.
- **A `ValueTransformer` error aborts the read.** Return an error rather than the untransformed value, so a caller never silently receives ciphertext.
- **Keep the module's requires to the base.** Tests that install the plugin in a format belong in a separate test module (this repo uses `integration/`), so users of the plugin do not inherit format dependencies.

## Related

- [Value plugins overview](overview.md) (Lookuper vs Resolver, ordering, caching)
- Ready-made plugins to read as references: [gopass](gopass.md), [file](file.md), [extends](extends.md), [vault](vault.md), [aws](aws.md)
- Base interfaces: [root README § plugin](../../../README.md#plugin); dotenv's own hooks (`EntryObserver`, `SaveGuard(*File)`): [dotenv README § writing a plugin](../../../dotenv/README.md#writing-a-plugin)
- Example: [`examples/plugins/custom-resolver`](../../../examples/plugins/custom-resolver)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugin](https://pkg.go.dev/github.com/ubgo/textdata/plugin)
