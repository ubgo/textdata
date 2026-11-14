# Save a document safely

`doc.Save()` writes a JSONC document back to its file so that a reader never sees a half-written file and a crash leaves the old contents intact: a temp file next to the target, fsync, an explicit chmod, then a rename over the target. It keeps the file's permissions, writes through a symlink without replacing it, and lets `SaveGuard` plugins veto a save before anything touches the disk. Use it for every write of a config people also edit; together with the byte-preserving edits ([edit](edit.md)), the saved file differs from the original only where you changed it.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config), which works in a temp folder. Save goes through the base [`atomicfile`](../../../README.md#atomicfile) package. The file facts live on the document: `Path()` (where `Save` writes), `Existed()` (whether `Open` found the file) and `Mode()` (the permission `Save` writes with).

## Step 1 — Save edits in place

Open, edit, save. The example's file was created `0640`; after `Save` it is still `0640`, not a default. For the edits it saves, see [edit](edit.md); the file differs from the original only in those bytes.

<!-- ds:block id=saveinplace-fta7uvyk -->
**saveInPlace** · [`examples/jsonc/edit-config/main.go:146-156`](../../../examples/jsonc/edit-config/main.go#L146-L156)

```go
func saveInPlace(w io.Writer, doc *jsonc.Doc) error {
	if err := doc.Save(); err != nil {
		return err
	}
	info, err := os.Stat(doc.Path())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "saved %s, mode %v\n", filepath.Base(doc.Path()), info.Mode().Perm())
	return nil
}
```
<!-- /ds:block hash=a8604a -->

## Step 2 — Create the file on the first run

`Open` on a missing file is not an error: it returns an empty document with `Existed() == false`, and `Save` creates the file. A new file gets `0600` (owner only), because config files often hold secrets.

<!-- ds:block id=createonfirstrun-ncqexkt5 -->
**createOnFirstRun** · [`examples/jsonc/edit-config/main.go:163-181`](../../../examples/jsonc/edit-config/main.go#L163-L181)

```go
func createOnFirstRun(w io.Writer, dir string) error {
	doc, err := jsonc.Open(filepath.Join(dir, "new.jsonc"))
	if err != nil {
		return err
	}
	existed := doc.Existed()
	if err := doc.Set("port", 8080); err != nil {
		return err
	}
	if err := doc.Save(); err != nil {
		return err
	}
	info, err := os.Stat(doc.Path())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "first run: existed=%v, created %s mode %v\n%s", existed, filepath.Base(doc.Path()), info.Mode().Perm(), doc.Render())
	return nil
}
```
<!-- /ds:block hash=d5c5e9 -->

## Step 3 — Save through a symlink, or to another path

Dotfile managers keep configs as symlinks. `Save` resolves the link and replaces the file it points to, so the link stays a link. `SaveAs(path)` writes somewhere else (a backup, an export) and leaves `Path()` unchanged; an existing destination keeps its own mode, a new one gets the document's.

<!-- ds:block id=savethroughsymlink-dufagknt -->
**saveThroughSymlink** · [`examples/jsonc/edit-config/main.go:188-213`](../../../examples/jsonc/edit-config/main.go#L188-L213)

```go
func saveThroughSymlink(w io.Writer, dir string) error {
	link := filepath.Join(dir, "link.jsonc")
	if err := os.Symlink(fileName, link); err != nil {
		return err
	}
	doc, err := jsonc.Open(link)
	if err != nil {
		return err
	}
	if err := doc.Set("sync.remote", "upstream"); err != nil {
		return err
	}
	if err := doc.Save(); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	backup := filepath.Join(dir, "backup.jsonc")
	if err := doc.SaveAs(backup); err != nil {
		return err
	}
	fmt.Fprintln(w, "link still a symlink:", info.Mode()&fs.ModeSymlink != 0, "| Path after SaveAs:", filepath.Base(doc.Path()))
	return nil
}
```
<!-- /ds:block hash=b496ea -->

## Step 4 — Give a parsed document a path

A document from `Parse` or `ParseReader` has no path, so `Save` refuses with a message saying what to do. `SetPath` names the file; `SaveAs` also works without one.

<!-- ds:block id=saveparsed-v9mu2nvu -->
**saveParsed** · [`examples/jsonc/edit-config/main.go:219-231`](../../../examples/jsonc/edit-config/main.go#L219-L231)

```go
func saveParsed(w io.Writer, dir string) error {
	doc, err := jsonc.Parse([]byte("{\"port\": 8080}\n"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "no path:", doc.Save())
	doc.SetPath(filepath.Join(dir, "from-bytes.jsonc"))
	if err := doc.Save(); err != nil {
		return err
	}
	fmt.Fprintln(w, "after SetPath, saved", filepath.Base(doc.Path()))
	return nil
}
```
<!-- /ds:block hash=ba3ad7 -->

## Step 5 — Veto a save with a SaveGuard

A `plugin.SaveGuard` sees a read-only `plugin.Document` of exactly what would be written, on a clone, and can refuse. Every installed guard runs before any write; the first refusal aborts with the file untouched, wrapped in an error naming the plugin. This one refuses a literal password and allows a `${scheme:ref}` reference:

<!-- ds:block id=errplaintextsecret-9xt5krkp -->
**errPlaintextSecret** · [`examples/jsonc/edit-config/guard.go:22`](../../../examples/jsonc/edit-config/guard.go#L22-L22)

```go
var errPlaintextSecret = errors.New("plaintext secret; write a ${scheme:ref} reference instead")
```
<!-- /ds:block hash=79e771 -->

<!-- ds:block id=noplaintextsecret-tngjgu3d -->
**noPlaintextSecret** · [`examples/jsonc/edit-config/guard.go:29`](../../../examples/jsonc/edit-config/guard.go#L29-L29)

```go
type noPlaintextSecret struct{}
```
<!-- /ds:block hash=c0e168 -->

<!-- ds:block id=noplaintextsecret-name-xjvfd6gh -->
**noPlaintextSecret.Name** · [`examples/jsonc/edit-config/guard.go:32`](../../../examples/jsonc/edit-config/guard.go#L32-L32)

```go
func (noPlaintextSecret) Name() string { return "no-plaintext-secret" }
```
<!-- /ds:block hash=f10e9a -->

<!-- ds:block id=noplaintextsecret-guardsave-222q27ev -->
**noPlaintextSecret.GuardSave** · [`examples/jsonc/edit-config/guard.go:35-41`](../../../examples/jsonc/edit-config/guard.go#L35-L41)

```go
func (noPlaintextSecret) GuardSave(doc plugin.Document) error {
	v, ok := doc.ValueAt(secretPath)
	if ok && !strings.HasPrefix(v, "${") {
		return errPlaintextSecret
	}
	return nil
}
```
<!-- /ds:block hash=5872c2 -->

<!-- ds:block id=guardsave-t9bk2qdw -->
**guardSave** · [`examples/jsonc/edit-config/guard.go:47-65`](../../../examples/jsonc/edit-config/guard.go#L47-L65)

```go
func guardSave(w io.Writer, dir string) error {
	path := filepath.Join(dir, "guarded.jsonc")
	doc, err := jsonc.Open(path, jsonc.WithPlugin(noPlaintextSecret{}))
	if err != nil {
		return err
	}
	if err := doc.Set(secretPath, "hunter2"); err != nil {
		return err
	}
	refused := doc.Save()
	_, statErr := os.Stat(path)
	// The error names the file; print it without the scratch folder.
	fmt.Fprintln(w, "refused:", strings.ReplaceAll(fmt.Sprint(refused), dir+string(filepath.Separator), ""))
	fmt.Fprintln(w, "is errPlaintextSecret:", errors.Is(refused, errPlaintextSecret), "| file written:", !errors.Is(statErr, fs.ErrNotExist))
	if err := doc.Set(secretPath, "${gopass:db}"); err != nil {
		return err
	}
	return doc.Save()
}
```
<!-- /ds:block hash=174a8d -->

Guards also run for `SaveAs`. They are plugins like any other, installed with `WithPlugin`, and the same guard works for a dotenv file.

## Step 6 — Update a file several processes write

`Open`, `Set` and `Save` are safe for one writer. When several processes write the same config (a CLI, its daemon, two runs at once), each `Save` is atomic, but the later one writes back a copy read before the earlier one landed, and that edit is lost. [`jsonc.Update(ctx, path, fn, opts...)`](ds:block?id=update-v2hk4zns) closes the gap: it takes the cross-process lock for `path` ([`atomicfile.Lock`](../base/atomicfile.md#step-5--lock-around-a-read-modify-write)), opens the file, calls `fn` with the document, saves if `fn` changed it, and releases the lock. The runnable program is [`examples/jsonc/update-config`](../../../examples/jsonc/update-config): six workers each add their own entry and bump a shared counter at once, and every edit survives with the comments intact ([pinned](ds:block?id=testupdateconcurrentkeepseveryedit-eugnqggz&assert=true)).

<!-- ds:block id=jsonc-update-recordrun-v5k2m8xq -->
**recordRun** · [`examples/jsonc/update-config/main.go:85-98`](../../../examples/jsonc/update-config/main.go#L85-L98)

```go
func recordRun(path, agent string) error {
	ctx, cancel := context.WithTimeout(context.Background(), lockWait)
	defer cancel()
	return jsonc.Update(ctx, path, func(d *jsonc.Doc) error {
		n, err := d.GetInt("runs")
		if err != nil {
			return err
		}
		if err := d.Set("agents."+agent, map[string]any{"enabled": true}); err != nil {
			return err
		}
		return d.Set("runs", n+1)
	})
}
```
<!-- /ds:block hash=8d00bf -->

```text
runs: 6
agents: 6
comment kept: true
```

The rules: `ctx` bounds only the wait for the lock (its deadline gives an error wrapping `atomicfile.ErrLockTimeout`); a missing file opens empty and is created at `0600` only if `fn` adds something; a file with syntax errors is refused before `fn` runs; when `fn` returns an error nothing is written and that error is returned; when `fn` leaves the bytes unchanged nothing is written; SaveGuards run as for `Save`. The lock is advisory, so every writer has to use `Update` (or `atomicfile.Lock`). `fn` must not keep `d` or call `SetPath`.

## Try it

```sh
cd examples/jsonc/edit-config && go run .
```

```text
--- after edits ---
{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "branch": "main",
    // "interval": 60,
  },

  /* feature switches */
  "features": {
    "alpha": true,
  },
  "log": {
    "level": "info"
  },
}
restored interval: 60 | second Restore is ErrExists: true
saved app.jsonc, mode -rw-r-----
first run: existed=false, created new.jsonc mode -rw-------
{
  "port": 8080
}
link still a symlink: true | Path after SaveAs: link.jsonc
no path: jsonc: no path set; use Open, or SetPath before Save
after SetPath, saved from-bytes.jsonc
refused: jsonc: plugin "no-plaintext-secret": guard save guarded.jsonc: plaintext secret; write a ${scheme:ref} reference instead
is errPlaintextSecret: true | file written: false
```

The lines from `saved app.jsonc` on belong to this page; the first block is the [edit](edit.md) and [disable](disable-restore.md) steps.

## Pitfalls

- **A document with syntax errors saves byte for byte.** `Save` never rewrites or "fixes" what it was given; check `Parse` or `Open`'s error if you must not write a broken file.
- **`Save` writes the raw document.** Expanded values never reach the disk unless you `Set` them yourself; a merged `Extended()` view carries the original's path, so saving it bakes the bases in. Save the original.
- **Modes: kept for an existing file, `0600` for a new one.** The explicit chmod after writing means the umask cannot widen or narrow it; a temp file is never wider than `0600`, even briefly.
- **The temp file is `<target>.jsonc-tmp` in the same folder**, so the folder must be writable (not only the file), and a leftover after a crash sits next to its file.
- **A symlink is followed, not replaced.** To make the path a regular file again, remove the link first.
- **A guard sees the document, not your intent.** It gets the bytes about to be written through `plugin.Document` (`ValueAt` is raw, unexpanded); a guard error wraps your error, so match it with `errors.Is`.
- **`Save` alone does not coordinate writers.** Two processes saving the same file each write atomically, and the last rename wins; use `Update` (Step 6) when several writers share a file.
- **`Update` leaves a `.lock` file next to the target** (`config.jsonc.lock`): the lock's sidecar, created once and never removed, because deleting it while another process waits could let two writers in.

## Related

- [Edit](edit.md) · [Disable and restore](disable-restore.md) · [Expansion and plugins](expansion-and-plugins.md) · [Parse and strict](parse-and-strict.md)
- Base: [atomicfile guide](../base/atomicfile.md), [plugin guide](../base/plugin.md)
- Module README: [jsonc § Saving](../../../jsonc/README.md#saving)
- Examples: [`examples/jsonc/edit-config`](../../../examples/jsonc/edit-config), [`examples/jsonc/update-config`](../../../examples/jsonc/update-config)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
