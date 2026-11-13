# file plugin

`github.com/ubgo/textdata/plugins/file` replaces `${file:./key.pem}` with the contents of that file. Use it for values that already live in files: TLS keys and certificates, Docker and Kubernetes secrets mounted under `/run/secrets`, a token written by another tool. The config file keeps the reference; the expanded read gets the contents.

## Before you start

```sh
go get github.com/ubgo/textdata/plugins/file@latest
```

The package name is `fileplugin`. You need the folder relative refs should resolve against, normally the document's own folder. The runnable program for this page is [`examples/plugins/file-confined`](../../../examples/plugins/file-confined); it builds this layout in a temporary folder:

```text
<top>/shared.txt      outside the config folder
<top>/cfg/key.pem     next to the documents
<top>/cfg/.env        TLS_KEY=${file:./key.pem}  SHARED=${file:../shared.txt}
<top>/cfg/app.jsonc   {"tls": {"key": "${file:./key.pem}"}}
```

## Step 1 — Install it confined (the default)

`fileplugin.New(baseDir)` resolves relative refs against `baseDir`, made absolute once in `New` (a later `os.Chdir` changes nothing). Confinement is on by default and the root is `baseDir` itself: a ref that leaves it with `..`, an absolute path, or a symlink pointing outside is refused with `plugin.ErrDenied` before a byte is read. The read goes through an `os.Root`, so a symlink swapped in after the check cannot escape either.

<!-- ds:block id=file-readconfined-mx9js8hm -->
**readConfined** · [`examples/plugins/file-confined/main.go:68-91`](../../../examples/plugins/file-confined/main.go#L68-L91)

```go
func readConfined(w io.Writer, cfg string) error {
	p := fileplugin.New(cfg) // the root defaults to cfg itself
	env, err := dotenv.Open(filepath.Join(cfg, envFile), dotenv.WithPlugin(p))
	if err != nil {
		return err
	}
	key, _, err := env.GetExpanded("TLS_KEY")
	if err != nil {
		return err
	}
	_, _, escape := env.GetExpanded("SHARED")
	doc, err := jsonc.Open(filepath.Join(cfg, jsoncFile), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	jkey, err := doc.GetExpanded("tls.key")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv TLS_KEY: %s\n", key)
	fmt.Fprintf(w, "jsonc tls.key:  %v\n", jkey)
	fmt.Fprintf(w, "../shared.txt:  denied=%t\n", errors.Is(escape, plugin.ErrDenied))
	return nil
}
```
<!-- /ds:block hash=866bca -->

## Step 2 — Set limits

Every read is capped by a size limit, 1 MiB by default (config values are keys and certificates, not data sets). A bigger file fails with `fileplugin.ErrTooLarge`; it is never truncated. One trailing `\n` or `\r\n` is trimmed by default, because an editor-saved key ends in a newline the value almost never wants; `WithTrimNewline(false)` returns the exact bytes. `Read(ref)` applies the same rules without a document:

<!-- ds:block id=file-readwithlimits-es378e6n -->
**readWithLimits** · [`examples/plugins/file-confined/main.go:97-109`](../../../examples/plugins/file-confined/main.go#L97-L109)

```go
func readWithLimits(w io.Writer, cfg string) error {
	small := fileplugin.New(cfg, fileplugin.WithMaxSize(smallLimit))
	_, err := small.Read(keyFile)
	fmt.Fprintf(w, "over %d bytes:   too large=%t\n", smallLimit, errors.Is(err, fileplugin.ErrTooLarge))

	exact := fileplugin.New(cfg, fileplugin.WithTrimNewline(false))
	v, err := exact.Read(keyFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "untrimmed:      %s\n", strconv.Quote(v))
	return nil
}
```
<!-- /ds:block hash=e9a4cb -->

## Step 3 — Widen the root only as far as needed

`WithRoot(dir)` moves the boundary: a relative `dir` is taken against the base folder, so `WithRoot("..")` lets a document reach its parent and no further. Absolute refs stay refused while any root is set. `Unconfined()` removes the boundary entirely, absolute paths and outward symlinks included; use it only for documents you fully trust.

<!-- ds:block id=file-widenroot-45ec6u4b -->
**widenRoot** · [`examples/plugins/file-confined/main.go:115-134`](../../../examples/plugins/file-confined/main.go#L115-L134)

```go
func widenRoot(w io.Writer, cfg string) error {
	parent := fileplugin.New(cfg, fileplugin.WithRoot(".."))
	v, err := parent.Read("../" + sharedFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "WithRoot(..):   %s\n", v)

	abs := filepath.Join(filepath.Dir(cfg), sharedFile)
	_, err = fileplugin.New(cfg).Read(abs)
	fmt.Fprintf(w, "confined abs:   denied=%t\n", errors.Is(err, plugin.ErrDenied))

	free := fileplugin.New(cfg, fileplugin.Unconfined())
	v, err = free.Read(abs)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Unconfined abs: %s\n", v)
	return nil
}
```
<!-- /ds:block hash=6cea86 -->

| Option | Default | Effect |
|---|---|---|
| `WithRoot(dir)` | the base folder | confinement root; relative to the base folder; also undoes an earlier `Unconfined` |
| `Unconfined()` | off | no root: any readable path, absolute refs and symlinks included |
| `WithMaxSize(n)` | 1 MiB (`DefaultMaxSize`) | larger files fail with `ErrTooLarge`; `n <= 0` keeps the default |
| `WithTrimNewline(on)` | on | remove one trailing `\n` or `\r\n` |

## Try it

```sh
cd examples/plugins/file-confined && go run .
```

```text
dotenv TLS_KEY: pem-body
jsonc tls.key:  pem-body
../shared.txt:  denied=true
over 4 bytes:   too large=true
untrimmed:      "pem-body\n"
WithRoot(..):   shared-value
confined abs:   denied=true
Unconfined abs: shared-value
```

## Pitfalls

- **Refs are relative to the base folder given to `New`, not to the process's working directory.** Pass the document's folder (`filepath.Dir(path)`); `New("")` means the working directory at construction (`TestBaseFixedAtConstruction`).
- **Mounted secrets need a wider root.** `${file:/run/secrets/db}` is an absolute ref and is refused while confined. Either point the base or root at `/run/secrets` and write `${file:db}`, or use `Unconfined()` for a document only you write.
- **The last of `WithRoot` and `Unconfined` wins** (`TestOptionOrder`). `New(dir, Unconfined(), WithRoot(dir))` is confined again.
- **A root that does not exist is reported on every read**, not by `New`: the plugin is still built, and each `Resolve` returns the construction error.
- **Only regular files are read.** A directory, device, FIFO or socket fails with `fileplugin.ErrNotRegular`; the open file is checked again, so a file swapped for a FIFO after the check cannot block the read (`TestFIFORefused`).
- **Errors name the ref, not the absolute path**, and map to the base sentinels: missing is `plugin.ErrNotFound`, outside the root or no permission is `plugin.ErrDenied`.
- **A symlinked root is fine.** Confinement compares real paths, so a root reached through a symlink (macOS `/tmp`) still contains its own files (`TestSymlinkedRoot`).

## Related

- [Value plugins overview](overview.md), [extends plugin](extends.md) (merging whole files rather than reading one value)
- Package reference: [plugins/file README](../../../plugins/file/README.md)
- Example: [`examples/plugins/file-confined`](../../../examples/plugins/file-confined); cross-format test: [`integration/file`](../../../integration/file)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugins/file](https://pkg.go.dev/github.com/ubgo/textdata/plugins/file)
