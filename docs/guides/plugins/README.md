# Value plugins

Plugins that feed values into a config file at read time, each its own Go module that requires only the base, so it works in dotenv and jsonc alike. Start with the overview, then the backend you use. To choose one, see [Which value plugin or secret backend](../../choosing/value-plugins.md).

| Guide | Module | What you learn | Example |
|---|---|---|---|
| [overview.md](overview.md) | - | Lookuper or Resolver, installing one plugin in both formats, ordering, caching | [`file-confined`](../../../examples/plugins/file-confined/) |
| [gopass.md](gopass.md) | `plugins/gopass` | `${gopass:path}`, name mapping, a fake runner for tests | [`gopass-fake-runner`](../../../examples/plugins/gopass-fake-runner/) |
| [file.md](file.md) | `plugins/file` | `${file:path}`, confinement, size limit, trimming | [`file-confined`](../../../examples/plugins/file-confined/) |
| [extends.md](extends.md) | `plugins/extends` | the disk and `fs.FS` includers for `"$extends"` (jsonc) | [`extends-fs`](../../../examples/plugins/extends-fs/) |
| [vault.md](vault.md) | `plugins/vault` | KV v1/v2 references, `FromEnv`, lookups, errors | [`vault-httptest`](../../../examples/plugins/vault-httptest/) |
| [aws.md](aws.md) | `plugins/aws` | Secrets Manager and SSM references, credentials, lookups | [`aws-httptest`](../../../examples/plugins/aws-httptest/) |
| [write-a-value-plugin.md](write-a-value-plugin.md) | base `plugin` | your own Resolver and Lookuper, errors, install order | [`custom-resolver`](../../../examples/plugins/custom-resolver/) |

Every example runs against a fake (a runner, a temp folder, an `fs.FS` or an `httptest` server), never a real store.
