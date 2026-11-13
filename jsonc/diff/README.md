# jsonc/diff

Compare two JSONC documents by meaning: which values were added, removed or changed, addressed by path, with the line and column of each side. Comments, layout, key order, trailing commas and number spelling (`1` vs `1.0`) do not count; byte-level comparison is what `diff(1)` is for. `textdatactl jsonc diff` is built on it.

Package `github.com/ubgo/textdata/jsonc/diff`, inside the [jsonc](../README.md) module. Standard library plus the stdlib-only textdata base.

```sh
go get github.com/ubgo/textdata/jsonc
```

## Example

<!-- snippet: jsonc/diff/example_test.go#readme_Diff -->
```go
// Example compares two configs by meaning: comments and layout do not count.
func Example() {
	a, _ := jsonc.Parse([]byte("{\n  // dev port\n  \"port\": 8080,\n  \"debug\": true,\n}"))
	b, _ := jsonc.Parse([]byte(`{"port": 9090, "hosts": ["a"]}`))
	changes, err := diff.Diff(a, b)
	if err != nil {
		panic(err)
	}
	for _, c := range changes {
		fmt.Printf("%s %s %v -> %v (a %s, b %s)\n", c.Kind, c.Path, c.Old, c.New, c.OldPos, c.NewPos)
	}
	// Output:
	// removed debug true -> <nil> (a 4:12, b -)
	// added hosts <nil> -> [a] (a -, b 1:25)
	// changed port 8080 -> 9090 (a 3:11, b 1:10)
}
```

## API

- `Diff(a, b *jsonc.Doc) ([]Change, error)`: the changes that turn `a` into `b`; an empty (non-nil) slice when they mean the same. A document with syntax errors is refused with `jsonc.ErrInvalid`; a nil document with `ErrNilDoc`. An empty document reads as `null`.
- `Change{Kind, Path, Pointer, Old, New, OldPos, NewPos}`: `Kind` is `Added`, `Removed` or `Changed` (string constants, `"added"` etc.). `Path` is the canonical dotted path (`server.hosts[2]`, `"a.b"` quoted, `""` for the root); `Pointer` is the same path as an RFC 6901 JSON Pointer, which is never ambiguous and is the form to pass back to `jsonc.Doc` methods. `Old`/`New` are decoded like `encoding/json` with `UseNumber` (`map[string]any`, `[]any`, `string`, `json.Number`, `bool`, `nil`), nil on the side that has no value. `OldPos`/`NewPos` are where the value starts in each file (`pos.Pos`), the zero `Pos` on the side that has no value.
- `Equal(x, y any) bool`: the equality rule `Diff` uses, for values decoded the same way.

## Rules

- Objects compare key by key; a key on one side only is one `Added` or `Removed` entry carrying its whole value. Duplicate keys read last-wins, like `jsonc.Doc.Get`.
- Arrays compare by index: element `i` against element `i`, extra elements are `Added` or `Removed` at the end. An insertion near the front shows as a run of changes; in return the diff is cheap, deterministic and always applies cleanly.
- A value whose kind differs (object, array, scalar) is one `Changed` entry at that path; the diff does not descend into it.
- Numbers compare by value and never through float rounding: two integer literals compare exactly at any size (`9007199254740993` differs from `9007199254740992`), other pairs as float64.
- Order is deterministic: depth first, object keys in byte order, array elements by index.

## Guarantees

Pinned by tests in this package: table tests for every rule above; a property test that replaying the changes on `a`'s decoded value yields `b`'s for every pair of a document set; a fuzz target (`FuzzDiff`) that `Diff` never panics, refuses invalid input, returns nothing for `Diff(x, x)`, satisfies the replay property, and never changes either document.
