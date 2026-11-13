# Editing a .env file

Change values, place new keys beside related ones, comment settings out and back in, generate whole files, and derive one environment's file from another, while every line you did not touch keeps its exact bytes. You want this for any tool that writes a `.env` a person also edits: a setup script, a secret rotator, a CLI like `dotenvctl set`.

Every edit works on a `*dotenv.File` in memory. Nothing reaches disk until you call `Save` or `SaveAs` ([saving.md](saving.md)).

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

You need a `*dotenv.File`: `dotenv.Open(path)` for a file on disk (a missing file opens empty, ready to create) or `dotenv.Parse(content)` for a string ([reading.md](reading.md)). Steps 1 to 6 are functions from [`examples/dotenv/editing`](../../../examples/dotenv/editing/), which uses `Parse` and never touches disk; Step 7 is from [`examples/dotenv/clone-env`](../../../examples/dotenv/clone-env/). Steps 1 to 4 edit this file:

```env
# app
export PORT = 8080 # dev only
HOST=localhost
DB_HOST=db.internal
#DEBUG=true
```

The package has two families of write methods. Pick by what you are doing:

| Family | Methods | What it does | Use it for |
|---|---|---|---|
| Upsert | `Set`, `SetAfter`, `SetBefore` | Keeps one active entry per key. Updates in place if the key exists, adds it if not. | Editing a file a person wrote. |
| Literal | `Append`, `InsertAfter`, `InsertBefore` | Places exactly the entries you pass, duplicates included. | Generating a file: banners, blank lines, sections. |

## Step 1 — Change or add a value with Set

[`Set(key, value)`](ds:block?id=set-jusdwzq7) updates the key where it is, or appends `KEY=value` at the end when the key is missing. It returns `created`, which is `true` only when it appended. When a key appears more than once, `Set` changes the last one, because that is the one a reader gets.

What `Set` keeps when it changes a value: the `export` prefix, the spacing on both sides of the delimiter, the delimiter itself (a Compose-style `:` pair is never rewritten to `=`), the inline comment (`# dev only`), and every other line byte for byte. Setting the value a key already has is a true no-op, even when the source used quotes or odd spacing, so a tool can call `Set` on every run without producing a diff.

`Set` chooses the quoting of a changed value. The value is bare when it reads back the same; double-quoted when it contains a space, a tab, `#`, any quote character or a backslash; a double-quoted block across several lines when it contains a newline. A value containing `$` is [stored literally](ds:block?id=quotingtriggers-xsur5zum): in single quotes, or in backticks when it also contains a single quote. Written bare, `p$$w` would read back as `p$w`. The one value that cannot be stored literally holds `$`, a single quote and a backtick together; it is double-quoted and its `$` stays a reference. Whichever quoting `Set` picks, `GetExpanded` gives the same answer before and after the file is saved and read again.

<!-- ds:block id=setvalues-m6tc9w2k -->
**setValues** · [`examples/dotenv/editing/main.go:61-76`](../../../examples/dotenv/editing/main.go#L61-L76)

```go
func setValues(w io.Writer) {
	f := dotenv.Parse(app)
	created := f.Set("PORT", "9090")
	fmt.Fprintln(w, "PORT created:", created)
	created = f.Set("LOG_LEVEL", "info")
	fmt.Fprintln(w, "LOG_LEVEL created:", created)

	before := f.Render()
	f.Set("HOST", "localhost")
	fmt.Fprintln(w, "same value is a no-op:", f.Render() == before)

	f.Set("GREETING", "hello world")
	f.Set("PASSWORD", "p$$w")
	f.Set("MOTD", "line one\nline two")
	fmt.Fprint(w, f.Render())
}
```
<!-- /ds:block hash=c4e4cb -->

A commented-out key is not active, so `Set` on it appends a new live entry and leaves the comment alone. Use `Restore` ([Step 3](#step-3--turn-a-setting-off-and-back-on)) to bring back the commented line itself.

## Step 2 — Place a new key next to a related one

`Set` always appends new keys at the end. To keep related settings together, name an anchor key that is already in the file: `SetAfter(anchor, key, value)` and `SetBefore(anchor, key, value)` put a new key right after or before it. A key that already exists is updated where it is and is not moved, because moving it would rewrite two places in the file for a one-value change. When the anchor is a multi-line value, the new key goes after the whole block, never inside it.

If the anchor has no active entry, both return an error that names the anchor and wraps [`dotenv.ErrAnchorNotFound`](ds:block?id=erranchornotfound-7696gb9k) and change nothing. They do not fall back to appending, because then the call would report success while ignoring the one thing you asked for. Check with `errors.Is` and decide yourself whether to fall back to `Set`:

<!-- ds:block id=placekeys-y3fd7h5n -->
**placeKeys** · [`examples/dotenv/editing/main.go:83-98`](../../../examples/dotenv/editing/main.go#L83-L98)

```go
func placeKeys(w io.Writer) error {
	f := dotenv.Parse(app)
	if _, err := f.SetAfter("DB_HOST", "DB_PORT", "5432"); err != nil {
		return err
	}
	if _, err := f.SetBefore("PORT", "APP_NAME", "shop"); err != nil {
		return err
	}
	_, err := f.SetAfter("NOPE", "CACHE_URL", "redis://cache")
	if errors.Is(err, dotenv.ErrAnchorNotFound) {
		fmt.Fprintln(w, "anchor missing:", err)
		f.Set("CACHE_URL", "redis://cache")
	}
	fmt.Fprint(w, f.Render())
	return nil
}
```
<!-- /ds:block hash=9579ca -->

## Step 3 — Turn a setting off and back on

[`Unset(key, del)`](ds:block?id=unset-ahj88kj2) deactivates the last active entry for `key` and reports whether it found one. With `del = false` it comments out every physical line of the entry [with `# `](ds:block?id=commentmarker-6gh5ejhx): the setting stays in the file, readable and reversible. With `del = true` it removes the entry. Calling it again on a key that is already commented out returns `false`.

`Disabled()` lists commented-out settings (`KindDisabledPair`, whether a person or `Unset` commented them out) as `[]dotenv.Pair` in file order; a plain comment such as `# just a note` is not listed. That lets a tool say "DB_USER is disabled" instead of "DB_USER is missing", which are different problems with different fixes.

[`Restore(key)`](ds:block?id=restore-jyvxum4p) re-activates the last commented-out entry for `key`. It removes the exact comment marker recorded for that line, so a line written by hand as `#DEBUG=true` comes back as `DEBUG=true` without inventing or removing a space, and an indented `   # KEY=v` keeps its indentation. `Unset(key, false)` followed by `Restore(key)` gives back the original bytes.

<!-- ds:block id=toggle-r8kb2q6v -->
**toggle** · [`examples/dotenv/editing/main.go:105-116`](../../../examples/dotenv/editing/main.go#L105-L116)

```go
func toggle(w io.Writer) {
	f := dotenv.Parse(app)
	fmt.Fprintln(w, "Unset HOST:", f.Unset("HOST", false))
	fmt.Fprintln(w, "Unset HOST again:", f.Unset("HOST", false))
	fmt.Fprintln(w, "Unset DB_HOST, delete:", f.Unset("DB_HOST", true))
	fmt.Fprintln(w, "Disabled:", f.Disabled())
	fmt.Fprint(w, f.Render())

	fmt.Fprintln(w, "Restore HOST:", f.Restore("HOST"))
	fmt.Fprintln(w, "Restore DEBUG:", f.Restore("DEBUG"))
	fmt.Fprint(w, f.Render())
}
```
<!-- /ds:block hash=5e9b7c -->

## Step 4 — Handle inherited declarations

Docker Compose's env-file format allows a line that is only a name, such as `HOME`; to Compose it means "take this variable from the environment". This package records such lines as `KindInherited` but never resolves them, because it never reads the process environment. `Inherited()` returns the declared names in file order, without duplicates, and leaves the decision to you. In a real program, replace the fixed map with `os.LookupEnv` or whatever source your app trusts.

<!-- ds:block id=inherited-j4wz6c8p -->
**inherited** · [`examples/dotenv/editing/main.go:123-136`](../../../examples/dotenv/editing/main.go#L123-L136)

```go
func inherited(w io.Writer) error {
	f := dotenv.Parse("PORT=8080\nHOME\nexport AWS_REGION\nHOME\n")
	source := map[string]string{"HOME": "/home/app"}
	for _, name := range f.Inherited() {
		v, ok := source[name]
		fmt.Fprintf(w, "%s: %q found=%v\n", name, v, ok)
	}
	v, _, err := f.GetExpanded("HOME")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "GetExpanded(HOME) = %q\n", v)
	return nil
}
```
<!-- /ds:block hash=43cc18 -->

Inherited names are not active keys. A `${HOME}` reference to one expands to `""` like any other unset variable, unless a lookup plugin supplies it.

## Step 5 — Generate a file from entries

Three constructors build entries that are not in any file yet:

| Constructor | Builds |
|---|---|
| `NewPair(key, value)` | A `KEY=value` entry. Always uses `=`. Quoting follows the same rules as `Set`. |
| `NewComment(lines...)` | One comment line per argument. Plain text gets `# `; a line that already starts with `#` (after optional spaces) is kept as is. No arguments gives a bare `#`. |
| `NewBlank()` | One empty line, for separating sections. |

`Append(entries...)` adds them at the end, exactly as given. `InsertAfter(anchor, entries...)` and `InsertBefore(anchor, entries...)` place a block next to an anchor key as one unit; a missing anchor returns `ErrAnchorNotFound` and changes nothing.

A constructed entry has no rendered text until it is placed, because its line ending comes from the destination file: the same `NewPair` renders with `\n` in one file and `\r\n` in a CRLF file. An entry taken from another file's `Entries()` is already attached, so it is copied verbatim: quotes, spacing, inline comment and its own line ending all come along. That is the way to write a reference such as `${HOST}` into a file, since `Set` stores a value containing `$` literally.

<!-- ds:block id=generate-e9ts3m7f -->
**generate** · [`examples/dotenv/editing/main.go:144-162`](../../../examples/dotenv/editing/main.go#L144-L162)

```go
func generate(w io.Writer) error {
	f := dotenv.Parse("")
	f.Append(
		dotenv.NewComment("Copy to .env and fill in.", "# Do not commit .env."),
		dotenv.NewBlank(),
		dotenv.NewPair("HOST", "localhost"),
		dotenv.NewPair("GREETING", "hello world"),
	)
	ref := dotenv.Parse("URL=https://${HOST}/v1 # derived\n").Entries()[0]
	if err := f.InsertAfter("HOST", dotenv.NewComment("derived"), ref); err != nil {
		return err
	}
	fmt.Fprint(w, f.Render())

	crlf := dotenv.Parse("A=1\r\n")
	crlf.Append(dotenv.NewPair("B", "2"))
	fmt.Fprintf(w, "CRLF file: %q\n", crlf.Render())
	return nil
}
```
<!-- /ds:block hash=9a966a -->

## Step 6 — Keep upsert and literal apart

Mixing the two families up is how a key silently ends up twice in a file. `Append` and `Insert*` add a duplicate if the key already exists; only the last one is in effect when the file is read. Use `Set`, `SetAfter` or `SetBefore` when you want one entry per key.

<!-- ds:block id=appendvsset-h7nr4x2c -->
**appendVsSet** · [`examples/dotenv/editing/main.go:168-175`](../../../examples/dotenv/editing/main.go#L168-L175)

```go
func appendVsSet(w io.Writer) {
	upsert := dotenv.Parse("PORT=8080\n")
	upsert.Set("PORT", "9090")
	literal := dotenv.Parse("PORT=8080\n")
	literal.Append(dotenv.NewPair("PORT", "9090"))
	fmt.Fprintf(w, "Set:    %q Count=%d\n", upsert.Render(), upsert.Count("PORT"))
	fmt.Fprintf(w, "Append: %q Count=%d\n", literal.Render(), literal.Count("PORT"))
}
```
<!-- /ds:block hash=1c7702 -->

## Step 7 — Derive another environment with Clone

`Clone()` returns an independent copy of a file; editing either side leaves the other alone. The copy keeps the source's line-ending style, trailing-newline state, file mode, `Path`, `Existed()` state and parse options (plugins included). It is cheaper and more faithful than `Parse(f.Render())`: it does not re-parse, so no entry can be classified differently on a second pass.

Use it to derive `.env.prod` from `.env.staging`: clone, change only what differs, and write the result with `SaveAs`. Every line you did not change stays byte-identical, including `${reference}` lines, because writing never expands anything.

<!-- ds:block id=derivevariants-t6qx2n8d -->
**deriveVariants** · [`examples/dotenv/clone-env/main.go:81-98`](../../../examples/dotenv/clone-env/main.go#L81-L98)

```go
func deriveVariants(src string) error {
	base, err := dotenv.Open(src)
	if err != nil {
		return err
	}
	dir := filepath.Dir(src)

	prod := base.Clone()
	prod.Set("DOMAIN", "acme.io")
	prod.Set("LOG_LEVEL", "warn")
	if err := prod.SaveAs(filepath.Join(dir, prodFile)); err != nil {
		return err
	}

	qa := base.Clone()
	qa.Set("DOMAIN", "qa.acme.io")
	return qa.SaveAs(filepath.Join(dir, qaFile))
}
```
<!-- /ds:block hash=43f28a -->

A reference is a formula, like `=A1*2` in a spreadsheet: keep the formula and it follows its source, bake its result and it stops following. If you build the new file from `ExpandedMap()` instead of cloning, every reference is baked. The result looks right on the day you write it, but `API_URL` no longer follows `DOMAIN`:

<!-- ds:block id=bake-p5kc8v3w -->
**bake** · [`examples/dotenv/clone-env/main.go:104-124`](../../../examples/dotenv/clone-env/main.go#L104-L124)

```go
func bake(src, dst string) error {
	f, err := dotenv.Open(src)
	if err != nil {
		return err
	}
	m, err := f.ExpandedMap()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := dotenv.Parse("")
	for _, k := range keys {
		out.Append(dotenv.NewPair(k, m[k]))
	}
	out.Set("DOMAIN", "acme.io")
	return out.SaveAs(dst)
}
```
<!-- /ds:block hash=e9a39f -->

To freeze one value on purpose, for example a secret resolved once at deploy time, read it with `GetExpanded` and write it back with `Set`: a deliberate choice for that one key, not a side effect.

## Try it

```sh
cd examples/dotenv/editing && go run .
```

```text
== Set
PORT created: false
LOG_LEVEL created: true
same value is a no-op: true
# app
export PORT = 9090 # dev only
HOST=localhost
DB_HOST=db.internal
#DEBUG=true
LOG_LEVEL=info
GREETING="hello world"
PASSWORD='p$$w'
MOTD="line one
line two"
== SetAfter and SetBefore
anchor missing: dotenv: anchor key not found: "NOPE"
# app
APP_NAME=shop
export PORT = 8080 # dev only
HOST=localhost
DB_HOST=db.internal
DB_PORT=5432
#DEBUG=true
CACHE_URL=redis://cache
== Unset and Restore
Unset HOST: true
Unset HOST again: false
Unset DB_HOST, delete: true
Disabled: [{HOST localhost} {DEBUG true}]
# app
export PORT = 8080 # dev only
# HOST=localhost
#DEBUG=true
Restore HOST: true
Restore DEBUG: true
# app
export PORT = 8080 # dev only
HOST=localhost
DEBUG=true
== Inherited
HOME: "/home/app" found=true
AWS_REGION: "" found=false
GetExpanded(HOME) = ""
== generate
# Copy to .env and fill in.
# Do not commit .env.

HOST=localhost
# derived
URL=https://${HOST}/v1 # derived
GREETING="hello world"
CRLF file: "A=1\r\nB=2\r\n"
== Append vs Set
Set:    "PORT=9090\n" Count=1
Append: "PORT=8080\nPORT=9090\n" Count=2
```

```sh
cd examples/dotenv/clone-env && go run .
```

```text
== .env.staging (API_URL -> https://api.staging.acme.io)
# shared
DOMAIN=staging.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.prod (API_URL -> https://api.acme.io)
# shared
DOMAIN=acme.io
LOG_LEVEL=warn

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.qa (API_URL -> https://api.qa.acme.io)
# shared
DOMAIN=qa.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.baked (API_URL -> https://api.staging.acme.io)
API_URL=https://api.staging.acme.io
CDN_URL=https://cdn.staging.acme.io
DOMAIN=acme.io
LOG_LEVEL=debug
```

The baked file says `DOMAIN=acme.io` but still points its URLs at staging. More tested examples, one method each, are in [`dotenv/example_edit_test.go`](../../../dotenv/example_edit_test.go) (`ExampleFile_Set`, `ExampleFile_Set_quoting`, `ExampleFile_SetAfter_anchorNotFound`, `ExampleFile_Restore`, `ExampleFile_Append_verbatim`, `ExampleFile_Clone_baked` and more).

## Pitfalls

- **`Set` cannot write a reference.** A changed value containing `$` is stored literally, so `Set("URL", "${HOST}/v1")` writes `URL='${HOST}/v1'`, and `GetExpanded` returns that text. To write a reference, place a parsed line with `Append` or `InsertAfter`, as in Step 5. Setting the value an entry already has is still a no-op, so an existing reference stays a reference.
- **Appending after an unterminated quote.** If the last value in a file opens a quote and never closes it, everything you append is read back as part of that value, because the open quote keeps consuming lines. The source was already broken, and guessing where the quote should have closed would corrupt real multi-line values. Check files you did not write before appending (pinned by `TestAppend_UnterminatedQuoteSwallowsWhatFollows`).
- **`Restore` targets the last disabled entry.** With more than one commented-out entry for a key, `Restore` re-enables the last one, which is not always the one `Unset` just created (pinned by `TestRestore_TargetsTheLastDisabledEntry`).
- **A commented-out multi-line value cannot be restored after a reload.** In one session, `Unset` keeps the block together and `Restore` reverses it fully. Once saved and re-read, each commented line is a separate comment.
- **Prose shaped like a setting counts as disabled.** `# TODO=fix this` is listed by `Disabled()` and can be re-enabled by `Restore("TODO")`. See [entries.md › Pitfalls](entries.md#pitfalls).
- **`Set` on an inherited name adds a new pair.** For a file with a bare `HOME` line, `Set("HOME", "/x")` appends `HOME=/x` and keeps the `HOME` declaration, so `Inherited()` still lists `HOME`.
- **Match `ErrAnchorNotFound` with `errors.Is`, not `==`.** The returned error wraps the sentinel with the anchor key (`dotenv: anchor key not found: "NOPE"`), so a direct comparison never matches.
- **A clone keeps the source's `Path`.** `clone.Save()` overwrites the file you cloned from. Write a clone with `SaveAs(newPath)`, or set `clone.Path` first.
- **Edits are not safe for concurrent use.** A `*File` has no locking; do not edit one from several goroutines at once.

## Related

- [saving.md](saving.md): `Save`, `SaveAs`, file modes, line endings, trailing newline.
- [reading.md](reading.md) and [entries.md](entries.md): `Open`, `Parse`, `Get`, `Map`, `Entry` and the six kinds.
- [expansion.md](expansion.md): why `Get`, not `GetExpanded`, is the value to write back.
- Module README: [Writing](../../../dotenv/README.md#writing), [Generating a file](../../../dotenv/README.md#generating-a-file), [Cloning between environments](../../../dotenv/README.md#cloning-between-environments).
- Examples: [`examples/dotenv/editing`](../../../examples/dotenv/editing/), [`examples/dotenv/clone-env`](../../../examples/dotenv/clone-env/).
- The CLI wraps these methods as `dotenvctl set`, `unset` and `restore`: [`dotenv/cli/README.md`](../../../dotenv/cli/README.md).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv](https://pkg.go.dev/github.com/ubgo/textdata/dotenv).
