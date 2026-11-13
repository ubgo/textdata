# jsonc diff

Compares two JSONC configs by meaning with `github.com/ubgo/textdata/jsonc/diff`: which values were added, removed or changed, at which path, and where each value starts in each file. Nothing is read from disk.

It shows:

- `diff.Diff` listing `Change{Kind, Path, Pointer, Old, New, OldPos, NewPos}` in a deterministic order.
- Arrays compared by index: inserting one host at the front shows as two changes and one addition.
- Grouping by the closed set `diff.Added`, `diff.Removed`, `diff.Changed`, and the dotted `Path` next to the JSON `Pointer` (a key with a dot is quoted in `Path`).
- Reordered keys, other comments and `1.0` vs `1` giving zero changes.
- `diff.Equal` on decoded values: numbers by value, big integers exactly, objects regardless of key order, arrays by position.

## Run

```sh
cd examples/jsonc-tools/diff && go run .
```

Expected output (pinned by `main_test.go` against `testdata/expected.txt`):

```text
== base -> prod
removed debug         true -> <nil> (base.jsonc 4:12, prod.jsonc -)
changed hosts[0]      a.example -> z.example (base.jsonc 6:13, prod.jsonc 4:13)
changed hosts[1]      b.example -> a.example (base.jsonc 6:26, prod.jsonc 4:26)
added   hosts[2]      <nil> -> b.example (base.jsonc -, prod.jsonc 4:39)
changed "log.level"   info -> warn (base.jsonc 7:16, prod.jsonc 5:16)
changed server.port   8080 -> 443 (base.jsonc 3:22, prod.jsonc 3:41)
added   server.tls    <nil> -> true (base.jsonc -, prod.jsonc 3:53)
== by kind
added: 2
  hosts[2]      /hosts/2
  server.tls    /server/tls
removed: 1
  debug         /debug
changed: 4
  hosts[0]      /hosts/0
  hosts[1]      /hosts/1
  "log.level"   /log.level
  server.port   /server/port
== base -> same meaning, new layout
changes: 0
== Equal
1 == 1.0: true
9007199254740993 == 9007199254740992: false
map[a:true b:<nil>] == map[a:true b:<nil>]: true
[a b] == [b a]: false
```

Guide: [Diff two JSONC files](../../../docs/guides/jsonc-tools/diff.md).
