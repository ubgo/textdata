# first-read

Reads the `app.env` (dotenv format) and `config.jsonc` files in this folder: first into maps with `dotenv.Read` and `jsonc.Read`, then as documents with `dotenv.Open` (raw `Get` next to `GetExpanded`) and `jsonc.Open` (typed getters by path, `Decode` into a struct).

Guide: [install-and-first-read](../../../docs/guides/getting-started/install-and-first-read.md).

## Run

```sh
cd examples/getting-started/first-read && go run .
go test .   # runs the program and checks its output
```

Run it from this folder: it opens the files by relative path.

## Expected output

```text
dotenv.Read: DB_PORT=5432 LOG_LEVEL=debug DATABASE_URL=postgres://localhost:5432/app
dotenv.Open: keys=[DB_HOST DB_PORT DATABASE_URL LOG_LEVEL]
  Get=postgres://${DB_HOST}:${DB_PORT}/app
  GetExpanded=postgres://localhost:5432/app
  has API_SECRET=false
jsonc.Read: server=map[host:0.0.0.0 port:8080]
jsonc.Open: server.port=8080 features[0]=search decoded={Server:{Host:0.0.0.0 Port:8080} Features:[search billing]}
```
