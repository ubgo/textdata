# jsonc/schema

JSON Schema (draft 2020-12) for JSONC config files: build a schema from a Go config struct, load one from a file, and validate a `*jsonc.Doc` against it with `file:line:col` errors. Also a plugin: a `plugin.Validator`, so `doc.Validate()` can run it, and a `plugin.SaveGuard`, so `Save` refuses a document with schema errors.

Part of the `github.com/ubgo/textdata/jsonc` module. Standard library only.

## Install

```sh
go get github.com/ubgo/textdata/jsonc
```

## Example

```go
s, err := schema.FromStruct(Config{})          // or schema.Load("config.schema.json")
b, _ := s.JSON()                               // write it for editors (go generate)

doc, _ := jsonc.Open("config.jsonc")
res := schema.Validate(doc, s)
for _, w := range res.Warnings {
	fmt.Println("warning:", w)                 // config.jsonc:7:3: tpyo: unknown key "tpyo"
}
if err := res.Err(); err != nil {
	return err                                 // config.jsonc:3:22: server.port: must be <= 65535
}
```

As a plugin:

```go
doc, _ := jsonc.Open("config.jsonc", jsonc.WithPlugin(schema.Plugin(s, schema.OnWarning(logWarning))))
err := doc.Validate()                       // on demand
err = doc.Save()                            // refused while the document has schema errors
```

Warnings never fail `Validate` or refuse a save. `Plugin` accepts any `plugin.Document`; one that is not a `*jsonc.Doc` is parsed from its `Bytes()` and named by its `Location()`.

## FromStruct

Reads the same tags as `jsonc/generate` (cfgkit's tags), so a file written by `generate.FromStruct` validates clean against `schema.FromStruct` of the same struct.

| Go / tag | Schema |
|---|---|
| `json` tags | `properties`, in field order, with `encoding/json`'s naming rules |
| `doc:"text"` | `description` |
| `default:"v"` | `default`, parsed for the field's type then written as JSON (never for `secret:"true"` fields) |
| `enum:"a,b"` | `enum`, each value parsed for the field's type |
| `env:",required"`, `env:",notempty"`, `required:"true"` | `required`; `notempty` also adds `minLength: 1` to a string |
| `string`, `TextMarshaler` | `"type": "string"` |
| ints / uints | `"integer"`, with the type's range for 8/16/32-bit types and `minimum: 0` for uints |
| floats | `"number"` |
| `time.Duration` | `"integer"` (nanoseconds, as `encoding/json` writes it) |
| `time.Time` | `"string"`, `format: date-time` |
| `[]byte` | `"string"`, `contentEncoding: base64` |
| slices / arrays | `"array"` with `items` (arrays also `minItems` = `maxItems` = length) |
| maps | `"object"` with `additionalProperties` = the element schema |
| pointers | also allow `null` (a struct pointer as `anyOf: [struct, null]`) |
| a struct used twice, or recursively | one `$defs` entry and `$ref`s to it |
| `any`, `json.Marshaler` | `{}` (anything) |

Every struct object gets `additionalProperties: false` (unknown keys become warnings) unless `AllowAdditional()` is passed. The root allows a `"$schema"` string. `WithID` and `WithTitle` set `$id` and `title`. A default or enum value that does not parse is an error naming the field.

## Validate

Keywords checked: `type` (`integer` is any number without a fraction, `1.0` included), `enum`, `const` (JSON equality: `1 == 1.0`, key order ignored), `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` (exact decimal comparison, so 64-bit integers do not round), `minLength`, `maxLength` (in characters), `pattern`, `items`, `minItems`, `maxItems`, `properties`, `required`, `additionalProperties`, `allOf`, `anyOf`, `oneOf`, `not`, `$ref`.

- `additionalProperties: false` reports each unknown key as a **warning**, positioned at the key: a typo or a newer option should be pointed out without refusing the file. A schema-valued `additionalProperties` validates the extra values normally (errors).
- `$ref` must be local: `#` or a JSON Pointer into `$defs`, `properties`, `items`, `additionalProperties`, `allOf`/`anyOf`/`oneOf`/`not`. Remote references and anchors are refused by `Parse`. Sibling keywords next to `$ref` apply too (2020-12).
- `pattern` is a Go regexp (RE2): lookaround and backreferences are not supported and make `Parse` fail. Like ECMA-262 it matches anywhere unless anchored with `^`/`$`.
- Annotations (`title`, `description`, `default`, `examples`, `format`, `deprecated`, ...) are kept and not checked; `format` is an annotation in 2020-12.
- Any other keyword (`if`/`then`/`else`, `patternProperties`, `uniqueItems`, `minProperties`, `multipleOf`, ...) is kept for round trips and produces one **warning** where it applies: it is never silently treated as satisfied.
- When an `anyOf`/`oneOf` fails and exactly one branch accepts the value's type, that branch's errors are reported with their own positions (a nullable section with a bad key inside points at the key, not the section).

`Result` holds `Errors` and `Warnings` (`Issue{File, Pos, Path, Keyword, Message}`), each in document order; `Err()` is nil without errors, otherwise a `pos.ErrorList`. A document with syntax errors yields those errors. `WithFileName` overrides the reported file name.

## Parse and Load

`Parse(src, jsonc options...)` reads a schema written as JSON or JSONC (comments allowed) and checks keyword types, every `$ref` and every `pattern`; errors carry `file:line:col` and wrap `ErrSchema`. `Load(path)` reads a file. `*Schema` also implements `json.Marshaler` (fixed keyword order, properties in their original order, so output is diff-stable) and `json.Unmarshaler`.

## Guarantees (pinned by tests)

- `Validate` never modifies the schema or the document, and is safe for concurrent use on one schema.
- Any schema text and any document never panic or hang (fuzzed): reference loops end at a hop limit and a check budget, huge exponents are compared without allocating huge numbers.
- A parsed schema re-parses from its own `JSON()` output.
- Every issue's position lies inside the document.
