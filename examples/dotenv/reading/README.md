# dotenv/reading

Read, Open, Parse and ParseReader, the six read accessors over duplicates, disabled and inherited lines, and Existed, Mode and Path.

Guide: [docs/guides/dotenv/reading.md](../../../docs/guides/dotenv/reading.md).

## Run it

```sh
cd examples/dotenv/reading && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/reading/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== Read
API_URL=https://api.example.com
DOMAIN=example.com
missing file is an error: true
== Open
https://api.${DOMAIN} true existed: true
missing file existed: false keys: 0
created: "PORT=8080\n"
== Parse and ParseReader
NAME: my app
round trip: true
keys: [PORT HOST]
== accessors
Get(DB_HOST) = "db.internal" true, Has = true, Count = 2
Get(DB_USER) = "" false, Has = false, Count = 0
Get(HOME) = "" false, Has = false, Count = 0
Keys: [DB_HOST DB_PORT]
Pairs: DB_HOST=localhost
Pairs: DB_PORT=5432
Pairs: DB_HOST=db.internal
Map: 2 keys, DB_HOST = db.internal
duplicate: DB_HOST is set 2 times
== Existed, Mode, Path
existing: true -rw-r----- .env.shared
missing:  false -rw------- .env.missing
parsed:   false -rw------- ""
```
