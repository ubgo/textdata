# jsonc schema plugin

Validates a JSONC config against a JSON Schema built from the app's config struct, with `github.com/ubgo/textdata/jsonc/schema`. The program works in a temporary folder it creates and removes.

It shows:

- `schema.FromStruct` with `WithTitle`, written with `JSON()` for editors and read back with `schema.Load`.
- `schema.Validate` returning `Result{Errors, Warnings}`: an `enum` and a `maximum` error, an unknown key as a warning, each with `file:line:col`.
- `schema.Parse` of a hand-written JSONC schema, and `Parse` refusing a `pattern` Go's RE2 cannot compile (`ErrSchema`).
- `schema.Plugin` installed with `jsonc.WithPlugin` and run by `doc.Validate()`, with `OnWarning` receiving the warnings.
- The same plugin as a `plugin.SaveGuard`: installed, it makes `Save` refuse an edit that breaks the schema, and the file on disk keeps the last good version.

## Run

```sh
cd examples/jsonc-tools/schema-plugin && go run .
```

Expected output (pinned by `main_test.go` against `testdata/expected.txt`):

```text
== config.schema.json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "app config",
  "type": "object",
  "properties": {
    "$schema": {
      "description": "The JSON Schema this file follows.",
      "type": "string"
    },
    "name": {
      "description": "Service name.",
      "type": "string"
    },
    "logLevel": {
      "type": "string",
      "enum": [
        "debug",
        "info",
        "warn",
        "error"
      ],
      "default": "info"
    },
    "port": {
      "type": "integer",
      "default": 8080,
      "minimum": 0,
      "maximum": 65535
    }
  },
  "required": [
    "name"
  ],
  "additionalProperties": false
}
== Validate
error [enum]: config.jsonc:4:15: logLevel: must be one of "debug", "info", "warn", "error"; got "verbose"
error [maximum]: config.jsonc:5:11: port: must be <= 65535
warning [additionalProperties]: config.jsonc:6:3: tpyo: unknown key "tpyo"
ok: false
== Parse a hand-written schema
inline.jsonc:1:10: name: must match the pattern "^[a-z]+$"
bad schema: schema: invalid schema: #/pattern: pattern does not compile as a Go (RE2) regexp: error parsing regexp: invalid or unsupported Perl syntax: `(?!` (ErrSchema: true)
== Plugin
warning: unknown key "tpyo"
validate: jsonc: plugin "schema": validate: config.jsonc:4:15: logLevel: must be one of "debug", "info", "warn", "error"; got "verbose" (and 1 more error)
== SaveGuard
save fixed: <nil>
save broken: jsonc: plugin "schema": guard save config.jsonc: config.jsonc:5:11: port: must be >= 0
port on disk: 8443
```

Guide: [Validate with a JSON Schema](../../../docs/guides/jsonc-tools/schema.md).
