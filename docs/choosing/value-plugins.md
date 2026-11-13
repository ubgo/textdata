# Which value plugin or secret backend

A value plugin answers references in a config file at read time, so the file holds `${vault:secret/data/app#password}` and only the expanded read holds the secret. This page helps you choose the ability (Lookuper or Resolver), the backend, and when to write your own.

## Decide the ability: Lookuper or Resolver

| | Lookuper | Resolver |
|---|---|---|
| File says | `${DB_PASSWORD}` | `${vault:secret/data/app#password}` |
| Who decides the source | the program, by mapping names | the file, by naming the scheme |
| Method | `Lookup(name) (value, ok, err)` | `Schemes()` and `Resolve(scheme, ref)` |
| "Not here" | `ok=false`, the next Lookuper is asked | an error wrapping `plugin.ErrNotFound` |
| Pick it when | the same file must work with different backends (local values in dev, a store in prod) | readers of the file should see where each secret comes from |

- If the file is shared between environments that use different stores, keep plain `${NAME}` in the file and install a Lookuper per environment.
- If a reviewer should be able to tell from the file which values are secrets and where they live, write `${scheme:ref}` and install the Resolver.
- If you need both, the gopass, vault and aws plugins are both: their Resolver is always on and their Lookuper turns on when you map names.

Guide: [overview, Step 1](../guides/plugins/overview.md#step-1--pick-the-ability-lookuper-or-resolver). Example: [`examples/plugins/custom-resolver`](../../examples/plugins/custom-resolver).

## Decide the backend

| Your secrets live in | Plugin (`go get github.com/ubgo/textdata/plugins/…`) | Reference | Lookuper turned on by | Formats | Guide | Example |
|---|---|---|---|---|---|---|
| A gopass / pass store on the machine | `gopass` | `${gopass:path}` (first line of the entry) | `WithMapping`, `WithPrefix` | dotenv, jsonc | [gopass](../guides/plugins/gopass.md) | [`gopass-fake-runner`](../../examples/plugins/gopass-fake-runner) |
| Files: mounted secrets, key files, Docker/Kubernetes secret volumes | `file` | `${file:./tls.key}` | - | dotenv, jsonc | [file](../guides/plugins/file.md) | [`file-confined`](../../examples/plugins/file-confined) |
| HashiCorp Vault KV v1 or v2 | `vault` | `${vault:secret/data/app?version=2#password}` | `WithLookup`, `WithLookupPrefix` | dotenv, jsonc | [vault](../guides/plugins/vault.md) | [`vault-httptest`](../../examples/plugins/vault-httptest) |
| AWS Secrets Manager or SSM Parameter Store | `aws` | `${aws:sm/prod/db#password}`, `${aws:ssm//app/db_url}` | `WithLookup`, `WithLookupPrefix` | dotenv, jsonc | [aws](../guides/plugins/aws.md) | [`aws-httptest`](../../examples/plugins/aws-httptest) |
| A shared base config file | `extends` (an Includer, not a value) | `"$extends": "base.jsonc"` | - | jsonc only | [extends](../guides/plugins/extends.md) | [`extends-fs`](../../examples/plugins/extends-fs) |
| Anything else (an internal API, a KMS, a test map) | your own type | your scheme | your `Lookup` | dotenv, jsonc | [write a value plugin](../guides/plugins/write-a-value-plugin.md) | [`custom-resolver`](../../examples/plugins/custom-resolver) |

- If developers already use gopass, start there: no server, the plugin runs the `gopass` binary, and tests inject a runner (`WithRunner`) so they never touch a real store.
- If the platform mounts secrets as files, use `file`: it is confined to its base directory by default and refuses files over 1 MiB (`DefaultMaxSize`), so a reference cannot climb out with `../`.
- If you run Vault, use `vault` with `FromEnv()` (reads `VAULT_ADDR`, `VAULT_TOKEN`, `VAULT_NAMESPACE` once, in `New`). Only token auth exists: log in elsewhere and pass the token.
- If you run on AWS, use `aws` with `FromEnv()` or `FromSharedCredentials`. STS, SSO, `credential_process`, instance metadata and AssumeRole are not built in; supply them through `WithCredentialsSource`.
- If you want layered config files (base plus overrides), that is `extends` on jsonc, or [`Merge`](../guides/jsonc/merge-clone.md) in code; dotenv has no include mechanism.
- If none fits, write a plugin: a type with `Name()` plus `Schemes()`/`Resolve()` or `Lookup()`, importing only the base `plugin` package, so it works in every format.

## Install and order

- Build one plugin instance and pass it to `dotenv.WithPlugin` and `jsonc.WithPlugin`; it shares one client and one cache across documents.
- Several Lookupers: the first that answers wins, in install order. Several Resolvers for one scheme: the first one wins. Put the most specific first ([overview, Step 3](../guides/plugins/overview.md#step-3--order-plugins-on-purpose)).
- The gopass, vault and aws plugins cache successes for the life of the plugin. Vault and aws have `Purge()` for rotation; gopass has `WithCache(false)`.

## Pitfalls

- **A misspelt scheme is not an error.** `${valut:…}` is treated as an unknown reference and expands to empty in the default expansion; review references, or use `${…:?}` on values that must be present.
- **An unreachable store must not look like a missing value.** Built-in plugins return an error wrapping `plugin.ErrUnavailable`; a plugin you write should too, never `ok=false`.
- **The file on disk keeps the reference.** Expansion happens on read only; saving after an expanded read never writes the secret, as long as you edit with the raw getters.
- **`extends` is a jsonc ability.** `dotenv.WithPlugin` refuses it with `dotenv: plugin "extends": no ability dotenv supports (Includer)`, reported by `Open`, `Read`, or the first `GetExpanded`/`ExpandedMap`/`Save` on a parsed file. `schema.Plugin` installs on a dotenv file as a SaveGuard only; its Validator side is not used there.
- **Plugins read the environment only where they say so** (`FromEnv`, `FromSharedCredentials`), once, in `New`; the formats never read or change the process environment.

## Related

- [Value plugins overview](../guides/plugins/overview.md), [dotenv plugins](../guides/dotenv/plugins.md), [jsonc expansion and plugins](../guides/jsonc/expansion-and-plugins.md)
- [The plugin interfaces](../guides/base/plugin.md), [`examples/base/registry`](../../examples/base/registry)
- [Options for your kind of app](app-setups.md)
