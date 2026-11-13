# property-harness

A toy `key=value` line format built the way every textdata format is (keep the source bytes, edit by splicing), with its guarantees tested through package `texttest` in `harness_test.go`: `RoundTrip` over `Corpus`, a fuzz target seeded by `Seed`, `CheckUntouched`, `Untouched` catching a line-ending reflow, and `DiffWindow`.

Guide: [texttest](../../../docs/guides/base/texttest.md).

## Run

```sh
cd examples/base/property-harness && go run .
go test .   # runs the program and checks its output
```

The texttest checks live in the tests: `go test -v -run Test .` shows them, `go test -run '^$' -fuzz FuzzRoundTrip -fuzztime 10s .` fuzzes.

## Expected output

```text
before: "# service\r\nhost=db.internal\r\n\r\nport=5432\r\n"
after:  "# service\r\nhost=db.internal\r\n\r\nport=6543\r\n"
replaced bytes [36,40)
```
