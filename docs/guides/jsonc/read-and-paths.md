# Read values and paths

Once a JSONC file is parsed, every read takes a path and returns the raw value written in the file: typed getters for the common case, structure queries when you do not know the shape, the comment a person wrote above a value, plain JSON for a consumer that cannot take comments, a struct through `encoding/json`, and the `line:col` of any value for your own error messages. Reads never expand `${VAR}` references and never touch the environment; that is a separate, opt-in step ([expansion and plugins](expansion-and-plugins.md)).

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from [`examples/jsonc/read-config`](../../../examples/jsonc/read-config), which parses this file (see [parse and strict](parse-and-strict.md) for the parse step):

<!-- ds:block id=configsrc-38thfgz2 -->
**configSrc** · [`examples/jsonc/read-config/main.go:26-39`](../../../examples/jsonc/read-config/main.go#L26-L39)

```go
`{
  // Public name of the service
  "name": "billing",
  "port": 8080,
  "debug": false,
  "ratio": 0.75,
  /* upstream servers, tried in order */
  "servers": [
    {"host": "a.internal", "tags": {"x.y": true}},
    {"host": "b.internal"},
  ],
  "a/b": "slash key",
}
`
```
<!-- /ds:block hash=ccc9ea -->

## Step 1 — Read typed values

`GetString`, `GetBool`, `GetFloat` and `GetInt` return the value or an `ErrType` error when the value has another kind. `GetInt` is exact: a number with a fraction or out of the `int64` range is `ErrType`, never a rounded value. `Get` decodes any value the way `encoding/json` decodes into `any`: `map[string]any`, `[]any`, `string`, `float64`, `bool` or `nil`.

<!-- ds:block id=readvalues-cx6g68s8 -->
**readValues** · [`examples/jsonc/read-config/main.go:99-125`](../../../examples/jsonc/read-config/main.go#L99-L125)

```go
func readValues(w io.Writer, doc *jsonc.Doc) error {
	name, err := doc.GetString("name")
	if err != nil {
		return err
	}
	port, err := doc.GetInt("port")
	if err != nil {
		return err
	}
	debug, err := doc.GetBool("debug")
	if err != nil {
		return err
	}
	ratio, err := doc.GetFloat("ratio")
	if err != nil {
		return err
	}
	_, err = doc.GetInt("ratio")
	fmt.Fprintln(w, "name:", name, "port:", port, "debug:", debug, "ratio:", ratio)
	fmt.Fprintln(w, "GetInt(ratio) is ErrType:", errors.Is(err, jsonc.ErrType))
	val, err := doc.Get("servers[1]")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Get(servers[1]): %v\n", val)
	return nil
}
```
<!-- /ds:block hash=23aa39 -->

## Step 2 — Inspect the structure

When the shape is not fixed, ask first. `Has` never errors; `Keys` lists member names in document order (each once, even with duplicates); `Len` counts array elements or distinct keys; `Kind` returns one of `KindObject`, `KindArray`, `KindString`, `KindNumber`, `KindBool`, `KindNull`. The empty path `""` is the root. A missing path is `ErrNotFound` from every getter.

<!-- ds:block id=inspectstructure-73xc6dju -->
**inspectStructure** · [`examples/jsonc/read-config/main.go:131-149`](../../../examples/jsonc/read-config/main.go#L131-L149)

```go
func inspectStructure(w io.Writer, doc *jsonc.Doc) error {
	keys, err := doc.Keys("")
	if err != nil {
		return err
	}
	n, err := doc.Len("servers")
	if err != nil {
		return err
	}
	kind, err := doc.Kind("servers")
	if err != nil {
		return err
	}
	_, missing := doc.GetString("timeout")
	fmt.Fprintln(w, "keys:", keys)
	fmt.Fprintln(w, "servers:", kind, "of", n, "| has timeout:", doc.Has("timeout"))
	fmt.Fprintln(w, "GetString(timeout) is ErrNotFound:", errors.Is(missing, jsonc.ErrNotFound))
	return nil
}
```
<!-- /ds:block hash=10283b -->

## Step 3 — Read the comment of a value

Comments attach to values: the comment lines directly above a member, or else a comment after it on the same line. `Comment` returns the text without the `//` or `/* */` markers, several leading lines joined by `\n`, and `""` when there is none.

<!-- ds:block id=readcomments-66jq6c8d -->
**readComments** · [`examples/jsonc/read-config/main.go:155-164`](../../../examples/jsonc/read-config/main.go#L155-L164)

```go
func readComments(w io.Writer, doc *jsonc.Doc) error {
	for _, p := range []string{"name", "servers", "port"} {
		c, err := doc.Comment(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "comment of %s: %q\n", p, c)
	}
	return nil
}
```
<!-- /ds:block hash=8acdf5 -->

## Step 4 — Address values with paths

Paths come from the base [`path`](../../../README.md#path) package and take two spellings, picked by the first character:

| Spelling | Example | Notes |
|---|---|---|
| dotted | `servers[0].host` | quote a name holding `.`, `[`, `]`, `"` or `\`: `tags."x.y"`; a leading index addresses a root array: `[0].name` |
| JSON Pointer (RFC 6901) | `/servers/0/host` | used when the path starts with `/`; `~1` is `/` and `~0` is `~` in a key: `/a~1b` is the key `a/b` |
| root | `""` | the whole document |

<!-- ds:block id=readpaths-55tx8e6t -->
**readPaths** · [`examples/jsonc/read-config/main.go:170-189`](../../../examples/jsonc/read-config/main.go#L170-L189)

```go
func readPaths(w io.Writer, doc *jsonc.Doc) error {
	dotted, err := doc.GetString("servers[0].host")
	if err != nil {
		return err
	}
	pointer, err := doc.GetString("/servers/0/host")
	if err != nil {
		return err
	}
	quoted, err := doc.GetBool(`servers[0].tags."x.y"`)
	if err != nil {
		return err
	}
	slash, err := doc.GetString("/a~1b")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "paths:", dotted, pointer, quoted, slash)
	return nil
}
```
<!-- /ds:block hash=6756be -->

## Step 5 — Report positions

`Pos(p)` returns where the value starts and `KeyPos(p)` where its member name starts, as a base `pos.Pos` (1-based line, byte column). For an array element or the root, `KeyPos` is the value's position. Validators, lint, diffs and config loaders use them to print `file:line:col`; `FileName()` is the name to print with them. Both are safe to call from several goroutines on a document nobody is editing.

<!-- ds:block id=readpositions-q9u8htda -->
**readPositions** · [`examples/jsonc/read-config/main.go:195-206`](../../../examples/jsonc/read-config/main.go#L195-L206)

```go
func readPositions(w io.Writer, doc *jsonc.Doc) error {
	val, err := doc.Pos("port")
	if err != nil {
		return err
	}
	key, err := doc.KeyPos("port")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "port value at %s:%s, key at %s\n", doc.FileName(), val, key)
	return nil
}
```
<!-- /ds:block hash=ac63cc -->

## Step 6 — Hand plain JSON to another tool

`JSON()` drops comments, trailing commas and a BOM and keeps every other byte as written, so the result is valid for any JSON consumer (here it parses under `Strict()`).

<!-- ds:block id=toplainjson-fgubdyv8 -->
**toPlainJSON** · [`examples/jsonc/read-config/main.go:212-227`](../../../examples/jsonc/read-config/main.go#L212-L227)

```go
func toPlainJSON(w io.Writer, doc *jsonc.Doc) error {
	out, err := doc.JSON()
	if err != nil {
		return err
	}
	sub, err := jsonc.Parse(out, jsonc.Strict())
	if err != nil {
		return err
	}
	servers, err := sub.Len("servers")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "JSON() parses under Strict():", servers, "servers")
	return nil
}
```
<!-- /ds:block hash=f23455 -->

## Step 7 — Decode into a struct

`Decode(v)` runs `encoding/json` over the document, so struct tags and the usual rules apply (unknown keys are ignored, missing ones keep their zero value).

<!-- ds:block id=config-e43h99u8 -->
**Config** · [`examples/jsonc/read-config/main.go:233-239`](../../../examples/jsonc/read-config/main.go#L233-L239)

```go
type Config struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Servers []struct {
		Host string `json:"host"`
	} `json:"servers"`
}
```
<!-- /ds:block hash=3ed852 -->

<!-- ds:block id=decodestruct-xzsp4vpj -->
**decodeStruct** · [`examples/jsonc/read-config/main.go:244-251`](../../../examples/jsonc/read-config/main.go#L244-L251)

```go
func decodeStruct(w io.Writer, doc *jsonc.Doc) error {
	var cfg Config
	if err := doc.Decode(&cfg); err != nil {
		return err
	}
	fmt.Fprintf(w, "decoded: %s on %d via %s\n", cfg.Name, cfg.Port, cfg.Servers[1].Host)
	return nil
}
```
<!-- /ds:block hash=6b0d49 -->

## Step 8 — Read a file in one call

`jsonc.Read(path, opts...)` opens the file and decodes it into a `map[string]any`. With an Includer plugin installed it merges `$extends` bases first ([extends](extends.md)); with `WithExpand()` it expands references next ([expansion](expansion-and-plugins.md)). Unlike `Open`, `Read` fails on a missing file: the error names the path and wraps `fs.ErrNotExist`, so `errors.Is(err, fs.ErrNotExist)` tells a missing file from a broken one.

<!-- ds:block id=readfile-gm56cbjv -->
**readFile** · [`examples/jsonc/read-config/main.go:297-308`](../../../examples/jsonc/read-config/main.go#L297-L308)

```go
func readFile(w io.Writer, dir string) error {
	path := filepath.Join(dir, configName)
	if err := os.WriteFile(path, []byte(configSrc), 0o600); err != nil {
		return err
	}
	m, err := jsonc.Read(path)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Read:", m["name"], m["port"])
	return nil
}
```
<!-- /ds:block hash=d8e5f7 -->

## Try it

```sh
cd examples/jsonc/read-config && go run .
```

```text
round trip exact: true
name: billing port: 8080 debug: false ratio: 0.75
GetInt(ratio) is ErrType: true
Get(servers[1]): map[host:b.internal]
keys: [name port debug ratio servers a/b]
servers: array of 2 | has timeout: false
GetString(timeout) is ErrNotFound: true
comment of name: "Public name of the service"
comment of servers: "upstream servers, tried in order"
comment of port: ""
paths: a.internal a.internal true slash key
port value at app.jsonc:4:11, key at 4:3
JSON() parses under Strict(): 2 servers
decoded: billing on 8080 via b.internal
strict: true
strict error: data.json:1:12: trailing comma is not allowed in strict JSON
ParseReader, Bytes equals Render: true
syntax error: app.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)
still renders input: true
Set is ErrInvalid: true
Read: billing 8080
```

## Pitfalls

- **Numbers from `Get` and `Read` are `float64`.** That is what `encoding/json` does with `any`. Use `GetInt` for an exact integer, or `Decode` into a typed struct.
- **`GetInt` refuses `0.75` and `1e30`** with `ErrType` rather than rounding or overflowing.
- **A key with a dot needs quotes in a dotted path.** `tags.x.y` walks into `tags` → `x` → `y`; write `tags."x.y"` or the pointer `/tags/x.y`.
- **A path into the wrong kind is `ErrType`, not `ErrNotFound`.** `Keys("name")` on a string reports `"name" is a string, not an object` at the value's position.
- **Duplicate keys read the last member.** `Keys` and `Len` count a duplicated key once. Run `Lint` to find them.
- **Reads return raw text.** `"${HOST}"` reads back as `${HOST}`; call `GetExpanded` or `Expanded` to resolve it.
- **`Decode`, `JSON` and the getters refuse a document with syntax errors** (`ErrInvalid`); check the error from `Parse` first.

## Related

- [Parse and strict](parse-and-strict.md) · [Edit](edit.md) · [Expansion and plugins](expansion-and-plugins.md) · [Extends](extends.md)
- Module README: [jsonc § Reading](../../../jsonc/README.md#reading), [§ Paths](../../../jsonc/README.md#paths), [§ Positions](../../../jsonc/README.md#positions)
- Example: [`examples/jsonc/read-config`](../../../examples/jsonc/read-config)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
