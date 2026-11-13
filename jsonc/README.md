# jsonc

Read, edit and write JSON with comments (JSONC) in Go without disturbing a byte you did not ask to change. Comments, blank lines, key order, indentation, trailing commas and line endings survive every edit; plain JSON is the same package with `jsonc.Strict()`.

Module `github.com/ubgo/textdata/jsonc`, one format of [textdata](../README.md). Standard library plus the stdlib-only textdata base; no outside parser.

```sh
go get github.com/ubgo/textdata/jsonc
```

Status: released; the current tag is `jsonc/v0.1.4`.

**Contents:** [Quick tour](#quick-tour) · [Format](#format) · [Parsing](#parsing) · [Reading](#reading) · [Paths](#paths) · [Editing](#editing) · [Disable and Restore](#disable-and-restore) · [Positions](#positions) · [Saving](#saving) · [Expansion and plugins](#expansion-and-plugins) · [Guarantees](#guarantees) · [Conformance and tests](#conformance-and-tests) · [More in this module](#more-in-this-module)

## Quick tour

Edit a commented config: set a value, add a key next to a sibling, comment one out. Only the touched bytes change.

<!-- snippet: exempt — copied verbatim from jsonc/example_test.go (Example), which has no snippet region yet -->
```go
// Example edits a commented config: only the touched bytes change.
func Example() {
	src := `{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "interval": 30,
  },
}
```

## Format

- All of JSON (RFC 8259), plus `// line` and `/* block */` comments anywhere whitespace is allowed, trailing commas in objects and arrays, and an optional UTF-8 BOM. LF and CRLF are kept as found.
- Duplicate keys are parsed and kept; reading and editing use the last one, like `encoding/json`.
- JSON5 is not JSONC: unquoted keys, single quotes, hex numbers and `NaN` / `Infinity` are errors with a message that says so.
- `jsonc.Strict()` is plain JSON: comments, trailing commas and a BOM become errors too, and the parser accepts exactly what `encoding/json.Valid` accepts. There is no separate json module.

## Parsing

| Call | Does |
|---|---|
| `Parse(src []byte, opts...) (*Doc, error)` | parse bytes (copied) |
| `ParseReader(r io.Reader, opts...) (*Doc, error)` | read to the end and parse |
| `Open(path, opts...) (*Doc, error)` | read and parse a file; a missing file is not an error but an empty document that `Save` creates, so `Open → Set → Save` works on a first run |
| `Read(path, opts...) (map[string]any, error)` | the shortcut: open and decode into a map (with `WithExpand()`, references are expanded first); unlike `Open`, a missing file is an error wrapping `fs.ErrNotExist` |

Parsing is **tolerant**: `Parse` always returns a `*Doc`, even with syntax errors. The document then holds the tree up to the first error, `Render` returns the input byte for byte, and `Save` writes it unchanged; edits, `Decode` and `JSON` refuse with `ErrInvalid`. The error is a `pos.ErrorList` whose entries carry `file:line:col` (the name comes from `Open`, or from `WithFileName` for bytes); `doc.Err()` returns it again later.

<!-- snippet: exempt — copied verbatim from jsonc/example_test.go (ExampleParse_errors), which has no snippet region yet -->
```go
// ExampleParse_errors shows the file:line:col error for a JSON5-ism.
func ExampleParse_errors() {
	_, err := jsonc.Parse([]byte("{\n  port: 8080\n}"), jsonc.WithFileName("config.jsonc"))
	fmt.Println(err)
	// Output:
	// config.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)
}
```

Under `Strict()` the JSONC extensions are errors as well:

<!-- snippet: exempt — no Example test yet; verified by hand against the code on 2026-11-12 -->
```go
_, err := jsonc.Parse([]byte(`{"a": [1, 2,]}`), jsonc.Strict(), jsonc.WithFileName("data.json"))
fmt.Println(err)
// data.json:1:12: trailing comma is not allowed in strict JSON
```

## Reading

Every read takes a [path](#paths). Raw values only: reading never expands anything (see [Expansion](#expansion-and-plugins)).

| Method | Returns |
|---|---|
| `Get(p)` | the value decoded the way `encoding/json` decodes into `any`: `map[string]any`, `[]any`, `string`, `float64`, `bool` or `nil` |
| `GetString(p)` / `GetBool(p)` / `GetFloat(p)` | the typed value, or an `ErrType` error |
| `GetInt(p)` | an exact `int64`: a fraction or an out-of-range number is `ErrType`, never a rounded value |
| `Has(p)` | whether a value exists at the path |
| `Kind(p)` | `KindObject`, `KindArray`, `KindString`, `KindNumber`, `KindBool` or `KindNull` |
| `Keys(p)` / `Len(p)` | member names in document order (each once) / element or distinct-key count |
| `Comment(p)` | the comment attached to the value: its leading comment lines joined by `\n`, or else its trailing comment |
| `JSON()` | plain JSON: the document without comments, trailing commas and BOM, every other byte as written |
| `Decode(v)` | decode the whole document into `v` with `encoding/json` |

Missing paths are `ErrNotFound`. Comments attach to values: the comment lines directly above a member, or a comment after it on the same line.

## Paths

Paths come from the base [`path`](../README.md#path) package and take two spellings, chosen by the first character:

- dotted: `servers[0].host`, with quotes for names that contain `.`, `[`, `]`, `"` or `\` (`tags."x.y"`); a leading index addresses a root array (`[0].name`);
- JSON Pointer (RFC 6901), when the path starts with `/`: `/servers/0/host`, `/a~1b` for the key `a/b`.

The empty string is the root.

<!-- snippet: exempt — no Example test yet; verified by hand against the code on 2026-11-12 -->
```go
doc, _ := jsonc.Parse([]byte(`{"servers": [{"host": "a", "tags": {"x.y": true}}]}`))
h1, _ := doc.GetString("servers[0].host")
h2, _ := doc.GetString("/servers/0/host")
t, _ := doc.GetBool(`servers[0].tags."x.y"`)
fmt.Println(h1, h2, t)
// a a true
```

## Editing

Every edit is a splice of the source followed by a re-parse, so "only the touched bytes change" holds by construction. Values are encoded with `encoding/json`; a `jsonc.Raw` value is inserted verbatim (it must parse as one value; comments and layout inside it are kept).

| Method | Does |
|---|---|
| `Set(p, v)` | replace the value, or create it (and any missing parent objects) at the end of its object, in that object's indentation and comma style; array elements are not created by `Set` |
| `SetAfter(anchor, p, v)` / `SetBefore(anchor, p, v)` | as `Set`, but a new key lands right after / before the sibling `anchor`; an existing key is updated where it is |
| `Delete(p)` | remove the value with its key, its attached comments and its comma; deleting a key `Set` just created restores the original bytes |
| `Append(p, v)` / `Insert(p, i, v)` / `Remove(p, i)` | array elements |
| `SetComment(p, text)` / `RemoveComment(p)` | set the comment above a value (each line becomes `// `; a `/* */` block for a value sharing its line) / remove its leading and trailing comments |
| `Render()` | the document's bytes: exactly the input for an unedited document |

Edits refuse on a document with syntax errors (`ErrInvalid`), and anything that needs a comment refuses under `Strict()` (`ErrStrict`).

## Disable and Restore

`Disable(p)` turns an object member into `// ` comment lines, so it no longer counts but stays in the file; `Restore(p)` brings it back byte for byte. The member must sit on lines of its own, with its comma on its value's last line (`ErrLayout` otherwise). `Restore` returns `ErrNotFound` when no disabled member with that key exists in the parent object, and `ErrExists` when the key is live again. The [quick tour](#quick-tour) disables `sync.interval`.

## Positions

`Pos(p)` returns where the value at a path starts and `KeyPos(p)` where its member name starts (the value's position for array elements and the root), as a base [`pos.Pos`](../README.md#pos) with 1-based line and byte column. Validators, lint rules, diffs and config sources use them to report `file:line:col` for a path.

<!-- snippet: exempt — no Example test yet; verified by hand against the code on 2026-11-12 -->
```go
doc, _ := jsonc.Parse([]byte("{{\n  // port\n  \"port\": 8080\n}}"), jsonc.WithFileName("app.jsonc"))
p, _ := doc.Pos("port")
k, _ := doc.KeyPos("port")
fmt.Println(p, k)
// 3:11 3:3
```

## Saving

`Save()` writes the document to its path atomically through the base [`atomicfile`](../README.md#atomicfile) package: a sibling temp file at 0600, fsync, an explicit chmod, a rename over the target. It keeps the file's mode (a new file is 0600) and writes through a symlink, so the link survives. `SaveAs(path)` writes elsewhere and leaves `Path()` unchanged; an existing destination keeps its own mode, a new one gets the document's. `SetPath` changes where `Save` writes; `Path`, `Existed` and `Mode` report the file facts.

Every installed `SaveGuard` runs first, on a clone, and the first refusal aborts with the file untouched. A document with syntax errors is saved byte for byte: `Save` never rewrites what it was given.

## Expansion and plugins

Dynamic values are off unless the caller turns them on, and expansion never changes the document: `Expanded()` returns an expanded copy and `GetExpanded(p)` reads one path from it, while `Get`, `JSON`, `Decode`, `Render` and `Save` always see the raw text (`WithExpand()` only makes the one-call `Read` expand).

Inside **string values** (keys are never expanded), through the base [`expand`](../README.md#expand) engine:

- `$VAR`, `${VAR}` and the operator family `${VAR:-d}`, `${VAR-d}`, `${VAR:+a}`, `${VAR+a}`, `${VAR:?e}`, `${VAR?e}`, nested; `$$` is a literal dollar. Names resolve through the installed Lookupers only (`WithLookup(os.LookupEnv)` for the environment; the package never reads it on its own); an unknown name is `""`.
- `${scheme:ref}` goes to the `Resolver` plugin that owns the scheme (`${gopass:personal/db}`, `${file:./key.pem}`); a scheme nobody owns reads as an ordinary reference.
- `$(command)` runs only with `WithCommandSubstitution(true)` (`sh -c`) or an installed `CommandRunner`; a failing command is an error (`*expand.CommandError`, with the value's `file:line:col`).
- After expansion every string passes through the `ValueTransformer`s with its canonical dotted path.

<!-- snippet: exempt — copied verbatim from jsonc/example_test.go (ExampleDoc_Expanded), which has no snippet region yet -->
```go
// ExampleDoc_Expanded reads ${VAR} references through a lookup the caller
// chooses; the document itself keeps the references.
func ExampleDoc_Expanded() {
	doc, _ := jsonc.Parse([]byte(`{"url": "postgres://${HOST:-localhost}/app"}`),
		jsonc.WithLookup(func(name string) (string, bool) {
			return map[string]string{"HOST": "db.internal"}[name], name == "HOST"
		}))
	x, _ := doc.Expanded()
	url, _ := x.GetString("url")
	raw, _ := doc.GetString("url")
	fmt.Println(url)
	fmt.Println(raw)
	// Output:
	// postgres://db.internal/app
	// postgres://${HOST:-localhost}/app
}
```

`WithPlugin(ps...)` installs plugins written against the shared [`github.com/ubgo/textdata/plugin`](../README.md#plugin) package, so a plugin written once works for dotenv and jsonc alike. Capabilities are detected once, at install, and `Plugins()` reports what was detected. jsonc uses `Lookuper`, `Resolver`, `CommandRunner` (last installed wins), `ValueTransformer` (chained), `ExpandObserver`, `SaveGuard` (veto only, before `Save` / `SaveAs`) and `Validator` (run only by `Validate()`, never on read). A plugin that fails is an error naming it, never a silent empty value. `*Doc` implements `plugin.Document` (`FormatName` `"jsonc"`, `Location`, `ValuePaths` with the canonical path of every scalar and empty container, `ValueAt`, `Bytes`), the read-only view guards and validators receive.

<!-- snippet: exempt — no Example test yet; verified by hand against the code on 2026-11-12 -->
```go
type gopass struct{{}}

func (gopass) Name() string      {{ return "gopass" }}
func (gopass) Schemes() []string {{ return []string{{"gopass"}} }}
func (gopass) Resolve(scheme, ref string) (string, error) {{
	if ref == "personal/db" {{
		return "s3cret", nil
	}}
	return "", plugin.ErrNotFound
}}

doc, _ := jsonc.Parse([]byte(`{{"db": {{"password": "${{gopass:personal/db}}"}}}}`), jsonc.WithPlugin(gopass{{}}))
pw, _ := doc.GetExpanded("db.password") // "s3cret"
raw, _ := doc.GetString("db.password")  // "${{gopass:personal/db}}", which is what Save writes
fmt.Println(doc.Plugins()[0])           // gopass: Resolver
```

Ready-made value plugins live in their own modules: [`plugins/gopass`](../plugins/gopass), [`plugins/file`](../plugins/file), [`plugins/vault`](../plugins/vault), [`plugins/aws`](../plugins/aws), and [`plugins/extends`](../plugins/extends) for `$extends`.

## Guarantees

Pinned by fuzz and property tests, not by prose:

- `Parse(x).Render() == x`, byte for byte, for every input, valid or not.
- An edit changes only the bytes of what it touches; comments, blank lines, key order, indentation and line endings elsewhere survive.
- Reading never expands anything and never touches the process environment; a command runs only when the caller turned command substitution on.
- Errors carry `file:line:col`.
- Standard library plus the stdlib-only textdata base.

## Conformance and tests

- **JSONTestSuite** (checked in under `testdata/JSONTestSuite`, MIT): under `Strict()` the accept/reject verdict matches `encoding/json.Valid` on every file; in JSONC mode every `y_` file parses, an `n_` file is accepted only when it uses a JSONC extension (comment, trailing comma, BOM), and every accepted file's `JSON()` is valid JSON. `Render` returns every file byte for byte.
- **JSONC case set**: comments in every position, trailing commas, CRLF, BOM, comment-like strings, scalar roots, duplicate keys; each renders byte for byte and `JSON()` decodes to the expected value.
- **Error positions**: one pinned message and `file:line:col` per class of mistake, JSON5-isms included.
- **Edit properties** (base `texttest`): across a table of layouts, every edit leaves the bytes outside its window untouched.
- **Fuzz**: `FuzzParseRender` (round trip, never panics), `FuzzStrictMatchesEncodingJSON` (differential against `encoding/json`), `FuzzSetDelete` (`Set` then `Delete` returns the original bytes), `FuzzEditSequence` (random edit sequences: every edit either fails and leaves the bytes alone or leaves a valid document, and `Disable` then `Restore` is byte-identical). Crashers stay as seeds under `testdata/fuzz`.
- **Benchmarks** against `encoding/json` (`go test -run '^$' -bench . -benchmem`); `Decode` is about 1.5x `encoding/json.Unmarshal` on a 40 KB file.

```sh
go test -race ./...                        # from this directory
task fuzz -- ./jsonc FuzzParseRender 30s   # from the repository root
```

## More in this module

### Merge and Clone

`Merge(other)` is a deep merge for base + override files: objects merge by key; arrays and scalars replace whole; a new key is added in this document's layout with the other file's single-line leading comment; comments and untouched bytes are kept, and a second merge is a no-op. It refuses (`ErrInvalid` / `ErrType`) and leaves the document unchanged when either file has syntax errors or a root is not an object. `Clone()` is deep, options included.

### Lint

`Lint(doc, opts...)` reports duplicate keys, trailing commas (with `ForbidTrailingCommas()`), mixed or inconsistent indentation, and comments between a key and its value or before a comma. Each `Issue` has `file:line:col`, a rule id and a message; `LintOnly(rules...)` picks rules.

### `$extends`

```go
doc, _ := jsonc.Open(p, jsonc.WithPlugin(extends.New(extends.WithRoot(dir))))
cfg, err := doc.Extended()
```

Bases merge in list order, then the file on top; refs are relative to each file; a cycle is an error naming the chain. `doc.Render()` and `Save` still write `$extends` as written. Without an Includer, `$extends` is a plain key. Expand after extending: `cfg.Expanded()`. One-call `Read` applies `$extends` when an Includer is installed.

### Other expansion details

`ExpandRequested()` tells a caller whether `WithExpand()` was given; expansion errors carry `file:line:col` and still unwrap to the underlying `RequiredError` or `plugin.Error`.

### Packages

- [`jsonc/format`](format): `Format(doc, Style)` re-indents keeping comments; `Minify`.
- [`jsonc/generate`](generate): builders for commented files and `FromStruct` from cfgkit-tagged structs.
- [`jsonc/schema`](schema): JSON Schema 2020-12 from a struct, `Validate` with `file:line:col`, a plugin that is a `Validator` and a `SaveGuard` (refuses to save a document with schema errors).
- [`jsonc/diff`](diff): changes by path, ignoring comments and layout.
- [`jsonc/cli`](cli): the command package mounted as `textdatactl jsonc` (get, set, unset, restore, delete, fmt, validate, diff, to-json).
