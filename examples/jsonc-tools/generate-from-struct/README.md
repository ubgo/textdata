# jsonc generate from a struct

Writes a commented config file the way an `app init` command would, with `github.com/ubgo/textdata/jsonc/generate`: once from the app's config struct and its cfgkit tags, once by hand with the builder API. The program works in a temporary folder it creates and removes.

It shows:

- `generate.FromStruct` turning `json`, `doc`, `default`, `enum`, `env:",required"`/`notempty` and `secret` tags into keys, values and comments, with `SchemaURL` adding `$schema`.
- `time.Duration` written as nanoseconds with the readable form as a trailing comment; a `secret` field written empty even with a default.
- Reading the file back with `jsonc.Open` and `Decode`: the struct with every default filled in.
- The builders (`New`, `Header`, `Object`, `Array`, `Set`, `Comment`, `Blank`, `Doc`, `Trailing`, `BlankBefore`, `jsonc.Raw`) with `Tabs()` and `TrailingCommas(true)`.
- `Comments(false)` with `Spaces(4)`: plain JSON.

## Run

```sh
cd examples/jsonc-tools/generate-from-struct && go run .
```

Expected output (pinned by `main_test.go` against `testdata/expected.txt`):

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

Guide: [Generate a JSONC file](../../../docs/guides/jsonc-tools/generate.md).
