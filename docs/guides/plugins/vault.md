# vault plugin

`github.com/ubgo/textdata/plugins/vault` reads secrets from HashiCorp Vault's KV engines. A file says `DB_PASSWORD=${vault:secret/data/app#password}`; the expanded read fetches that field over Vault's HTTP API. One instance is a `Resolver` for `${vault:...}` and, when you map names, a `Lookuper` for plain `${NAME}`, sharing one HTTP client and one cache. Use it when your services already get secrets from Vault and you want config files to point at them instead of holding them.

## Before you start

```sh
go get github.com/ubgo/textdata/plugins/vault@latest
```

You need a Vault address and a token with read access (or a Vault Agent / proxy that adds the token for you). The plugin talks to the HTTP API with `net/http`; there is no Vault SDK dependency. The runnable program for this page is [`examples/plugins/vault-httptest`](../../../examples/plugins/vault-httptest); an `httptest` server stands in for Vault.

## Step 1 — Know the reference grammar

```text
ref     = path [ "?version=" N ] [ "#" field ]
```

| Reference | Reads |
|---|---|
| `${vault:secret/data/app#password}` | KV v2: field `password` of the latest version (the path includes `data/`) |
| `${vault:secret/data/app?version=1#password}` | KV v2: the same field of version 1 |
| `${vault:kv/legacy#token}` | KV v1: field `token` (no `data/` segment) |
| `${vault:kv/legacy}` | the only field of a one-field secret |

The path is the API path below `/v1/`, written exactly as the API wants it. Which engine answered is detected from the response shape, so KV v1 and v2 need no configuration. A field that is not a string comes back as its JSON text (`42`, `true`, `{"a":1}`). `.`, `..` and empty segments are refused, so a reference can only reach the path it spells.

## Step 2 — Fake Vault for tests

The fake answers like both engines (KV v2 nests fields under `data.data` next to `metadata`; KV v1 puts them under `data`), honours `?version=`, refuses a wrong token with 403, and counts requests:

<!-- ds:block id=vault-newfakevault-wnk7tqk8 -->
**newFakeVault** · [`examples/plugins/vault-httptest/main.go:64-82`](../../../examples/plugins/vault-httptest/main.go#L64-L82)

```go
func newFakeVault(requests *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Vault-Token") != demoToken || r.URL.Path == pathForbidden {
			reply(w, http.StatusForbidden, bodyDenied)
			return
		}
		switch {
		case r.URL.Path == pathKV2 && r.URL.Query().Get(queryVersion) == pinnedVersion:
			reply(w, http.StatusOK, bodyKV2V1)
		case r.URL.Path == pathKV2:
			reply(w, http.StatusOK, bodyKV2Latest)
		case r.URL.Path == pathKV1:
			reply(w, http.StatusOK, bodyKV1)
		default:
			reply(w, http.StatusNotFound, bodyNotFound)
		}
	}))
}
```
<!-- /ds:block hash=64a683 -->

## Step 3 — Build the plugin

`vault.New` needs an address; it sends no request, so an unreachable Vault shows up on the first read. `WithLookup(name, ref)` maps one plain name; `WithLookupPrefix(prefix)` answers any name by reading `prefix+NAME`, here a field of one secret:

<!-- ds:block id=vault-newplugin-sx7bn45q -->
**newPlugin** · [`examples/plugins/vault-httptest/main.go:95-102`](../../../examples/plugins/vault-httptest/main.go#L95-L102)

```go
func newPlugin(addr string) (*vault.Plugin, error) {
	return vault.New(
		vault.WithAddress(addr),
		vault.WithToken(demoToken),
		vault.WithLookup("API_TOKEN", "kv/legacy#token"),
		vault.WithLookupPrefix("secret/data/app#"),
	)
}
```
<!-- /ds:block hash=cec632 -->

| Option | Default | Effect |
|---|---|---|
| `WithAddress(url)` | none (required) | `http` or `https` URL without credentials |
| `WithToken(t)` | none | sent as `X-Vault-Token`; empty sends no token (Vault Agent, proxy) |
| `WithNamespace(ns)` | none | Vault Enterprise namespace (`X-Vault-Namespace`) |
| `FromEnv()` | off | fills address, token and namespace from `VAULT_ADDR`, `VAULT_TOKEN`, `VAULT_NAMESPACE`, once, in `New` |
| `WithLookup(name, ref)` | none | `${name}` reads `ref`; tried before prefixes |
| `WithLookupPrefix(p)` | none | `${NAME}` reads `p+NAME`; several prefixes are tried in order |
| `WithHTTPClient(c)` | `&http.Client{}` | TLS settings, proxies, tests |
| `WithTimeout(d)` | 10s | bound on one request |
| `WithName(n)`, `WithScheme(s)` | `vault`, `vault` | run two instances side by side: `${vault:...}` and `${vault-dr:...}` |

## Step 4 — Read from the environment in production

`FromEnv()` is the only way the plugin reads the environment, and only once, in `New`. Options after it override it:

<!-- ds:block id=vault-connectfromenv-9bnckh59 -->
**connectFromEnv** · [`examples/plugins/vault-httptest/main.go:109-111`](../../../examples/plugins/vault-httptest/main.go#L109-L111)

```go
func connectFromEnv() (*vault.Plugin, error) {
	return vault.New(vault.FromEnv(), vault.WithLookupPrefix("secret/data/app#"))
}
```
<!-- /ds:block hash=65a2b8 -->

## Step 5 — Read through dotenv and JSONC

Install the instance with `WithPlugin` in each format. Every reference shape works in both; here each dotenv key is expanded in turn, and the JSONC read of the same secret comes from the cache:

<!-- ds:block id=vault-readdotenv-yt53dn7r -->
**readDotenv** · [`examples/plugins/vault-httptest/main.go:116-126`](../../../examples/plugins/vault-httptest/main.go#L116-L126)

```go
func readDotenv(w io.Writer, p *vault.Plugin) error {
	f := dotenv.Parse(envSource, dotenv.WithPlugin(p))
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-13s %s\n", key+":", v)
	}
	return nil
}
```
<!-- /ds:block hash=8e93b5 -->

## Step 6 — Branch on the errors

A 404, a deleted or destroyed version, or a missing field wraps `plugin.ErrNotFound`; 401 or 403 wraps `plugin.ErrDenied`; a connection failure, 429, 5xx or a standby not caught up (412, 473) wraps `plugin.ErrUnavailable`. A malformed reference, or a multi-field secret read without `#field`, wraps `vault.ErrInvalidRef`:

<!-- ds:block id=vault-showerrors-znzb8ew9 -->
**showErrors** · [`examples/plugins/vault-httptest/main.go:146-151`](../../../examples/plugins/vault-httptest/main.go#L146-L151)

```go
func showErrors(w io.Writer, p *vault.Plugin) {
	_, err := p.Resolve("vault", "secret/data/locked#password")
	fmt.Fprintf(w, "%-13s denied=%t\n", "locked:", errors.Is(err, plugin.ErrDenied))
	_, err = p.Resolve("vault", "secret/data/app")
	fmt.Fprintf(w, "%-13s invalid=%t\n", "no #field:", errors.Is(err, vault.ErrInvalidRef))
}
```
<!-- /ds:block hash=7840d8 -->

## Try it

```sh
cd examples/plugins/vault-httptest && go run .
```

```text
DB_PASSWORD:  pw-v2
OLD_PASSWORD: pw-v1
LEGACY_TOKEN: kv1-token
API_TOKEN:    kv1-token
USER:         app
jsonc:        pw-v2
locked:       denied=true
no #field:    invalid=true
requests:     4
```

Eight reads, four requests: two fields of one secret share a request, the JSONC read is cached, and the refused read is not.

## Pitfalls

- **KV v2 paths include `data/`.** `secret/app#password` against a KV v2 mount is a 404; write `secret/data/app#password`. The `vault kv get` CLI hides that segment, the HTTP API does not.
- **Login methods are not supported yet.** The plugin sends a token; it does not log in with AppRole, Kubernetes, AWS IAM or any other auth method, and does not renew tokens. Obtain the token yourself (or run Vault Agent with auto-auth) and pass it with `WithToken`, or `WithToken("")` against an Agent or proxy that injects it.
- **`#field` is safe in an unquoted dotenv value.** dotenv starts an inline comment only at a `#` preceded by a space or tab, so `${vault:secret/data/app#password}` stays whole. Do not single-quote it: single-quoted values are never expanded.
- **The cache holds one version of each secret for the plugin's lifetime**, so one config read sees one consistent version. Call `Purge()` to pick up a rotation. Failures are never cached (`TestCache`).
- **No error contains a secret value.** Vault's own error text is repeated (truncated to 300 bytes), field names are listed on a missing field, values never are; a response that fails to decode never repeats the parser's message.
- **A Lookuper miss is quiet, a refusal is not.** A mapped reference that does not exist answers `ok=false` and the next Lookuper is tried; denied and unavailable are always errors (`TestLookup`).
- **The plugin is read-only.** It only issues GET requests; nothing writes to Vault.

## Related

- [Value plugins overview](overview.md), [aws plugin](aws.md), [write a value plugin](write-a-value-plugin.md)
- Package reference: [plugins/vault README](../../../plugins/vault/README.md)
- Example: [`examples/plugins/vault-httptest`](../../../examples/plugins/vault-httptest); package [`Example`](../../../plugins/vault/example_test.go); cross-format test: [`integration/vault`](../../../integration/vault)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugins/vault](https://pkg.go.dev/github.com/ubgo/textdata/plugins/vault)
