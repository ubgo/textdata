# dotenv/clone-env

Deriving .env.prod and .env.qa from .env.staging with Clone and SaveAs, keeping every `${reference}`, and what goes wrong when a file is built from ExpandedMap instead.

Guide: [docs/guides/dotenv/editing.md#step-7--derive-another-environment-with-clone](../../../docs/guides/dotenv/editing.md#step-7--derive-another-environment-with-clone).

## Run it

```sh
cd examples/dotenv/clone-env && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/clone-env/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== .env.staging (API_URL -> https://api.staging.acme.io)
# shared
DOMAIN=staging.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.prod (API_URL -> https://api.acme.io)
# shared
DOMAIN=acme.io
LOG_LEVEL=warn

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.qa (API_URL -> https://api.qa.acme.io)
# shared
DOMAIN=qa.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.baked (API_URL -> https://api.staging.acme.io)
API_URL=https://api.staging.acme.io
CDN_URL=https://cdn.staging.acme.io
DOMAIN=acme.io
LOG_LEVEL=debug
```
