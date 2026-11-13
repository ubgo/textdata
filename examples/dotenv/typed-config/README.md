# dotenv/typed-config

Binding expanded values to a typed struct with strconv, time and errors.Join (every bad value reported at once), and the per-key adapter for struct-tag binders with its hidden-error trap.

Guide: [docs/guides/dotenv/typed-config.md](../../../docs/guides/dotenv/typed-config.md).

## Run it

```sh
cd examples/dotenv/typed-config && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/typed-config/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== .env
{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}
== .env.bad
PORT: "eighty" is not an integer
WAIT: "soon" is not a duration
TOKEN is required
== adapter
HOSTS: api.acme.io;www.acme.io true
AUTH: "" false
ExpandedMap: dotenv: required variable MISSING is not set or is empty: set MISSING
```
