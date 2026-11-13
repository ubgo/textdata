# Layer files with $extends

A JSONC file can name base files in a root `"$extends"` member: `"$extends": "../base.jsonc"` or a list. `doc.Extended()` loads every base, deep-merges them in order, puts the file itself on top and drops `$extends`, the same way `tsconfig.json` works. Use it when several configs share defaults (per environment, per machine, per package) and you want each file to say what it builds on. The file on disk is never changed: `Render` and `Save` still write `$extends` as written.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
go get github.com/ubgo/textdata/plugins/extends@latest
```

`$extends` is an ordinary key until you install a `plugin.Includer`, so reading a file never opens another file unless you asked. The ready-made Includer is the [`plugins/extends`](../../../plugins/extends) module (Go 1.24 or newer): `extends.New(...)` reads the disk, `extends.NewFS(fsys)` reads an `fs.FS` (`embed.FS`, `os.DirFS`, `fstest.MapFS`). The code comes from [`examples/jsonc/extends`](../../../examples/jsonc/extends), which keeps its files in memory:

<!-- ds:block id=files-6kdabnqd -->
**files** · [`examples/jsonc/extends/main.go:29-44`](../../../examples/jsonc/extends/main.go#L29-L44)

```go
var files = fstest.MapFS{
	"defaults/logging.jsonc": {Data: []byte(`{"log": {"level": "warn", "format": "json"}}`)},
	"defaults/base.jsonc": {Data: []byte(`{
  "$extends": "logging.jsonc",
  "server": {"host": "0.0.0.0", "port": 8080},
}`)},
	appPath: {Data: []byte(`{
  // Shared defaults first, then this file on top
  "$extends": ["../defaults/base.jsonc"],
  "server": {"port": 9090},
  "log": {"level": "debug"},
}
`)},
	"loop/a.jsonc": {Data: []byte(`{"$extends": "b.jsonc"}`)},
	"loop/b.jsonc": {Data: []byte(`{"$extends": "a.jsonc"}`)},
}
```
<!-- /ds:block hash=55d22b -->

## Step 1 — Install an Includer

Parse the file with its name, so its references resolve from its own folder, and install the Includer with `WithPlugin`. `Open(path, jsonc.WithPlugin(extends.New()))` is the same for a file on disk.

<!-- ds:block id=openwithincluder-67anw3rh -->
**openWithIncluder** · [`examples/jsonc/extends/main.go:81-87`](../../../examples/jsonc/extends/main.go#L81-L87)

```go
func openWithIncluder(fsys fs.FS, name string) (*jsonc.Doc, error) {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return jsonc.Parse(src, jsonc.WithFileName(name), jsonc.WithPlugin(extends.NewFS(fsys)))
}
```
<!-- /ds:block hash=636217 -->

## Step 2 — Build the merged view

`Extends()` lists the references as written without loading anything. `Extended()` returns a new document: the bases merged in list order (each one first resolving its own `$extends`, relative to its own folder), then this file on top, with every `$extends` removed. The merge rules are [`Merge`](merge-clone.md)'s: objects merge by key, arrays and scalars replace.

<!-- ds:block id=extend-93jurhr8 -->
**extend** · [`examples/jsonc/extends/main.go:93-106`](../../../examples/jsonc/extends/main.go#L93-L106)

```go
func extend(w io.Writer, doc *jsonc.Doc) error {
	refs, err := doc.Extends()
	if err != nil {
		return err
	}
	cfg, err := doc.Extended()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "references:", refs)
	fmt.Fprintf(w, "merged view:\n%s\n", cfg.Render())
	fmt.Fprintln(w, "merged view has $extends:", cfg.Has(jsonc.ExtendsKey), "| original still has it:", doc.Has(jsonc.ExtendsKey))
	return nil
}
```
<!-- /ds:block hash=12b4e1 -->

Here `app.jsonc` extends `base.jsonc`, which extends `logging.jsonc`: `log.format` comes from logging, `server.host` from base, `server.port` and `log.level` from app.

## Step 3 — Expect errors, not silence

With `$extends` present and no Includer installed, `Extended` is `ErrNoIncluder`, so a missing plugin cannot quietly drop your bases.

<!-- ds:block id=withoutincluder-scxbxr8t -->
**withoutIncluder** · [`examples/jsonc/extends/main.go:112-120`](../../../examples/jsonc/extends/main.go#L112-L120)

```go
func withoutIncluder(w io.Writer) error {
	doc, err := jsonc.Parse(files[appPath].Data, jsonc.WithFileName(appPath))
	if err != nil {
		return err
	}
	_, err = doc.Extended()
	fmt.Fprintln(w, "no includer is ErrNoIncluder:", errors.Is(err, jsonc.ErrNoIncluder))
	return nil
}
```
<!-- /ds:block hash=d43198 -->

A loop is `ErrExtendsCycle` naming the whole chain, at the `file:line:col` of the `$extends` value that closes it:

<!-- ds:block id=detectcycle-spct5f3s -->
**detectCycle** · [`examples/jsonc/extends/main.go:125-134`](../../../examples/jsonc/extends/main.go#L125-L134)

```go
func detectCycle(w io.Writer) error {
	doc, err := openWithIncluder(files, "loop/a.jsonc")
	if err != nil {
		return err
	}
	_, err = doc.Extended()
	fmt.Fprintln(w, "cycle:", err)
	fmt.Fprintln(w, "is ErrExtendsCycle:", errors.Is(err, jsonc.ErrExtendsCycle))
	return nil
}
```
<!-- /ds:block hash=17443d -->

The other errors carry the same position: a reference the Includer cannot load (its error, naming the plugin), a base with syntax errors, a base whose root is not an object, a value that is not a string or array of strings (`ErrExtendsValue`), and a chain deeper than `MaxExtendsDepth` (32, `ErrExtendsDepth`).

## Step 4 — Read a layered file in one call

`jsonc.Read` applies `$extends` whenever an Includer is installed, before decoding (and before expanding, with `WithExpand()`). On disk, `extends.WithRoot(dir)` confines every base to one folder, checked on the real path so a symlink cannot lead out.

<!-- ds:block id=readondisk-5bukm7sy -->
**readOnDisk** · [`examples/jsonc/extends/main.go:141-157`](../../../examples/jsonc/extends/main.go#L141-L157)

```go
func readOnDisk(w io.Writer, dir string) error {
	for name, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.Data, 0o600); err != nil {
			return err
		}
	}
	m, err := jsonc.Read(filepath.Join(dir, filepath.FromSlash(appPath)), jsonc.WithPlugin(extends.New(extends.WithRoot(dir))))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Read:", m["server"], m["log"])
	return nil
}
```
<!-- /ds:block hash=6a79fa -->

## Try it

```sh
cd examples/jsonc/extends && go run .
```

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

## Pitfalls

- **No Includer, no layering.** Without `WithPlugin(extends.New())` (or `NewFS`), `Get("$extends")` just returns the string, `Read` decodes it as a key, and `Extended` is `ErrNoIncluder`.
- **References are relative to the file that holds them**, not to the working directory. A document parsed from bytes needs `WithFileName` for that; without a name the disk Includer resolves from the working directory and the FS Includer from the FS root.
- **The merged view is for reading, not saving.** It carries the original's path and options, so `cfg.Save()` would bake every base into your file. Save the original document instead; its `$extends` is untouched.
- **The merged view's layout is the first base's.** Values from later files are re-indented into it and lose comments inside them, and the comment above `$extends` goes away with the key (compare `merged view` in the output with `app.jsonc`). Read values from it; show people the original files.
- **Expand after extending.** `Extended` does not expand; call `cfg.Expanded()` so references in a base read the same variables as the file on top.
- **`NewFS` refuses absolute references and `../` above the FS root** (`plugin.ErrDenied`); `New` allows absolute references unless `AllowAbsolute(false)`, and each target is capped at 10 MiB (`WithMaxSize`).
- **Several Includers are tried in install order**; the first that does not answer `plugin.ErrNotFound` wins, so a disk Includer can fall back to an embedded one.
- **Only the root `$extends` counts.** A `$extends` key in a nested object is ordinary data.

## Related

- [Merge and clone](merge-clone.md) · [Expansion and plugins](expansion-and-plugins.md) · [Read and paths](read-and-paths.md)
- The plugin itself: [extends plugin guide](../plugins/extends.md), [`plugins/extends` README](../../../plugins/extends/README.md)
- Module README: [jsonc § `$extends`](../../../jsonc/README.md#extends)
- Example: [`examples/jsonc/extends`](../../../examples/jsonc/extends)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc), [pkg.go.dev/github.com/ubgo/textdata/plugins/extends](https://pkg.go.dev/github.com/ubgo/textdata/plugins/extends)
