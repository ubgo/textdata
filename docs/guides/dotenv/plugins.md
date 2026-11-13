# Plugins

How to plug secret stores, decryption, sandboxed commands, save-time checks and auditing into the `dotenv` library without the library knowing about any of them, and how to write your own plugin type. You want a plugin when a value should read differently from how it is written (a password fetched from a vault, a ciphertext decrypted on read, an environment fallback), when a write must be refused (a plaintext secret, a missing key), or when you want to watch the file (an audit of disabled settings, a list of unused variables).

A plugin is any value with a `Name() string` method, installed with `dotenv.WithPlugin`. What it does depends on which of seven small capability interfaces it also implements. A plugin implements only the ones it needs, and one plugin may implement several, so a secret store can share one client and one cache between its lookup and its guard. For a one-function hook you do not need a type at all: `WithLookup`, `WithValueTransform`, `WithSaveGuard` and `WithCommandRunner` wrap a plain function.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

Snippets come from two runnable examples: [`examples/dotenv/plugin-hooks`](../../../examples/dotenv/plugin-hooks/) (function options, a Resolver, observers, ordering, the firewall) and [`examples/dotenv/write-a-plugin`](../../../examples/dotenv/write-a-plugin/) (a plugin type with a Lookuper and a SaveGuard). Every store in them is an in-memory map. Ready-made value plugins (gopass, file, extends, vault, aws) live in their own modules under [`plugins/`](../../../plugins/).

The seven hooks:

| Capability | Method | Runs | Several installed | Can fail | Function option (name it reports) |
|---|---|---|---|---|---|
| `EntryObserver` | `ObserveEntry(*Entry)` | once per entry while parsing, in file order | all run, install order | no | none |
| `Lookuper` | `Lookup(name) (string, bool, error)` | for a `${VAR}` the file does not define | **first to answer wins** | yes, aborts the read | `WithLookup` (`lookup`) |
| `Resolver` | `Schemes() []string`, `Resolve(scheme, ref) (string, error)` | for a `${scheme:ref}` whose scheme it owns | **first owner of the scheme wins** | yes, aborts the read | none |
| `CommandRunner` | `RunCommand(cmd) (string, error)` | for a `$(cmd)` in a value that interpolates | **last installed wins** (a replacement) | yes, aborts the read with an `*expand.CommandError` naming the command | `WithCommandRunner` (`command-runner`) |
| `ExpandObserver` | `ObserveExpand(key, name, resolved)` | after every reference resolution | all run, install order | no | none |
| `ValueTransformer` | `TransformValue(key, value) (string, error)` | after a value is expanded, on literal values too | **chained**, each sees the last output | yes, aborts the read | `WithValueTransform` (`value-transform`) |
| `SaveGuard` | `GuardSave(*File) error` | before `Save` or `SaveAs` writes anything | run in order; **first error aborts** | yes, the write is refused | `WithSaveGuard` (`save-guard`) |

`Lookuper`, `Resolver`, `CommandRunner`, `ValueTransformer` and `ExpandObserver` are aliases of the shared [`github.com/ubgo/textdata/plugin`](../../../README.md#plugin) interfaces, so a plugin written once against `plugin` works in every textdata format. `EntryObserver` and `SaveGuard` are dotenv's own because they see dotenv types; a cross-format guard written against `plugin.SaveGuard` (`GuardSave(plugin.Document) error`) is accepted too ([Step 3](#step-3--write-a-plugin-type)).

Expansion runs in `GetExpanded`, `ExpandedMap` and `Read`. `Get`, `Map`, `Pairs`, `Render` and `Save` never expand, so lookups, resolvers, runners, transformers and expand observers never touch them.

## Step 1 — Install one-function hooks

`WithLookup` takes a `func(name string) (string, bool)` and calls it for a `${VAR}` the file does not define. The file always wins: a fallback cannot override a value somebody wrote down (`ZONE` in the map below is ignored). A `${VAR:-default}` still falls back to its default when the lookup misses. In an application, pass `os.LookupEnv` to get Compose's environment fallback.

`WithValueTransform` takes a `func(key, value string) (string, error)` that rewrites each value after it is expanded; return the value unchanged when it is not yours. It also runs on single-quoted and backtick values: quoting controls interpolation, not transformation, and an encrypted value is often single-quoted to keep `$` in the ciphertext literal. Its output is not expanded again, and an error aborts the read, so you never get ciphertext back where you expected a secret. `Get` and `Render` still return the ciphertext, so that is what gets saved.

`WithSaveGuard` takes a `func(*File) error` that runs before `Save` or `SaveAs` writes anything. Returning an error refuses the write: no temp file is created and the destination keeps its old content. The error wraps yours (`errors.Is` reaches it) and names the plugin and the path. `WithCommandRunner` is covered in [expansion.md › Step 7](expansion.md#step-7--run-commands-opt-in).

<!-- ds:block id=functionoptions-u5cr9k2w -->
**functionOptions** · [`examples/dotenv/plugin-hooks/main.go:72-103`](../../../examples/dotenv/plugin-hooks/main.go#L72-L103)

```go
func functionOptions(w io.Writer) error {
	env := map[string]string{"REGION": "eu-west-1", "ZONE": "ignored"}
	keys := map[string]string{"c2VjcmV0": "hunter2"}
	f := dotenv.Parse("ZONE=${REGION}-a\nPASS='encrypted:c2VjcmV0'\n",
		dotenv.WithLookup(func(name string) (string, bool) {
			v, ok := env[name]
			return v, ok
		}),
		dotenv.WithValueTransform(func(key, v string) (string, error) {
			id, ok := strings.CutPrefix(v, cipherPrefix)
			if !ok {
				return v, nil
			}
			if plain, found := keys[id]; found {
				return plain, nil
			}
			return "", fmt.Errorf("%s: no key for %q", key, id)
		}),
		dotenv.WithSaveGuard(func(*dotenv.File) error { return errors.New("read-only") }),
	)
	zone, _, err := f.GetExpanded("ZONE")
	if err != nil {
		return err
	}
	// PASS's error is printed, not returned: a nil error here is the point.
	pass, _, passErr := f.GetExpanded("PASS")
	raw, _ := f.Get("PASS")
	fmt.Fprintln(w, "ZONE:", zone, "PASS:", pass, "Get(PASS):", raw, "err:", passErr)
	f.Path = "never-written.env" // the guard refuses before any write
	fmt.Fprintln(w, "save:", f.Save())
	return nil
}
```
<!-- /ds:block hash=6a4635 -->

## Step 2 — Resolve `${scheme:ref}` with a Resolver

A `Resolver` lists the schemes it owns and answers `Resolve(scheme, ref)`. `PASS=${mem:db/password}` then reads as the stored value through `GetExpanded`, while `Get`, `Render` and `Save` keep the reference. Schemes match case-insensitively. A scheme no installed Resolver owns reads exactly as before (an ordinary unset name, so `${vault:x}` is `""`), which means installing one never changes how other values read. A single-quoted `'${mem:x}'` stays literal, and a Resolver that fails aborts with an error naming the plugin, never an empty value. Return `plugin.ErrNotFound` for a missing ref, so callers can match it with `errors.Is`.

The example's `memStore` is a `map[string]string` with `Name()` returning `mem-store` and `Schemes()` returning `[]string{"mem"}`; its `Resolve`:

<!-- ds:block id=memstoreresolve-h5nx8q2w -->
**memStore.Resolve** · [`examples/dotenv/plugin-hooks/main.go:113-118`](../../../examples/dotenv/plugin-hooks/main.go#L113-L118)

```go
func (m memStore) Resolve(scheme, ref string) (string, error) {
	if v, ok := m[ref]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%w: %s:%s", plugin.ErrNotFound, scheme, ref)
}
```
<!-- /ds:block hash=2558e1 -->

<!-- ds:block id=resolver-g8wn2v5j -->
**resolver** · [`examples/dotenv/plugin-hooks/main.go:126-140`](../../../examples/dotenv/plugin-hooks/main.go#L126-L140)

```go
func resolver(w io.Writer) error {
	store := memStore{"db/password": "pa55"}
	src := "PASS=${mem:db/password}\nOTHER=[${vault:x}]\nLIT='${mem:db/password}'\nBAD=${mem:nope}\n"
	f := dotenv.Parse(src, dotenv.WithPlugin(store))
	for _, key := range []string{"PASS", "OTHER", "LIT"} {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s=%s\n", key, v)
	}
	_, _, err := f.GetExpanded("BAD")
	fmt.Fprintln(w, err)
	return nil
}
```
<!-- /ds:block hash=d59155 -->

## Step 3 — Write a plugin type

When hooks share state, write a type. This `vault` implements `Lookuper` (fill `${DB_PASSWORD}`) and `SaveGuard` (refuse a file that holds one of its secrets in plain text) over one store. Capabilities are detected once, at install time, by type assertion, so a plugin without a capability costs nothing on the expansion path.

<!-- ds:block id=vaulttype-c4nm8r2t -->
**vault** · [`examples/dotenv/write-a-plugin/main.go:40-43`](../../../examples/dotenv/write-a-plugin/main.go#L40-L43)

```go
type vault struct {
	secrets map[string]string
	down    bool
}
```
<!-- /ds:block hash=ac73f5 -->

Capabilities are optional interfaces, so a method with a wrong name or signature (`LookUp` instead of `Lookup`, or a `Lookup` without the `error` return) compiles, installs, and silently never runs. One assertion per capability turns that silence into a build error. Use `(*T)(nil)` for pointer receivers and `T{}` or `T(nil)` for value receivers.

<!-- ds:block id=vaultasserts-f8kw2n5d -->
**_** · [`examples/dotenv/write-a-plugin/main.go:49-53`](../../../examples/dotenv/write-a-plugin/main.go#L49-L53)

```go
var (
	_ dotenv.Plugin    = (*vault)(nil)
	_ dotenv.Lookuper  = (*vault)(nil)
	_ dotenv.SaveGuard = (*vault)(nil)
)
```
<!-- /ds:block hash=a7cc28 -->

A `Lookuper` returns `ok=false` for "I do not have this", and the next one is asked. It returns an error for "I could not check". An error aborts expansion, even when the reference has a default: an unreachable store must not look like an unset variable, or a deploy proceeds with an empty password. Wrap one of the base's named errors (`plugin.ErrUnavailable`, `ErrDenied`, `ErrNotFound`, `ErrUnsupportedScheme`) so callers can tell outages apart.

<!-- ds:block id=vaultlookup-h6xq3w9k -->
**vault.Lookup** · [`examples/dotenv/write-a-plugin/main.go:63-69`](../../../examples/dotenv/write-a-plugin/main.go#L63-L69)

```go
func (v *vault) Lookup(name string) (string, bool, error) {
	if v.down {
		return "", false, fmt.Errorf("%w: connection refused", plugin.ErrUnavailable)
	}
	s, ok := v.secrets[name]
	return s, ok, nil
}
```
<!-- /ds:block hash=a476f0 -->

A `SaveGuard` may only veto. The `*File` it receives is a copy made for that call, so it can read anything but cannot change what is written. Read raw values with `Get`: a key set to an unexpanded `${REF}` is what will be saved.

<!-- ds:block id=vaultguard-m2dz7f5p -->
**vault.GuardSave** · [`examples/dotenv/write-a-plugin/main.go:75-85`](../../../examples/dotenv/write-a-plugin/main.go#L75-L85)

```go
func (v *vault) GuardSave(f *dotenv.File) error {
	for _, key := range f.Keys() {
		raw, _ := f.Get(key)
		for name, secret := range v.secrets {
			if strings.Contains(raw, secret) {
				return fmt.Errorf("%w: %s holds %s", errLeak, key, name)
			}
		}
	}
	return nil
}
```
<!-- /ds:block hash=f60978 -->

A guard written against the shared `plugin.SaveGuard` works in every format. `*File` implements `plugin.Document`: `FormatName` is `"dotenv"`, `Location` the path, `ValuePaths` the active keys, `ValueAt` the raw value, `Bytes` exactly what `Save` would write. Both kinds of guard run in one install-ordered list. The example's `requireKeys` is a `[]string` with `Name()` returning `require-keys`:

<!-- ds:block id=requirekeysguard-t3pc7v9m -->
**requireKeys.GuardSave** · [`examples/dotenv/write-a-plugin/main.go:96-103`](../../../examples/dotenv/write-a-plugin/main.go#L96-L103)

```go
func (r requireKeys) GuardSave(doc plugin.Document) error {
	for _, key := range r {
		if _, ok := doc.ValueAt(key); !ok {
			return fmt.Errorf("%s: %s is required", doc.FormatName(), key)
		}
	}
	return nil
}
```
<!-- /ds:block hash=ddadbc -->

Install both with one `WithPlugin` call. [`f.Plugins()`](ds:block?id=plugins-upbxam5w) returns one `PluginInfo` per installed plugin, in install order, with the capabilities it was detected as having; a capability missing from the list means [the method does not match the interface](ds:block?id=hascapability-tcbcz8hy). The [`Capability` constants](ds:block?id=capabilities-kmsggtyr) are `CapLookup`, `CapResolver`, `CapValueTransformer`, `CapCommandRunner`, `CapSaveGuard`, `CapEntryObserver` and `CapExpandObserver`, listed in that fixed order; `PluginInfo.String` renders `name: Cap, Cap`, or `name: (none)`.

<!-- ds:block id=install-w3tj8c5n -->
**install** · [`examples/dotenv/write-a-plugin/main.go:149-165`](../../../examples/dotenv/write-a-plugin/main.go#L149-L165)

```go
func install(w io.Writer, path string, v *vault) (*dotenv.File, error) {
	f, err := dotenv.Open(path, dotenv.WithPlugin(v, requireKeys{"DB_URL"}))
	if err != nil {
		return nil, err
	}
	for _, info := range f.Plugins() {
		fmt.Fprintln(w, "plugin", info)
	}
	url, _, err := f.GetExpanded("DB_URL")
	if err != nil {
		return nil, err
	}
	raw, _ := f.Get("DB_URL")
	fmt.Fprintln(w, "GetExpanded:", url)
	fmt.Fprintln(w, "Get:        ", raw)
	return f, nil
}
```
<!-- /ds:block hash=3c6ca5 -->

A refused save writes nothing. The full error reads `dotenv: plugin "<name>": guard save <path>: <your error>`:

<!-- ds:block id=guard-k7fs2x4b -->
**guard** · [`examples/dotenv/write-a-plugin/main.go:172-197`](../../../examples/dotenv/write-a-plugin/main.go#L172-L197)

```go
func guard(w io.Writer, f *dotenv.File) error {
	before, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	f.Set("DB_PASSWORD", "hush") // someone pastes the secret in
	err = f.Save()
	fmt.Fprintln(w, "leak refused:", errors.Is(err, errLeak))
	fmt.Fprintln(w, strings.Replace(err.Error(), f.Path, envFile, 1))

	f.Unset("DB_PASSWORD", true)
	f.Unset("DB_URL", true)
	err = f.Save()
	fmt.Fprintln(w, strings.Replace(err.Error(), f.Path, envFile, 1))

	after, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Dir(f.Path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "file unchanged:", string(before) == string(after), "files in dir:", len(entries))
	return nil
}
```
<!-- /ds:block hash=15f742 -->

When the store is down, the read fails even though `${DB_PASSWORD:-}` has a default:

<!-- ds:block id=storedown-p8gw3m6d -->
**storeDown** · [`examples/dotenv/write-a-plugin/main.go:203-209`](../../../examples/dotenv/write-a-plugin/main.go#L203-L209)

```go
func storeDown(w io.Writer, v *vault) {
	v.down = true
	f := dotenv.Parse("PASS=${DB_PASSWORD:-}\n", dotenv.WithPlugin(v))
	_, _, err := f.GetExpanded("PASS")
	fmt.Fprintln(w, err)
	fmt.Fprintln(w, "unavailable:", errors.Is(err, plugin.ErrUnavailable))
}
```
<!-- /ds:block hash=94e768 -->

## Step 4 — Watch with observers

An `EntryObserver` sees every entry as it is parsed: comments, blank lines, pairs, commented-out pairs (`KindDisabledPair`), name-only declarations (`KindInherited`) and unrecognised lines (`KindOther`). A commented-out setting is a finding for an auditor, so nothing is filtered. A multi-line value is one entry. Each call gets a copy of the entry, so the observer cannot change the parsed file. It fires from `Parse`, `ParseReader` and `Open`, not from `Set`, `Append` or other edits, and it cannot fail; to reject a file, use a `SaveGuard`.

An `ExpandObserver` is told about every reference resolution: `key` is the entry being expanded, `name` the variable it referenced, `resolved` the result. Branches of `${VAR:-default}` and `${VAR:+alt}` are reported too; literal values report nothing. It fires only during expansion, so call `ExpandedMap` first. Recording the names gives dead-key detection.

The example's `auditor` has both: its `ObserveExpand` records `used[name] = true`, and its `ObserveEntry` collects disabled keys.

<!-- ds:block id=auditorobserveentry-w2gk6t4c -->
**auditor.ObserveEntry** · [`examples/dotenv/plugin-hooks/main.go:157-161`](../../../examples/dotenv/plugin-hooks/main.go#L157-L161)

```go
func (a *auditor) ObserveEntry(e *dotenv.Entry) {
	if e.Kind == dotenv.KindDisabledPair {
		a.disabled = append(a.disabled, e.Key)
	}
}
```
<!-- /ds:block hash=f3afa4 -->

<!-- ds:block id=observers-d3qp9w6r -->
**observers** · [`examples/dotenv/plugin-hooks/main.go:170-185`](../../../examples/dotenv/plugin-hooks/main.go#L170-L185)

```go
func observers(w io.Writer) error {
	a := &auditor{used: map[string]bool{}}
	f := dotenv.Parse("HOST=db\nPORT=5432\nURL=pg://${HOST}\n# DEBUG=true\n", dotenv.WithPlugin(a))
	if _, err := f.ExpandedMap(); err != nil {
		return err
	}
	var unused []string
	for _, key := range f.Keys() {
		if !a.used[key] {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	fmt.Fprintln(w, "disabled:", a.disabled, "never referenced:", unused)
	return nil
}
```
<!-- /ds:block hash=67ac63 -->

## Step 5 — Combine several plugins

Each capability combines in its own way:

- **Lookuper**: the file's own keys first, then each `Lookuper` in install order. The first that returns `ok=true` wins and later ones are not asked. A lookup error stops the chain.
- **Resolver**: the first installed Resolver that lists the scheme answers.
- **ValueTransformer**: chained in install order. Each sees the previous one's output, so one plugin can dereference a value and another decode it.
- **CommandRunner**: a replacement, not a chain. The last runner installed is the only one used.
- **SaveGuard**: run in install order. The first error refuses the write and later guards do not run.
- **EntryObserver** and **ExpandObserver**: all run, in install order.

Install order is the order of the options, and of the plugins within one `WithPlugin(a, b)` call. The function options count as plugins in that order.

<!-- ds:block id=ordering-y2kc6f9t -->
**ordering** · [`examples/dotenv/plugin-hooks/main.go:192-213`](../../../examples/dotenv/plugin-hooks/main.go#L192-L213)

```go
func ordering(w io.Writer) error {
	look := func(answer string) func(string) (string, bool) {
		return func(string) (string, bool) { return answer, answer != "" }
	}
	wrap := func(tag string) func(string, string) (string, error) {
		return func(_, v string) (string, error) { return tag + "(" + v + ")", nil }
	}
	runner := func(tag string) func(string) (string, error) {
		return func(string) (string, error) { return tag, nil }
	}
	f := dotenv.Parse("A=${X}\nB=$(cmd)\n",
		dotenv.WithLookup(look("")), dotenv.WithLookup(look("second")), dotenv.WithLookup(look("third")),
		dotenv.WithValueTransform(wrap("t1")), dotenv.WithValueTransform(wrap("t2")),
		dotenv.WithCommandRunner(runner("first-runner")), dotenv.WithCommandRunner(runner("last-runner")),
	)
	m, err := f.ExpandedMap()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "A:", m["A"], "B:", m["B"])
	return nil
}
```
<!-- /ds:block hash=5d941a -->

## Step 6 — Rely on the firewall

> A hook may change what a value **reads** as. It may never change what gets **written**.

Lookups, resolvers, runners, transformers and expand observers all live in the expansion path, which [`Render`](ds:block?id=render-c8njbs4p) and `Save` never call, and they are handed only strings. So a file round-trips byte for byte whatever is installed, and a decrypted secret cannot be written back in plaintext. The two hooks that receive structured state get copies: a `SaveGuard` is handed a clone of the file made for that call, and an `EntryObserver` a copy of each entry. A guard that calls `Set`, or an observer that rewrites `e.Raw`, changes only its copy, during the call or later through a kept pointer.

The example's `meddler` tries all three: its transformer upper-cases every value, its guard calls `f.Set("A", "changed")`, and its observer overwrites `e.Value` and `e.Raw`.

<!-- ds:block id=firewall-v7bn4h2m -->
**firewall** · [`examples/dotenv/plugin-hooks/main.go:229-246`](../../../examples/dotenv/plugin-hooks/main.go#L229-L246)

```go
func firewall(w io.Writer, dir string) error {
	const src = "A=secret\n# B=off\n"
	f := dotenv.Parse(src, dotenv.WithPlugin(meddler{}))
	read, _, err := f.GetExpanded("A")
	if err != nil {
		return err
	}
	f.Path = filepath.Join(dir, ".env")
	if err := f.Save(); err != nil {
		return err
	}
	saved, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "reads as:", read, "render unchanged:", f.Render() == src, "saved unchanged:", string(saved) == src)
	return nil
}
```
<!-- /ds:block hash=389206 -->

The rule is also asserted by `TestPlugins_NeverAffectBytes`, which renders and edits one file with every value-changing hook and with none, and by the fuzz target `FuzzPluginsNeverAffectBytes`, which does the same for arbitrary input.

## Try it

```sh
cd examples/dotenv/plugin-hooks && go run .
```

```text
== function options
ZONE: eu-west-1-a PASS: hunter2 Get(PASS): encrypted:c2VjcmV0 err: <nil>
save: dotenv: plugin "save-guard": guard save never-written.env: read-only
== Resolver
PASS=pa55
OTHER=[]
LIT=${mem:db/password}
dotenv: plugin "mem-store": resolve mem:nope: not found: mem:nope
== observers
disabled: [DEBUG] never referenced: [PORT URL]
== ordering
A: t2(t1(second)) B: t2(t1(last-runner))
== firewall
reads as: SECRET render unchanged: true saved unchanged: true
```

```sh
cd examples/dotenv/write-a-plugin && go run .
```

```text
== install and read
plugin vault: Lookuper, SaveGuard
plugin require-keys: SaveGuard
GetExpanded: postgres://app@db/app?password=hush
Get:         postgres://${DB_USER}@db/app?password=${DB_PASSWORD}
== guard
leak refused: true
dotenv: plugin "vault": guard save .env: plain-text secret in file: DB_PASSWORD holds DB_PASSWORD
dotenv: plugin "require-keys": guard save .env: dotenv: DB_URL is required
file unchanged: true files in dir: 1
== store down
dotenv: plugin "vault": lookup DB_PASSWORD: plugin unavailable: connection refused
unavailable: true
```

More tested examples (`ExampleWithPlugin`, `ExampleWithPlugin_lookupError`, `ExampleWithPlugin_guard`, `ExampleCommandRunner`, `ExampleEntryObserver`, `ExampleExpandObserver`, `ExampleFile_Plugins_ordering`, `ExampleFile_Plugins_firewall`, `ExampleCapability`, `ExamplePluginInfo`) are in [`dotenv/example_plugin_test.go`](../../../dotenv/example_plugin_test.go).

## Pitfalls

- **A lookup error beats a default.** `${DB_PASSWORD:-}` still fails when the `Lookuper` returns an error. Return `ok=false`, not an error, for "not found".
- **A self-referencing default asks the `Lookuper` only when no earlier line defines the key.** As in Docker Compose, `ZONE=${ZONE:-default-zone}` sees the definitions above it first, so an earlier `ZONE=` line beats the `Lookuper` ([expansion.md › Step 4](expansion.md#step-4--nest-chain-and-extend-a-value)).
- **A runner error fails the read.** A `CommandRunner` that returns an error makes `GetExpanded` and `ExpandedMap` return an `*expand.CommandError` naming the command and unwrapping to your error. To let a command fail harmlessly, return a fallback value with a nil error.
- **The function options name every hook the same.** Two `WithLookup` calls both report as `lookup`, so an error does not tell you which one failed. Write a type with its own `Name()` when that matters.
- **Editing what a guard or observer is handed does nothing.** `GuardSave` receives a copy of the file and `ObserveEntry` a copy of the entry, so a `Set` or a changed `e.Value` there is silently lost. To change a value before it is written, call `Set` on the file yourself before `Save`.
- **A typed-nil plugin panics.** `WithPlugin((*MyPlugin)(nil))` satisfies the interface and panics on first use. A plain `nil` is ignored, and so is a `nil` function passed to `WithLookup`, `WithValueTransform` or `WithSaveGuard`. `WithCommandRunner(nil)` installs nothing but still turns substitution on with the default `sh -c` runner.
- **A plugin dotenv cannot use is refused.** A plugin whose only abilities are ones dotenv never acts on (the `extends` Includer, a bare `Validator`) is not installed: `Open`, `ParseReader` and `Read` return `dotenv: plugin "<name>": no ability dotenv supports (<abilities>)` (`errors.Is(err, dotenv.ErrUnsupportedPlugin)`), and on a file from `Parse` the error comes from `GetExpanded`, `ExpandedMap`, `Save` and `SaveAs`. A plugin with no recognised ability at all still installs and shows as `name: (none)`.
- **A wrong method signature is silent.** Add the compile-time assertions from Step 3 to every plugin type, and check `f.Plugins()` when a hook seems to do nothing.
- **Plugins do not reach the process environment by themselves.** The library never reads it. Pass `WithLookup(os.LookupEnv)` when you want that.
- **`Clone` keeps the plugins.** A clone of a file with a `SaveGuard` is guarded too.

## Related

- [expansion.md](expansion.md): the `${…}` operators and `$(…)` substitution that lookups, runners and observers hook into.
- [entries.md](entries.md): the entry kinds an `EntryObserver` sees.
- [saving.md](saving.md): `Save` and `SaveAs`, where guards run.
- [typed-config.md](typed-config.md): turning the expanded strings into a typed struct.
- Module README: [Plugins](../../../dotenv/README.md#plugins), [The seven hooks](../../../dotenv/README.md#the-seven-hooks), [The firewall](../../../dotenv/README.md#the-firewall), [Writing a plugin](../../../dotenv/README.md#writing-a-plugin).
- Shared interfaces: [root README › plugin](../../../README.md#plugin).
- Examples: [`examples/dotenv/plugin-hooks`](../../../examples/dotenv/plugin-hooks/), [`examples/dotenv/write-a-plugin`](../../../examples/dotenv/write-a-plugin/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv#Plugin](https://pkg.go.dev/github.com/ubgo/textdata/dotenv#Plugin).
