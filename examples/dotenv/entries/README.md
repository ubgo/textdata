# dotenv/entries

The six entry kinds, Raw versus Value, a linter that walks Entries(), and the lines whose kind surprises people.

Guide: [docs/guides/dotenv/entries.md](../../../docs/guides/dotenv/entries.md).

## Run it

```sh
cd examples/dotenv/entries && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/entries/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== kinds
pair          key="PORT"       active=true
comment       key=""           active=false
blank         key=""           active=false
other         key=""           active=false
disabled-pair key="DEBUG"      active=false
inherited     key="AWS_REGION" active=false
out of range: Kind(99)
== fields
pair URL: Raw=["URL=\"https://${HOST}/v1\""] Value="https://${HOST}/v1"
pair CERT: Raw=["CERT=\"line one" "line two\""] Value="line one\nline two"
== lint
entry 1: PORT is set (1 line(s))
entry 2: DB_USER is disabled
entry 3: not a setting: "MY KEY=value"
entry 4: HOME comes from the environment
entry 5: CERT is set (2 line(s))
Disabled(): [{DB_USER admin}] Inherited(): [HOME]
== surprises
disabled-pair key="TODO"  "# TODO=fix this"
comment       key=""      "# Note: rotate monthly"
disabled-pair key="KEY"   "# KEY: value"
comment       key=""      "# http://example.com"
pair          key="http"  "http://example.com"
other         key=""      "MY KEY=value"
comment       key=""      "# CERT=\"one"
comment       key=""      "# two\""
```
