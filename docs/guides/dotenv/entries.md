# Entries and kinds

How the `dotenv` library sees a file line by line: the `Entry` type, the six `Kind` values, and how to walk every entry, comments and blanks included. You want this when the read accessors are not enough: a linter that flags disabled settings or unrecognised lines, a tool that says "DB_USER is disabled" rather than "DB_USER is missing", or a viewer that shows comments next to values.

A parsed `File` is a list of entries in file order. An entry is one logical unit: a pair (which can span several physical lines when a quoted value runs on), a comment line, a blank line, or a line the parser does not recognise. Nothing is dropped, which is why an untouched file renders back byte for byte. The accessors in [reading.md](reading.md) (`Get`, `Keys`, `Map` and the rest) only see active pairs; `f.Entries()` sees everything.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

You need a `*dotenv.File` from `Open`, `Parse` or `ParseReader` (see [reading.md](reading.md)). Every snippet is a function from the runnable example [`examples/dotenv/entries`](../../../examples/dotenv/entries/).

## Step 1 — Know the six kinds

[`Kind`](ds:block?id=kinds-77m444t8) is a closed set: the parser produces nothing else.

| Kind | `String()` | A line that produces it | Active? | `Key` | `Value` |
|---|---|---|---|---|---|
| `KindPair` | `pair` | `PORT=8080`, `PORT: 8080`, `export PORT=8080` | yes | set | decoded value |
| `KindComment` | `comment` | `# a note` (first non-space character is `#`, and the body is not a pair) | no | empty | empty |
| `KindBlank` | `blank` | an empty or whitespace-only line | no | empty | empty |
| `KindOther` | `other` | `not a setting!`, `MY KEY=value`: anything not recognised | no | empty | empty |
| `KindDisabledPair` | `disabled-pair` | `# DEBUG=true`: a comment whose body parses as a pair on one line (a `:` pair only when its value is one token; see [Pitfalls](#pitfalls)) | no | set | decoded value |
| `KindInherited` | `inherited` | `export AWS_REGION` or `HOME`: a name with no delimiter and no value | no | set | empty |

"Active" means `Get`, `Has`, `Count`, `Keys`, `Pairs` and `Map` see it. Only `KindPair` is active. `Kind` implements `fmt.Stringer`, so `%s` and `fmt.Println` print its name instead of an integer, which keeps logs and test failures readable; a value outside the six prints as `Kind(n)`.

<!-- ds:block id=printkinds-s8vq3j6t -->
**printKinds** · [`examples/dotenv/entries/main.go:61-67`](../../../examples/dotenv/entries/main.go#L61-L67)

```go
func printKinds(w io.Writer) {
	f := dotenv.Parse(everyKind)
	for _, e := range f.Entries() {
		fmt.Fprintf(w, "%-13s key=%-12q active=%v\n", e.Kind, e.Key, e.Kind == dotenv.KindPair)
	}
	fmt.Fprintln(w, "out of range:", dotenv.Kind(99))
}
```
<!-- /ds:block hash=9b3988 -->

What each kind is for:

- **`KindPair`** is a setting. The `export` prefix and the delimiter (`=` or `:`) are kept when you edit the value.
- **`KindComment`** is prose, kept verbatim.
- **`KindBlank`** is kept verbatim, whitespace included.
- **`KindOther`** is a line the parser does not understand. It is preserved rather than rejected, so one stray line never makes a file unopenable. The cost is that a malformed pair is invisible to `Get` and friends instead of reported.
- **`KindDisabledPair`** is a setting somebody turned off, by hand or with `Unset`. `f.Disabled()` lists them and `f.Restore(key)` turns one back on (see [editing.md](editing.md#step-3--turn-a-setting-off-and-back-on)).
- **`KindInherited`** is the Compose way to say "take this variable from the process environment". The library records the name but never reads the environment; `f.Inherited()` lists the names so your program can decide.

## Step 2 — Read an entry's fields

`Entry` has four exported fields:

| Field | Type | Holds |
|---|---|---|
| `Kind` | `Kind` | which of the six kinds this is |
| `Raw` | `[]string` | the exact physical lines, without line terminators (a CRLF file keeps its `\r`); one element per line, several for a multi-line value |
| `Key` | `string` | the variable name, for `KindPair`, `KindDisabledPair` and `KindInherited`; empty otherwise |
| `Value` | `string` | the decoded value, for `KindPair` and `KindDisabledPair`: quotes stripped, escapes interpreted, lines of a multi-line value joined with `\n`; empty otherwise |

`Value` is raw in the expansion sense: `${REF}` is not resolved. Everything else the library needs to edit an entry without disturbing it (the `export` prefix, spacing, the delimiter, the inline comment, the quote style, the comment marker of a disabled pair) is kept in unexported fields.

<!-- ds:block id=printfields-g5hn2w9r -->
**printFields** · [`examples/dotenv/entries/main.go:74-79`](../../../examples/dotenv/entries/main.go#L74-L79)

```go
func printFields(w io.Writer) {
	f := dotenv.Parse("URL=\"https://${HOST}/v1\"\nCERT=\"line one\nline two\"\n")
	for _, e := range f.Entries() {
		fmt.Fprintf(w, "%s %s: Raw=%q Value=%q\n", e.Kind, e.Key, e.Raw, e.Value)
	}
}
```
<!-- /ds:block hash=2f7341 -->

Treat entries from `Entries()` as read-only. The slice and the entries are the file's own, not copies; change the file through `Set`, `Unset`, `Restore`, `Append` and `Insert*` so `Raw` stays in step with `Key` and `Value` (see [editing.md](editing.md)).

## Step 3 — Walk every entry

`f.Entries()` returns every entry in file order. Switch on `Kind` to handle each one; listing all six cases makes a missing one visible in review. For a quick list of one kind you do not need to walk: `f.Disabled()` returns the disabled pairs as `[]Pair`, and `f.Inherited()` the inherited names (once each), both in file order.

<!-- ds:block id=lint-x7cm4b1z -->
**lint** · [`examples/dotenv/entries/main.go:86-102`](../../../examples/dotenv/entries/main.go#L86-L102)

```go
func lint(w io.Writer, f *dotenv.File) {
	for i, e := range f.Entries() {
		switch e.Kind {
		case dotenv.KindPair:
			fmt.Fprintf(w, "entry %d: %s is set (%d line(s))\n", i, e.Key, len(e.Raw))
		case dotenv.KindDisabledPair:
			fmt.Fprintf(w, "entry %d: %s is disabled\n", i, e.Key)
		case dotenv.KindInherited:
			fmt.Fprintf(w, "entry %d: %s comes from the environment\n", i, e.Key)
		case dotenv.KindOther:
			fmt.Fprintf(w, "entry %d: not a setting: %q\n", i, e.Raw[0])
		case dotenv.KindComment, dotenv.KindBlank:
			// Prose and spacing: nothing to report.
		}
	}
	fmt.Fprintln(w, "Disabled():", f.Disabled(), "Inherited():", f.Inherited())
}
```
<!-- /ds:block hash=bae2ea -->

The multi-line `CERT` value is one entry with two `Raw` lines, which is why the walk reports entry 5 and not entries 5 and 6.

## Step 4 — Check the lines that surprise people

Each of these lines is parsed as documented below, and each still renders back byte for byte. They are surprising only if you expect a different reading.

<!-- ds:block id=surprises-d2pw8k5v -->
**surprises** · [`examples/dotenv/entries/main.go:109-121`](../../../examples/dotenv/entries/main.go#L109-L121)

```go
func surprises(w io.Writer) {
	src := "# TODO=fix this\n" +
		"# Note: rotate monthly\n" +
		"# KEY: value\n" +
		"# http://example.com\n" +
		"http://example.com\n" +
		"MY KEY=value\n" +
		"# CERT=\"one\n" +
		"# two\"\n"
	for _, e := range dotenv.Parse(src).Entries() {
		fmt.Fprintf(w, "%-13s key=%-7q %q\n", e.Kind, e.Key, e.Raw[0])
	}
}
```
<!-- /ds:block hash=a1149d -->

## Try it

```sh
cd examples/dotenv/entries && go run .
```

```text
== kinds
pair          key="PORT"       active=true
comment       key=""           active=false
blank         key=""           active=false
other         key=""           active=false
disabled-pair key="DEBUG"      active=false
inherited     key="AWS_REGION" active=false
out of range: Kind(99)
== fields
pair URL: Raw=["URL=\"https://${HOST}/v1\""] Value="https://${HOST}/v1"
pair CERT: Raw=["CERT=\"line one" "line two\""] Value="line one\nline two"
== lint
entry 1: PORT is set (1 line(s))
entry 2: DB_USER is disabled
entry 3: not a setting: "MY KEY=value"
entry 4: HOME comes from the environment
entry 5: CERT is set (2 line(s))
Disabled(): [{DB_USER admin}] Inherited(): [HOME]
== surprises
disabled-pair key="TODO"  "# TODO=fix this"
comment       key=""      "# Note: rotate monthly"
disabled-pair key="KEY"   "# KEY: value"
comment       key=""      "# http://example.com"
pair          key="http"  "http://example.com"
other         key=""      "MY KEY=value"
comment       key=""      "# CERT=\"one"
comment       key=""      "# two\""
```

More tested examples: `ExampleFile_Entries`, `ExampleKind`, `ExampleKind_pitfalls`, `ExampleKind_String` and `ExampleEntry` in [`dotenv/example_read_test.go`](../../../dotenv/example_read_test.go).

## Pitfalls

- **Prose shaped like an assignment is a disabled setting.** `# TODO=fix this` becomes `KindDisabledPair` with key `TODO`. This is deliberate: a human skimming the file reads it the same way. It shows up in `f.Disabled()`, and `f.Restore("TODO")` would turn it into a live pair.
- **A commented colon line is a setting only when its value is one token.** Inside a comment the `:` form [counts as a disabled pair](ds:block?id=colonprose-cthvdumn) only when the value is quoted or a single word and does not start with `//`. So `# Note: rotate monthly` and `# http://example.com` are comments, while `# KEY: value`, `# KEY: "a b"` and `# DB_URL: postgres://db/app` are disabled pairs. The `=` form is not judged this way. Two consequences: a one-word heading such as `# Owner: alice` still reads as a disabled pair, and an unquoted colon value with a space, `KEY: two words`, cannot be told apart from prose, so after `Unset`, `Save` and a reload it is a plain comment that `Restore` cannot find (within one `File`, `Unset` then `Restore` still works). Quote such a value, or write it with `=`, if it must survive being commented out.
- **A bare URL line is a pair.** `http://example.com` on its own line parses as key `http`, value `//example.com`. Docker Compose reads it the same way, so the library does not special-case it (pinned by `TestColonDelimiter`).
- **A key with a space is not an error.** `MY KEY=value` is `KindOther`: kept, but invisible to `Get`, `Keys` and `Map`. For strict parsing, walk `Entries()` and treat `KindOther` as an error yourself, as Step 3 does.
- **A commented-out multi-line value is several comments.** Only a disabled pair whose quote closes on the same line is `KindDisabledPair`; `# CERT="one` followed by `# two"` is two `KindComment` entries, so it cannot be restored from a re-read file.
- **`KindInherited` is never resolved.** `HOME` has a `Key` but no `Value`, and a `${HOME}` reference to it expands to `""` like any unset variable. Apply `os.LookupEnv` over `f.Inherited()` if you want Compose's behaviour.
- **`Raw` and `Value` differ for quoted and multi-line values.** Compare against `Value` for the setting, against `Raw` for the bytes in the file.

## Related

- [reading.md](reading.md): `Read`, `Open`, `Parse` and the read accessors.
- [editing.md](editing.md): `Unset`, `Restore`, `Disabled`, `Inherited` and the entry constructors.
- [syntax.md](syntax.md): every quoting, comment and escape rule.
- [plugins.md](plugins.md#step-4--watch-with-observers): an `EntryObserver` sees each entry while the file is parsed.
- Module README: [Disabled settings](../../../dotenv/README.md#disabled-settings) and [Inherited declarations](../../../dotenv/README.md#inherited-declarations).
- Example: [`examples/dotenv/entries`](../../../examples/dotenv/entries/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv#Entry](https://pkg.go.dev/github.com/ubgo/textdata/dotenv#Entry).
