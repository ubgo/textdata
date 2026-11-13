# extends from an fs.FS and from disk

Merges JSONC `$extends` bases with the extends includer plugin (`github.com/ubgo/textdata/plugins/extends`). The first half reads from an in-memory `testing/fstest.MapFS` through `extends.NewFS`; the same code serves an `embed.FS` or `os.DirFS`. The second half writes the same tree to a temporary folder and reads it with the disk includer, `extends.New`, confined to that folder.

It shows:

- A three-file chain (`app` → `team` → `base`), each reference relative to the file that writes it, deep-merged with the outer file on top.
- `Extended` returning a new document: the original still renders with its `$extends` line.
- The errors to branch on: `plugin.ErrDenied` for a reference that climbs out of the root, `jsonc.ErrExtendsCycle`, and `jsonc.ErrNoIncluder` when no includer is installed.
- `jsonc.Read` merging bases automatically once an includer is installed.

## Run

```sh
cd examples/plugins/extends-fs && go run .
```

Expected output:

```text
extends:     [team.jsonc]
merged:      port=8080 db.host=db.internal db.pool=10
app intact:  true
escape:      denied=true
loop:        cycle=true
no includer: true
disk read:   port=8080 db=map[host:db.internal pool:10]
disk escape: denied=true
```

Guide: [extends plugin](../../../docs/guides/plugins/extends.md).
