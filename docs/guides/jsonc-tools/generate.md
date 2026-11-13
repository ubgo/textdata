# Generate a JSONC file

`github.com/ubgo/textdata/jsonc/generate` writes a JSONC file from scratch, with comments, blank lines and a chosen indent style. `generate.FromStruct` builds that file straight from your config struct, using the struct tags `github.com/ubgo/cfgkit` reads, so one struct drives the defaults, the comments and (with [`jsonc/schema`](schema.md)) the schema. Use it for an `init` command that creates a commented config, or to ship an example config that never drifts from the code.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

`generate` is a package inside the `jsonc` module, standard library only. You need a config struct with `json` tags (and optionally the cfgkit tags below); cfgkit itself is not required. The runnable program for this page is [`examples/jsonc-tools/generate-from-struct`](../../../examples/jsonc-tools/generate-from-struct).

## Step 1 — Tag your config struct

`FromStruct` reads these tags:

| Tag | Effect on the file |
|---|---|
| `json:"name"` | the key, with `encoding/json`'s rules: embedded structs flattened, `json:"-"` and unexported fields skipped, `,string` honoured |
| `doc:"text"` | the comment above the key |
| `default:"v"` | the value when the struct's own value is zero, parsed with cfgkit's rules (`5s`, RFC 3339 times, `a,b` slices, `k:v,k2:v2` maps, `TextUnmarshaler`) |
| `env:"KEY,required"`, `required:"true"` | a `Required.` note in the comment |
| `env:"KEY,notempty"` | a `Required, must not be empty.` note |
| `enum:"a,b,c"` | a `One of: a, b, c.` note (split on `delim`, default `,`) |
| `secret:"true"` | always written as the zero value, never your value or the default, with a `Secret: do not commit a real value.` note |
| `delim`, `kvdelim` | separators for slice and map defaults, as in cfgkit |

<!-- ds:block id=jt-gen-config-2tsxk8dz -->
**Config** · [`examples/jsonc-tools/generate-from-struct/main.go:36-43`](../../../examples/jsonc-tools/generate-from-struct/main.go#L36-L43)

```go
type Config struct {
	LogLevel string        `json:"logLevel" enum:"debug,info,warn,error" default:"info" doc:"Log verbosity."`
	Addr     string        `json:"addr" env:"ADDR,notempty" default:":8080" doc:"Listen address."`
	Timeout  time.Duration `json:"timeout" default:"5s" doc:"Request timeout."`
	Hosts    []string      `json:"hosts" default:"a.example,b.example"`
	Password string        `json:"password" env:"DB_PASSWORD,required" secret:"true" default:"changeme"`
	DB       DB            `json:"db" doc:"Primary database."`
}
```
<!-- /ds:block hash=f2f7ca -->

`DB` is a plain struct with `host` (default `localhost`) and `port` (default `5432`); nested structs become nested objects.

## Step 2 — Generate and write the file

Pass the zero value so every key shows its default. `generate.SchemaURL` adds `"$schema"` as the first key, so editors offer completion and validation from the schema you generate in [the schema guide](schema.md). `Bytes` renders the file; `f.Doc()` renders and parses it if you want to keep editing before saving.

<!-- ds:block id=jt-gen-template-43vpkj7x -->
**writeTemplate** · [`examples/jsonc-tools/generate-from-struct/main.go:101-111`](../../../examples/jsonc-tools/generate-from-struct/main.go#L101-L111)

```go
func writeTemplate(name string) error {
	f, err := generate.FromStruct(Config{}, generate.SchemaURL(schemaURL))
	if err != nil {
		return err // a default or enum tag that does not parse names the field
	}
	b, err := f.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(name, b, filePerm)
}
```
<!-- /ds:block hash=75db8b -->

A blank line sets off every documented member and every nested section. `time.Duration` is written as `encoding/json` writes it, an integer of nanoseconds, with the readable form as a trailing comment (`5000000000, // 5s`), because cfgkit's `FromJSON` reads the file with `encoding/json`.

## Step 3 — Read it back

Decoding the generated file with `doc.Decode` yields the struct with every default filled in (pinned by the package's golden-file and round-trip tests). The secret field stays empty even though it has a default.

<!-- ds:block id=jt-gen-readback-bvrqdekr -->
**readBack** · [`examples/jsonc-tools/generate-from-struct/main.go:117-125`](../../../examples/jsonc-tools/generate-from-struct/main.go#L117-L125)

```go
func readBack(name string) (Config, error) {
	var cfg Config
	d, err := jsonc.Open(name)
	if err != nil {
		return cfg, err
	}
	err = d.Decode(&cfg)
	return cfg, err
}
```
<!-- /ds:block hash=ae28eb -->

## Step 4 — Build a file by hand

When there is no struct behind the file, use the builders. A `File` is the root object plus `Header`, `Root`, `Bytes` and `Doc`. An `Object` has `Set`, `Object`, `Array`, `Comment` and `Blank`; an `Array` has `Add`, `Object`, `Array`, `Comment` and `Blank`. Item options decorate one member: `Doc(text)` (comment lines above), `Trailing(text)` (comment after the value on its line), `BlankBefore()`. A value is any Go value `encoding/json` can write, or `jsonc.Raw`, written verbatim.

<!-- ds:block id=jt-gen-byhand-6vwrn8qr -->
**buildByHand** · [`examples/jsonc-tools/generate-from-struct/main.go:131-143`](../../../examples/jsonc-tools/generate-from-struct/main.go#L131-L143)

```go
func buildByHand() ([]byte, error) {
	f := generate.New(generate.Tabs(), generate.TrailingCommas(true))
	f.Header("Created by `app init`. Edit freely.")
	srv := f.Object("server", generate.Doc("HTTP server"))
	srv.Set("port", 8080, generate.Trailing("default"))
	srv.Set("host", "0.0.0.0")
	f.Blank()
	f.Comment("Hosts allowed to connect.")
	f.Blank()
	f.Array("hosts").Add("a.example").Add("b.example")
	f.Set("retry", jsonc.Raw(`[1, /* then */ 5]`), generate.BlankBefore())
	return f.Bytes()
}
```
<!-- /ds:block hash=7dbe9a -->

## Step 5 — Pick the options

Options apply to both `New` and `FromStruct`:

| Option | Effect |
|---|---|
| `Indent(unit)`, `Spaces(n)`, `Tabs()` | indent unit, default two spaces; anything but spaces and tabs makes `Bytes` fail |
| `CRLF()` | `\r\n` line endings |
| `Comments(false)` | drop every comment and header |
| `TrailingCommas(true)` | a comma after the last member of every non-empty container (JSONC only) |
| `SchemaURL(url)` | `"$schema": url` as the first key |
| `FinalNewline(false)` | no line ending after the closing brace |

With `Comments(false)` and no trailing commas the output is plain JSON that `jsonc.Strict()` and `encoding/json` accept:

<!-- ds:block id=jt-gen-plain-2w8drnge -->
**plainJSON** · [`examples/jsonc-tools/generate-from-struct/main.go:149-155`](../../../examples/jsonc-tools/generate-from-struct/main.go#L149-L155)

```go
func plainJSON() ([]byte, error) {
	f, err := generate.FromStruct(DB{}, generate.Comments(false), generate.Spaces(4))
	if err != nil {
		return nil, err
	}
	return f.Bytes()
}
```
<!-- /ds:block hash=0ca343 -->

## Try it

```sh
cd examples/jsonc-tools/generate-from-struct && go run .
```

```text
== FromStruct -> config.jsonc
{
  "$schema": "./config.schema.json",

  // Log verbosity.
  // One of: debug, info, warn, error.
  "logLevel": "info",

  // Listen address.
  // Required, must not be empty.
  "addr": ":8080",

  // Request timeout.
  // Duration in nanoseconds.
  "timeout": 5000000000, // 5s
  "hosts": [
    "a.example",
    "b.example"
  ],

  // Required. Secret: do not commit a real value.
  "password": "",

  // Primary database.
  "db": {
    "host": "localhost",
    "port": 5432
  }
}
== decoded
{LogLevel:info Addr::8080 Timeout:5s Hosts:[a.example b.example] Password: DB:{Host:localhost Port:5432}}
== by hand, tabs, trailing commas
// Created by `app init`. Edit freely.
{
	// HTTP server
	"server": {
		"port": 8080, // default
		"host": "0.0.0.0",
	},

	// Hosts allowed to connect.

	"hosts": [
		"a.example",
		"b.example",
	],

	"retry": [1, /* then */ 5],
}
== Comments(false)
{
    "host": "localhost",
    "port": 5432
}
```

## Pitfalls

- **A bad `default` or `enum` tag is an error, even when your value is set.** `FromStruct` parses every default for its field's type and returns an error naming the field (`generate: field Server.Port: ...`); fix the tag, not the value.
- **A `secret` field is always written empty.** Neither your value nor its `default` reaches the file; that is the point. Fill secrets from the environment or a value plugin.
- **cfgkit's `Defaulter` is not called.** Call `Defaults()` on your value yourself before passing it to `FromStruct`.
- **Durations are nanoseconds.** The file holds `5000000000`, not `"5s"`; the readable form is only a comment. That matches how `encoding/json` (and cfgkit's `FromJSON`) reads a `time.Duration`.
- **A standalone `Comment` directly above a member becomes that member's comment once the file is parsed.** Follow it with `Blank()` to keep it separate, as Step 4 does.
- **`jsonc.Raw` is written verbatim.** The output is guaranteed to parse only for values that go through `encoding/json`; raw text is your responsibility.
- **Setting a key twice replaces it in place**, keeping its first position; it does not append a duplicate.
- **Unsupported field types fail.** A field `encoding/json` cannot write (a channel, a func) is an error naming the field. A nil pointer back to a type already being written (a recursive type) is written as `null`.
- **A `File` is not safe for concurrent use** while it is being built.

## Related

- [Validate with a JSON Schema](schema.md) (the schema from the same struct; a generated file validates with no errors and no warnings), [Format a JSONC file](format.md), [Diff two JSONC files](diff.md)
- Package reference: [jsonc/generate README](../../../jsonc/generate/README.md) (builders, tags, options, guarantees)
- Example: [`examples/jsonc-tools/generate-from-struct`](../../../examples/jsonc-tools/generate-from-struct); the package's own [`ExampleNew`](../../../jsonc/generate/example_test.go)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc/generate](https://pkg.go.dev/github.com/ubgo/textdata/jsonc/generate)
