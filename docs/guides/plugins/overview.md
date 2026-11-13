# Value plugins: overview

A value plugin feeds values into a config file at read time: a secret from gopass, Vault or AWS, the contents of a key file, or a base file to merge. You install it on a document; the file on disk keeps the reference (`${vault:secret/data/app#password}`) and only the expanded read sees the secret. Reach for one when a value must not live in the file itself, or lives somewhere better (a password store, a mounted secret, a shared base config).

## Before you start

Each plugin is its own Go module that requires only the textdata base, so you pull in only the ones you use:

```sh
go get github.com/ubgo/textdata/plugins/gopass@latest
go get github.com/ubgo/textdata/plugins/file@latest
go get github.com/ubgo/textdata/plugins/extends@latest
go get github.com/ubgo/textdata/plugins/vault@latest
go get github.com/ubgo/textdata/plugins/aws@latest
```

You also need a format: `github.com/ubgo/textdata/dotenv` or `github.com/ubgo/textdata/jsonc`. Every plugin works in both. The runnable programs for this section live in [`examples/plugins`](../../../examples/plugins); each runs against a fake (an injected runner, a temp folder, an in-memory `fs.FS` or an `httptest` server), never a real store.

| Plugin | Reference | Ability | Guide | Example |
|---|---|---|---|---|
| gopass | `${gopass:path}` | Resolver, optional Lookuper | [gopass.md](gopass.md) | [`gopass-fake-runner`](../../../examples/plugins/gopass-fake-runner) |
| file | `${file:./key.pem}` | Resolver | [file.md](file.md) | [`file-confined`](../../../examples/plugins/file-confined) |
| extends | `"$extends": "base.jsonc"` | Includer | [extends.md](extends.md) | [`extends-fs`](../../../examples/plugins/extends-fs) |
| vault | `${vault:secret/data/app#password}` | Resolver, optional Lookuper | [vault.md](vault.md) | [`vault-httptest`](../../../examples/plugins/vault-httptest) |
| aws | `${aws:sm/prod/db#password}`, `${aws:ssm//app/db_url}` | Resolver, optional Lookuper | [aws.md](aws.md) | [`aws-httptest`](../../../examples/plugins/aws-httptest) |
| your own | anything | any of the above | [write-a-value-plugin.md](write-a-value-plugin.md) | [`custom-resolver`](../../../examples/plugins/custom-resolver) |

## Step 1 — Pick the ability: Lookuper or Resolver

The shared package `github.com/ubgo/textdata/plugin` defines what a plugin can do. A format detects each ability once, at install, by type assertion; a plugin implements only what it needs. Two of them supply values:

| | `Lookuper` | `Resolver` |
|---|---|---|
| Answers | a plain `${NAME}` the file does not define | `${scheme:ref}` for the schemes it lists |
| Method | `Lookup(name) (value, ok, err)` | `Schemes() []string` and `Resolve(scheme, ref) (value, err)` |
| "I don't have it" | `ok=false`, and the next Lookuper is asked | an error wrapping `plugin.ErrNotFound` |
| Several installed | first one that answers wins | first one owning the scheme wins |
| Typical use | map `DB_PASSWORD` to a store path without changing the file | spell the store in the file: `${gopass:personal/db}` |

`${scheme:ref}` is the explicit form: the reader of the file sees where the value comes from. A Lookuper is the quiet form: the file says `${DB_PASSWORD}` and the installed plugins decide. The gopass, vault and aws plugins hold both abilities over one client and one cache; their Lookuper is off until you map names (`WithMapping`, `WithPrefix`, `WithLookup`, `WithLookupPrefix`).

Two more abilities matter here. An `Includer` loads `$extends` targets (the extends plugin). `ValueTransformer`, `CommandRunner`, `SaveGuard`, `ExpandObserver` and `Validator` exist too; the [write a value plugin](write-a-value-plugin.md) guide lists them all.

## Step 2 — Install one plugin in both formats

Build the plugin once and pass the same instance to `dotenv.WithPlugin` and `jsonc.WithPlugin`. Here a confined file plugin reads a key next to both documents, and a reference that climbs out of the folder is refused:

<!-- ds:block id=file-readconfined-mx9js8hm -->
**readConfined** · [`examples/plugins/file-confined/main.go:68-91`](../../../examples/plugins/file-confined/main.go#L68-L91)

```go
func readConfined(w io.Writer, cfg string) error {
	p := fileplugin.New(cfg) // the root defaults to cfg itself
	env, err := dotenv.Open(filepath.Join(cfg, envFile), dotenv.WithPlugin(p))
	if err != nil {
		return err
	}
	key, _, err := env.GetExpanded("TLS_KEY")
	if err != nil {
		return err
	}
	_, _, escape := env.GetExpanded("SHARED")
	doc, err := jsonc.Open(filepath.Join(cfg, jsoncFile), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	jkey, err := doc.GetExpanded("tls.key")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv TLS_KEY: %s\n", key)
	fmt.Fprintf(w, "jsonc tls.key:  %v\n", jkey)
	fmt.Fprintf(w, "../shared.txt:  denied=%t\n", errors.Is(escape, plugin.ErrDenied))
	return nil
}
```
<!-- /ds:block hash=866bca -->

`Get`, `Render` and `Save` still return and write the reference; only `GetExpanded` (dotenv) and `GetExpanded` / `Expanded` (jsonc) call the plugin. That split is structural: every value hook sits on the expansion path, which rendering never calls, so a decrypted secret can never be written back to disk.

In JSONC, `jsonc.Read(path, jsonc.WithPlugin(p), jsonc.WithExpand())` expands before decoding; without `WithExpand`, `Read` returns raw values. An `Includer` is the exception: installing one is what makes `Read` merge `$extends`.

## Step 3 — Order plugins on purpose

When several plugins are installed, install order decides who answers. The rules, all shown by the [`custom-resolver`](../../../examples/plugins/custom-resolver) program:

1. **The file wins.** A name the file defines never reaches a Lookuper, so adding a fallback cannot override a value somebody wrote down.
2. **Lookupers: first non-miss wins.** Each is asked in install order; `ok=false` passes to the next; an error stops expansion.
3. **Resolvers: first owner of the scheme wins.** A later plugin with the same scheme is never asked. To run two instances side by side (two Vault servers, two AWS accounts), give one another scheme with `WithScheme`.
4. **Includers:** tried in install order; the first that does not answer `plugin.ErrNotFound` wins.

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

`Plugins()` (dotenv) reports each installed plugin and the abilities it was detected as having, for example `primary: Lookuper, Resolver`.

## Step 4 — Rely on the cache, purge it when secrets rotate

The gopass, vault and aws plugins cache every successful read for the life of the instance: a document expanded twice, or the same secret read from a dotenv file and a JSONC file, costs one call. Two fields of one Vault secret, or two keys of one AWS JSON secret, share one request. Failures are never cached, so a transient outage is retried on the next reference.

A long-running process that wants rotated secrets calls `ClearCache()` (gopass) or `Purge()` (vault, aws), or builds a new instance. gopass can turn its cache off with `WithCache(false)`. The file and extends plugins do not cache; they read the disk each time.

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

## Pitfalls

- **A misspelt scheme reads as empty, not as an error.** `${mme:prod/db}` with no plugin owning `mme` is read as an ordinary unset name, so it expands to `""` (the `TYPO` line above). That is deliberate (installing a resolver never changes how other values read) but it means a typo is silent. Guard required values with `${NAME:?message}` in the file, or check the expanded value.
- **`${scheme:ref:-default}` does not apply a default.** Everything after the first colon is the ref, so the plugin receives `ref:-default`. Defaults work only on plain names.
- **A ref is not expanded.** `${vault:secret/$ENV/app}` sends the literal `$ENV` to the plugin, and the resolved value is not expanded again either.
- **Single-quoted dotenv values stay literal.** `KEY='${gopass:x}'` never calls the plugin, like every single-quoted dotenv value.
- **A store that is down is an error, never an empty value.** Every plugin wraps `plugin.ErrUnavailable`, `plugin.ErrDenied` or `plugin.ErrNotFound`, and the format wraps that in a `*plugin.Error` naming the plugin: `dotenv: plugin "vault": resolve vault:secret/data/app#password: ...`. In JSONC the error also carries `file:line:col`. Branch with `errors.Is`, not on the text.
- **A Lookuper's miss is `ok=false`, a Resolver's miss is an error.** A mapped name whose entry is missing falls through to the next Lookuper and finally reads as unset; the same entry spelled `${gopass:...}` fails the read.
- **Plugins never read the environment on their own.** vault and aws read `VAULT_*` / `AWS_*` only when you pass `FromEnv()`; the formats never fall back to `os.Getenv`. Use `dotenv.WithLookup(os.LookupEnv)` or `jsonc.WithLookup(os.LookupEnv)` if you want that.

## Related

- Plugin guides: [gopass](gopass.md), [file](file.md), [extends](extends.md), [vault](vault.md), [aws](aws.md), [write a value plugin](write-a-value-plugin.md)
- Base interfaces: [root README § plugin](../../../README.md#plugin); dotenv's hooks: [dotenv README § plugins](../../../dotenv/README.md#plugins); JSONC: [jsonc README § expansion and plugins](../../../jsonc/README.md#expansion-and-plugins)
- Examples: [`examples/plugins`](../../../examples/plugins)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugin](https://pkg.go.dev/github.com/ubgo/textdata/plugin)
