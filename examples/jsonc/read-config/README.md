# jsonc: read a config

Parses a commented JSONC config and reads it every way the `jsonc` package offers: the byte-exact round trip, `Strict()` for plain JSON, tolerant parsing with `file:line:col` errors, `ParseReader` and `Bytes`, the typed getters (`GetString`, `GetInt`, `GetBool`, `GetFloat`, `Get`), `Has` / `Keys` / `Len` / `Kind`, `Comment`, dotted paths and JSON Pointer, `Pos` / `KeyPos`, `JSON()`, `Decode` into a struct and the one-call `jsonc.Read`.

## Run it

```sh
cd examples/jsonc/read-config && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
round trip exact: true
name: billing port: 8080 debug: false ratio: 0.75
GetInt(ratio) is ErrType: true
Get(servers[1]): map[host:b.internal]
keys: [name port debug ratio servers a/b]
servers: array of 2 | has timeout: false
GetString(timeout) is ErrNotFound: true
comment of name: "Public name of the service"
comment of servers: "upstream servers, tried in order"
comment of port: ""
paths: a.internal a.internal true slash key
port value at app.jsonc:4:11, key at 4:3
JSON() parses under Strict(): 2 servers
decoded: billing on 8080 via b.internal
strict: true
strict error: data.json:1:12: trailing comma is not allowed in strict JSON
ParseReader, Bytes equals Render: true
syntax error: app.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)
still renders input: true
Set is ErrInvalid: true
Read: billing 8080
```

## Learn more

- Guides: [parse and strict](../../../docs/guides/jsonc/parse-and-strict.md), [read values and paths](../../../docs/guides/jsonc/read-and-paths.md)
- Module README: [jsonc](../../../jsonc/README.md)
