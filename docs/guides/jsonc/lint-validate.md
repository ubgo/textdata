# Lint and validate

A JSONC file can parse cleanly and still be wrong: a key written twice (only the last one counts), indentation that mixes tabs and spaces, a comment where formatters drop it, a trailing comma a strict consumer rejects. `jsonc.Lint` finds those, each with `file:line:col` and a rule id, without changing the document. `Doc.Validate` is the other half: it runs `Validator` plugins, your own rules or a JSON Schema, on demand. Use both in a CI step or a `config check` command.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

The code comes from [`examples/jsonc/lint`](../../../examples/jsonc/lint), which lints this file:

<!-- ds:block id=src-xm6dgddf -->
**src** · [`examples/jsonc/lint/main.go:26-31`](../../../examples/jsonc/lint/main.go#L26-L31)

```go
const src = "{\n" +
	"  \"port\": 8080,\n" +
	"  \"port\": 9090,\n" +
	"\t\"debug\": true,\n" +
	"  \"name\": /* service */ \"billing\",\n" +
	"}\n"
```
<!-- /ds:block hash=eb82da -->

The four rules, `jsonc.AllLintRules()`:

| Rule | Constant | Reports |
|---|---|---|
| `duplicate-key` | `LintDuplicateKey` | an object repeats a key; readers keep the last one, so the earlier ones are dead text that looks live |
| `trailing-comma` | `LintTrailingComma` | a comma after the last member or element; only with `ForbidTrailingCommas()`, because JSONC allows it |
| `mixed-indent` | `LintMixedIndent` | a member's indentation mixes tabs and spaces, or is not its container's plus the file's indent unit |
| `odd-comment` | `LintOddComment` | a comment between a key and its value, or between a value and its comma; it belongs to no member, so tools move or drop it |

## Step 1 — Run every rule

`Lint(doc, opts...)` returns a `[]Issue` sorted by position; an empty slice means clean. `Issue.String()` formats as `file:line:col: message (rule)`, the shape editors and CI annotations understand.

<!-- ds:block id=lintall-nxrpgbfp -->
**lintAll** · [`examples/jsonc/lint/main.go:57-62`](../../../examples/jsonc/lint/main.go#L57-L62)

```go
func lintAll(w io.Writer, doc *jsonc.Doc) {
	fmt.Fprintln(w, "--- all rules, trailing commas forbidden ---")
	for _, issue := range jsonc.Lint(doc, jsonc.ForbidTrailingCommas()) {
		fmt.Fprintln(w, issue)
	}
}
```
<!-- /ds:block hash=356ef6 -->

## Step 2 — Pick rules and read the fields

`LintOnly(rules...)` limits the run, for a gate that cares about one kind of mistake. Each `Issue` has `File`, `Pos` (a base `pos.Pos`: `Line`, `Col`, `Offset`), `Rule` and `Msg`, for your own report format.

<!-- ds:block id=lintsome-ey8ud5cw -->
**lintSome** · [`examples/jsonc/lint/main.go:68-74`](../../../examples/jsonc/lint/main.go#L68-L74)

```go
func lintSome(w io.Writer, doc *jsonc.Doc) {
	fmt.Fprintln(w, "--- duplicate keys only ---")
	for _, issue := range jsonc.Lint(doc, jsonc.LintOnly(jsonc.LintDuplicateKey)) {
		fmt.Fprintf(w, "rule=%s line=%d col=%d msg=%q\n", issue.Rule, issue.Pos.Line, issue.Pos.Col, issue.Msg)
	}
	fmt.Fprintln(w, "rules available:", jsonc.AllLintRules())
}
```
<!-- /ds:block hash=0af651 -->

## Step 3 — Write a Validator plugin

Lint knows nothing about what your config means. For rules of your own, write a `plugin.Validator` against the shared [`plugin`](../../../README.md#plugin) package: it receives a read-only `plugin.Document` (`ValuePaths`, `ValueAt`, `Bytes`, `Location`, `FormatName`), not the jsonc tree, so it depends only on the base package.

<!-- ds:block id=errmissingowner-8q6x3kye -->
**errMissingOwner** · [`examples/jsonc/lint/main.go:79`](../../../examples/jsonc/lint/main.go#L79-L79)

```go
var errMissingOwner = errors.New("missing required key " + requiredKey)
```
<!-- /ds:block hash=23ced5 -->

<!-- ds:block id=requireowner-rd48xv57 -->
**requireOwner** · [`examples/jsonc/lint/main.go:85`](../../../examples/jsonc/lint/main.go#L85-L85)

```go
type requireOwner struct{}
```
<!-- /ds:block hash=8cf65f -->

<!-- ds:block id=requireowner-name-regush65 -->
**requireOwner.Name** · [`examples/jsonc/lint/main.go:88`](../../../examples/jsonc/lint/main.go#L88-L88)

```go
func (requireOwner) Name() string { return "require-owner" }
```
<!-- /ds:block hash=05684a -->

<!-- ds:block id=requireowner-validate-84ecf37m -->
**requireOwner.Validate** · [`examples/jsonc/lint/main.go:91-96`](../../../examples/jsonc/lint/main.go#L91-L96)

```go
func (requireOwner) Validate(doc plugin.Document) error {
	if _, ok := doc.ValueAt(requiredKey); !ok {
		return errMissingOwner
	}
	return nil
}
```
<!-- /ds:block hash=26c3ae -->

## Step 4 — Run Validate

Install it with `WithPlugin` and call `Validate()`. It runs every installed Validator in install order on a clone, and returns the first failure wrapped in a `plugin.Error` that names the plugin; `errors.Is` still reaches your own error. Validators never run on read, only when you call `Validate`.

<!-- ds:block id=validate-tnfzj9rp -->
**validate** · [`examples/jsonc/lint/main.go:102-115`](../../../examples/jsonc/lint/main.go#L102-L115)

```go
func validate(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(`{"name": "billing"}`), jsonc.WithPlugin(requireOwner{}))
	if err != nil {
		return err
	}
	verr := doc.Validate()
	fmt.Fprintln(w, "validate:", verr)
	fmt.Fprintln(w, "is errMissingOwner:", errors.Is(verr, errMissingOwner))
	if err := doc.Set(requiredKey, "payments-team"); err != nil {
		return err
	}
	fmt.Fprintln(w, "after Set, validate:", doc.Validate())
	return nil
}
```
<!-- /ds:block hash=fa0573 -->

For JSON Schema, install `schema.Plugin(s)` from [`jsonc/schema`](../../../jsonc/schema) the same way; its issues carry `file:line:col` of the offending value (see the [schema guide](../jsonc-tools/schema.md)).

## Try it

```sh
cd examples/jsonc/lint && go run .
```

```text
--- all rules, trailing commas forbidden ---
app.jsonc:3:3: duplicate key "port" (first at 2:3); the last one wins (duplicate-key)
app.jsonc:4:1: indented 1 tab, expected 2 spaces (the file indents by 2 spaces) (mixed-indent)
app.jsonc:5:11: comment between key "name" and its value; formatters and other tools may move or drop it (odd-comment)
app.jsonc:5:34: trailing comma after the last member (trailing-comma)
--- duplicate keys only ---
rule=duplicate-key line=3 col=3 msg="duplicate key \"port\" (first at 2:3); the last one wins"
rules available: [duplicate-key trailing-comma mixed-indent odd-comment]
validate: jsonc: plugin "require-owner": validate: missing required key owner
is errMissingOwner: true
after Set, validate: <nil>
```

## Pitfalls

- **Syntax errors are not lint issues.** On a document with syntax errors `Lint` checks only the part that parsed; report `doc.Err()` (or `Parse`'s error) first.
- **`trailing-comma` is silent without `ForbidTrailingCommas()`**, even when `LintOnly(LintTrailingComma)` asks for it.
- **`LintOnly` replaces the set, it does not add to it.** Two `LintOnly` options: the last one wins.
- **The indent unit is the file's own.** `mixed-indent` takes the unit from the first member that sits on a line of its own (`the file indents by 2 spaces`), not from a fixed width; a file indented consistently with tabs is clean. If that first member is the odd one out, every other line is reported instead.
- **`Validate` refuses a document with syntax errors** (`ErrInvalid`) before any plugin runs.
- **`Validate` stops at the first failing plugin.** To collect every finding, write one Validator that gathers them, or call each check yourself.
- **The `plugin.Document` a Validator gets is raw**: `ValueAt` returns `${VAR}` as written. Validate the expanded document by calling `Validate` on `doc.Expanded()`'s result, which carries the same plugins.
- **Lint never fixes anything.** To re-lay a file out, use [`jsonc/format`](../jsonc-tools/format.md); `textdatactl jsonc validate --forbid-trailing-commas` runs Lint and schema validation from the shell.

## Related

- [Parse and strict](parse-and-strict.md) · [Expansion and plugins](expansion-and-plugins.md) · [Schema guide](../jsonc-tools/schema.md) · [Format guide](../jsonc-tools/format.md)
- Module README: [jsonc § Lint](../../../jsonc/README.md#lint), [§ Expansion and plugins](../../../jsonc/README.md#expansion-and-plugins)
- Example: [`examples/jsonc/lint`](../../../examples/jsonc/lint)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc](https://pkg.go.dev/github.com/ubgo/textdata/jsonc)
