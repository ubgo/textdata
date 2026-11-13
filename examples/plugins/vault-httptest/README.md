# vault against an httptest server

Installs the vault value plugin (`github.com/ubgo/textdata/plugins/vault`) in a dotenv file and a JSONC document. An `httptest` server answers like Vault's KV v1 and KV v2 engines and refuses any request without the demo token, so nothing talks to a real Vault.

It shows:

- `${vault:secret/data/app#password}` (KV v2, one field), `?version=1` for an older version, and `${vault:kv/legacy}` (KV v1, a one-field secret, no `#field` needed).
- `WithLookup` and `WithLookupPrefix` answering plain `${API_TOKEN}` and `${DB_USER}`.
- The cache: eight reads, four requests (three secrets plus the refused one; failures are not cached).
- `plugin.ErrDenied` for a 403 and `vault.ErrInvalidRef` for a multi-field secret read without `#field`.
- `connectFromEnv`, the production constructor reading `VAULT_ADDR` and `VAULT_TOKEN` through `FromEnv`; the test runs it against the fake server.

## Run

```sh
cd examples/plugins/vault-httptest && go run .
```

Expected output:

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

Guide: [vault plugin](../../../docs/guides/plugins/vault.md).
