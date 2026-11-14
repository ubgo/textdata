# file-lock

`atomicfile.Lock` around a read-modify-write: eight workers bump one counter file 25 times each and no increment is lost, then a `Lock` with a deadline returns an error wrapping `atomicfile.ErrLockTimeout` while the lock is held. The workers are goroutines so the output is stable; the lock is the OS's own (`flock`, `LockFileEx`), so separate processes exclude each other the same way.

Guide: [atomicfile](../../../docs/guides/base/atomicfile.md).

## Run

```sh
cd examples/base/file-lock && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
counter: 200 (want 200)
lock file: runs.count.lock
second Lock: timeout=true
```
