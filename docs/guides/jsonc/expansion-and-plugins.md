# Expansion and plugins

A JSONC config often holds references instead of values: `"postgres://${HOST}/app"`, `"${vault:db/password}"`, `"$(git rev-parse HEAD)"`. jsonc can expand them, but only when you ask, and never in place: `Expanded()` returns an expanded copy and `GetExpanded(path)` one value, while `Get`, `JSON`, `Decode`, `Render` and `Save` always see the raw text. So a tool can read secrets at run time and still write the file back with the references, never the secrets. Where the values come from is up to plugins you install.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from [`examples/jsonc/expand`](../../../examples/jsonc/expand); every value source in it is a fake the program owns. It expands this file:

<!-- ds:block id=src-62pdqreb -->
**src** · [`examples/jsonc/expand/main.go:25-34`](../../../examples/jsonc/expand/main.go#L25-L34)

```go
`{
  // ${HOST} comes from the lookup; PORT falls back to 5432
  "db": {
    "url": "postgres://${HOST}:${PORT:-5432}/app",
    "password": "${vault:db/password}",
  },
  "build": "$(git rev-parse --short HEAD)",
  "price": "$$5",
}
`
```
<!-- /ds:block hash=e33da3 -->

The reference forms, inside **string values only** (keys are never expanded), through the base [`expand`](../../../README.md#expand) engine:

| Form | Becomes |
|---|---|
| `$VAR`, `${VAR}` | the value from the installed Lookupers; an unknown name is `""` |
| `${VAR:-d}` / `${VAR-d}` | `d` when unset or empty / only when unset |
| `${VAR:+a}` / `${VAR+a}` | `a` when set and not empty / when set |
| `${VAR:?e}` / `${VAR?e}` | an error with message `e` when unset or empty / only when unset |
| `${scheme:ref}` | the `Resolver` plugin that owns `scheme`; a scheme nobody owns is an ordinary name |
| `$(command)` | the command's output, only with command substitution on; verbatim otherwise |
| `$$` | a literal `$` |

## Step 1 — Expand through a lookup

jsonc never reads the process environment on its own. `WithLookup(fn)` supplies names; pass `os.LookupEnv` for the real environment, or a map as here.

<!-- ds:block id=vars-h9p7r6fq -->
**vars** · [`examples/jsonc/expand/main.go:40`](../../../examples/jsonc/expand/main.go#L40-L40)

```go
var vars = map[string]string{"HOST": "db.internal"}
```
<!-- /ds:block hash=b9f4b8 -->

<!-- ds:block id=lookup-cyykhfrd -->
**lookup** · [`examples/jsonc/expand/main.go:45-48`](../../../examples/jsonc/expand/main.go#L45-L48)

```go
func lookup(name string) (string, bool) {
	v, ok := vars[name]
	return v, ok
}
```
<!-- /ds:block hash=3421fa -->

<!-- ds:block id=expandvars-uq8v3dfv -->
**expandVars** · [`examples/jsonc/expand/main.go:84-113`](../../../examples/jsonc/expand/main.go#L84-L113)

```go
func expandVars(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(src), jsonc.WithLookup(lookup))
	if err != nil {
		return err
	}
	x, err := doc.Expanded()
	if err != nil {
		return err
	}
	url, err := x.GetString("db.url")
	if err != nil {
		return err
	}
	raw, err := doc.GetString("db.url")
	if err != nil {
		return err
	}
	build, err := doc.GetExpanded("build")
	if err != nil {
		return err
	}
	price, err := doc.GetExpanded("price")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "expanded:", url)
	fmt.Fprintln(w, "raw:     ", raw)
	fmt.Fprintln(w, "build (commands off):", build, "| price:", price)
	return nil
}
```
<!-- /ds:block hash=49cade -->

`raw` still holds the reference: that is what `Save` writes. `build` is left as written, because command substitution is off.

## Step 2 — Fail loudly on a missing value

`${VAR:?message}` exists for values that must be set. The error carries the `file:line:col` of the value and unwraps to `*expand.RequiredError` (`Key`, `Message`).

<!-- ds:block id=requirevar-fxndfpjq -->
**requireVar** · [`examples/jsonc/expand/main.go:119-133`](../../../examples/jsonc/expand/main.go#L119-L133)

```go
func requireVar(w io.Writer) error {
	doc, err := jsonc.Parse([]byte("{\n  \"token\": \"${API_TOKEN:?set API_TOKEN}\"\n}"),
		jsonc.WithFileName(configName), jsonc.WithLookup(lookup))
	if err != nil {
		return err
	}
	_, err = doc.GetExpanded("token")
	var req *expand.RequiredError
	if !errors.As(err, &req) {
		return fmt.Errorf("want a RequiredError, got %v", err)
	}
	fmt.Fprintln(w, "required:", err)
	fmt.Fprintln(w, "missing key:", req.Key)
	return nil
}
```
<!-- /ds:block hash=d09def -->

## Step 3 — Install value plugins

`WithPlugin(ps...)` installs plugins written against the shared [`plugin`](../../../README.md#plugin) package, so one plugin works for dotenv and jsonc alike. jsonc detects each plugin's capabilities once, at install, and `Plugins()` reports them. The ones expansion uses:

| Capability | Interface | Used for |
|---|---|---|
| `Lookuper` | `Lookup(name) (string, bool, error)` | `$VAR` names; `WithLookup` installs one named `lookup` |
| `Resolver` | `Schemes()` + `Resolve(scheme, ref)` | `${scheme:ref}` |
| `CommandRunner` | `RunCommand(cmd)` | `$(command)`; installing one turns substitution on; the last one installed wins |
| `ValueTransformer` | `TransformValue(path, value)` | every expanded string, with its canonical dotted path; chained in install order |
| `ExpandObserver` | `ObserveExpand(path, name, resolved)` | sees every resolution, e.g. to list the variables a file uses |

A fake Resolver for `${vault:...}`:

<!-- ds:block id=fakevault-qkhb7zj3 -->
**fakeVault** · [`examples/jsonc/expand/plugins.go:21`](../../../examples/jsonc/expand/plugins.go#L21-L21)

```go
type fakeVault struct{ secrets map[string]string }
```
<!-- /ds:block hash=85ae7d -->

<!-- ds:block id=fakevault-name-mjgwk5tq -->
**fakeVault.Name** · [`examples/jsonc/expand/plugins.go:24`](../../../examples/jsonc/expand/plugins.go#L24-L24)

```go
func (fakeVault) Name() string { return "fake-vault" }
```
<!-- /ds:block hash=16cc47 -->

<!-- ds:block id=fakevault-schemes-pjpp743s -->
**fakeVault.Schemes** · [`examples/jsonc/expand/plugins.go:27`](../../../examples/jsonc/expand/plugins.go#L27-L27)

```go
func (fakeVault) Schemes() []string { return []string{vaultScheme} }
```
<!-- /ds:block hash=9e33b6 -->

<!-- ds:block id=fakevault-resolve-ppavttx8 -->
**fakeVault.Resolve** · [`examples/jsonc/expand/plugins.go:30-35`](../../../examples/jsonc/expand/plugins.go#L30-L35)

```go
func (v fakeVault) Resolve(_, ref string) (string, error) {
	if s, ok := v.secrets[ref]; ok {
		return s, nil
	}
	return "", plugin.ErrNotFound
}
```
<!-- /ds:block hash=29c311 -->

A ValueTransformer and a CommandRunner (`Name` methods as above, see the example source):

<!-- ds:block id=trimspace-butyt4h4 -->
**trimSpace** · [`examples/jsonc/expand/plugins.go:41`](../../../examples/jsonc/expand/plugins.go#L41-L41)

```go
type trimSpace struct{}
```
<!-- /ds:block hash=1de054 -->

<!-- ds:block id=trimspace-transformvalue-6yurxd5v -->
**trimSpace.TransformValue** · [`examples/jsonc/expand/plugins.go:46-48`](../../../examples/jsonc/expand/plugins.go#L46-L48)

```go
func (trimSpace) TransformValue(_, value string) (string, error) {
	return strings.TrimSpace(value), nil
}
```
<!-- /ds:block hash=01b5cc -->

<!-- ds:block id=fakegit-xgrn2dcy -->
**fakeGit** · [`examples/jsonc/expand/plugins.go:54`](../../../examples/jsonc/expand/plugins.go#L54-L54)

```go
type fakeGit struct{}
```
<!-- /ds:block hash=9dbe8e -->

<!-- ds:block id=fakegit-runcommand-9hny8xsp -->
**fakeGit.RunCommand** · [`examples/jsonc/expand/plugins.go:59-61`](../../../examples/jsonc/expand/plugins.go#L59-L61)

```go
func (fakeGit) RunCommand(_ string) (string, error) {
	return "a1b2c3d", nil
}
```
<!-- /ds:block hash=051859 -->

Installed together:

<!-- ds:block id=withplugins-3au5u4sa -->
**withPlugins** · [`examples/jsonc/expand/plugins.go:67-86`](../../../examples/jsonc/expand/plugins.go#L67-L86)

```go
func withPlugins(w io.Writer) error {
	vault := fakeVault{secrets: map[string]string{"db/password": "  hush \n"}}
	doc, err := jsonc.Parse([]byte(src), jsonc.WithLookup(lookup), jsonc.WithPlugin(vault, trimSpace{}, fakeGit{}))
	if err != nil {
		return err
	}
	pw, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	build, err := doc.GetExpanded("build")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "password: %q | build: %v\n", pw, build)
	for _, info := range doc.Plugins() {
		fmt.Fprintln(w, "plugin:", info)
	}
	return nil
}
```
<!-- /ds:block hash=6011cf -->

Ready-made value plugins live in their own modules: [`plugins/gopass`](../../../plugins/gopass), [`plugins/file`](../../../plugins/file), [`plugins/vault`](../../../plugins/vault), [`plugins/aws`](../../../plugins/aws); see the [plugins guides](../plugins/overview.md).

## Step 4 — Turn on command substitution, deliberately

`$(command)` runs nothing by default. `WithCommandSubstitution(true)` runs it through the shared runner, [`shell.Run`](../base/shell.md) (trailing newlines trimmed), and installing any `CommandRunner` turns it on with that runner instead. With it on, expanding a file runs whatever the file says, so use it only for files you trust.

The default runner picks the shell for the OS: `sh -c` on Linux, macOS and other Unix-likes; on Windows `pwsh -NoProfile -NonInteractive -Command` when PowerShell 7 is on `PATH`, else `powershell.exe` with the same flags. A command written for `sh` does not run in PowerShell, so on Windows write it for PowerShell or install a `CommandRunner` that runs another shell ([shell § Step 3](../base/shell.md#step-3--use-another-shell)). Stdin is closed, so a prompting command fails instead of hanging. [`WithCommandContext(ctx)`](ds:block?id=withcommandcontext-3z9bvvrh) bounds the default runner: with `context.WithTimeout`, a command still running when the context ends is killed and `Expanded` and `GetExpanded` fail with an error wrapping `shell.ErrTimeout`.

<!-- ds:block id=commandsubstitution-npg99swj -->
**commandSubstitution** · [`examples/jsonc/expand/plugins.go:92-103`](../../../examples/jsonc/expand/plugins.go#L92-L103)

```go
func commandSubstitution(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(`{"greeting": "$(echo hello)"}`), jsonc.WithCommandSubstitution(true))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("greeting")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "sh -c:", v)
	return nil
}
```
<!-- /ds:block hash=58d710 -->

## Step 5 — Expand in the one-call Read

`WithExpand()` affects only `jsonc.Read`: it expands (after `$extends`) before decoding. Methods on `*Doc` ignore it; `ExpandRequested()` reports it so code holding a `Doc` can honour the caller's wish.

<!-- ds:block id=readexpanded-n5grq4ta -->
**readExpanded** · [`examples/jsonc/expand/plugins.go:110-129`](../../../examples/jsonc/expand/plugins.go#L110-L129)

```go
func readExpanded(w io.Writer, dir string) error {
	path := filepath.Join(dir, configName)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		return err
	}
	m, err := jsonc.Read(path, jsonc.WithExpand(), jsonc.WithLookup(lookup))
	if err != nil {
		return err
	}
	doc, err := jsonc.Open(path, jsonc.WithExpand())
	if err != nil {
		return err
	}
	db, ok := m["db"].(map[string]any)
	if !ok {
		return fmt.Errorf("db is %T, want an object", m["db"])
	}
	fmt.Fprintln(w, "Read:", db["url"], "| ExpandRequested:", doc.ExpandRequested())
	return nil
}
```
<!-- /ds:block hash=fc896a -->

## Try it

```sh
cd examples/jsonc/expand && go run .
```

```text
expanded: postgres://db.internal:5432/app
raw:      postgres://${HOST}:${PORT:-5432}/app
build (commands off): $(git rev-parse --short HEAD) | price: $5
required: app.jsonc:2:12: expand "token": expand: required variable API_TOKEN is not set or is empty: set API_TOKEN
missing key: API_TOKEN
password: "hush" | build: a1b2c3d
plugin: lookup: Lookuper
plugin: fake-vault: Resolver
plugin: trim-space: ValueTransformer
plugin: fake-git: CommandRunner
sh -c: hello
Read: postgres://db.internal:5432/app | ExpandRequested: true
```

## Pitfalls

- **Reads are raw.** `GetString("db.url")` returns `${HOST}`; only `Expanded` and `GetExpanded` expand. This is what keeps secrets out of saved files.
- **An unknown `${NAME}` is `""`, silently.** Use `${NAME:?message}` for anything required, or `${NAME:-default}`.
- **A failing command is an error, not `""`**, whether it runs through the default shell (non-zero exit, or a timeout from `WithCommandContext`) or your `CommandRunner` (any returned error): `Expanded` and `GetExpanded` fail with an `*expand.CommandError` naming the command, at the value's `file:line:col`.
- **A plugin error is an error naming the plugin**, never an empty value: a Resolver returning an error fails the expansion with `jsonc: plugin "<name>": ...` at the value's `file:line:col`.
- **`plugin.ErrNotFound` from a Resolver is still an error** for that reference; a scheme only falls back to an ordinary name when no Resolver claims it.
- **Expansion does not see `$extends` bases.** Call `Extended()` first, then `Expanded()` on the result; `Read` does both in that order.
- **Comments are never expanded**, and neither are keys; `${HOST}` in a comment stays as written.
- **Expansion refuses a document with syntax errors** (`ErrInvalid`).

## Related

- [Extends](extends.md) · [Lint and validate](lint-validate.md) · [Save](save.md) (SaveGuards) · [Read and paths](read-and-paths.md)
- Base engine: [expand guide](../base/expand.md), [plugin guide](../base/plugin.md)
- Plugins: [overview](../plugins/overview.md), [write a value plugin](../plugins/write-a-value-plugin.md)
- Module README: [jsonc § Expansion and plugins](../../../jsonc/README.md#expansion-and-plugins)
- Example: [`examples/jsonc/expand`](../../../examples/jsonc/expand)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
