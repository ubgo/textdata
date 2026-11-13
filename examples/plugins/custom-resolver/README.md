# custom value plugin

Writes a value plugin from scratch against the shared `github.com/ubgo/textdata/plugin` package: a `Store` type that is a `Resolver` for `${mem:ref}` and a `Lookuper` for plain `${NAME}`, backed by a map that stands in for a real secret store. The same type is installed in a dotenv file and a JSONC document.

It shows:

- The three methods and the compile-time assertions that keep a typo from silently disabling a capability.
- `Plugins()` reporting what each installed plugin was detected as.
- Ordering: the file's own value beats every Lookuper (`ZONE`), the first Lookuper with an answer wins (`DB_HOST`), the next one is asked only on a miss (`CACHE_HOST`), and the first Resolver owning a scheme wins (`DB_PASSWORD`).
- A scheme no plugin owns (`${mme:...}`, a typo) reading as an empty value.
- A store outage arriving as a `*plugin.Error` that names the plugin and wraps `plugin.ErrUnavailable`.

## Run

```sh
cd examples/plugins/custom-resolver && go run .
```

Expected output:

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

Guide: [write a value plugin](../../../docs/guides/plugins/write-a-value-plugin.md).
