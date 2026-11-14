# Variable expansion

How `GetExpanded` and `ExpandedMap` resolve `${VAR}`, `$VAR`, the Docker Compose `${…}` operators, `$$` and opt-in `$(command)`. You want this whenever one value in a `.env` is built from another (`API_URL=https://api.${DOMAIN}`), a value must be required, or a file is shared with Docker Compose and must read the same in both.

A value like `URL=postgres://${HOST}/app` is stored on disk with the reference in it. `Get` returns that raw text. `GetExpanded` resolves the reference against the other keys in the **same file**, the way Compose does when it reads a `.env`. The process environment is never read: an unresolved reference is `""`, unless you install a lookup (`WithLookup(os.LookupEnv)` or a `Lookuper` plugin, see [plugins.md](plugins.md)), which is consulted only for names the file does not define.

Expansion is for reading. `Render`, `Save` and `Set` always work with the raw value, so references stay references on disk. The engine itself lives in the shared base package `github.com/ubgo/textdata/expand`; dotenv plugs its own rules into it (file first, quoting, `*RequiredError`).

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

Every snippet is a function from the runnable example [`examples/dotenv/expansion`](../../../examples/dotenv/expansion/). It parses in-memory strings and never touches the disk, the environment or a shell. How a value is quoted decides whether it is expanded at all, so read [syntax.md](syntax.md) first if quoting is new to you. Most steps print through a small helper, `expandAll`, which calls `GetExpanded` for every key and prints the result with `%q`.

## Step 1 — Get the raw or the expanded value

`GetExpanded(key)` returns `(value, ok, err)`. `ok` is false only when the key has no active pair. `err` is non-nil only for an unsatisfied `${VAR:?msg}` or `${VAR?msg}` ([Step 6](#step-6--fail-loudly-with-requirederror)), or when an installed lookup, resolver or value transformer fails. A plain reference to a key that does not exist is `""`, never an error, unless you pass `WithStrictExpand()` ([Step 8](#step-8--refuse-unresolved-references-with-withstrictexpand)).

Both forms work: `${NAME}` and `$NAME`. Use braces when a name is followed by more name characters (`${HOST}_backup`), or when you need an operator. Order in the file does not matter: a reference can point at a key defined further down.

<!-- ds:block id=getvsexpanded-f3wd8k2m -->
**getVsExpanded** · [`examples/dotenv/expansion/main.go:70-84`](../../../examples/dotenv/expansion/main.go#L70-L84)

```go
func getVsExpanded(w io.Writer) error {
	f := dotenv.Parse("URL=postgres://${HOST}:$PORT/app\nHOST=db\nPORT=5432\nMISSING=[${NOPE}]\n")
	raw, _ := f.Get("URL")
	url, ok, err := f.GetExpanded("URL")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Get=%s\nGetExpanded=%s ok=%v\n", raw, url, ok)
	missing, _, err := f.GetExpanded("MISSING")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "MISSING:", missing)
	return nil
}
```
<!-- /ds:block hash=ac7271 -->

## Step 2 — Use the `${…}` operators

Six operators cover the Docker Compose interpolation spec. The colon forms treat a key that exists with an empty value like an unset one.

| Form | Meaning |
|---|---|
| `${VAR}` / `$VAR` | the value, or `""` |
| `${VAR:-default}` | `default` when VAR is unset **or empty** |
| `${VAR-default}` | `default` only when VAR is unset |
| `${VAR:+alt}` | `alt` when VAR is set **and non-empty**, else `""` |
| `${VAR+alt}` | `alt` when VAR is set, even if empty, else `""` |
| `${VAR:?message}` | error when VAR is unset **or empty** |
| `${VAR?message}` | error only when VAR is unset |

The example prints the four value operators against a key that is set (`SET=value`), set but empty (`EMPTY=`), and absent (`UNSET`); the two error forms are in [Step 6](#step-6--fail-loudly-with-requirederror).

<!-- ds:block id=operators-x9qm4t7c -->
**operators** · [`examples/dotenv/expansion/main.go:91-110`](../../../examples/dotenv/expansion/main.go#L91-L110)

```go
func operators(w io.Writer) error {
	forms := []string{"${%s:-def}", "${%s-def}", "${%s:+alt}", "${%s+alt}"}
	var b strings.Builder
	b.WriteString("SET=value\nEMPTY=\n")
	for _, name := range []string{"SET", "EMPTY", "UNSET"} {
		for i, form := range forms {
			fmt.Fprintf(&b, "%s_%d=%s\n", name, i, fmt.Sprintf(form, name))
		}
	}
	f := dotenv.Parse(b.String())
	for _, key := range f.Keys()[2:] {
		raw, _ := f.Get(key)
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-14s -> %q\n", raw, v)
	}
	return nil
}
```
<!-- /ds:block hash=9e0818 -->

Details the table does not show:

- The text after an operator is itself expanded, so `${A:-${B}}` works. It is expanded only when it is used: `${A:-${B:?need}}` raises no error while `A` has a value.
- The default or alternate may be empty (`${VAR:-}`) and may contain spaces (`${VAR:-a b}`).
- The message after `?` may be empty: `${VAR:?}` still fails, with a message that just names the variable.
- A disabled pair (`# VAR=x`) and an inherited name-only line (`VAR`) do not count as set.
- Shell-only forms such as `${VAR/foo/bar}` or `${#VAR}` are not supported, as in Compose. They are not parsed as operators, so the whole text inside the braces is treated as a name, which is never set, and expands to `""`.

## Step 3 — Write a literal dollar

The character right after a `$` decides what it means. The value is read once, left to right:

| After `$` | Meaning | In the file | `GetExpanded` |
|---|---|---|---|
| `$` | one literal `$` | `cost 5$$` | `cost 5$` |
| `{` | braced reference | `${NAME}` | value of NAME |
| letter or `_` | bare reference | `$NAME/x` | value of NAME, then `/x` |
| `(` | command (only when enabled, [Step 7](#step-7--run-commands-opt-in)) | `$(whoami)` | `$(whoami)` unchanged by default |
| anything else, or end of value | literal `$` | `$5`, `100$` | unchanged |

`$${NAME}` gives the literal text `${NAME}`: the `$$` becomes one `$`, and `{NAME}` after it is ordinary text. An unclosed `${NAME` is kept as written. A backslash does **not** escape a dollar: `"\$NAME"` keeps the backslash and still expands `$NAME`.

<!-- ds:block id=dollars-k6vb3n9s -->
**dollars** · [`examples/dotenv/expansion/main.go:117-125`](../../../examples/dotenv/expansion/main.go#L117-L125)

```go
func dollars(w io.Writer) error {
	src := "NAME=world\n" +
		"COST=cost 5$$\n" +
		"TEMPLATE=$${NAME}\n" +
		"PRICE=$5 or 100$\n" +
		"UNCLOSED=${NAME\n" +
		"BACKSLASH=\"\\$NAME\"\n"
	return expandAll(w, dotenv.Parse(src))
}
```
<!-- /ds:block hash=cdea5f -->

## Step 4 — Nest, chain and extend a value

References nest inside operator arguments, and a referenced value is expanded in turn, so `C=${B}` where `B=${A}` resolves all the way through. With duplicate keys, a reference sees the last one.

A key that refers to itself is the exception, and it follows Docker Compose. Compose reads a `.env` from top to bottom and expands each line against what it has read so far, so a self-reference sees the definition **above** it: `LIST=one` then `LIST=${LIST},two` gives `one,two`, and `PATH_EXTRA=${PATH_EXTRA}:/opt/bin` appends to an earlier line. With nothing above, the reference asks your `Lookuper` and is otherwise unset, so `ZONE=${ZONE:-eu-west}` gives the Lookuper's `ZONE` if it has one, else `eu-west`. An earlier line in the file still wins over the Lookuper.

A cycle cannot hang the caller. While a variable is being expanded, a reference back into it sees only the definitions above the line being expanded, then the Lookuper, then nothing. The key you asked for is on that stack from the start, so `CYCLE_A=a${CYCLE_B}` with `CYCLE_B=b${CYCLE_A}` gives `ab` for `CYCLE_A` and `ba` for `CYCLE_B`. (Compose gives `ba` for `CYCLE_B` too; for `CYCLE_A` it gives `a`, because it never looks ahead to a later line.) Two separate references to the same variable in one value are not a cycle and both resolve. As a backstop, chains stop after [32 levels](ds:block?id=maxexpanddepth-hr5dfhv2).

<!-- ds:block id=nesting-b8rj5w2x -->
**nesting** · [`examples/dotenv/expansion/main.go:132-142`](../../../examples/dotenv/expansion/main.go#L132-L142)

```go
func nesting(w io.Writer) error {
	src := "A=1\n" +
		"B=${A}0\n" +
		"C=${B}0\n" +
		"D=${NOPE:-${C}}\n" +
		"LIST=one\n" +
		"LIST=${LIST},two\n" +
		"CYCLE_A=a${CYCLE_B}\n" +
		"CYCLE_B=b${CYCLE_A}\n"
	return expandAll(w, dotenv.Parse(src))
}
```
<!-- /ds:block hash=ff01f8 -->

The only place this package and Compose part ways is a reference to a key defined further down: Compose leaves it unset, this package resolves it. Everything Compose resolves, this package resolves to the same value.

## Step 5 — Know which names a reference can reach

A reference name is narrower than a key name. A bare `$name` takes letters, digits and `_`, starting with a letter or `_`, and stops at the first other character, so `$spring.url` is `$spring` followed by `.url`. Inside `${…}` the name runs up to the first operator character (`:`, `-`, `+`, `?`), so dotted and bracketed keys are reachable with braces but dashed keys are not: `${MY-KEY}` means "`MY`, with default `KEY`". Rename a dashed key with `_` if you need to reference it. Names are case sensitive.

Single-quoted and backtick values are never expanded: `$`, `$$`, `${…}` and `$(…)` come back exactly as written. That holds when another key refers to the literal one too: `REF=${LIT}` inserts the text as written, without expanding it a second time, as Compose does.

<!-- ds:block id=names-q2hc7m4z -->
**names** · [`examples/dotenv/expansion/main.go:149-165`](../../../examples/dotenv/expansion/main.go#L149-L165)

```go
func names(w io.Writer) error {
	src := "spring.url=jdbc\n" +
		"MY-KEY=dashed\n" +
		"BARE=$spring.url\n" +
		"BRACED=${spring.url}\n" +
		"DASHED=${MY-KEY}\n" +
		"LITERAL='${spring.url}'\n"
	f := dotenv.Parse(src)
	for _, key := range []string{"BARE", "BRACED", "DASHED", "LITERAL"} {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s=%q\n", key, v)
	}
	return nil
}
```
<!-- /ds:block hash=c7eea4 -->

## Step 6 — Fail loudly with RequiredError

`ExpandedMap()` returns every active pair expanded, as a `map[string]string`, with the last duplicate winning like `Map`. Only that last duplicate is expanded, so an overridden line cannot fail the map. The package-level `Read(path)` is `Open` followed by `ExpandedMap`. It stops at the first error, in file order, and returns a `nil` map with it: a half-expanded map would leave you unable to tell which values are trustworthy.

`${VAR:?message}` and `${VAR?message}` exist to fail loudly when configuration is missing. The error is a `*RequiredError`; use `errors.As` to tell it apart from a plugin failure and read its fields:

| Field | Meaning |
|---|---|
| `Key` | the variable that was missing (not the key whose value held the reference) |
| `Message` | the text after `?`, or `""` |
| `Empty` | `true` for `:?` (unset or empty), `false` for `?` (unset only) |

`Error()` always names the variable, and includes the message when there is one.

<!-- ds:block id=requiredvars-n7tf2c5k -->
**requiredVars** · [`examples/dotenv/expansion/main.go:171-187`](../../../examples/dotenv/expansion/main.go#L171-L187)

```go
func requiredVars(w io.Writer) error {
	ok := dotenv.Parse("TOKEN=t-123\nAUTH=Bearer ${TOKEN:?set TOKEN}\n")
	m, err := ok.ExpandedMap()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "AUTH:", m["AUTH"])

	bad := dotenv.Parse("TOKEN=\nAUTH=Bearer ${TOKEN:?set TOKEN}\nDB=${DB_URL?}\n")
	m, err = bad.ExpandedMap()
	var req *dotenv.RequiredError
	if errors.As(err, &req) {
		fmt.Fprintf(w, "nil map: %v key=%s message=%q empty=%v\n", m == nil, req.Key, req.Message, req.Empty)
		fmt.Fprintln(w, err)
	}
	return nil
}
```
<!-- /ds:block hash=18f5de -->

In the bad file both `AUTH` and `DB` fail, but only the first is reported. The error is returned for whichever key you read: `GetExpanded` on a key with no failing reference succeeds even if another key would fail, so call `ExpandedMap` once at startup if you want every requirement checked.

## Step 7 — Run commands (opt-in)

`$(command)` runs a command and inserts its output. It is [off by default](ds:block?id=withcommandsubstitution-5kmt8q5c), because turning it on means that merely reading a `.env` file runs whatever commands it contains: a `.env` from a repository, a container image or a pull request would become a way to run code on your machine or in CI. With it off, `$(…)` is returned exactly as written, so nothing is lost.

There are two ways to turn it on:

- `WithCommandSubstitution(true)` runs each command through the shared runner, [`shell.Run`](../base/shell.md), with trailing newlines removed from the output. Use this only for files you trust as much as your own code.
- `WithCommandRunner(fn)` runs each command through your function instead, and enables substitution. Use it to allow-list commands, run them in a sandbox, or fake them in tests. Passing `nil` enables substitution with the default shell runner.

The default runner picks the shell for the OS: `sh -c` on Linux, macOS and other Unix-likes; on Windows `pwsh -NoProfile -NonInteractive -Command` when PowerShell 7 is on `PATH`, else `powershell.exe` with the same flags. Stdin is closed, so a command that prompts fails at once instead of waiting for input. A `.env` written for `sh` does not run in PowerShell: on Windows, write its commands for PowerShell or pass `WithCommandRunner(shell.Runner{Shell: []string{"bash", "-c"}}.RunCommand)` to use a POSIX shell such as Git for Windows' bash ([shell § Step 3](../base/shell.md#step-3--use-another-shell)).

A command can hang: a password manager waiting for a GUI unlock, a network tool with no timeout. [`WithCommandContext(ctx)`](ds:block?id=withcommandcontext-v6889xsb) bounds the default runner: pass `context.WithTimeout(ctx, 5*time.Second)` and a command still running when it ends is killed, and `GetExpanded` returns an `*expand.CommandError` wrapping `shell.ErrTimeout`. Without it, a command may run as long as it likes.

The command text is expanded first, so `$(${GIT} rev-parse --short HEAD)` reaches the runner as `git rev-parse --short HEAD`. A command that fails, exits non-zero or is refused by your runner makes `GetExpanded` and `ExpandedMap` return an `*expand.CommandError` naming the command (it unwraps to the runner's error), so a failed command never reads as an empty value (`WHO` below). Single-quoted and backtick values never run commands. Options apply in order, so a later `WithCommandSubstitution(false)` switches substitution back off, even after `WithCommandRunner`.

<!-- ds:block id=commands-w5pk9d3r -->
**commands** · [`examples/dotenv/expansion/main.go:197-229`](../../../examples/dotenv/expansion/main.go#L197-L229)

```go
func commands(w io.Writer) error {
	src := "GIT=git\nREV=$(${GIT} rev-parse --short HEAD)\nWHO=$(whoami)\nLIT='$(whoami)'\n"
	off := dotenv.Parse(src)
	rev, _, err := off.GetExpanded("REV")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "off: REV =", rev)

	runner := func(cmd string) (string, error) {
		out, ok := allowed[cmd]
		if !ok {
			return "", fmt.Errorf("command %q not allowed", cmd)
		}
		return out, nil
	}
	on := dotenv.Parse(src, dotenv.WithCommandRunner(runner))
	for _, key := range on.Keys() {
		v, _, err := on.GetExpanded(key)
		if err != nil {
			fmt.Fprintf(w, "%s error: %v\n", key, err)
			continue
		}
		fmt.Fprintf(w, "%s=%q\n", key, v)
	}
	// Options apply in order: a later switch-off wins over the runner.
	later := dotenv.Parse(src, dotenv.WithCommandRunner(runner), dotenv.WithCommandSubstitution(false))
	if rev, _, err = later.GetExpanded("REV"); err != nil {
		return err
	}
	fmt.Fprintln(w, "runner then off: REV =", rev)
	return nil
}
```
<!-- /ds:block hash=01c6c0 -->

Command runners are also plugins (`CommandRunner`); the last one installed wins. See [plugins.md](plugins.md#step-5--combine-several-plugins).

## Step 8 — Refuse unresolved references with `WithStrictExpand`

A program that reads a token or a connection string needs to know it got a real value. With [`WithStrictExpand()`](ds:block?id=withstrictexpand-at8hy283), `GetExpanded`, `ExpandedMap` and `Read` fail instead of reading a reference as `""` or keeping it as text:

| Reference | Without | With `WithStrictExpand()` |
|---|---|---|
| a name neither the file nor a lookup defines (`${DB_HOTS}`) | `""` | `*expand.UnsetError` |
| a `${scheme:ref}` no installed resolver claims (`${vault:…}` with no vault plugin) | `""` | `*expand.UnknownSchemeError` |
| `$(command)` with substitution off | kept as written | `*expand.CommandOffError` |

`${VAR:-d}` and `${VAR-d}` still use their default, `${VAR:?msg}` keeps its `*RequiredError`, and a key set to an empty value is set. Single-quoted and backtick values are never expanded, so they never fail. The error is a `*pos.Error` at the `file:line:col` where the entry's value starts, wrapping the typed error, which names the variable and its byte offset inside the decoded value (never a value); every one matches `expand.ErrStrict` ([pinned](ds:block?id=teststrictexpand-59jbhqpv&assert=true)). The runnable program is [`examples/dotenv/strict-env`](../../../examples/dotenv/strict-env/):

<!-- ds:block id=dotenv-strict-run-m2v8k5qa -->
**run** · [`examples/dotenv/strict-env/main.go:38-53`](../../../examples/dotenv/strict-env/main.go#L38-L53)

```go
func run(w io.Writer) error {
	f := dotenv.Parse(src, dotenv.WithStrictExpand())
	f.Path = ".env" // the name errors report
	for _, key := range []string{"DSN", "REPLICA", "TOKEN", "STAMP"} {
		v, _, err := f.GetExpanded(key)
		if err == nil {
			fmt.Fprintf(w, "%-8s %s\n", key, v)
			continue
		}
		if !errors.Is(err, expand.ErrStrict) {
			return err
		}
		fmt.Fprintf(w, "%-8s %v\n", key, err)
	}
	return nil
}
```
<!-- /ds:block hash=9e05ae -->

```text
DSN      postgres://db.internal/app
REPLICA  .env:4:9: expand REPLICA: expand: variable DB_HOTS is not set (offset 11)
TOKEN    .env:5:7: expand TOKEN: expand: no resolver for scheme "vault" in ${vault:secret/app#token} (offset 0)
STAMP    .env:6:7: expand STAMP: expand: command substitution is off: $(date +%s) (offset 0)
```

The same switch for a single value is [`value.WithStrict()`](../base/value.md#step-7--make-unresolved-references-errors-with-withstrict); the engine underneath is `expand.Engine.Strict` ([expand](../base/expand.md#step-7--refuse-unresolved-references-with-enginestrict)).

## Try it

```sh
cd examples/dotenv/expansion && go run .
```

```text
== Get vs GetExpanded
Get=postgres://${HOST}:$PORT/app
GetExpanded=postgres://db:5432/app ok=true
MISSING: []
== operators
${SET:-def}    -> "value"
${SET-def}     -> "value"
${SET:+alt}    -> "alt"
${SET+alt}     -> "alt"
${EMPTY:-def}  -> "def"
${EMPTY-def}   -> ""
${EMPTY:+alt}  -> ""
${EMPTY+alt}   -> "alt"
${UNSET:-def}  -> "def"
${UNSET-def}   -> "def"
${UNSET:+alt}  -> ""
${UNSET+alt}   -> ""
== dollars
NAME="world"
COST="cost 5$"
TEMPLATE="${NAME}"
PRICE="$5 or 100$"
UNCLOSED="${NAME"
BACKSLASH="\\world"
== nesting, self-reference, cycles
A="1"
B="10"
C="100"
D="100"
LIST="one,two"
CYCLE_A="ab"
CYCLE_B="ba"
== reachable names
BARE=".url"
BRACED="jdbc"
DASHED="KEY"
LITERAL="${spring.url}"
== ExpandedMap and RequiredError
AUTH: Bearer t-123
nil map: true key=TOKEN message="set TOKEN" empty=true
dotenv: required variable TOKEN is not set or is empty: set TOKEN
== command substitution
off: REV = $(${GIT} rev-parse --short HEAD)
GIT="git"
REV="a1b2c3d"
WHO error: expand: command "whoami" failed: command "whoami" not allowed
LIT="$(whoami)"
runner then off: REV = $(${GIT} rev-parse --short HEAD)
```

More tested examples: `ExampleFile_GetExpanded_operators`, `ExampleFile_GetExpanded_selfReference`, `ExampleRequiredError`, `ExampleWithCommandRunner_allowList` and others in [`dotenv/example_expand_test.go`](../../../dotenv/example_expand_test.go).

## Pitfalls

- **Dashed keys cannot be referenced.** `${MY-KEY}` is the `-` default operator ([Step 5](#step-5--know-which-names-a-reference-can-reach)).
- **A bare reference stops at `.`.** `$spring.url` is `$spring` followed by `.url`. Use `${spring.url}`.
- **Use `Get`, not `GetExpanded`, for anything you write back.** Writing an expanded value replaces the reference with a copy of its current target, and the link is lost.
- **`WithCommandRunner(nil)` runs real shell commands.** It enables substitution with the default shell runner (`sh -c`, or PowerShell on Windows), not a no-op runner.
- **`WithCommandContext` bounds the default runner only.** A runner you install with `WithCommandRunner` is called as is; give it its own limit, for example `shell.Runner{Timeout: 5 * time.Second}.RunCommand`.
- **A failing command fails the read.** Any runner error or non-zero exit is an `*expand.CommandError` naming the command (`WHO` above), so one bad `$(…)` stops `ExpandedMap` and `Read`. If a command may fail harmlessly, make it succeed in the shell (`$(cmd || true)`) or have your runner return a fallback value instead of an error.
- **A typo'd reference is `""` by default.** `${DB_HOTS}` for `${DB_HOST}` reads as empty and nothing complains; `WithStrictExpand()` makes it an error at its line.
- **`ExpandedMap` reports only the first missing variable.** Fix it and rerun to see the next one.
- **A lookup error beats a default.** `${DB_PASSWORD:-}` still fails when a `Lookuper` returns an error, so an unreachable secret store never reads as empty ([plugins.md](plugins.md#step-3--write-a-plugin-type)).

## Related

- [syntax.md](syntax.md): quoting, comments, escapes, and which values are expanded.
- [reading.md](reading.md): `Read`, `Open`, `Parse` and the raw `Get`/`Map` view.
- [plugins.md](plugins.md): `WithLookup` for an environment or secret-store fallback, `${scheme:ref}` resolvers, value transformers, `CommandRunner`, and `ExpandObserver`.
- [typed-config.md](typed-config.md): binding the expanded values to a struct.
- Module README: [Variable expansion](../../../dotenv/README.md#variable-expansion) and [Interpolation](../../../dotenv/README.md#interpolation).
- Examples: [`examples/dotenv/expansion`](../../../examples/dotenv/expansion/), [`examples/dotenv/strict-env`](../../../examples/dotenv/strict-env/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv#File.GetExpanded](https://pkg.go.dev/github.com/ubgo/textdata/dotenv#File.GetExpanded).
