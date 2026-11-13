# Testing a format's guarantees with `texttest`

Package `github.com/ubgo/textdata/texttest` is the property harness behind the two promises every textdata format keeps: `Parse(x).Render() == x` byte for byte, for every input, valid or not; and an edit changes only the bytes it touches. It gives you a corpus of awkward inputs, a round-trip check, a fuzz seeder, and an "untouched bytes" check that reports exactly where an edit leaked. Use it when you write a new format (YAML, TOML, an INI dialect) or any lossless editor of your own, and run it from `_test.go` files.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

`texttest` is in the base module and imports `testing`, so use it only from test files. The runnable program is [`examples/base/property-harness`](../../../examples/base/property-harness): a toy `key=value` line format in `main.go`, and its guarantees in `harness_test.go`.

| Function | What it checks |
|---|---|
| `RoundTrip(t, input, render)` | `render(input)` returns `input` byte for byte |
| `Corpus` | empty input, BOMs, every line-ending style, NUL bytes, invalid UTF-8, a 70,000-byte line, unbalanced `{ [ " ' /* ${ $(` |
| `Seed(f, extra…)` | adds every `Corpus` entry and your own seeds to a fuzz target |
| `Untouched(before, after, from, to)` | every byte of `before` outside `[from, to)` survives in `after`; returns an error saying where it did not |
| `CheckUntouched(t, …)` | `Untouched`, failing the test on error |
| `DiffWindow(a, b)` | the smallest window outside which `a` and `b` are equal |

## Step 1 — Build the format so the guarantees can hold

A format keeps its source bytes and edits by splicing: find the byte range of the value, replace only that range. Render is then the source itself. The toy format keeps the whole input and never fails to parse:

<!-- ds:block id=doc-pbus3bse -->
**Doc** · [`examples/base/property-harness/main.go:34`](../../../examples/base/property-harness/main.go#L34-L34)

```go
type Doc struct{ src []byte }
```
<!-- /ds:block hash=e479bd -->

<!-- ds:block id=parse-nejpc267 -->
**Parse** · [`examples/base/property-harness/main.go:39`](../../../examples/base/property-harness/main.go#L39-L39)

```go
func Parse(src []byte) *Doc { return &Doc{src: bytes.Clone(src)} }
```
<!-- /ds:block hash=939411 -->

`valueSpan` finds the byte range of a key's value without building a tree:

<!-- ds:block id=doc-valuespan-mxcapbk5 -->
**Doc.valueSpan** · [`examples/base/property-harness/main.go:47-60`](../../../examples/base/property-harness/main.go#L47-L60)

```go
func (d *Doc) valueSpan(key string) (from, to int, ok bool) {
	for start := 0; start < len(d.src); {
		end := bytes.IndexByte(d.src[start:], '\n')
		if end < 0 {
			end = len(d.src) - start
		}
		line := bytes.TrimRight(d.src[start:start+end], "\r")
		if k, _, found := bytes.Cut(line, []byte{separator}); found && string(k) == key {
			return start + len(k) + 1, start + len(line), true
		}
		start += end + 1
	}
	return 0, 0, false
}
```
<!-- /ds:block hash=9fc839 -->

`Set` replaces the value's bytes and returns the range of the old source it replaced, which is exactly what the untouched check needs:

<!-- ds:block id=doc-set-m9smkbsq -->
**Doc.Set** · [`examples/base/property-harness/main.go:66-73`](../../../examples/base/property-harness/main.go#L66-L73)

```go
func (d *Doc) Set(key, value string) (from, to int, ok bool) {
	from, to, ok = d.valueSpan(key)
	if !ok {
		return 0, 0, false
	}
	d.src = append(d.src[:from:from], append([]byte(value), d.src[to:]...)...)
	return from, to, true
}
```
<!-- /ds:block hash=f87d73 -->

## Step 2 — Round-trip the corpus

Run `RoundTrip` over `texttest.Corpus` plus the inputs your own parser treats specially. `render` gets the input and returns what your format renders after parsing it.

<!-- ds:block id=testroundtrip-yatrkq2c -->
**TestRoundTrip** · [`examples/base/property-harness/harness_test.go:17-22`](../../../examples/base/property-harness/harness_test.go#L17-L22)

```go
func TestRoundTrip(t *testing.T) {
	render := func(src []byte) ([]byte, error) { return Parse(src).Render(), nil }
	for _, in := range append(texttest.Corpus, formatSeeds...) {
		texttest.RoundTrip(t, []byte(in), render)
	}
}
```
<!-- /ds:block hash=4a30d7 -->

## Step 3 — Fuzz the round trip

`Seed` adds the corpus and your seeds to a fuzz target that takes one `[]byte`; `go test -fuzz` then mutates them. A failing input is saved under `testdata/fuzz/` and replayed by every later `go test`.

<!-- ds:block id=fuzzroundtrip-56575zwy -->
**FuzzRoundTrip** · [`examples/base/property-harness/harness_test.go:28-33`](../../../examples/base/property-harness/harness_test.go#L28-L33)

```go
func FuzzRoundTrip(f *testing.F) {
	texttest.Seed(f, formatSeeds...)
	f.Fuzz(func(t *testing.T, src []byte) {
		texttest.RoundTrip(t, src, func(b []byte) ([]byte, error) { return Parse(b).Render(), nil })
	})
}
```
<!-- /ds:block hash=d0ce41 -->

## Step 4 — Check an edit leaves other bytes alone

`CheckUntouched(t, before, after, from, to)` passes when `after` is `before[:from] + anything + before[to:]`. Pass the range your edit replaced.

<!-- ds:block id=testsetleavesotherbytes-sraxrcv2 -->
**TestSetLeavesOtherBytes** · [`examples/base/property-harness/harness_test.go:39-47`](../../../examples/base/property-harness/harness_test.go#L39-L47)

```go
func TestSetLeavesOtherBytes(t *testing.T) {
	before := []byte(sample)
	d := Parse(before)
	from, to, ok := d.Set("port", "6543")
	if !ok {
		t.Fatal("port not found")
	}
	texttest.CheckUntouched(t, before, d.Render(), from, to)
}
```
<!-- /ds:block hash=245933 -->

`Untouched` returns the same verdict as an error, so you can also assert that a buggy edit is caught. `reflow` sets a value correctly but also converts CRLF to LF, the kind of change a careless editor makes:

<!-- ds:block id=testuntouchedcatchesreflow-2cczecty -->
**TestUntouchedCatchesReflow** · [`examples/base/property-harness/harness_test.go:53-61`](../../../examples/base/property-harness/harness_test.go#L53-L61)

```go
func TestUntouchedCatchesReflow(t *testing.T) {
	before := []byte(sample)
	from, to, _ := Parse(before).Set("port", "6543")
	err := texttest.Untouched(before, reflow(before, "port", "6543"), from, to)
	if err == nil {
		t.Fatal("a reflowing edit passed the untouched check")
	}
	t.Logf("caught: %v", err)
}
```
<!-- /ds:block hash=82a3df -->

## Step 5 — Measure an edit's footprint

`DiffWindow(a, b)` returns `start, endA, endB` with `a[:start] == b[:start]` and `a[endA:] == b[endB:]`. Use it to assert that an edit's footprint is no wider than the value it replaced, or to print a short diff in a failure message.

<!-- ds:block id=testdiffwindow-nqg6djv4 -->
**TestDiffWindow** · [`examples/base/property-harness/harness_test.go:67-76`](../../../examples/base/property-harness/harness_test.go#L67-L76)

```go
func TestDiffWindow(t *testing.T) {
	before := []byte(sample)
	d := Parse(before)
	from, to, _ := d.Set("port", "6543")
	start, endA, endB := texttest.DiffWindow(before, d.Render())
	if start < from || endA > to {
		t.Fatalf("diff window [%d,%d) is wider than the edit [%d,%d)", start, endA, from, to)
	}
	t.Logf("edit [%d,%d), diff window before[%d:%d] after[%d:%d]", from, to, start, endA, start, endB)
}
```
<!-- /ds:block hash=35ad3b -->

## Try it

```sh
cd examples/base/property-harness && go run . && go test -v -run Test .
```

```text
before: "# service\r\nhost=db.internal\r\n\r\nport=5432\r\n"
after:  "# service\r\nhost=db.internal\r\n\r\nport=6543\r\n"
replaced bytes [36,40)
=== RUN   TestRoundTrip
--- PASS: TestRoundTrip (0.00s)
=== RUN   TestSetLeavesOtherBytes
--- PASS: TestSetLeavesOtherBytes (0.00s)
=== RUN   TestUntouchedCatchesReflow
    harness_test.go:60: caught: texttest: bytes before the edit changed at offset 9
--- PASS: TestUntouchedCatchesReflow (0.00s)
=== RUN   TestDiffWindow
    harness_test.go:75: edit [36,40), diff window before[36:40] after[36:40]
--- PASS: TestDiffWindow (0.00s)
=== RUN   TestRun
--- PASS: TestRun (0.00s)
PASS
ok  	github.com/ubgo/textdata/examples/base/property-harness	0.005s
```

To fuzz for a few seconds:

```sh
go test -run '^$' -fuzz FuzzRoundTrip -fuzztime 3s .
```

## Pitfalls

- **Pass the range in `before`'s offsets.** `from` and `to` are positions in the original bytes, the range the edit replaced, not where the new value ends in `after`. Return that range from your edit function, as `Set` does here.
- **`Untouched` checks bytes, not meaning.** An edit that re-indents a sibling, normalises line endings or re-sorts keys fails, even though the data is the same. That is the point: a file a person wrote must not change where nobody asked.
- **`RoundTrip` needs a parser that accepts everything.** If your `Parse` rejects invalid input, render the error case from the source too (keep the bytes and an error list), or the corpus entries that are not valid in your format will fail. textdata's formats parse invalid input and report errors alongside.
- **The corpus is generic.** It covers encodings, line endings and unbalanced delimiters, not your grammar. Add seeds for your format's own edge cases (quoted keys, continuation lines, empty values) through `Seed`'s `extra` and your round-trip loop.
- **Fuzzing runs only with `-fuzz`.** A plain `go test` replays the seeds and saved failures, not new inputs. Run the fuzz target for a while before a release.
- **Keep `texttest` out of non-test code.** It imports `testing`; importing it from a program pulls the testing package into the binary.

## Related

- [Root README § Guarantees every format keeps](../../../README.md#guarantees-every-format-keeps) and [§ texttest](../../../README.md#texttest).
- [jsonc README § Conformance and tests](../../../jsonc/README.md#conformance-and-tests), [dotenv README § Testing](../../../dotenv/README.md#testing): how the shipped formats use the harness.
- [Paths and positions](pos-and-path.md): the other base packages a new format needs.
- Example: [`examples/base/property-harness`](../../../examples/base/property-harness).
- API: [texttest on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/texttest).
