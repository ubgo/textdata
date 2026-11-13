# jsonc/generate

Write a JSONC file from scratch, with comments, blank lines and a chosen indent style: what an `init` command needs to create a commented config file. `FromStruct` builds that file straight from a config struct, using the struct tags `github.com/ubgo/cfgkit` reads, so one struct drives the environment, the file, its comments and its schema (`jsonc/schema`).

Part of the `github.com/ubgo/textdata/jsonc` module. Standard library only.

## Install

```sh
go get github.com/ubgo/textdata/jsonc
```

## Build a file by hand

```go
f := generate.New(generate.SchemaURL("./app.schema.json"))
f.Header("Created by `app init`. Edit freely.")
srv := f.Object("server", generate.Doc("HTTP server"), generate.BlankBefore())
srv.Set("port", 8080, generate.Trailing("default"))
srv.Set("host", "0.0.0.0")
f.Array("hosts").Add("a.example").Add("b.example")
f.Set("raw", jsonc.Raw(`[1, /* two */ 2]`))

b, err := f.Bytes()        // []byte
doc, err := f.Doc()        // *jsonc.Doc, ready for further edits
```

```jsonc
// Created by `app init`. Edit freely.
{
  "$schema": "./app.schema.json",

  // HTTP server
  "server": {
    "port": 8080, // default
    "host": "0.0.0.0"
  },
  "hosts": [
    "a.example",
    "b.example"
  ],
  "raw": [1, /* two */ 2]
}
```

Builders: `File` (the root object plus `Header`, `Root`, `Bytes`, `Doc`), `Object` (`Set`, `Object`, `Array`, `Comment`, `Blank`), `Array` (`Add`, `Object`, `Array`, `Comment`, `Blank`). Item options: `Doc(text)` (comment lines above), `Trailing(text)` (comment after the value), `BlankBefore()`. A value is any Go value `encoding/json` can write (maps and slices are laid out over several lines) or `jsonc.Raw`, written verbatim. Setting a key twice replaces the member in place. A standalone `Comment` directly above a member becomes that member's comment when the file is parsed; follow it with `Blank()` to keep it separate.

## From a config struct

```go
type Config struct {
	LogLevel string        `json:"logLevel" enum:"debug,info,warn,error" default:"info" doc:"Log verbosity."`
	Addr     string        `json:"addr" env:"ADDR,notempty" default:":8080" doc:"Listen address."`
	Timeout  time.Duration `json:"timeout" default:"5s"`
	Password string        `json:"password" env:"DB_PASSWORD,required" secret:"true"`
	DB       *DB           `json:"db"`
}

f, err := generate.FromStruct(Config{}, generate.SchemaURL("./config.schema.json"))
```

| Tag | Effect |
|---|---|
| `json:"name"` | the key, with `encoding/json`'s rules: embedded structs flattened, name clashes resolved the same way, `json:"-"` and unexported fields skipped, `,string` honoured |
| `doc:"text"` | the comment above the key |
| `default:"v"` | the value when the struct's own value is zero, parsed with cfgkit's rules (`5s`, RFC 3339 times, `a,b` slices, `k:v,k2:v2` maps, `TextUnmarshaler`); a default that does not parse is an error naming the field |
| `env:"KEY,required"` / `env:"KEY,notempty"` / `required:"true"` | a "Required." note in the comment |
| `enum:"a,b,c"` | a "One of: a, b, c." note (split on `delim`, default `,`) |
| `secret:"true"` | always written as the zero value, with a "do not commit a real value" note |
| `delim`, `kvdelim` | separators for slice and map defaults, as in cfgkit |

Nested structs and nil pointers to structs become nested objects; a nil pointer back to a type already being written (a recursive type) is `null`. Nil maps and slices are written as `{}` and `[]`. `time.Duration` is written the way `encoding/json` writes it, an integer of nanoseconds, with the readable form as a trailing comment (`5000000000, // 5s`), because cfgkit's `FromJSON` reads the file with `encoding/json`. cfgkit's `Defaulter` is not called: call `Defaults()` on the value before passing it.

## Options

| Option | Effect |
|---|---|
| `Indent(unit)`, `Spaces(n)`, `Tabs()` | indent unit (default two spaces); anything but spaces and tabs makes `Bytes` fail |
| `CRLF()` | `\r\n` line endings |
| `Comments(false)` | drop every comment and header: plain JSON |
| `TrailingCommas(true)` | a comma after the last member (JSONC only) |
| `SchemaURL(url)` | `"$schema": url` as the first key |
| `FinalNewline(false)` | no line ending after the closing brace |

## Guarantees (pinned by tests)

- The output parses with `jsonc.Parse` with zero errors, for any key, value and comment text (fuzzed).
- With `Comments(false)` and no trailing commas the output is plain JSON: `jsonc.Strict()` and `encoding/json` accept it.
- Decoding `FromStruct`'s output with `doc.Decode` yields the struct with its defaults filled in (golden file plus round-trip test).
- The keys `FromStruct` writes are exactly the keys `encoding/json` writes for the struct.
- A file from `FromStruct` validates with no errors and no warnings against `schema.FromStruct` of the same struct.
- Output is deterministic.
