# jsonc: layer files with $extends

Layers three JSONC files with `"$extends"` (app extends base, base extends logging) read from an in-memory `fstest.MapFS` through `extends.NewFS` from [`plugins/extends`](../../../plugins/extends). Shows `Extends`, `Extended` (the original keeps `$extends`), `ErrNoIncluder` without a plugin, a cycle reported with its chain, and the one-call `jsonc.Read` on a temp folder with `extends.New(extends.WithRoot(dir))`.

## Run it

```sh
cd examples/jsonc/extends && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
references: [../defaults/base.jsonc]
merged view:
{"log": {"level": "debug", "format": "json"}, "server": {"host":"0.0.0.0","port":9090}}
merged view has $extends: false | original still has it: true
no includer is ErrNoIncluder: true
cycle: loop/a.jsonc:1:14: $extends "b.jsonc": loop/b.jsonc:1:14: $extends "a.jsonc" closes a cycle: loop/a.jsonc -> loop/b.jsonc -> loop/a.jsonc: jsonc: $extends cycle
is ErrExtendsCycle: true
Read: map[host:0.0.0.0 port:9090] map[format:json level:debug]
```

## Learn more

- Guide: [extends](../../../docs/guides/jsonc/extends.md)
- Module README: [jsonc § `$extends`](../../../jsonc/README.md#extends), [plugins/extends](../../../plugins/extends/README.md)
