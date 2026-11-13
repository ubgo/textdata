# Install and read your first file

textdata reads and edits config files that people write by hand, `.env` and JSONC (JSON with comments) today, without losing a byte you did not change. This page gets both formats into a Go program in five minutes: install, read a file into a map in one call, then open it as a document so you can ask what it holds. Start here if you have never used textdata; the next page, [Edit and save](edit-and-save.md), changes the same files and writes them back.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
go get github.com/ubgo/textdata/jsonc@latest
```

You need Go 1.22 or later. Each format is its own module and needs only the small base module (`github.com/ubgo/textdata`), which `go get` pulls in for you; install only the formats you use. Neither module has third-party dependencies.

The runnable program for this page is [`examples/getting-started/first-read`](../../../examples/getting-started/first-read). It reads two files that sit next to it:

```sh
# .env
# Database settings for local development
DB_HOST=localhost
DB_PORT=5432

# Read with GetExpanded to resolve the reference
DATABASE_URL=postgres://${DB_HOST}:${DB_PORT}/app
export LOG_LEVEL="debug" # quoted, with an inline comment
```

```jsonc
// config.jsonc
{
  // Where the API listens.
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
  },
  /* Features switched on in this environment. */
  "features": ["search", "billing"],
}
```

## Step 1 — Read a .env file into a map

When all you want is the configuration, `dotenv.Read` opens, parses and expands the file in one call and returns `map[string]string`. References such as `${DB_HOST}` arrive resolved, the `export` prefix and quotes are gone, and inline comments are not part of the value. It never touches the process environment: what you do with the map is up to you.

<!-- ds:block id=readenvmap-mv7e44qp -->
**readEnvMap** · [`examples/getting-started/first-read/main.go:55-62`](../../../examples/getting-started/first-read/main.go#L55-L62)

```go
func readEnvMap(w io.Writer) error {
	values, err := dotenv.Read(envPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv.Read: DB_PORT=%s LOG_LEVEL=%s DATABASE_URL=%s\n", values["DB_PORT"], values["LOG_LEVEL"], values["DATABASE_URL"])
	return nil
}
```
<!-- /ds:block hash=148d76 -->

## Step 2 — Open the .env file as a document

`dotenv.Open` returns a `*dotenv.File` that keeps every line of the file. Use it when you want more than values: the key order, whether a key exists, the raw value as written (`Get`) next to the value with its references resolved (`GetExpanded`), and later edits that keep the comments. A missing file opens as an empty document, so `Open` also bootstraps a file that does not exist yet.

<!-- ds:block id=openenv-mf8nqgu4 -->
**openEnv** · [`examples/getting-started/first-read/main.go:68-80`](../../../examples/getting-started/first-read/main.go#L68-L80)

```go
func openEnv(w io.Writer) error {
	f, err := dotenv.Open(envPath)
	if err != nil {
		return err
	}
	raw, _ := f.Get("DATABASE_URL") // as written: what Save would write back
	url, _, err := f.GetExpanded("DATABASE_URL")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv.Open: keys=%v\n  Get=%s\n  GetExpanded=%s\n  has API_SECRET=%v\n", f.Keys(), raw, url, f.Has("API_SECRET"))
	return nil
}
```
<!-- /ds:block hash=a8081d -->

## Step 3 — Read a JSONC file into a map

`jsonc.Read` is the same shortcut for JSONC: comments and trailing commas are accepted and the result is what `encoding/json` would give you for the same data, `map[string]any`.

<!-- ds:block id=readjsoncmap-ap2395qz -->
**readJSONCMap** · [`examples/getting-started/first-read/main.go:87-94`](../../../examples/getting-started/first-read/main.go#L87-L94)

```go
func readJSONCMap(w io.Writer) error {
	m, err := jsonc.Read(configPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc.Read: server=%v\n", m["server"])
	return nil
}
```
<!-- /ds:block hash=f24266 -->

## Step 4 — Open the JSONC file, read by path, decode into a struct

`jsonc.Open` returns a `*jsonc.Doc`. Typed getters (`GetString`, `GetInt`, `GetFloat`, `GetBool`) read one value by path; a path is dotted (`server.port`, `features[0]`) or a JSON Pointer (`/server/port`). `Decode` fills a struct through `encoding/json`, so your usual `json` tags apply.

<!-- ds:block id=config-apr2nsth -->
**Config** · [`examples/getting-started/first-read/main.go:26-32`](../../../examples/getting-started/first-read/main.go#L26-L32)

```go
type Config struct {
	Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"server"`
	Features []string `json:"features"`
}
```
<!-- /ds:block hash=44786d -->

<!-- ds:block id=openjsonc-vp5aznqj -->
**openJSONC** · [`examples/getting-started/first-read/main.go:100-119`](../../../examples/getting-started/first-read/main.go#L100-L119)

```go
func openJSONC(w io.Writer) error {
	doc, err := jsonc.Open(configPath)
	if err != nil {
		return err
	}
	port, err := doc.GetInt("server.port")
	if err != nil {
		return err
	}
	first, err := doc.GetString("features[0]")
	if err != nil {
		return err
	}
	var cfg Config
	if err := doc.Decode(&cfg); err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc.Open: server.port=%d features[0]=%s decoded=%+v\n", port, first, cfg)
	return nil
}
```
<!-- /ds:block hash=b057a0 -->

## Try it

```sh
cd examples/getting-started/first-read && go run .
```

```text
dotenv.Read: DB_PORT=5432 LOG_LEVEL=debug DATABASE_URL=postgres://localhost:5432/app
dotenv.Open: keys=[DB_HOST DB_PORT DATABASE_URL LOG_LEVEL]
  Get=postgres://${DB_HOST}:${DB_PORT}/app
  GetExpanded=postgres://localhost:5432/app
  has API_SECRET=false
jsonc.Read: server=map[host:0.0.0.0 port:8080]
jsonc.Open: server.port=8080 features[0]=search decoded={Server:{Host:0.0.0.0 Port:8080} Features:[search billing]}
```

The program reads `app.env` (a dotenv file; the name keeps it from being mistaken for a real secrets file) and `config.jsonc` by relative path, so run it from its own folder.

## Pitfalls

- **`dotenv.Read` expands, `jsonc.Read` does not.** `dotenv.Read` is `Open` followed by `ExpandedMap`, so `${DB_HOST}` arrives resolved. `jsonc.Read` returns values as written unless you pass `jsonc.WithExpand()`; on a `*jsonc.Doc`, `Get` is always raw and `GetExpanded` always expands.
- **A missing file is an error for `Read`, not for `Open`.** `dotenv.Open` of a path that does not exist returns an empty document (check `Existed()`), because it is also how you start a new file. `dotenv.Read` and `jsonc.Read` are for consuming a file that should be there, and fail.
- **Never save an expanded value.** `Get` returns what is written in the file; `GetExpanded` returns what a consumer sees. If you copy `GetExpanded`'s result back with `Set`, the reference `${DB_HOST}` is replaced by a frozen copy of its target and later changes to `DB_HOST` no longer flow through.
- **Every dotenv value is a string.** `PORT=8443` is four characters; convert it yourself with `strconv`, or bind the map to a struct (see the dotenv README's typed configuration section). JSONC values keep their JSON types, so `GetInt` on `"8080"` (a string) is an error.
- **JSON5 is not JSONC.** Unquoted keys, single-quoted strings, hex numbers and `NaN` are reported as syntax errors; only `//` and `/* */` comments, trailing commas and a BOM are added to JSON. `jsonc.Strict()` rejects those too, for plain JSON.
- **Nothing reaches `os.Environ`.** Neither format sets environment variables. To run a program with a `.env` file's values, use `textdatactl dotenv run` (see [textdatactl](textdatactl.md)) or apply your own precedence over `File.Map()`.

## Related

- [Edit and save](edit-and-save.md): change these files and write them back with their comments.
- [textdatactl](textdatactl.md): the same reads and edits from the command line.
- [dotenv README § Reading](../../../dotenv/README.md#reading) and [jsonc README § Reading](../../../jsonc/README.md#reading), [§ Paths](../../../jsonc/README.md#paths).
- [Paths and positions](../base/pos-and-path.md): the path syntax every tree format uses.
- Example: [`examples/getting-started/first-read`](../../../examples/getting-started/first-read).
- API: [dotenv on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/dotenv), [jsonc on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/jsonc).
