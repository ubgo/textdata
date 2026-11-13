# atomic-write

Uses `atomicfile.Write` in a temp directory: a new file at 0600, an overwrite with `KeepMode`, a write through a dangling symlink (and `Resolve`), and an injected rename failure that leaves the old contents and no temp file.

Guide: [atomicfile](../../../docs/guides/base/atomicfile.md).

## Run

```sh
cd examples/base/atomic-write && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
new file:     new.env -rw------- "TOKEN=abc\n"
kept mode:    shared.env -rw-r--r-- "A=2\n"
resolve: real.env, link still a link: true
via link:     real.env -rw------- "VIA=link\n"
write error: atomicfile: rename temp file into place: disk full (is disk full: true)
temp file left behind: false
after failure: safe.env -rw------- "OLD=1\n"
```
