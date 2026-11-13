# Validate with a JSON Schema

`github.com/ubgo/textdata/jsonc/schema` validates a JSONC config against a JSON Schema (draft 2020-12) and reports every problem with `file:line:col`. It builds the schema from your Go config struct (`FromStruct`), or reads one written by hand (`Parse`, `Load`), and plugs into a document as a `plugin.Validator` and a `plugin.SaveGuard`. Use it to catch a wrong type, an out-of-range port or a typo in a key before the app starts, in an editor (through `"$schema"`), in CI, or on save.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

`schema` is a package inside the `jsonc` module, standard library only. The runnable program for this page is [`examples/jsonc-tools/schema-plugin`](../../../examples/jsonc-tools/schema-plugin); it validates this file:

```jsonc
{
  // service
  "name": "api",
  "logLevel": "verbose",
  "port": 70000,
  "tpyo": true
}
```

## Step 1 — Build the schema from your config struct

`FromStruct` reads the same tags as [`generate.FromStruct`](generate.md) (cfgkit's tags), so a file written by the generator validates clean against the schema of the same struct.

<!-- ds:block id=jt-schema-config-zyjqzp98 -->
**Config** · [`examples/jsonc-tools/schema-plugin/main.go:35-39`](../../../examples/jsonc-tools/schema-plugin/main.go#L35-L39)

```go
type Config struct {
	Name     string `json:"name" env:"NAME,required" doc:"Service name."`
	LogLevel string `json:"logLevel" enum:"debug,info,warn,error" default:"info"`
	Port     uint16 `json:"port" default:"8080"`
}
```
<!-- /ds:block hash=1820be -->

| Go / tag | Schema |
|---|---|
| `json` tags | `properties`, in field order, with `encoding/json`'s naming rules |
| `doc:"text"` | `description` |
| `default:"v"` | `default`, parsed for the field's type (never for `secret:"true"` fields) |
| `enum:"a,b"` | `enum`, each value parsed for the field's type |
| `env:",required"`, `env:",notempty"`, `required:"true"` | `required`; `notempty` also adds `minLength: 1` to a string |
| ints, uints | `"integer"`, with the type's range for 8/16/32-bit types and `minimum: 0` for uints |
| `time.Duration` | `"integer"` (nanoseconds) |
| `time.Time` | `"string"`, `format: date-time` |
| slices, maps, pointers | `"array"` with `items`; `"object"` with `additionalProperties`; pointers also allow `null` |
| a struct used twice, or recursively | one `$defs` entry and `$ref`s to it |

Every struct object gets `additionalProperties: false`, so unknown keys become warnings, unless you pass `schema.AllowAdditional()`. `WithID` and `WithTitle` set `$id` and `title`. Write the schema next to the config with `s.JSON()` (fixed keyword order, diff-stable), for example from a `go generate` step:

<!-- ds:block id=jt-schema-write-zcrt59ud -->
**writeSchema** · [`examples/jsonc-tools/schema-plugin/main.go:113-123`](../../../examples/jsonc-tools/schema-plugin/main.go#L113-L123)

```go
func writeSchema(name string) error {
	s, err := schema.FromStruct(Config{}, schema.WithTitle("app config"))
	if err != nil {
		return err // a default or enum tag that does not parse names the field
	}
	b, err := s.JSON()
	if err != nil {
		return err
	}
	return os.WriteFile(name, b, filePerm)
}
```
<!-- /ds:block hash=ab1803 -->

## Step 2 — Validate a file

`schema.Validate(doc, s)` returns a `*Result` with two lists, each in document order:

- `Errors` fail validation: wrong type, out of range, not in the enum, a missing required key.
- `Warnings` only point something out: an unknown key under `additionalProperties: false` (a typo or a newer option), or a keyword this package does not check.

Each entry is an `Issue{File, Pos, Path, Keyword, Message}` and prints as `file:line:col: path: message`. `res.OK()` is true without errors; `res.Err()` is nil or a `pos.ErrorList`. `WithFileName` overrides the file name in issues.

<!-- ds:block id=jt-schema-validate-jtqgg2uk -->
**validateFile** · [`examples/jsonc-tools/schema-plugin/main.go:129-143`](../../../examples/jsonc-tools/schema-plugin/main.go#L129-L143)

```go
func validateFile(w io.Writer, name string, s *schema.Schema) error {
	d, err := jsonc.Open(name)
	if err != nil {
		return err
	}
	res := schema.Validate(d, s, schema.WithFileName(configName))
	for _, e := range res.Errors {
		fmt.Fprintf(w, "error [%s]: %v\n", e.Keyword, e)
	}
	for _, x := range res.Warnings {
		fmt.Fprintf(w, "warning [%s]: %v\n", x.Keyword, x)
	}
	fmt.Fprintf(w, "ok: %t\n", res.OK())
	return nil
}
```
<!-- /ds:block hash=1619b5 -->

## Step 3 — Load or parse a schema written by hand

`schema.Load(path)` reads a schema file; `schema.Parse(src, jsonc options...)` reads one from bytes. Both accept JSON or JSONC (comments allowed) and check the schema itself: keyword types, every `$ref`, every `pattern`. A broken schema is an error wrapping `schema.ErrSchema`, so you learn about it when loading, not on the first validation.

<!-- ds:block id=jt-schema-parse-fy6ebs7g -->
**validateWithHandSchema** · [`examples/jsonc-tools/schema-plugin/main.go:149-164`](../../../examples/jsonc-tools/schema-plugin/main.go#L149-L164)

```go
func validateWithHandSchema(w io.Writer) error {
	s, err := schema.Parse([]byte(handSchema))
	if err != nil {
		return err
	}
	d, err := jsonc.Parse([]byte(`{"name": "API"}`), jsonc.WithFileName("inline.jsonc"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, schema.Validate(d, s).Err())

	// pattern is a Go regexp (RE2): lookaround is refused when parsing.
	_, err = schema.Parse([]byte(`{"pattern": "^(?!admin)"}`))
	fmt.Fprintf(w, "bad schema: %v (ErrSchema: %t)\n", err, errors.Is(err, schema.ErrSchema))
	return nil
}
```
<!-- /ds:block hash=b9a248 -->

Keywords checked: `type`, `enum`, `const`, `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `minLength`, `maxLength`, `pattern`, `items`, `minItems`, `maxItems`, `properties`, `required`, `additionalProperties`, `allOf`, `anyOf`, `oneOf`, `not`, `$ref`. Annotations (`title`, `description`, `default`, `format`, ...) are kept and not checked. Any other keyword (`if`/`then`/`else`, `patternProperties`, `uniqueItems`, ...) is kept and produces one warning where it applies; it is never silently treated as satisfied.

## Step 4 — Run it as a plugin

`schema.Plugin(s, opts...)` returns a `*schema.Checker` named `schema`: a `plugin.Validator` and a `plugin.SaveGuard` (Step 5). Install it with `jsonc.WithPlugin` and run it with `doc.Validate()`, which returns the first validator's failure naming the plugin. `doc.Validate()` never runs on its own: reading a file does not validate it. The `error` return can only carry errors, so pass `schema.OnWarning` to receive the warnings.

<!-- ds:block id=jt-schema-plugin-tmevwhee -->
**validateAsPlugin** · [`examples/jsonc-tools/schema-plugin/main.go:171-182`](../../../examples/jsonc-tools/schema-plugin/main.go#L171-L182)

```go
func validateAsPlugin(w io.Writer, name string, s *schema.Schema) error {
	onWarning := schema.OnWarning(func(i schema.Issue) {
		fmt.Fprintln(w, "warning:", i.Message)
	})
	checker := schema.Plugin(s, onWarning, schema.WithFileName(configName))
	d, err := jsonc.Open(name, jsonc.WithPlugin(checker))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "validate:", d.Validate())
	return nil
}
```
<!-- /ds:block hash=7f5ca6 -->

The checker accepts any `plugin.Document`; one that is not a `*jsonc.Doc` is parsed from its `Bytes()` and named by its `Location()`.

## Step 5 — Refuse to save a file that breaks the schema

The checker is also a `plugin.SaveGuard`, so installing it is enough: `Save` and `SaveAs` validate the document first and refuse to write it when it has schema errors. Warnings (unknown keys) never refuse a save. Every guard runs on a clone before anything is written, and the first refusal aborts the save with the file untouched.

<!-- ds:block id=jt-schema-save-4fhp9stf -->
**guardSave** · [`examples/jsonc-tools/schema-plugin/main.go:189-210`](../../../examples/jsonc-tools/schema-plugin/main.go#L189-L210)

```go
func guardSave(w io.Writer, name string, s *schema.Schema) error {
	checker := schema.Plugin(s, schema.WithFileName(configName))
	d, err := jsonc.Open(name, jsonc.WithPlugin(checker))
	if err != nil {
		return err
	}
	if err := errors.Join(d.Set("logLevel", "warn"), d.Set("port", 8443), d.Delete("tpyo")); err != nil {
		return err
	}
	fmt.Fprintln(w, "save fixed:", d.Save())
	if err := d.Set("port", -1); err != nil {
		return err
	}
	fmt.Fprintln(w, "save broken:", withoutDir(d.Save(), filepath.Dir(name)))
	onDisk, err := jsonc.Open(name)
	if err != nil {
		return err
	}
	port, err := onDisk.GetInt("port")
	fmt.Fprintln(w, "port on disk:", port)
	return err
}
```
<!-- /ds:block hash=d41e57 -->

## Try it

```sh
cd examples/jsonc-tools/schema-plugin && go run .
```

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

The numbers: 2 errors and 1 warning for the file; the plugin reports the first error plus a count (`and 1 more error`); after the refused save the file on disk still holds port `8443`. The program prints `config.jsonc` where the real error names the full path of the temporary file.

The same from the command line (warnings print with `warning:` and do not fail; any error exits 1):

```sh
textdatactl jsonc validate config.jsonc --schema config.schema.json
```

## Pitfalls

- **Unknown keys are warnings, not errors.** With `additionalProperties: false` a typo such as `tpyo` does not fail `Validate`, `res.Err()` or `doc.Validate()`. Check `res.Warnings` (or pass `OnWarning` to the plugin) if you want to act on them.
- **The plugin drops warnings unless you pass `OnWarning`.** `plugin.Validator` returns one `error`; warnings have nowhere else to go.
- **Installing the plugin validates on save, never on read.** `Save` refuses a document with schema errors (Step 5); to check a file you only read, call `doc.Validate()`.
- **`pattern` is a Go regexp (RE2).** Lookaround and backreferences are refused by `Parse` with `ErrSchema`. Like ECMA-262 it matches anywhere unless anchored with `^`/`$`.
- **`$ref` must be local**: `#` or a JSON Pointer into the same schema. Remote references and anchors are refused by `Parse`.
- **`integer` accepts `1.0`.** Any number without a fraction is an integer; bounds are compared as exact decimals, so 64-bit values do not round.
- **A document with syntax errors yields those errors**, with keyword `syntax`; an empty document is an error unless the schema is `true`.
- **A bad `default` or `enum` tag fails `FromStruct`** with an error naming the field.
- **Unsupported keywords are not ignored silently**: each produces one warning where it applies. Read the warnings once when adopting a third-party schema.

## Related

- [Generate a JSONC file](generate.md) (the file from the same struct validates clean), [Format a JSONC file](format.md), [Diff two JSONC files](diff.md)
- Package reference: [jsonc/schema README](../../../jsonc/schema/README.md) (full tag table, keyword rules, guarantees)
- CLI: [jsonc/cli README § verbs](../../../jsonc/cli/README.md#verbs) (`validate --schema`)
- Example: [`examples/jsonc-tools/schema-plugin`](../../../examples/jsonc-tools/schema-plugin); the package's own [`ExampleValidate`](../../../jsonc/schema/example_test.go)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc/schema](https://pkg.go.dev/github.com/ubgo/textdata/jsonc/schema), [plugin.SaveGuard](https://pkg.go.dev/github.com/ubgo/textdata/plugin#SaveGuard)
