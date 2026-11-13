# vault

`github.com/ubgo/textdata/plugins/vault` reads secrets from HashiCorp Vault for every textdata format (dotenv, jsonc, and any later one). One plugin instance is a `plugin.Resolver` for `${vault:...}` references and a `plugin.Lookuper` for plain `${NAME}` references, sharing one HTTP client and one cache.

Stdlib only: it talks to Vault's HTTP API with `net/http`. It only reads (GET); it never writes to Vault.

## Install

```sh
go get github.com/ubgo/textdata/plugins/vault
```

## Example

```go
p, err := vault.New(
	vault.WithAddress("https://vault.example.com:8200"),
	vault.WithToken(token),                      // from your own secret source
	vault.WithLookupPrefix("secret/data/app#"),  // ${NAME} -> field NAME of secret/data/app
)
if err != nil { return err }

f := dotenv.Parse(src, dotenv.WithPlugin(p))
doc, err := jsonc.Parse(data, jsonc.WithPlugin(p)) // the same instance works in every format
```

```dotenv
DB_PASSWORD=${vault:secret/data/app#password}
API_KEY=${vault:kv/app#api_key}
OLD=${vault:secret/data/app?version=3#password}
USER=${user}
```

The resolved value is used in memory only; the file always keeps (and writes back) the reference.

## Reference grammar

```
ref     = path [ "?version=" digits ] [ "#" field ]
path    = segment *( "/" segment )   ; a leading "/" is ignored
```

- `path` is the Vault API path below `/v1/`, exactly as the API wants it: a KV v2 secret includes its `data/` segment (`secret/data/app`), a KV v1 secret does not (`kv/app`). The engine version is detected from the response shape, so both work without configuration.
- `?version=N` (KV v2) reads an older version.
- `#field` picks one key of the secret. It may be left out when the secret holds exactly one key. A field that is not a string (number, bool, object) is returned as its compact JSON text.
- Empty, `.` and `..` segments are refused, so a reference can only reach the path it spells.

## Options

| Option | Does |
|---|---|
| `WithAddress(url)` | Vault address; required (or via `FromEnv`). http or https, no credentials in the URL |
| `WithToken(t)` | sent as `X-Vault-Token`; empty sends none (Vault Agent / proxy) |
| `WithNamespace(ns)` | Vault Enterprise namespace (`X-Vault-Namespace`) |
| `WithHTTPClient(c)` | TLS settings, proxies, tests |
| `WithTimeout(d)` | per request; default 10s |
| `WithName(n)`, `WithScheme(s)` | run two instances side by side (`${vault-dr:...}`) |
| `WithLookup(name, ref)` | `${name}` reads `ref` |
| `WithLookupPrefix(prefix)` | `${NAME}` reads `prefix+NAME`; prefixes are tried in order |
| `FromEnv()` | fills address, token and namespace from `VAULT_ADDR`, `VAULT_TOKEN`, `VAULT_NAMESPACE`, once, in `New`; later options override |

`Purge()` empties the cache. `Read(Ref)` and `ParseRef` are exported for callers that want a value without a document.

## Guarantees

- **No implicit environment.** The process environment is read only when `FromEnv()` is passed, and only in `New`. Nothing else is read from the environment or the disk.
- **Errors by meaning.** 403 and 401 wrap `plugin.ErrDenied`; network failures, timeouts, 429, 5xx (a sealed Vault is 503) and standby 412/473 wrap `plugin.ErrUnavailable`; a missing path, a deleted version, or a missing field wraps `plugin.ErrNotFound`; a malformed reference wraps `vault.ErrInvalidRef`. A Vault that is down is never a silent "unset".
- **No secret in an error.** Errors repeat the reference, the status and Vault's own error text (truncated), never a response body or a value; a malformed response does not repeat the JSON parser's message, which can quote bytes. A missing field lists the field names, never values.
- **Lookuper contract.** An unmapped name, or a mapped reference that does not exist, answers `ok=false` so the next Lookuper is tried; denied and unavailable are errors.
- **Per-plugin cache.** Each secret is read once per plugin (two fields of one secret cost one request), so one config read sees one version of it. Failures are not cached. Two plugin instances never share a cache. Safe for concurrent use.
