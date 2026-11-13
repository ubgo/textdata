# paths

Parses dotted paths and JSON Pointers with package `path`, builds paths in code, maps byte offsets to `line:col` with package `pos`, collects `file:line:col` errors in an `ErrorList`, and reads the position of a real jsonc syntax error with `pos.First`.

Guide: [pos-and-path](../../../docs/guides/base/pos-and-path.md).

## Run

```sh
cd examples/base/paths && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
dotted:  servers."eu.west"[0].port  pointer form /servers/eu.west/0/port
pointer: servers."eu.west".0.port  segment 2 index=true key=true
ParseAny("/a~1b/c") = 2 segments, String "a/b.c"
ParseAny("a.b") = 2 segments, String "a.b"
ParseAny("") = 0 segments, String ""
bad path: path: invalid syntax: "a[x]" at byte 1: index "x" is not a number (syntax=true)
built: plugins[2]."with.dot"  parent plugins[2]  last key "with.dot" ok=true  base still plugins
offset 14 -> 2:12 (line 2 col 12), lines=4, zero Pos prints -
empty list err is nil: true
list: config.jsonc:1:1: first problem (and 1 more error)
first: config.jsonc:1:1: first problem
jsonc: config.jsonc:3:3: missing comma before '"'
  at line 3 col 3: missing comma before '"'
```
