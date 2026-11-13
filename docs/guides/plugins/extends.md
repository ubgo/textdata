# extends plugin

`github.com/ubgo/textdata/plugins/extends` loads the files a JSONC document names in `"$extends"`, so one config can build on a shared base: `{"$extends": "../base.jsonc", "port": 8080}`. It is a `plugin.Includer`. Without an includer installed, `$extends` is an ordinary key and reading a file never opens another one; installing one is the opt-in. Use the disk includer for files on disk and the `fs.FS` includer for configs embedded in the binary or served from any other file system.

## Before you start

```sh
go get github.com/ubgo/textdata/plugins/extends@latest
go get github.com/ubgo/textdata/jsonc@latest
```

`$extends` is a JSONC feature today; dotenv has no include syntax, though the includer itself knows nothing about formats and will serve any later format that adds one. The runnable program for this page is [`examples/plugins/extends-fs`](../../../examples/plugins/extends-fs). Its tree:

```text
base.jsonc          {"port": 80, "db": {"host": "db.internal", "pool": 5}}
conf/team.jsonc     {"$extends": "../base.jsonc", "db": {"pool": 10}}
conf/app.jsonc      {"$extends": "team.jsonc", "port": 8080}
conf/escape.jsonc   {"$extends": "../../etc/passwd"}
conf/loop.jsonc     {"$extends": "loop.jsonc"}
```

## Step 1 — Open a document with the fs.FS includer

`extends.NewFS(fsys)` reads from any `fs.FS`: an `embed.FS`, `os.DirFS`, or the `testing/fstest.MapFS` used here. Locations are slash-separated FS paths. The document's own location (from `jsonc.Open`, or `jsonc.WithFileName` for parsed bytes) is the folder its relative references resolve from:

<!-- ds:block id=extends-openfs-9ab42gm6 -->
**openFS** · [`examples/plugins/extends-fs/main.go:42-48`](../../../examples/plugins/extends-fs/main.go#L42-L48)

```go
func openFS(name string) (*jsonc.Doc, error) {
	data, err := layout.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return jsonc.Parse(data, jsonc.WithFileName(name), jsonc.WithPlugin(extends.NewFS(layout)))
}
```
<!-- /ds:block hash=3a5557 -->

## Step 2 — Merge the chain

`Extended()` loads every base named by the root `$extends` (a string or an array of strings), each resolved from the folder of the file that names it, deep-merges them in order, and puts the document on top: objects merge by key, any other value replaces. A base may extend others. The result is a new document for reading; the original keeps its `$extends` line, and saving it never bakes the bases in. `Extends()` lists the references without loading anything.

<!-- ds:block id=extends-mergefromfs-6whqb5u8 -->
**mergeFromFS** · [`examples/plugins/extends-fs/main.go:54-78`](../../../examples/plugins/extends-fs/main.go#L54-L78)

```go
func mergeFromFS(w io.Writer) error {
	doc, err := openFS(appPath)
	if err != nil {
		return err
	}
	merged, err := doc.Extended()
	if err != nil {
		return err
	}
	var cfg struct {
		Port int `json:"port"`
		DB   struct {
			Host string `json:"host"`
			Pool int    `json:"pool"`
		} `json:"db"`
	}
	if err := merged.Decode(&cfg); err != nil {
		return err
	}
	refs, _ := doc.Extends()
	fmt.Fprintf(w, "extends:     %v\n", refs)
	fmt.Fprintf(w, "merged:      port=%d db.host=%s db.pool=%d\n", cfg.Port, cfg.DB.Host, cfg.DB.Pool)
	fmt.Fprintf(w, "app intact:  %t\n", string(doc.Render()) == string(layout[appPath].Data))
	return nil
}
```
<!-- /ds:block hash=5a8457 -->

## Step 3 — Handle refusals

Each failure is a named error, wrapped with the `file:line:col` of the `$extends` value: climbing above the FS root is `plugin.ErrDenied`, a file that extends itself (directly or through others) is `jsonc.ErrExtendsCycle`, a chain longer than `jsonc.MaxExtendsDepth` (32) is `jsonc.ErrExtendsDepth`, and `$extends` with no includer installed is `jsonc.ErrNoIncluder`:

<!-- ds:block id=extends-refusals-5a7q3uxx -->
**refusals** · [`examples/plugins/extends-fs/main.go:84-106`](../../../examples/plugins/extends-fs/main.go#L84-L106)

```go
func refusals(w io.Writer) error {
	escape, err := openFS(escapePath)
	if err != nil {
		return err
	}
	_, err = escape.Extended()
	fmt.Fprintf(w, "escape:      denied=%t\n", errors.Is(err, plugin.ErrDenied))

	loop, err := openFS(loopPath)
	if err != nil {
		return err
	}
	_, err = loop.Extended()
	fmt.Fprintf(w, "loop:        cycle=%t\n", errors.Is(err, jsonc.ErrExtendsCycle))

	bare, err := jsonc.Parse(layout[appPath].Data)
	if err != nil {
		return err
	}
	_, err = bare.Extended()
	fmt.Fprintf(w, "no includer: %t\n", errors.Is(err, jsonc.ErrNoIncluder))
	return nil
}
```
<!-- /ds:block hash=ef1846 -->

## Step 4 — Read from disk, confined

`extends.New()` reads the local disk. With no options it follows any relative or absolute reference, so give it `WithRoot` when the documents are not fully yours: every target must then lie under that folder, checked on the real path (symlinks resolved) and opened through an `os.Root`. `AllowAbsolute(false)` also refuses absolute references. `jsonc.Read` merges bases automatically once an includer is installed:

<!-- ds:block id=extends-readfromdisk-pgp6k8xu -->
**readFromDisk** · [`examples/plugins/extends-fs/main.go:112-131`](../../../examples/plugins/extends-fs/main.go#L112-L131)

```go
func readFromDisk(w io.Writer, root string) error {
	for name, f := range layout {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.Data, 0o600); err != nil {
			return err
		}
	}
	inc := extends.New(extends.WithRoot(root), extends.AllowAbsolute(false))
	m, err := jsonc.Read(filepath.Join(root, filepath.FromSlash(appPath)), jsonc.WithPlugin(inc))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "disk read:   port=%v db=%v\n", m["port"], m["db"])
	_, err = jsonc.Read(filepath.Join(root, filepath.FromSlash(escapePath)), jsonc.WithPlugin(inc))
	fmt.Fprintf(w, "disk escape: denied=%t\n", errors.Is(err, plugin.ErrDenied))
	return nil
}
```
<!-- /ds:block hash=ed87e3 -->

| Option | Applies to | Default | Effect |
|---|---|---|---|
| `WithRoot(dir)` | `New` | none (no confinement) | every target under `dir`; outside is `plugin.ErrDenied`; ignored by `NewFS` |
| `AllowAbsolute(b)` | `New` | `true` | `false` refuses `/etc/app/base.jsonc` with `plugin.ErrDenied`; `NewFS` always refuses |
| `WithMaxSize(n)` | both | 10 MiB (`DefaultMaxSize`) | larger targets fail with `extends.ErrTooLarge`; `n <= 0` restores the default |

## Try it

```sh
cd examples/plugins/extends-fs && go run .
```

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

## Pitfalls

- **The disk includer is unconfined by default.** `extends.New()` with no `WithRoot` follows `../../anything` and absolute paths. That suits your own repo; for documents from anyone else, set `WithRoot` and `AllowAbsolute(false)` (`TestDiskRootConfinement`, `TestDiskRootConfinementSymlinks`).
- **`WithRoot` with a relative dir is relative to the working directory at `Include` time**, unlike the file plugin's root, which is fixed against the document folder in `New`. Pass an absolute path.
- **A parsed document needs a location.** Bytes passed to `jsonc.Parse` without `WithFileName` resolve their references from the working directory (disk) or the FS root (`NewFS`) (`TestDiskEmptyFromIsWorkingDirectory`).
- **Arrays replace, they do not concatenate.** Only objects merge key by key; an array in the outer file replaces the base's array whole.
- **Expansion is separate.** Call `Expanded()` on the merged result, so `${VAR}` in a base reads the same variables as the document; `jsonc.Read` with `WithExpand()` does both in that order.
- **Only regular files are read.** A directory, device or FIFO fails with `extends.ErrNotRegular`, checked before the open so a named pipe cannot block (`TestDiskFIFODoesNotBlock`).
- **Missing targets wrap `plugin.ErrNotFound` and `fs.ErrNotExist`.** With several includers installed, a not-found passes to the next one, so a disk includer can back an embedded one.

## Related

- [Value plugins overview](overview.md), [file plugin](file.md) (one value from a file rather than a whole base)
- Package reference: [plugins/extends README](../../../plugins/extends/README.md); format side: [jsonc README § extends](../../../jsonc/README.md#extends)
- Example: [`examples/plugins/extends-fs`](../../../examples/plugins/extends-fs); cross-format test: [`integration/extends`](../../../integration/extends)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugins/extends](https://pkg.go.dev/github.com/ubgo/textdata/plugins/extends)
