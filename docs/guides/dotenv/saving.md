# Saving a .env file

Write an edited file back to disk atomically, with its permissions, line endings and trailing newline kept, or write a copy somewhere else. `.env` files usually hold credentials, so saving is built to never expose a half-written file, never widen its permissions, and never produce a diff you did not ask for.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

You need a `*dotenv.File` with the edits you want, from `dotenv.Open` (which sets `Path`) or `dotenv.Parse` (which does not); editing is covered in [editing.md](editing.md). Every snippet is a function from the runnable example [`examples/dotenv/saving`](../../../examples/dotenv/saving/), which works in a fresh temp directory and uses a small helper, `write`, to create a file with an exact mode.

`Save()` and `SaveAs(path)` share one write path, built on the base package [`atomicfile`](../../../README.md#atomicfile):

1. Every save guard plugin runs first, each on its own copy of the file. If one refuses, nothing is written and the destination is untouched ([plugins.md › Step 3](plugins.md#step-3--write-a-plugin-type)).
2. A symlinked destination is resolved to the file it points to, so the link survives the save.
3. The rendered content is written to a sibling temp file, [`<path>.dotenv-tmp`](ds:block?id=tempsuffix-vba443ms), created with mode `0600` and synced to disk. The file is never readable by others, even for a moment.
4. The temp file is `chmod`ed to the target mode. This explicit step is needed because the mode passed when a file is created is filtered by the process umask, so it cannot reproduce a mode on its own.
5. The temp file is [renamed over the destination](ds:block?id=writeto-eemankjv) and the directory is synced. A rename within one directory is atomic, so a reader sees either the old file or the new one, never half of one, and a crash before the rename leaves the original intact.
6. If any step fails, the temp file is removed, so no stray copy of your secrets is left beside the real file.

What is written is exactly [`Render()`](ds:block?id=render-c8njbs4p) (`Bytes()` returns the same as a `[]byte`): every untouched line keeps its original bytes, and `${references}` are written as references. Nothing in the write path expands a value.

## Step 1 — Save to the file you opened

`Open` a path (a missing file opens empty), edit, then [`Save`](ds:block?id=save-fcetxc66), which writes to `f.Path`. The directory listing afterwards shows that no temp file was left behind.

<!-- ds:block id=saveopened-b6tw2q8m -->
**saveOpened** · [`examples/dotenv/saving/main.go:77-100`](../../../examples/dotenv/saving/main.go#L77-L100)

```go
func saveOpened(w io.Writer, dir string) error {
	path, err := write(dir, ".env", "# app\nPORT=8080\n", 0o600)
	if err != nil {
		return err
	}
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	f.Set("PORT", "9090")
	if err := f.Save(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%q, files in dir: %d\n", data, len(entries))
	return nil
}
```
<!-- /ds:block hash=dee577 -->

A `File` made with `Parse` has no path, so `Save` returns an error instead of guessing. Set `f.Path` first, or use `SaveAs`:

<!-- ds:block id=savenopath-k3fx9c5r -->
**saveNoPath** · [`examples/dotenv/saving/main.go:106-119`](../../../examples/dotenv/saving/main.go#L106-L119)

```go
func saveNoPath(w io.Writer, dir string) error {
	f := dotenv.Parse("A=1\n")
	fmt.Fprintln(w, f.Save())
	f.Path = filepath.Join(dir, ".env.parsed")
	if err := f.Save(); err != nil {
		return err
	}
	info, err := os.Stat(f.Path)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "saved with mode", info.Mode().Perm())
	return nil
}
```
<!-- /ds:block hash=e7f556 -->

## Step 2 — Know which mode the file gets

| Case | Mode written |
|---|---|
| `Save` on a file that did not exist when opened (or a `Parse`d file with `Path` set) | [`0600`](ds:block?id=defaultfilemode-jwzcsmwz) |
| `Save` on a file that existed when opened | the mode it had when `Open` read it |
| `SaveAs` to a path that already exists | the destination's own current mode |
| `SaveAs` to a new path | this file's mode (`Mode()`) |

`f.Mode()` returns the mode `Save` will apply. The rules exist so that a save never widens a file someone locked down and never quietly loosens a file it is about to fill with secrets. In the example, a file opened with mode `0400` is saved back over itself and to a new path (both `0400`), then onto an existing `0644` file, which stays `0644`:

<!-- ds:block id=modes-q8vm4z2h -->
**modes** · [`examples/dotenv/saving/main.go:125-150`](../../../examples/dotenv/saving/main.go#L125-L150)

```go
func modes(w io.Writer, dir string) error {
	locked, err := write(dir, ".env.locked", "A=1\n", lockedMode)
	if err != nil {
		return err
	}
	shared, err := write(dir, ".env.shared", "", sharedMode)
	if err != nil {
		return err
	}
	f, err := dotenv.Open(locked)
	if err != nil {
		return err
	}
	f.Set("A", "2")
	for _, target := range []string{locked, filepath.Join(dir, ".env.copy"), shared} {
		if err := f.SaveAs(target); err != nil {
			return err
		}
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-12s %v\n", filepath.Base(target), info.Mode().Perm())
	}
	return nil
}
```
<!-- /ds:block hash=fb7886 -->

## Step 3 — Keep line endings and the trailing newline

Line terminators are kept per line, not per file. Lines the package writes (new keys, changed values, appended entries) [end in `\r\n` if the source contains any `\r\n` line](ds:block?id=eol-eefm88t3), and in `\n` otherwise, so appending to a Windows file does not introduce a stray `\n`. A file with mixed endings is not normalised; only the lines you change take the file's style.

A file that ended without a trailing newline still ends without one after edits, and a file that had one keeps it: adding or removing it would be a spurious diff on every save. An empty `Parse("")` counts as having a trailing newline, so a file you build from scratch ends with one.

<!-- ds:block id=lineendings-n5dk7w3x -->
**lineEndings** · [`examples/dotenv/saving/main.go:156-174`](../../../examples/dotenv/saving/main.go#L156-L174)

```go
func lineEndings(w io.Writer, _ string) error {
	crlf := dotenv.Parse("A=1\r\nB=2\r\n")
	crlf.Set("B", "3")
	crlf.Set("C", "4")
	fmt.Fprintf(w, "CRLF:       %q\n", crlf.Render())

	mixed := dotenv.Parse("A=1\nB=2\r\n")
	mixed.Set("A", "x")
	fmt.Fprintf(w, "mixed:      %q\n", mixed.Render())

	noNewline := dotenv.Parse("A=1")
	noNewline.Set("B", "2")
	fmt.Fprintf(w, "no newline: %q\n", noNewline.Render())

	scratch := dotenv.Parse("")
	scratch.Set("A", "1")
	fmt.Fprintf(w, "scratch:    %q\n", scratch.Render())
	return nil
}
```
<!-- /ds:block hash=b7abfa -->

## Step 4 — Write a copy with SaveAs

[`SaveAs(path)`](ds:block?id=saveas-tpakytbt) writes the file to another path and leaves `f.Path` unchanged. With [Clone](editing.md#step-7--derive-another-environment-with-clone), one loaded file can emit as many variants as you need, and the source file is never touched. An empty path is an error.

<!-- ds:block id=saveas-c2hr6p9t -->
**saveAs** · [`examples/dotenv/saving/main.go:180-197`](../../../examples/dotenv/saving/main.go#L180-L197)

```go
func saveAs(w io.Writer, dir string) error {
	f := dotenv.Parse("DOMAIN=staging.acme.io\n")
	f.Path = filepath.Join(dir, ".env.staging")
	for _, v := range []struct{ name, domain string }{{".env.prod", "acme.io"}, {".env.qa", "qa.acme.io"}} {
		f.Set("DOMAIN", v.domain)
		if err := f.SaveAs(filepath.Join(dir, v.name)); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, v.name))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %q\n", v.name, data)
	}
	fmt.Fprintln(w, "Path still:", filepath.Base(f.Path))
	fmt.Fprintln(w, "empty path:", f.SaveAs(""))
	return nil
}
```
<!-- /ds:block hash=c1641a -->

## Step 5 — Save through a symlink

A `.env` is often a symlink into a shared or private directory. `Save` writes through the link: the file it points to is replaced atomically, and the link stays a link. The temp file is created next to the target, not next to the link.

<!-- ds:block id=symlink-x4bj8m2f -->
**symlink** · [`examples/dotenv/saving/main.go:203-230`](../../../examples/dotenv/saving/main.go#L203-L230)

```go
func symlink(w io.Writer, dir string) error {
	target, err := write(dir, "real.env", "A=1\n", 0o600)
	if err != nil {
		return err
	}
	link := filepath.Join(dir, "link.env")
	if err := os.Symlink(target, link); err != nil {
		return err
	}
	f, err := dotenv.Open(link)
	if err != nil {
		return err
	}
	f.Set("A", "2")
	if err := f.Save(); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "still a link: %v, target now %q\n", info.Mode()&fs.ModeSymlink != 0, data)
	return nil
}
```
<!-- /ds:block hash=758166 -->

## Try it

```sh
cd examples/dotenv/saving && go run .
```

```text
== Save
"# app\nPORT=9090\n", files in dir: 1
== no path
dotenv: no path set; use Open, or set Path before Save
saved with mode -rw-------
== modes
.env.locked  -r--------
.env.copy    -r--------
.env.shared  -rw-r--r--
== line endings
CRLF:       "A=1\r\nB=3\r\nC=4\r\n"
mixed:      "A=x\r\nB=2\r\n"
no newline: "A=1\nB=2"
scratch:    "A=1\n"
== SaveAs
.env.prod: "DOMAIN=acme.io\n"
.env.qa: "DOMAIN=qa.acme.io\n"
Path still: .env.staging
empty path: dotenv: SaveAs needs a path
== symlink
still a link: true, target now "A=2\n"
```

The modes are Unix permission bits. More tested examples: `ExampleFile_Save`, `ExampleFile_Save_noPath`, `ExampleFile_Save_mode`, `ExampleFile_Save_lineEndings`, `ExampleFile_Save_trailingNewline` and `ExampleFile_SaveAs` in [`dotenv/example_edit_test.go`](../../../dotenv/example_edit_test.go); the symlink case is pinned by `TestSaveWritesThroughSymlink`.

## Pitfalls

- **A clone keeps the source's `Path`.** `Clone()` copies `Path`, so `clone.Save()` overwrites the file you cloned from. Use `SaveAs` for a clone.
- **`SaveAs` onto an existing file keeps that file's mode, not yours.** Overwriting a `0644` file with `SaveAs` leaves it `0644` even if the source is `0600`. Tighten the destination yourself if it holds secrets.
- **A read-only file is still replaced.** The save is a rename in the directory, so a `0400` file in a writable directory is overwritten (and keeps `0400`), as the example shows. File permissions do not protect a `.env` from `Save`; directory permissions do.
- **The mode comes from `Open` time.** `Save` reapplies the mode read when the file was opened. If someone `chmod`s the file between your `Open` and your `Save`, the save puts the old mode back.
- **The temp file needs a writable directory.** It is created next to the destination (next to the symlink's target for a link), so `Save` fails in a directory you can read but not write, even if the file itself is writable. Saving into a directory that does not exist fails too; it is not created for you.
- **The temp name is fixed.** Two processes saving the same path at the same moment use the same `<path>.dotenv-tmp` name. Serialise saves to one file yourself.
- **Editing a line in a mixed-ending file changes its terminator.** In a file with any `\r\n` line, a line you change is written with `\r\n` even if it had `\n` (`mixed` above). Untouched lines keep theirs.
- **An unterminated quote swallows what you append.** If the source's last value opens a quote that never closes, appended content is read back as part of that value. See [editing.md › Pitfalls](editing.md#pitfalls).
- **Modes are Unix modes.** Windows has no Unix permission bits, so the mode rules hold only on Unix-like systems.

## Related

- [editing.md](editing.md): `Set`, `Unset`, `Restore`, `Append`, `Insert*`, `Clone`.
- [reading.md](reading.md): `Open`, `Existed`, `Mode`, `Path`.
- [plugins.md](plugins.md): save guards, which can refuse a save.
- Module README: [Saving safely](../../../dotenv/README.md#saving-safely) and [Line endings and trailing newlines](../../../dotenv/README.md#line-endings-and-trailing-newlines).
- Base package: [root README › atomicfile](../../../README.md#atomicfile).
- Example: [`examples/dotenv/saving`](../../../examples/dotenv/saving/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv#File.Save](https://pkg.go.dev/github.com/ubgo/textdata/dotenv#File.Save).
