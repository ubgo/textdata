# Atomic writes with `atomicfile`

Package `github.com/ubgo/textdata/atomicfile` replaces a file's contents so that a reader never sees a half-written file and a crash leaves the old contents intact. It writes a sibling temp file at `0600`, fsyncs it, sets the final mode, renames it over the target, and fsyncs the directory. It follows symlinks so the link survives, and it can keep the file's existing permissions. Every textdata format's `Save` uses it; call it yourself when your own tool writes a config file, a lock file or anything a person or another process may read while you write.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

`atomicfile` is in the base module (standard library only). The runnable program is [`examples/base/atomic-write`](../../../examples/base/atomic-write); it works in a temp directory.

The API is one function and an options struct for writing, and one lock for read-modify-write:

| Name | What |
|---|---|
| `Write(path, data, opts)` | replace `path` with `data` atomically |
| `Options.Mode` | permission of the written file; `0` means `DefaultMode` (`0600`) |
| `Options.KeepMode` | keep an existing file's permission bits instead of `Mode` |
| `Options.NoFollow` | replace a symlink itself instead of the file it names |
| `Options.TempSuffix` | temp file name suffix; empty means `DefaultTempSuffix` (`.textdata-tmp`) |
| `Options.Chmod`, `Options.Rename` | replace `os.Chmod` and `os.Rename`, for tests and sandboxes |
| `Resolve(path)` | follow symlinks to the file they name, accepting a dangling link |
| `Lock(ctx, path)` | take the cross-process lock guarding `path`; `Close` releases it |
| `ErrLockTimeout`, `ErrLockUnsupported` | the deadline passed while another holder kept the lock; no OS lock on this platform |

## Step 1 — Write a new file

With zero `Options`, a new file is created `0600`: owner read and write only, because the files textdata writes often hold secrets.

<!-- ds:block id=writenew-wxy7xh4t -->
**writeNew** · [`examples/base/atomic-write/main.go:61-67`](../../../examples/base/atomic-write/main.go#L61-L67)

```go
func writeNew(w io.Writer, dir string) error {
	p := filepath.Join(dir, newFile)
	if err := atomicfile.Write(p, []byte("TOKEN=abc\n"), atomicfile.Options{}); err != nil {
		return err
	}
	return report(w, "new file", p)
}
```
<!-- /ds:block hash=68befe -->

## Step 2 — Overwrite and keep the file's mode

`KeepMode: true` keeps the permissions an existing file already has. Without it, every write sets `Mode` (or `0600`), which would silently change a file its owner made `0644`. The mode is applied with an explicit chmod, so the process umask does not change it.

<!-- ds:block id=keepmode-6ehrb3xd -->
**keepMode** · [`examples/base/atomic-write/main.go:73-82`](../../../examples/base/atomic-write/main.go#L73-L82)

```go
func keepMode(w io.Writer, dir string) error {
	p := filepath.Join(dir, sharedFile)
	if err := atomicfile.Write(p, []byte("A=1\n"), atomicfile.Options{Mode: sharedMode}); err != nil {
		return err
	}
	if err := atomicfile.Write(p, []byte("A=2\n"), atomicfile.Options{KeepMode: true}); err != nil {
		return err
	}
	return report(w, "kept mode", p)
}
```
<!-- /ds:block hash=049c69 -->

## Step 3 — Write through a symlink

By default `Write` resolves symlinks and replaces the file at the end of the chain, so a link such as `.env -> ../shared/.env` keeps pointing where it did. `Resolve` is the same lookup on its own; unlike `filepath.EvalSymlinks` it accepts a dangling link (the target will be created) and a path that does not exist yet. Set `NoFollow` to replace the link itself with a regular file.

<!-- ds:block id=throughsymlink-ynna9fr8 -->
**throughSymlink** · [`examples/base/atomic-write/main.go:88-106`](../../../examples/base/atomic-write/main.go#L88-L106)

```go
func throughSymlink(w io.Writer, dir string) error {
	target, link := filepath.Join(dir, realFile), filepath.Join(dir, linkFile)
	if err := os.Symlink(realFile, link); err != nil { // dangling until the write
		return err
	}
	resolved, err := atomicfile.Resolve(link)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(link, []byte("VIA=link\n"), atomicfile.Options{}); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "resolve: %s, link still a link: %v\n", filepath.Base(resolved), info.Mode()&fs.ModeSymlink != 0)
	return report(w, "via link", target)
}
```
<!-- /ds:block hash=464edf -->

## Step 4 — See a failure leave the old file

The rename is the commit point: until it succeeds, the target holds its old bytes. Here `Options.Rename` is replaced to fail, standing in for a full disk or a crash; the error wraps the cause, the target is unchanged, and the temp file is removed.

<!-- ds:block id=failedrename-2zj7reuu -->
**failedRename** · [`examples/base/atomic-write/main.go:112-125`](../../../examples/base/atomic-write/main.go#L112-L125)

```go
func failedRename(w io.Writer, dir string) error {
	p := filepath.Join(dir, safeFile)
	if err := atomicfile.Write(p, []byte("OLD=1\n"), atomicfile.Options{}); err != nil {
		return err
	}
	errDisk := errors.New("disk full")
	err := atomicfile.Write(p, []byte("NEW=1\n"), atomicfile.Options{
		Rename: func(string, string) error { return errDisk },
	})
	fmt.Fprintf(w, "write error: %v (is disk full: %v)\n", err, errors.Is(err, errDisk))
	_, statErr := os.Stat(p + atomicfile.DefaultTempSuffix)
	fmt.Fprintf(w, "temp file left behind: %v\n", !errors.Is(statErr, fs.ErrNotExist))
	return report(w, "after failure", p)
}
```
<!-- /ds:block hash=94427a -->

## Step 5 — Lock around a read-modify-write

`Write` makes each write atomic, but it does not stop two processes from losing each other's edits: A reads the file, B reads it, A writes, B writes back a copy that never saw A's change. [`Lock(ctx, path)`](ds:block?id=lock-hhsrdwus) closes that gap. Take it before reading, release it after writing, and every other `Lock` caller waits in between. The dotenv and jsonc `Update` helpers do exactly this around open, edit and save ([dotenv saving](../dotenv/saving.md), [jsonc save](../jsonc/save.md)).

<!-- ds:block id=filelock-bump-h3m8q2vz -->
**bump** · [`examples/base/file-lock/main.go:80-93`](../../../examples/base/file-lock/main.go#L80-L93)

```go
func bump(ctx context.Context, path string) error {
	lk, err := atomicfile.Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lk.Close()

	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b))) // a missing file is 0
	return atomicfile.Write(path, []byte(strconv.Itoa(n+1)), atomicfile.Options{})
}
```
<!-- /ds:block hash=1ee4ee -->

What it locks and why:

- **A sidecar file, not `path`.** The lock is on `path` (symlinks resolved) plus `.lock`, created `0600` when missing. Locking `path` itself would not work, because `Write` replaces it by rename: a lock held on the old file does not exclude a process that opens the new one. The sidecar is never removed, since deleting it while another process waits on it would let a third process lock a fresh file.
- **The OS's own lock.** `flock` on Linux, macOS and the BSDs, `LockFileEx` on Windows, both through the standard library (no `golang.org/x/sys`). The OS releases it when `Close` runs, when the file closes, or when the process dies, even by `SIGKILL`, so a crash never leaves a stale lock ([pinned by a test that kills the holder](ds:block?id=testlockreleasedwhenholderdies-w6jdfhs9&assert=true)). Other platforms return `ErrLockUnsupported`.
- **Per call, not per process.** Two goroutines of one program exclude each other just as two processes do ([four processes, no lost increment](ds:block?id=testlockprocessesdonotloseedits-vtgj7tkj&assert=true)). It does not nest: a second `Lock` of the same path from the holder waits for the first `Close`.
- **Advisory.** It excludes only other `Lock` callers, never a plain open or write, so every writer of the file has to take it.

`ctx` bounds the wait. Without a deadline `Lock` waits as long as the lock is held. When the deadline passes the error wraps [`ErrLockTimeout`](ds:block?id=errlocktimeout-vstm9t8y) and `context.DeadlineExceeded` and names the lock file; a cancelled context wraps `context.Canceled` instead ([pinned](ds:block?id=testlocktimeout-ahj9bk9q&assert=true)).

<!-- ds:block id=filelock-timeout-p6t2w9kc -->
**timeout** · [`examples/base/file-lock/main.go:99-112`](../../../examples/base/file-lock/main.go#L99-L112)

```go
func timeout(w io.Writer, path string) error {
	held, err := atomicfile.Lock(context.Background(), path)
	if err != nil {
		return err
	}
	defer held.Close()

	ctx, cancel := context.WithTimeout(context.Background(), waitLimit)
	defer cancel()
	_, err = atomicfile.Lock(ctx, path)
	fmt.Fprintf(w, "lock file: %s\n", filepath.Base(held.Path()))
	fmt.Fprintf(w, "second Lock: timeout=%t\n", errors.Is(err, atomicfile.ErrLockTimeout))
	return nil
}
```
<!-- /ds:block hash=6eff1c -->

## Try it

```sh
cd examples/base/atomic-write && go run .
```

```text
new file:     new.env -rw------- "TOKEN=abc\n"
kept mode:    shared.env -rw-r--r-- "A=2\n"
resolve: real.env, link still a link: true
via link:     real.env -rw------- "VIA=link\n"
write error: atomicfile: rename temp file into place: disk full (is disk full: true)
temp file left behind: false
after failure: safe.env -rw------- "OLD=1\n"
```

```sh
cd examples/base/file-lock && go run .
```

```text
counter: 200 (want 200)
lock file: runs.count.lock
second Lock: timeout=true
```

## Pitfalls

- **Zero `Options` does not keep an existing file's mode.** It writes `0600` whether the file is new or not. Set `KeepMode` when overwriting a file someone else created; the formats' `Save` does this for you (a file that existed keeps its mode, a new one is `0600`).
- **The directory must be writable.** The temp file is created next to the target (`<target>.textdata-tmp`), because a rename is only atomic within one filesystem. A file you may write but whose directory you may not cannot be replaced this way.
- **A file's identity changes.** After the rename the path names a new inode: a process holding the old file open keeps reading the old contents, and hard links to the old file are not updated. Owner and group are those of the writing process.
- **A leftover temp file is reused.** If a crash leaves `<target>.textdata-tmp` behind, the next `Write` truncates and reuses it. Two processes writing the same file at once share that temp name; serialise writers with `Lock` (Step 5).
- **Directory fsync is best-effort.** On platforms that cannot sync a directory the rename is still atomic but may not survive a power loss; the error is ignored on purpose.
- **`Lock` needs the directory to exist** and to be writable the first time, to create the `.lock` sidecar. Leave the sidecar in place; it is a few bytes and deleting it can let two holders in.
- **Symlink chains are bounded.** `Resolve` (and so `Write`) gives up with "too many levels of symbolic links" on a loop.

## Related

- [Edit and save](../getting-started/edit-and-save.md): `Save` and `SaveAs` in dotenv and jsonc.
- [dotenv README § Saving safely](../../../dotenv/README.md#saving-safely), [jsonc README § Saving](../../../jsonc/README.md#saving).
- [Root README § atomicfile](../../../README.md#atomicfile).
- Examples: [`examples/base/atomic-write`](../../../examples/base/atomic-write), [`examples/base/file-lock`](../../../examples/base/file-lock).
- API: [atomicfile on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/atomicfile).
