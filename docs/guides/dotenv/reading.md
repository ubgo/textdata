# Reading a .env file

How to load a `.env` with `github.com/ubgo/textdata/dotenv`, and what every read accessor returns, including for duplicate keys, commented-out settings and name-only lines. Read this first when a program needs configuration from a `.env`, or when a tool needs to inspect one before editing it.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

The module needs Go 1.22 or later and requires only the textdata base module. Every snippet on this page is a function from the runnable example [`examples/dotenv/reading`](../../../examples/dotenv/reading/), which works in a fresh temp directory.

There are four ways in. Pick by where the content lives and whether you will write it back.

| You have | You want | Call | Missing file |
|---|---|---|---|
| a path | the final values, `${refs}` resolved, read-only | `dotenv.Read(path)` → `map[string]string` | error |
| a path | to inspect, edit and save the file | `dotenv.Open(path)` → `*File` | not an error: an empty `File` |
| a string | to inspect or edit content already in memory | `dotenv.Parse(content)` → `*File` | n/a |
| an `io.Reader` | the same, from a pipe, an HTTP body or an open file | `dotenv.ParseReader(r)` → `*File` | n/a |

`Read` is `Open` followed by `ExpandedMap`, except that a missing file is an error. The other three give you a `*File`, whose accessors return **raw** values: a `${DOMAIN}` reference stays as written, so saving the file never flattens it. For resolved values from a `*File`, use `GetExpanded` or `ExpandedMap` (see [expansion.md](expansion.md)).

None of these touch the process environment. Reading a file and exporting it into the process are separate decisions; the library only does the first.

## Step 1 — Read the values in one call

Use `Read` when a program only consumes configuration. It skips comments, blanks, disabled settings and unrecognised lines, collapses duplicates to the last one, and resolves `${VAR}` references against the same file. Unlike `Open`, a missing file is an error, because `Read` is for a file that should exist.

<!-- ds:block id=readvalues-fe8hnc8t -->
**readValues** · [`examples/dotenv/reading/main.go:85-103`](../../../examples/dotenv/reading/main.go#L85-L103)

```go
func readValues(w io.Writer, path string) error {
	m, err := dotenv.Read(path)
	if err != nil {
		return err
	}
	// A map has no order; sort the keys so the output is stable.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s=%s\n", k, m[k])
	}

	_, err = dotenv.Read(filepath.Join(filepath.Dir(path), "missing.env"))
	fmt.Fprintln(w, "missing file is an error:", err != nil)
	return nil
}
```
<!-- /ds:block hash=4d7bd0 -->

The file it reads is `DOMAIN=example.com`, `API_URL=https://api.${DOMAIN}` and `# DEBUG=true`; the output shows `API_URL` resolved and `DEBUG` absent.

## Step 2 — Open a file you may edit

Use `Open` when you will inspect structure, edit or save. It records the path, the file's permissions and whether it existed. Values come back raw.

A missing file is **not** an error. You get an empty `File`; `Existed` reports `false`, and `Save` creates the file with mode `0600`. That is how a tool bootstraps a fresh `.env`.

<!-- ds:block id=openforedit-7y5rgd8w -->
**openForEdit** · [`examples/dotenv/reading/main.go:110-133`](../../../examples/dotenv/reading/main.go#L110-L133)

```go
func openForEdit(w io.Writer, path string) error {
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	v, ok := f.Get("API_URL")
	fmt.Fprintln(w, v, ok, "existed:", f.Existed())

	fresh, err := dotenv.Open(filepath.Join(filepath.Dir(path), ".env.new"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "missing file existed:", fresh.Existed(), "keys:", len(fresh.Keys()))
	fresh.Set("PORT", "8080")
	if err := fresh.Save(); err != nil {
		return err
	}
	data, err := os.ReadFile(fresh.Path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "created: %q\n", data)
	return nil
}
```
<!-- /ds:block hash=c5836d -->

`Open` returns an error for every other failure: a file it may not read, or a path that is a directory. The message names the path and the failed step (`dotenv: open <path>: …` or `dotenv: read <path>: …`).

## Step 3 — Parse a string or a reader

`Parse` takes a string and never fails: any line it does not understand is kept as an "other" entry rather than rejected (see [entries.md](entries.md)). Rendering an untouched `File` gives back the exact input bytes.

`ParseReader` takes any `io.Reader`. Prefer it over `io.ReadAll` followed by `Parse` for large input: it copies the content once into a `strings.Builder`, while `io.ReadAll` grows its buffer by doubling and the `[]byte` to `string` conversion copies everything again. Its only error is the reader's own, wrapped as `dotenv: read: …`.

<!-- ds:block id=parsecontent-p3m6q2zk -->
**parseContent** · [`examples/dotenv/reading/main.go:140-153`](../../../examples/dotenv/reading/main.go#L140-L153)

```go
func parseContent(w io.Writer) error {
	const content = "# app\nPORT=8080\nexport NAME='my app'\n"
	f := dotenv.Parse(content)
	name, _ := f.Get("NAME")
	fmt.Fprintln(w, "NAME:", name)
	fmt.Fprintln(w, "round trip:", f.Render() == content)

	r, err := dotenv.ParseReader(strings.NewReader("PORT=8080\nHOST=localhost\n"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "keys:", r.Keys())
	return nil
}
```
<!-- /ds:block hash=ca50dd -->

Both accept the same options as `Open` (for example `dotenv.WithEscapes`, or plugins). A `File` from `Parse` or `ParseReader` has an empty `Path` and `Existed() == false`; set `Path` before calling `Save` (see [Step 5](#step-5--check-where-the-file-came-from)).

## Step 4 — Query the file

Six accessors read a `File`. They differ only in how they treat duplicates; all six ignore everything that is not an **active** pair: comments, blanks, unrecognised lines, commented-out settings (`# DB_USER=admin`) and name-only inherited lines (`HOME`). The example queries this input:

```env
# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
```

| Call | Returns | Duplicates | Disabled `DB_USER` | Inherited `HOME` |
|---|---|---|---|---|
| `f.Get("DB_HOST")` | `"db.internal", true` | last line wins | `"", false` | `"", false` |
| `f.Has("DB_HOST")` | `true` | any active line counts | `false` | `false` |
| `f.Count("DB_HOST")` | `2` | counts every active line | `0` | `0` |
| `f.Keys()` | `[DB_HOST DB_PORT]` | each key once, first-seen order | left out | left out |
| `f.Pairs()` | `DB_HOST=localhost`, `DB_PORT=5432`, `DB_HOST=db.internal` | every line, file order | left out | left out |
| `f.Map()` | 2 keys: `DB_HOST` → `db.internal`, `DB_PORT` → `5432` | last line wins | left out | left out |

`Get` returns a value and a found flag; use the flag, because an empty value (`KEY=`) and a missing key both give `""`. `Count` is how you find duplicates: only the last line of a duplicated key is in effect, and the earlier ones are dead weight a reviewer should hear about. `Pairs` is the view a linear read of the file gives; `Map` is the effective view, the same answer `Get` gives for each key.

<!-- ds:block id=queryfile-r9kd2v6c -->
**queryFile** · [`examples/dotenv/reading/main.go:160-176`](../../../examples/dotenv/reading/main.go#L160-L176)

```go
func queryFile(w io.Writer, f *dotenv.File) {
	for _, key := range []string{"DB_HOST", "DB_USER", "HOME"} {
		v, ok := f.Get(key)
		fmt.Fprintf(w, "Get(%s) = %q %v, Has = %v, Count = %d\n", key, v, ok, f.Has(key), f.Count(key))
	}
	fmt.Fprintln(w, "Keys:", f.Keys())
	for _, p := range f.Pairs() {
		fmt.Fprintf(w, "Pairs: %s=%s\n", p.Key, p.Value)
	}
	m := f.Map()
	fmt.Fprintln(w, "Map:", len(m), "keys, DB_HOST =", m["DB_HOST"])
	for _, key := range f.Keys() {
		if n := f.Count(key); n > 1 {
			fmt.Fprintf(w, "duplicate: %s is set %d times\n", key, n)
		}
	}
}
```
<!-- /ds:block hash=a314a8 -->

To see disabled settings and inherited names rather than skip them, use `f.Disabled()`, `f.Inherited()` or walk `f.Entries()` (see [entries.md](entries.md) and [editing.md](editing.md)).

## Step 5 — Check where the file came from

Three facts describe a file's place on disk.

| Fact | `Open` of an existing file | `Open` of a missing file | `Parse` / `ParseReader` |
|---|---|---|---|
| `f.Existed()` | `true` | `false` | `false` |
| `f.Mode()` | the file's own permission bits | `0600` (`-rw-------`) | `0600` |
| `f.Path` | the path passed to `Open` | the path passed to `Open` | `""` |

`Existed` tells a fresh file apart from one already on disk, for example to print "created .env" rather than "updated .env". `Mode` is the permission set `Save` will apply: an existing file keeps its mode, a new one gets `0600`, because a `.env` holds credentials and a group-readable secret is a leak. `Path` is an exported field, not a method: `Save` writes to it, so you can retarget a `File` by assigning it, while `SaveAs` writes elsewhere without changing it (see [saving.md](saving.md)).

<!-- ds:block id=wherefrom-h2xw7b4n -->
**whereFrom** · [`examples/dotenv/reading/main.go:182-205`](../../../examples/dotenv/reading/main.go#L182-L205)

```go
func whereFrom(w io.Writer, dir string) error {
	path := filepath.Join(dir, ".env.shared")
	if err := os.WriteFile(path, []byte("A=1\n"), sharedMode); err != nil {
		return err
	}
	// WriteFile's mode is filtered by the umask; set it exactly.
	if err := os.Chmod(path, sharedMode); err != nil {
		return err
	}
	existing, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	missing, err := dotenv.Open(filepath.Join(dir, ".env.missing"))
	if err != nil {
		return err
	}
	parsed := dotenv.Parse("A=1\n")

	fmt.Fprintln(w, "existing:", existing.Existed(), existing.Mode(), filepath.Base(existing.Path))
	fmt.Fprintln(w, "missing: ", missing.Existed(), missing.Mode(), filepath.Base(missing.Path))
	fmt.Fprintf(w, "parsed:   %v %v %q\n", parsed.Existed(), parsed.Mode(), parsed.Path)
	return nil
}
```
<!-- /ds:block hash=ee88b6 -->

## Try it

```sh
cd examples/dotenv/reading && go run .
```

```text
== Read
API_URL=https://api.example.com
DOMAIN=example.com
missing file is an error: true
== Open
https://api.${DOMAIN} true existed: true
missing file existed: false keys: 0
created: "PORT=8080\n"
== Parse and ParseReader
NAME: my app
round trip: true
keys: [PORT HOST]
== accessors
Get(DB_HOST) = "db.internal" true, Has = true, Count = 2
Get(DB_USER) = "" false, Has = false, Count = 0
Get(HOME) = "" false, Has = false, Count = 0
Keys: [DB_HOST DB_PORT]
Pairs: DB_HOST=localhost
Pairs: DB_PORT=5432
Pairs: DB_HOST=db.internal
Map: 2 keys, DB_HOST = db.internal
duplicate: DB_HOST is set 2 times
== Existed, Mode, Path
existing: true -rw-r----- .env.shared
missing:  false -rw------- .env.missing
parsed:   false -rw------- ""
```

`go test ./dotenv/reading/` (from `examples/`) runs the same program and fails if a line changes. The module's own tested examples cover each accessor one at a time: `ExampleRead`, `ExampleOpen`, `ExampleOpen_missing`, `ExampleParse`, `ExampleParseReader`, `ExampleFile_Get` and the rest in [`dotenv/example_read_test.go`](../../../dotenv/example_read_test.go).

## Cost at scale

The parser holds the whole file in memory. That is deliberate: byte-exact rendering needs every original line, forward `${REF}` references need the entries further down, and insertion needs an index into a complete entry list. `ParseReader` lowers the cost of getting the bytes in; nothing streams.

- **Parsing is linear in the file size.** `TestParse_ScalesLinearly` parses a 1.25 MB and a 5 MB file of short `KEY_n=…` lines and fails if quadrupling the input costs more than 9 times as long (linear is about 4 times, quadratic about 16). It also checks the 5 MB file yields over 100,000 keys and renders back byte for byte. The test is skipped under `go test -short`.
- **`ParseReader` allocates less than `io.ReadAll` then `Parse`.** `BenchmarkParseLarge` parses a file of about 1.4 MB holding one huge quoted value; the figures recorded in the `ParseReader` doc comment are 9.8 MB and 67 allocations, against 13.0 MB and 93.
- **Per-entry cost** is measured by `BenchmarkParseManyEntries` (1 MB of short entries).

Benchmark numbers depend on the machine; re-run them with `cd dotenv && go test -run '^$' -bench . -benchmem .`.

## Pitfalls

- **`Read` returns expanded values; `Open` and `Parse` return raw ones.** Writing a value from `Read` or `ExpandedMap` back with `Set` replaces the `${REF}` with a copy of its target, and the link is lost. Read raw values with `Get` or `Map` when the file will be saved.
- **`Open` of a missing file succeeds.** A typo in the path gives an empty `File`, not an error. Check `f.Existed()` when the file must already exist, or use `Read`, which fails on a missing file.
- **Duplicates are silent.** `Get` and `Map` quietly return the last line. Run `Count` over `Keys()` (as Step 4 does) if a duplicate should be reported.
- **`Get` on a commented-out setting reports not found.** `# DB_USER=admin` is inactive. Use `f.Disabled()` to tell "disabled" apart from "missing".
- **A name-only line is not a value.** `HOME` on its own line declares a variable inherited from the environment. The library never reads the environment, so `Get("HOME")` is `"", false`; use `f.Inherited()` and apply `os.LookupEnv` yourself.
- **A malformed pair disappears from every accessor.** `MY KEY=value` (a space in the key) is an unrecognised line, not an error; walk `Entries()` and flag `KindOther` if you want strictness (see [entries.md](entries.md#pitfalls)).
- **`Parse` gives an empty `Path`.** `Save` on it fails until you assign `f.Path`; or use `SaveAs`.
- **`Read` expansion can fail.** A `${VAR:?message}` whose variable is unset makes `Read` and `ExpandedMap` return an error and no map. See [expansion.md](expansion.md#step-6--fail-loudly-with-requirederror).

## Related

- [entries.md](entries.md): the `Entry` type, the six kinds, walking `Entries()`.
- [syntax.md](syntax.md): quoting, comments and escapes, line by line.
- [expansion.md](expansion.md): `GetExpanded`, `ExpandedMap` and every `${…}` form.
- [editing.md](editing.md) and [saving.md](saving.md): changing the file and writing it back.
- [typed-config.md](typed-config.md): from strings to a typed struct.
- Module README: [Reading](../../../dotenv/README.md#reading).
- Example: [`examples/dotenv/reading`](../../../examples/dotenv/reading/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv](https://pkg.go.dev/github.com/ubgo/textdata/dotenv).
