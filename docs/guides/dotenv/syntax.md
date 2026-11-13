# .env syntax

Every rule the `dotenv` parser applies to a line, shown as the line → `Get` (decoded) → `GetExpanded` (references resolved). Read this when a value does not come back the way you wrote it, or before writing a file another tool (Docker Compose, Node dotenv) will also read.

`Get` returns the **decoded** value: quotes removed, escapes turned into characters, multi-line values joined with `\n`. `GetExpanded` returns that value with `$` references resolved, as described in [expansion.md](expansion.md). Showing both side by side is the fastest way to see what each rule does, because quoting decides both how a value is decoded and whether it is expanded.

The parser never fails. A line it cannot read is kept as it is, the rest of the file still parses, and `Render` writes an untouched file back byte for byte.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

Every snippet is a function from the runnable example [`examples/dotenv/syntax`](../../../examples/dotenv/syntax/). It reads in-memory strings with `dotenv.Parse`; `Open(path)` and `ParseReader(r)` apply exactly the same rules. Each step prints through this helper, which uses `%q` so trailing spaces and newlines are visible:

<!-- ds:block id=synshow-q4tz8m2d -->
**show** · [`examples/dotenv/syntax/main.go:56-66`](../../../examples/dotenv/syntax/main.go#L56-L66)

```go
func show(w io.Writer, f *dotenv.File) error {
	for _, key := range f.Keys() {
		raw, _ := f.Get(key)
		expanded, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: Get=%q GetExpanded=%q\n", key, raw, expanded)
	}
	return nil
}
```
<!-- /ds:block hash=103dbb -->

## Step 1 — Pick a quoting style

A value can be written four ways. Pick by what the value contains and whether `$` in it should be a reference:

| Style | Example | Escapes decoded | `$` expanded | Whitespace |
|---|---|---|---|---|
| Bare | `K=hello $NAME` | no | yes | leading and trailing spaces trimmed |
| Double quotes | `K="hello $NAME"` | yes (see [Step 8](#step-8--choose-an-escape-mode)) | yes | kept exactly |
| Single quotes | `K='hello $NAME'` | no | no | kept exactly |
| Backticks | ``K=`hello $NAME` `` | no | no | kept exactly |

`export ` in front of a key is accepted and ignored, and so are spaces or tabs around the `=`. An empty value (`K=` or `K=""`) is a real pair whose value is `""`.

<!-- ds:block id=quoting-c7wn3k5e -->
**quoting** · [`examples/dotenv/syntax/main.go:73-83`](../../../examples/dotenv/syntax/main.go#L73-L83)

```go
func quoting(w io.Writer) error {
	src := "NAME=world\n" +
		"BARE=hello $NAME   \n" +
		"DOUBLE=\"hello $NAME  \"\n" +
		"SINGLE='hello $NAME'\n" +
		"BACKTICK=`it's \"hi\" $NAME`\n" +
		"export EXPORTED=yes\n" +
		"  SPACED  =   padded value  \n" +
		"EMPTY=\n"
	return show(w, dotenv.Parse(src))
}
```
<!-- /ds:block hash=ba3c6f -->

Compare `BARE` and `DOUBLE` in the output: the bare value lost its three trailing spaces, the double-quoted one kept its two. Quote a value whenever its edges matter.

## Step 2 — Write a multi-line value

A double-quoted, single-quoted or backtick value continues across lines until its closing quote. The lines are joined with `\n` (a CRLF file's `\r` is dropped from the value). The whole block is one entry, so editing it rewrites the block as a unit. A bare value always ends at the end of its line. Backticks are the easy way to paste a PEM key or anything else that contains both `'` and `"`.

If the closing quote is missing, the value runs to the end of the file and swallows every line after it. The file still renders back unchanged, but the keys below the broken line disappear from `Get` and `Keys`; the example's `broken` input loses `C` and `D`.

<!-- ds:block id=multiline-v6pb9t1x -->
**multiline** · [`examples/dotenv/syntax/main.go:89-100`](../../../examples/dotenv/syntax/main.go#L89-L100)

```go
func multiline(w io.Writer) error {
	src := "CERT=\"line one\nline two\"\n" +
		"PEM=`-----BEGIN KEY-----\nab'c\"d\n-----END KEY-----`\n"
	if err := show(w, dotenv.Parse(src)); err != nil {
		return err
	}

	broken := "A=1\nB=\"never closed\nC=3\nD=4\n"
	f := dotenv.Parse(broken)
	fmt.Fprintln(w, "unterminated keys:", f.Keys(), "round trip:", f.Render() == broken)
	return nil
}
```
<!-- /ds:block hash=f2a32e -->

## Step 3 — Know where a value ends: comments

A line whose first non-space character is `#` is a comment. In a bare value, [`#` starts an inline comment only when a space or tab comes right before it](ds:block?id=inlinecommentre-5km4985g), so `PASSWORD=a#b` keeps the whole password. Inside any quotes, `#` is part of the value.

After a closing quote the value is finished. Whatever follows on that line, a `# comment` or any other text, is not part of the value: it is kept on disk and re-emitted when the value is edited, but `Get` never returns it (`AFTER="kept" dropped` reads as `kept`).

A comment that holds a complete assignment, such as `# DISABLED=off`, is a **disabled pair**: a setting that is switched off. `Get` does not see it, `Disabled()` lists it and `Restore` turns it back on (see [editing.md](editing.md#step-3--turn-a-setting-off-and-back-on)).

<!-- ds:block id=comments-k2rf5y8j -->
**comments** · [`examples/dotenv/syntax/main.go:107-120`](../../../examples/dotenv/syntax/main.go#L107-L120)

```go
func comments(w io.Writer) error {
	src := "# a full-line comment\n" +
		"PASSWORD=a#b\n" +
		"PORT=8080 # inline comment\n" +
		"QUOTED=\"a # b\" # c\n" +
		"AFTER=\"kept\" dropped\n" +
		"# DISABLED=off\n"
	f := dotenv.Parse(src)
	if err := show(w, f); err != nil {
		return err
	}
	fmt.Fprintln(w, "disabled:", f.Disabled())
	return nil
}
```
<!-- /ds:block hash=2dd049 -->

## Step 4 — Use the `:` delimiter

[`KEY: value`](ds:block?id=pairre-v3u356bt) is read exactly like `KEY=value`, because both Docker Compose's env-file grammar and Node dotenv accept it. A space after the colon is optional, and `export` and quoting work the same way. The author's delimiter is kept when the value is edited; new keys are always written with `=`.

A key cannot contain `:` or `/`, so `URL=http://example.com:8080` is an ordinary `=` pair. The catch is a line that holds **only** a URL: `http://example.com` reads as key `http` with value `//example.com`. That is also how Compose reads it, and the line still renders back unchanged. Comment stray URL lines out with `#`.

<!-- ds:block id=colondelimiter-u8dh4n6q -->
**colonDelimiter** · [`examples/dotenv/syntax/main.go:126-132`](../../../examples/dotenv/syntax/main.go#L126-L132)

```go
func colonDelimiter(w io.Writer) error {
	src := "HOST: db.internal\n" +
		"export PORT:5432\n" +
		"URL=http://example.com:8080\n" +
		"http://example.com\n"
	return show(w, dotenv.Parse(src))
}
```
<!-- /ds:block hash=e12b4c -->

## Step 5 — Name keys, and know what is not a pair

A key may use letters, digits, `_`, `.`, `-`, `[` and `]`, and may start with a digit. That is [Docker Compose's charset](ds:block?id=keyclass-46et9zsb), wider than Node dotenv's and godotenv's, so dotted (`spring.datasource.url`), dashed (`MY-SERVICE-TOKEN`), digit-first (`2FA_SECRET`) and bracketed (`SERVICES[0]`) keys are all real pairs. Keys are case sensitive and ASCII only.

A line that does not fit (a space inside the key, a `!`, no key before the `=`, a non-ASCII letter, free text) becomes a `KindOther` entry. It is not an error: the line is kept verbatim, `Get` ignores it, and the rest of the file parses normally. Docker Compose would refuse such a file; this package keeps it so an editor can still open and save around it. A line that is only a name, such as `HOME` or `export HOME`, is a Compose **inherited** declaration (`KindInherited`); see [entries.md](entries.md).

<!-- ds:block id=keynames-a5mx2c7w -->
**keyNames** · [`examples/dotenv/syntax/main.go:138-150`](../../../examples/dotenv/syntax/main.go#L138-L150)

```go
func keyNames(w io.Writer) error {
	src := "2FA_SECRET=a\n" +
		"spring.datasource.url=b\n" +
		"MY-SERVICE-TOKEN=c\n" +
		"SERVICES[0]=d\n" +
		"HAS KEY=e\n" +
		"=no key\n" +
		"HOME\n"
	for _, e := range dotenv.Parse(src).Entries() {
		fmt.Fprintf(w, "%-9s key=%q raw=%q\n", e.Kind, e.Key, e.Raw[0])
	}
	return nil
}
```
<!-- /ds:block hash=f1a18c -->

Being a valid key does not mean a reference can reach it: `${MY-SERVICE-TOKEN}` is read as a default operator, not a name with dashes. See [expansion.md › Step 5](expansion.md#step-5--know-which-names-a-reference-can-reach).

## Step 6 — Expect the last duplicate to win

When a key appears more than once, the last occurrence wins for `Get`, `GetExpanded`, `Map` and `ExpandedMap`, and `Set` updates that last one. This matches every mainstream dotenv tool and Compose. The earlier lines are not hidden: `Count` reports how many there are and `Pairs` lists them all, so a lint step can flag the mistake.

<!-- ds:block id=duplicates-n3ge6r9b -->
**duplicates** · [`examples/dotenv/syntax/main.go:156-161`](../../../examples/dotenv/syntax/main.go#L156-L161)

```go
func duplicates(w io.Writer) error {
	f := dotenv.Parse("LEVEL=debug\nLEVEL=info\n")
	v, _ := f.Get("LEVEL")
	fmt.Fprintln(w, "Get:", v, "Count:", f.Count("LEVEL"), "Pairs:", f.Pairs())
	return nil
}
```
<!-- /ds:block hash=cd08b2 -->

## Step 7 — Keep `encrypted:` values literal

A value such as `encrypted:BFt4x9` is an ordinary string to the parser. It is stored and returned verbatim; this package never decrypts anything. To decrypt on read, install a value transformer (`WithValueTransform` or a `ValueTransformer` plugin, see [plugins.md](plugins.md#step-1--install-one-function-hooks)); it runs after expansion and also on single-quoted values.

If the ciphertext can contain `$`, single-quote it. A bare value is expanded, so `$cd` in `BARE` is read as a reference to an unset key and vanishes from `GetExpanded`:

<!-- ds:block id=encrypted-w9js1f4h -->
**encrypted** · [`examples/dotenv/syntax/main.go:167-171`](../../../examples/dotenv/syntax/main.go#L167-L171)

```go
func encrypted(w io.Writer) error {
	src := "BARE=encrypted:ab$cd\n" +
		"SINGLE='encrypted:ab$cd'\n"
	return show(w, dotenv.Parse(src))
}
```
<!-- /ds:block hash=43b7cb -->

## Step 8 — Choose an escape mode

[Backslash escapes](ds:block?id=decodeescape-w7neknhj) are decoded **only inside double quotes**. Single-quoted, backtick and bare values keep every backslash as written. Which sequences are decoded depends on the [escape mode](ds:block?id=escapemodes-pqfu7w85) you pass with `WithEscapes`:

| In the file | `EscapeExtended` (default) | `EscapeCompose` |
|---|---|---|
| `\"` | `"` | `"` |
| `\\` | `\` | `\` |
| `\n` | newline | `\n` kept as two characters |
| `\r` | carriage return | `\r` kept |
| `\t` | tab | `\t` kept |
| anything else (`\d`, `\U`, `\$`) | kept, backslash included | kept, backslash included |

Use `EscapeExtended` for files written for Node dotenv, godotenv and most application tooling. Use `EscapeCompose` for files Docker Compose reads, and for values with Windows paths or regexes. The mode is part of parsing, so pass the same option to every `Open`/`Parse` of a file. `EscapeMode` implements `fmt.Stringer`: `extended` and `compose`.

<!-- ds:block id=escapemodes-b4kt7p2m -->
**escapeModes** · [`examples/dotenv/syntax/main.go:184-194`](../../../examples/dotenv/syntax/main.go#L184-L194)

```go
func escapeModes(w io.Writer) error {
	for _, mode := range []dotenv.EscapeMode{dotenv.EscapeExtended, dotenv.EscapeCompose} {
		fmt.Fprintln(w, "mode:", mode)
		f := dotenv.Parse(synEscapes, dotenv.WithEscapes(mode))
		for _, key := range f.Keys() {
			v, _ := f.Get(key)
			fmt.Fprintf(w, "  %s=%q\n", key, v)
		}
	}
	return nil
}
```
<!-- /ds:block hash=850711 -->

The case that bites in practice is `WINPATH="C:\new\temp"`: in the default mode `\n` and `\t` become a newline and a tab.

## Quick reference

Each row is pinned by the example above or by `dotenv/conformance_test.go` and `dotenv/edge_test.go`. `GetExpanded` assumes `NAME=world` is also in the file.

| Line in the file | `Get` | `GetExpanded` |
|---|---|---|
| `K=hello $NAME   ` | `hello $NAME` | `hello world` |
| `K="hello $NAME  "` | `hello $NAME␣␣` | `hello world␣␣` |
| `K='hello $NAME'` | `hello $NAME` | `hello $NAME` |
| ``K=`hello $NAME` `` | `hello $NAME` | `hello $NAME` |
| `export K=v` | `v` | `v` |
| `  K  =  v  ` | `v` | `v` |
| `K=` | (empty) | (empty) |
| `K: v` | `v` | `v` |
| `K=a=b=c` | `a=b=c` | `a=b=c` |
| `K=a#b` | `a#b` | `a#b` |
| `K=a #b` | `a` | `a` |
| `K="a # b" # c` | `a # b` | `a # b` |
| `K="a\nb"` (default mode) | `a`, newline, `b` | same |
| `K="a\nb"` (`EscapeCompose`) | `a\nb` | `a\nb` |
| `K='a\nb'` | `a\nb` | `a\nb` |
| `K=a\nb` (bare) | `a\nb` | `a\nb` |
| `K=encrypted:BFt4x9` | `encrypted:BFt4x9` | `encrypted:BFt4x9` |
| `HAS KEY=v` | not a pair (`KindOther`) | not a pair |
| `http://example.com` | key `http` = `//example.com` | same |

## Try it

```sh
cd examples/dotenv/syntax && go run .
```

```text
== quoting
NAME: Get="world" GetExpanded="world"
BARE: Get="hello $NAME" GetExpanded="hello world"
DOUBLE: Get="hello $NAME  " GetExpanded="hello world  "
SINGLE: Get="hello $NAME" GetExpanded="hello $NAME"
BACKTICK: Get="it's \"hi\" $NAME" GetExpanded="it's \"hi\" $NAME"
EXPORTED: Get="yes" GetExpanded="yes"
SPACED: Get="padded value" GetExpanded="padded value"
EMPTY: Get="" GetExpanded=""
== multi-line
CERT: Get="line one\nline two" GetExpanded="line one\nline two"
PEM: Get="-----BEGIN KEY-----\nab'c\"d\n-----END KEY-----" GetExpanded="-----BEGIN KEY-----\nab'c\"d\n-----END KEY-----"
unterminated keys: [A B] round trip: true
== comments
PASSWORD: Get="a#b" GetExpanded="a#b"
PORT: Get="8080" GetExpanded="8080"
QUOTED: Get="a # b" GetExpanded="a # b"
AFTER: Get="kept" GetExpanded="kept"
disabled: [{DISABLED off}]
== colon delimiter
HOST: Get="db.internal" GetExpanded="db.internal"
PORT: Get="5432" GetExpanded="5432"
URL: Get="http://example.com:8080" GetExpanded="http://example.com:8080"
http: Get="//example.com" GetExpanded="//example.com"
== key names
pair      key="2FA_SECRET" raw="2FA_SECRET=a"
pair      key="spring.datasource.url" raw="spring.datasource.url=b"
pair      key="MY-SERVICE-TOKEN" raw="MY-SERVICE-TOKEN=c"
pair      key="SERVICES[0]" raw="SERVICES[0]=d"
other     key="" raw="HAS KEY=e"
other     key="" raw="=no key"
inherited key="HOME" raw="HOME"
== duplicates
Get: info Count: 2 Pairs: [{LEVEL debug} {LEVEL info}]
== encrypted values
BARE: Get="encrypted:ab$cd" GetExpanded="encrypted:ab"
SINGLE: Get="encrypted:ab$cd" GetExpanded="encrypted:ab$cd"
== escape modes
mode: extended
  NEWLINE="a\nb"
  WINPATH="C:\new\temp"
  QUOTE="say \"hi\""
  SINGLE="a\\nb"
mode: compose
  NEWLINE="a\\nb"
  WINPATH="C:\\new\\temp"
  QUOTE="say \"hi\""
  SINGLE="a\\nb"
```

Change a source string in [`main.go`](../../../examples/dotenv/syntax/main.go) and run it again to see how the parser reads your own input. The module's tested examples for each rule are in [`dotenv/example_syntax_test.go`](../../../dotenv/example_syntax_test.go).

## Pitfalls

- **A missing closing quote eats the rest of the file.** Every key below the broken line stops being a pair, with no error. Check `Keys()` or `Count` after loading a file you did not write.
- **Text after a closing quote is silently ignored.** `K="kept" dropped` gives `kept`. The extra text stays on disk but is not part of the value.
- **Bare values lose their trailing whitespace.** Quote a value whose spaces matter.
- **`#` without a space before it is part of a bare value.** `K=value#note` is the value `value#note`. Put a space before the `#` to make it a comment.
- **`\n` means different things in the two escape modes.** A file written for Docker Compose and read with the default `EscapeExtended` turns `C:\new` into a newline. Choose the mode by who wrote the file.
- **A backslash never escapes `$`.** `"\$NAME"` keeps the backslash and still expands `$NAME`. Write `$$NAME` for a literal `$NAME`, or single-quote the value. See [expansion.md › Step 3](expansion.md#step-3--write-a-literal-dollar).
- **A bare URL line becomes a pair named `http`.** Compose reads it the same way. Comment such lines out.
- **Bad keys do not raise errors.** `HAS KEY=v` is silently a `KindOther` line. To reject malformed files, walk `Entries()` and fail on `KindOther` (see [entries.md](entries.md#step-3--walk-every-entry)).
- **Unquoted ciphertext or passwords containing `$` get expanded.** Single-quote them.

## Related

- [expansion.md](expansion.md): `${VAR}`, operators, `$$`, `RequiredError`, command substitution.
- [reading.md](reading.md) and [entries.md](entries.md): `Open`, `Parse`, `Get`, `Keys`, `Pairs`, `Entries`, `Inherited`.
- [editing.md](editing.md): how quoting is chosen when a value is written back, and `Disabled`/`Restore`.
- [plugins.md](plugins.md): value transformers for `encrypted:` values.
- Module README: [Feature support](../../../dotenv/README.md#feature-support).
- Example: [`examples/dotenv/syntax`](../../../examples/dotenv/syntax/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv](https://pkg.go.dev/github.com/ubgo/textdata/dotenv).
