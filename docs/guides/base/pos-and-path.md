# Paths and positions: `path` and `pos`

Two small base packages that every tree format uses. `github.com/ubgo/textdata/path` parses the addresses you pass to `Get`, `Set` and friends: the dotted form `servers."eu.west"[0].port` and RFC 6901 JSON Pointer `/servers/eu.west/0/port`. `github.com/ubgo/textdata/pos` turns byte offsets into `line:col` and renders errors as `file:line:col: message`, so an error reads the same whichever format raised it. Use `path` when you build or inspect paths in code (a CLI argument, a diff report, a schema walker); use `pos` when you write a parser or want the line and column out of a format's error.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

Both packages are in the base module (standard library only). The last step also uses `github.com/ubgo/textdata/jsonc`. The runnable program is [`examples/base/paths`](../../../examples/base/paths). Flat formats such as dotenv address values by key and do not use `path`.

The dotted grammar:

```text
path    = [ segment *( "." segment | index ) ]
segment = name | quoted | index
name    = 1*( any byte except '.', '[', '"' )
quoted  = '"' *( char | '\"' | '\\' ) '"'
index   = "[" 1*digit "]"
```

The empty string is the root; a leading index (`[0].name`) addresses a root array.

## Step 1 — Parse dotted paths and JSON Pointers

`path.Parse` reads the dotted form, `path.ParsePointer` a JSON Pointer (`~1` is `/`, `~0` is `~`), and `path.ParseAny` picks by the first byte: `/` means pointer. A `Path` is a slice of `Segment`s; `String()` renders the canonical dotted form and `Pointer()` the pointer form. A pointer token made only of digits is `KindEither`: an index in an array, a member name in an object, decided by the document. Every parse error wraps `path.ErrSyntax` and says at which byte it failed.

<!-- ds:block id=parsepaths-dseftc6n -->
**parsePaths** · [`examples/base/paths/main.go:43-64`](../../../examples/base/paths/main.go#L43-L64)

```go
func parsePaths(w io.Writer) error {
	dotted, err := path.Parse(`servers."eu.west"[0].port`)
	if err != nil {
		return err
	}
	pointer, err := path.ParsePointer("/servers/eu.west/0/port")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotted:  %s  pointer form %s\n", dotted, dotted.Pointer())
	fmt.Fprintf(w, "pointer: %s  segment 2 index=%v key=%v\n", pointer, pointer[2].MatchesIndex(), pointer[2].MatchesKey())
	for _, in := range []string{"/a~1b/c", "a.b", ""} {
		p, err := path.ParseAny(in)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "ParseAny(%q) = %d segments, String %q\n", in, len(p), p.String())
	}
	_, err = path.Parse(`a[x]`)
	fmt.Fprintf(w, "bad path: %v (syntax=%v)\n", err, errors.Is(err, path.ErrSyntax))
	return nil
}
```
<!-- /ds:block hash=1228af -->

## Step 2 — Build paths in code

`path.Key` and `path.Index` make segments; `Append` returns a new path (the receiver is never changed, so a base path can be shared), and `Parent` splits off the last segment. `MustParse` panics on a bad path and is meant for constants in source code. `String` quotes any name that needs it, so a key containing a dot round-trips.

<!-- ds:block id=buildpaths-9ckr88zf -->
**buildPaths** · [`examples/base/paths/main.go:70-75`](../../../examples/base/paths/main.go#L70-L75)

```go
func buildPaths(w io.Writer) {
	base := path.MustParse("plugins")
	item := base.Append(path.Index(2), path.Key("with.dot"))
	parent, last, ok := item.Parent()
	fmt.Fprintf(w, "built: %s  parent %s  last key %q ok=%v  base still %s\n", item, parent, last.Key, ok, base)
}
```
<!-- /ds:block hash=a2fdc4 -->

## Step 3 — Map byte offsets to line and column

Build a `pos.Index` once per source with `pos.NewIndex`; `Pos(offset)` is then a binary search. Lines and columns are 1-based and the column counts bytes, which is what editors that jump to `line:col` by byte expect and stays exact for invalid UTF-8. A `\r` before `\n` belongs to the line, so a CRLF file reports the same line numbers as an LF file. The zero `Pos` means "no position" and prints `-`.

<!-- ds:block id=positions-udy9nhr3 -->
**positions** · [`examples/base/paths/main.go:81-86`](../../../examples/base/paths/main.go#L81-L86)

```go
func positions(w io.Writer) {
	src := []byte("{\r\n  \"port\": 80x\r\n}\r\n")
	idx := pos.NewIndex(src)
	p := idx.Pos(14) // the 'x'
	fmt.Fprintf(w, "offset 14 -> %s (line %d col %d), lines=%d, zero Pos prints %s\n", p, p.Line, p.Col, idx.Lines(), pos.Pos{})
}
```
<!-- /ds:block hash=1dd4af -->

## Step 4 — Collect `file:line:col` errors

`pos.Errorf(file, p, format, args…)` builds a `*pos.Error`. A parser that reports every problem collects them in a `pos.ErrorList`: `Add` in any order, `Sort` by file then offset, and end with `return list.Err()`, which is nil for an empty list (no typed-nil error). `pos.First(err)` digs the first `*pos.Error` out of an error or a list.

<!-- ds:block id=errorlists-4uyzaxz7 -->
**errorLists** · [`examples/base/paths/main.go:92-100`](../../../examples/base/paths/main.go#L92-L100)

```go
func errorLists(w io.Writer) {
	idx := pos.NewIndex([]byte("a\nb\nc\n"))
	var list pos.ErrorList
	fmt.Fprintf(w, "empty list err is nil: %v\n", list.Err() == nil)
	list.Add(pos.Errorf(configName, idx.Pos(4), "second problem"))
	list.Add(pos.Errorf(configName, idx.Pos(0), "first problem"))
	list.Sort()
	fmt.Fprintf(w, "list: %v\nfirst: %v\n", list.Err(), pos.First(list.Err()))
}
```
<!-- /ds:block hash=275122 -->

## Step 5 — Read the position of a real format error

Formats return these types, so the same call works on a jsonc syntax error (or a dotenv one). `jsonc.WithFileName` sets the name shown in front of the position.

<!-- ds:block id=jsoncerrorposition-xdqrn438 -->
**jsoncErrorPosition** · [`examples/base/paths/main.go:106-114`](../../../examples/base/paths/main.go#L106-L114)

```go
func jsoncErrorPosition(w io.Writer) error {
	_, err := jsonc.Parse([]byte("{\n  \"a\": 1\n  \"b\": 2\n}\n"), jsonc.WithFileName(configName))
	first := pos.First(err)
	if first == nil {
		return fmt.Errorf("expected a positioned error, got %v", err)
	}
	fmt.Fprintf(w, "jsonc: %v\n  at line %d col %d: %s\n", err, first.Pos.Line, first.Pos.Col, first.Msg)
	return nil
}
```
<!-- /ds:block hash=386221 -->

## Try it

```sh
cd examples/base/paths && go run .
```

```text
dotted:  servers."eu.west"[0].port  pointer form /servers/eu.west/0/port
pointer: servers."eu.west".0.port  segment 2 index=true key=true
ParseAny("/a~1b/c") = 2 segments, String "a/b.c"
ParseAny("a.b") = 2 segments, String "a.b"
ParseAny("") = 0 segments, String ""
bad path: path: invalid syntax: "a[x]" at byte 1: index "x" is not a number (syntax=true)
built: plugins[2]."with.dot"  parent plugins[2]  last key "with.dot" ok=true  base still plugins
offset 14 -> 2:12 (line 2 col 12), lines=4, zero Pos prints -
empty list err is nil: true
list: config.jsonc:1:1: first problem (and 1 more error)
first: config.jsonc:1:1: first problem
jsonc: config.jsonc:3:3: missing comma before '"'
  at line 3 col 3: missing comma before '"'
```

## Pitfalls

- **A dot in a key needs quotes in the dotted form.** `servers.eu.west` is three segments; write `servers."eu.west"`, or use the pointer `/servers/eu.west`, where only `/` and `~` are special.
- **Pointer digits are not always indexes.** `/servers/0` reads `servers[0]` in an array but member `"0"` in an object. In dotted form such a segment renders as a key (`servers.0`), not `servers[0]`; `[0]` is always an index.
- **`ParseAny` treats a leading `/` as a pointer.** A dotted path whose first key starts with `/` must be quoted (`"/etc".x`); `Path.String` does this for you, so `ParseAny(p.String())` always reads back `p`.
- **Columns are bytes, not characters.** After a multi-byte character such as `é`, the column is larger than the number of characters you see. Editors that count runes will land a little to the left.
- **Offsets are clamped.** `Index.Pos` of an offset past the end returns the last position instead of failing, so a position is always printable; validate offsets yourself if out-of-range means a bug.
- **A trailing newline adds a line.** `Lines()` counts the empty line after a final `\n`, as editors show it: `"a\n"` has 2 lines.
- **`ErrorList.Error()` shows only the first error.** It prints the first message and how many more there are; range over the list (or use `errors.As` on each) to report them all.

## Related

- [jsonc README § Paths](../../../jsonc/README.md#paths) and [§ Positions](../../../jsonc/README.md#positions): `Doc.Pos`, `Doc.KeyPos` and every method that takes a path.
- [Install and read your first file](../getting-started/install-and-first-read.md): paths in use.
- [Root README § pos](../../../README.md#pos) and [§ path](../../../README.md#path).
- Example: [`examples/base/paths`](../../../examples/base/paths).
- API: [path on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/path), [pos on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/pos).
