# Diff two JSONC files

`github.com/ubgo/textdata/jsonc/diff` compares two JSONC documents by meaning: which values were added, removed or changed, at which path, and where each value starts in each file. Comments, layout, key order, trailing commas and number spelling (`1` vs `1.0`) do not count. Use it to answer "does prod's config differ from the default, and where" in a review tool, a drift check or a `diff` command; use `diff(1)` or `clikit/textdiff` when you care about bytes.

## Before you start

```sh
go get github.com/ubgo/textdata/jsonc@latest
```

`diff` is a package inside the `jsonc` module. You need two parsed `*jsonc.Doc` values. The runnable program for this page is [`examples/jsonc-tools/diff`](../../../examples/jsonc-tools/diff), which compares these two files:

```jsonc
// base.jsonc
{
  // shared defaults
  "server": {"port": 8080, "host": "0.0.0.0"},
  "debug": true,
  "replicas": 1.0,
  "hosts": ["a.example", "b.example"],
  "log.level": "info",
}
```

```jsonc
// prod.jsonc
{
  "replicas": 1,
  "server": {"host": "0.0.0.0", "port": 443, "tls": true},
  "hosts": ["z.example", "a.example", "b.example"],
  "log.level": "warn"
}
```

## Step 1 — List the changes

`diff.Diff(a, b)` returns the changes that turn `a` into `b`. Each `Change` carries:

| Field | Meaning |
|---|---|
| `Kind` | `diff.Added` (only in `b`), `diff.Removed` (only in `a`), `diff.Changed` (in both, different value or kind) |
| `Path` | canonical dotted path: `server.port`, `hosts[2]`, `"log.level"` (a key with a dot is quoted), `""` for the root |
| `Pointer` | the same path as an RFC 6901 JSON Pointer: `/server/port`, `/log.level` |
| `Old`, `New` | the values, decoded like `encoding/json` with `UseNumber` (`map[string]any`, `[]any`, `string`, `json.Number`, `bool`, `nil`); nil on the side that has none |
| `OldPos`, `NewPos` | where the value starts in each file (`pos.Pos`, prints as `line:col`); the zero `Pos` (prints `-`) on the side that has none |

<!-- ds:block id=jt-diff-print-th7pw4gm -->
**printChanges** · [`examples/jsonc-tools/diff/main.go:94-104`](../../../examples/jsonc-tools/diff/main.go#L94-L104)

```go
func printChanges(w io.Writer, a, b *jsonc.Doc) error {
	changes, err := diff.Diff(a, b)
	if err != nil {
		return err // a document with syntax errors is refused
	}
	for _, c := range changes {
		fmt.Fprintf(w, "%-7s %-13s %v -> %v (%s %s, %s %s)\n",
			c.Kind, c.Path, c.Old, c.New, baseName, c.OldPos, prodName, c.NewPos)
	}
	return nil
}
```
<!-- /ds:block hash=a61abd -->

The order is deterministic: depth first, object keys in byte order, array elements by index.

## Step 2 — Group by kind and use the pointer

`Kind` is a closed set of string constants (`"added"`, `"removed"`, `"changed"`), the same strings `textdatactl jsonc diff --json` prints. Switch or group on the constants, never on literals. When you feed a path back into a `jsonc.Doc` method (`Get`, `Set`, `Delete`, `Pos`), use `Pointer`: it is never ambiguous, even for a key that contains a dot or starts with `/`.

<!-- ds:block id=jt-diff-kinds-bxuyq8qf -->
**summarize** · [`examples/jsonc-tools/diff/main.go:114-130`](../../../examples/jsonc-tools/diff/main.go#L114-L130)

```go
func summarize(w io.Writer, a, b *jsonc.Doc) error {
	changes, err := diff.Diff(a, b)
	if err != nil {
		return err
	}
	byKind := map[diff.Kind][]diff.Change{}
	for _, c := range changes {
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	for _, k := range kindOrder {
		fmt.Fprintf(w, "%s: %d\n", k, len(byKind[k]))
		for _, c := range byKind[k] {
			fmt.Fprintf(w, "  %-13s %s\n", c.Path, c.Pointer)
		}
	}
	return nil
}
```
<!-- /ds:block hash=2aa8f4 -->

## Step 3 — Ask "is anything different at all"

When the documents mean the same thing, `Diff` returns an empty, non-nil slice. Reordered keys, other comments, another layout and `1.0` vs `1` all give zero changes.

<!-- ds:block id=jt-diff-count-y4exrkhe -->
**countChanges** · [`examples/jsonc-tools/diff/main.go:136-139`](../../../examples/jsonc-tools/diff/main.go#L136-L139)

```go
func countChanges(a, b *jsonc.Doc) (int, error) {
	changes, err := diff.Diff(a, b)
	return len(changes), err
}
```
<!-- /ds:block hash=2152b6 -->

## Step 4 — Compare values with the same rule

`diff.Equal(x, y)` is the equality rule `Diff` uses, exported for values you decoded yourself the same way (`encoding/json` with `UseNumber`), for example in a test that applies changes and checks the result. Numbers compare by value: two integer literals exactly at any size (no float64 rounding), any other pair as float64. Objects ignore key order; arrays compare by position.

<!-- ds:block id=jt-diff-equal-f2kuyava -->
**compareValues** · [`examples/jsonc-tools/diff/main.go:146-156`](../../../examples/jsonc-tools/diff/main.go#L146-L156)

```go
func compareValues(w io.Writer) {
	pairs := []struct{ x, y any }{
		{json.Number("1"), json.Number("1.0")},
		{json.Number("9007199254740993"), json.Number("9007199254740992")},
		{map[string]any{"a": true, "b": nil}, map[string]any{"b": nil, "a": true}},
		{[]any{"a", "b"}, []any{"b", "a"}},
	}
	for _, p := range pairs {
		fmt.Fprintf(w, "%v == %v: %t\n", p.x, p.y, diff.Equal(p.x, p.y))
	}
}
```
<!-- /ds:block hash=214957 -->

## Try it

```sh
cd examples/jsonc-tools/diff && go run .
```

```text
== base -> prod
removed debug         true -> <nil> (base.jsonc 4:12, prod.jsonc -)
changed hosts[0]      a.example -> z.example (base.jsonc 6:13, prod.jsonc 4:13)
changed hosts[1]      b.example -> a.example (base.jsonc 6:26, prod.jsonc 4:26)
added   hosts[2]      <nil> -> b.example (base.jsonc -, prod.jsonc 4:39)
changed "log.level"   info -> warn (base.jsonc 7:16, prod.jsonc 5:16)
changed server.port   8080 -> 443 (base.jsonc 3:22, prod.jsonc 3:41)
added   server.tls    <nil> -> true (base.jsonc -, prod.jsonc 3:53)
== by kind
added: 2
  hosts[2]      /hosts/2
  server.tls    /server/tls
removed: 1
  debug         /debug
changed: 4
  hosts[0]      /hosts/0
  hosts[1]      /hosts/1
  "log.level"   /log.level
  server.port   /server/port
== base -> same meaning, new layout
changes: 0
== Equal
1 == 1.0: true
9007199254740993 == 9007199254740992: false
map[a:true b:<nil>] == map[a:true b:<nil>]: true
[a b] == [b a]: false
```

The numbers: 7 changes (2 added, 1 removed, 4 changed). `replicas` (`1.0` vs `1`) and `server.host` are absent because they mean the same.

The same from the command line, which exits 1 when the files differ, like `diff(1)`:

```sh
textdatactl jsonc diff base.jsonc prod.jsonc
textdatactl jsonc diff base.jsonc prod.jsonc --json | jq '.data.changes[].path'
```

## Pitfalls

- **Arrays compare by index.** Inserting `z.example` at the front of `hosts` shows as two `changed` entries and one `added` at the end, not one insertion. That keeps the diff cheap, deterministic and always replayable; reorder-aware comparison is not offered.
- **A change of kind is one entry.** When a value turns from an object into an array or a scalar, `Diff` reports one `Changed` at that path with the whole old and new value; it does not descend.
- **Added and removed carry the whole value.** A new section is one `Added` entry whose `New` is a `map[string]any`, not one entry per key inside it.
- **Duplicate keys read last-wins**, like `jsonc.Doc.Get`; an earlier duplicate is invisible to the diff.
- **Use `Pointer`, not `Path`, for lookups.** The dotted `Path` quotes keys with dots (`"log.level"`) and is meant for people; `Pointer` is the form to pass back to `jsonc.Doc` methods.
- **Numbers keep their literal.** `Old`/`New` hold `json.Number`, not `float64`; compare them with `diff.Equal`, not `==` on strings (`"1"` and `"1.0"` are different strings but equal numbers).
- **Invalid input is refused.** A document with syntax errors returns its error (`jsonc.ErrInvalid`); a nil document returns `diff.ErrNilDoc`. An empty document reads as `null`.
- **Plugins and `${VAR}` expansion are not applied.** `Diff` compares the values as written in the files.

## Related

- [Format a JSONC file](format.md), [Generate a JSONC file](generate.md), [Validate with a JSON Schema](schema.md)
- Package reference: [jsonc/diff README](../../../jsonc/diff/README.md) (API, rules, guarantees)
- CLI: [jsonc/cli README § verbs](../../../jsonc/cli/README.md#verbs) (`diff`)
- Example: [`examples/jsonc-tools/diff`](../../../examples/jsonc-tools/diff); the package's own [`Example`](../../../jsonc/diff/example_test.go)
- API: [pkg.go.dev/github.com/ubgo/textdata/jsonc/diff](https://pkg.go.dev/github.com/ubgo/textdata/jsonc/diff)
