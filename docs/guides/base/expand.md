# The expand engine: `${VAR}`, `${scheme:ref}` and `$(cmd)`

Package `github.com/ubgo/textdata/expand` resolves `$` references inside one string: `$VAR`, `${VAR}`, the six Docker Compose operators (`${VAR:-default}` and friends), nested references, `$$`, `${scheme:ref}` and `$(command)`. It knows nothing about files. Every textdata format hands it a string plus callbacks, and the format decides the rest (which quoting turns expansion off, which definition a self-reference sees). Read this page when you build a new format, want Compose-style interpolation for your own strings, or want to understand exactly what dotenv's `GetExpanded` and jsonc's `Expanded` do underneath.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

`expand` is in the base module, which has no dependencies outside the standard library. If you only read `.env` or JSONC files, you do not call this package directly: use `GetExpanded` in [dotenv](../../../dotenv/README.md#variable-expansion) or [jsonc](../../../jsonc/README.md#expansion-and-plugins). The runnable program is [`examples/base/expand-engine`](../../../examples/base/expand-engine).

## Step 1 — Give the engine a Lookup

An `expand.Engine` needs one field, `Lookup`, which says what a name holds. It returns an `expand.Var`: `Value` (expanded in turn unless `Literal`), `Set` (false when nothing defines the name) and `At` (where the value came from, a position your format understands). The `*expand.Stack` argument holds the names currently being expanded; this Lookup reads a name already on it as unset, which is how a cycle ends.

<!-- ds:block id=maplookup-bhxv9kp2 -->
**mapLookup** · [`examples/base/expand-engine/main.go:44-52`](../../../examples/base/expand-engine/main.go#L44-L52)

```go
func mapLookup(vars map[string]string) func(string, *expand.Stack) (expand.Var, error) {
	return func(name string, st *expand.Stack) (expand.Var, error) {
		if _, busy := st.At(name); busy {
			return expand.Var{}, nil
		}
		v, ok := vars[name]
		return expand.Var{Value: v, Set: ok}, nil
	}
}
```
<!-- /ds:block hash=c9d5d5 -->

## Step 2 — Use the operators, nesting and `$$`

| Form | Result |
|---|---|
| `$VAR`, `${VAR}` | the value, or `""` when unset |
| `${VAR-d}` | `d` when unset |
| `${VAR:-d}` | `d` when unset or empty |
| `${VAR+a}` | `a` when set, else `""` |
| `${VAR:+a}` | `a` when set and non-empty, else `""` |
| `${VAR?msg}` | error when unset |
| `${VAR:?msg}` | error when unset or empty |
| `$$` | a literal `$` |

A default or alternate may itself contain references (`${A:-${B:-x}}`); the engine matches the outer brace, which `os.Expand` cannot.

<!-- ds:block id=operators-re38k7u7 -->
**operators** · [`examples/base/expand-engine/main.go:57-77`](../../../examples/base/expand-engine/main.go#L57-L77)

```go
func operators(w io.Writer) error {
	e := &expand.Engine{Lookup: mapLookup(map[string]string{
		"HOST":  "db.internal",
		"EMPTY": "",
		"URL":   "postgres://${HOST}:${PORT:-5432}/app",
	})}
	for _, in := range []string{
		"${URL}",                         // nested references expand in turn
		"${EMPTY-unset} ${EMPTY:-empty}", // '-' only when unset, ':-' also when empty
		"${HOST+set} [${EMPTY:+nonempty}]",
		"${MISSING:-${HOST:-x}}", // a default may itself hold a reference
		"price: $$5 for $HOST",   // $$ is a literal dollar
	} {
		out, err := e.Expand(in, nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-34s => %s\n", in, out)
	}
	return nil
}
```
<!-- /ds:block hash=3577e2 -->

## Step 3 — Answer `${scheme:ref}` with a resolver

Set `Resolve` to handle references with a scheme, such as `${vault:db/password}`. Return `ok=false` for a scheme you do not own and the token is read as an ordinary name, so adding a resolver never changes how other values read. `SplitScheme` and `ParseRef` are the parsers the engine uses; call them when your format needs to inspect a reference without expanding it (a linter, a "which secrets does this file use" report).

<!-- ds:block id=schemes-74khhwz9 -->
**schemes** · [`examples/base/expand-engine/main.go:83-102`](../../../examples/base/expand-engine/main.go#L83-L102)

```go
func schemes(w io.Writer) error {
	e := &expand.Engine{
		Lookup: mapLookup(nil),
		Resolve: func(scheme, ref string) (string, bool, error) {
			if scheme != vaultScheme {
				return "", false, nil
			}
			return "secret-for-" + ref, true, nil
		},
	}
	out, err := e.Expand("${vault:db/password} [${other:thing}]", nil)
	if err != nil {
		return err
	}
	scheme, ref, ok := expand.SplitScheme("vault:db/password")
	fmt.Fprintf(w, "resolved: %s\nsplit: %q %q %v\n", out, scheme, ref, ok)
	r := expand.ParseRef("PORT:-5432")
	fmt.Fprintf(w, "parsed: name=%s default-if-empty=%v arg=%s\n", r.Name, r.Op == expand.OpDefaultEmpty, r.Arg)
	return nil
}
```
<!-- /ds:block hash=0f972a -->

## Step 4 — Turn on `$(cmd)` with a runner

`Run` executes a command substitution. While `Run` is nil, `$(…)` stays in the output as written, so a value survives a round trip instead of being blanked and no command ever runs. A runner that returns an error stops `Expand` with a `*expand.CommandError` that names the command and unwraps to the runner's error, so a failed command never reads as an empty value. The command text is expanded first, so `$(echo $WHO)` sees `WHO`.

<!-- ds:block id=commands-7z2aksvq -->
**commands** · [`examples/base/expand-engine/main.go:108-137`](../../../examples/base/expand-engine/main.go#L108-L137)

```go
func commands(w io.Writer) error {
	vars := map[string]string{"WHO": "world"}
	off := &expand.Engine{Lookup: mapLookup(vars)}
	on := &expand.Engine{
		Lookup: mapLookup(vars),
		Run: func(cmd string) (string, error) {
			if strings.HasPrefix(cmd, "fail") {
				return "", errors.New("exit status 1")
			}
			return strings.ToUpper(cmd), nil // stands in for a real shell
		},
	}
	in := "a $(echo $WHO) b [$(fail)]"
	offOut, err := off.Expand(in, nil)
	if err != nil {
		return err
	}
	onOut, err := on.Expand("a $(echo $WHO) b", nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "run off: %s\nrun on:  %s\n", offOut, onOut)
	_, err = on.Expand(in, nil)
	var ce *expand.CommandError
	if !errors.As(err, &ce) {
		return fmt.Errorf("want a *expand.CommandError, got %v", err)
	}
	fmt.Fprintf(w, "run fail: %v (cmd=%q)\n", err, ce.Cmd)
	return nil
}
```
<!-- /ds:block hash=cffe2b -->

## Step 5 — Stop cycles with the Stack

When you expand the value of a named entry, push that name first with `st.Enter(name, at)` and call the returned `leave` when done. The engine pushes each name it follows in the same way, so your Lookup sees the whole chain. `A=a-$B`, `B=b-$A` then ends at `a-b-` instead of looping, and `SELF=$SELF$SELF$SELF` expands to `""`. `MaxDepth` (default `expand.DefaultMaxDepth`, 32) only backstops a deep chain that is not a cycle.

<!-- ds:block id=cycles-fh85r3fe -->
**cycles** · [`examples/base/expand-engine/main.go:144-161`](../../../examples/base/expand-engine/main.go#L144-L161)

```go
func cycles(w io.Writer) error {
	e := &expand.Engine{Lookup: mapLookup(map[string]string{
		"A": "a-$B", "B": "b-$A", "SELF": "$SELF$SELF$SELF",
	})}
	st := expand.NewStack()
	leave := st.Enter("A", 0) // we are expanding A's own value
	a, err := e.Expand("a-$B", st)
	leave()
	if err != nil {
		return err
	}
	self, err := e.Expand("$SELF", nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "A => %q, SELF => %q, default depth %d\n", a, self, expand.DefaultMaxDepth)
	return nil
}
```
<!-- /ds:block hash=9fd537 -->

## Step 6 — Fail loudly with `${VAR:?msg}`

An unsatisfied `?` or `:?` reference makes `Expand` return an error: by default a `*expand.RequiredError` with the `Key`, the `Message` and `Empty` (true for `:?`). Set `Engine.Required` to return your format's own error type instead; dotenv does, so its callers match `*dotenv.RequiredError`.

<!-- ds:block id=required-q5qdggzy -->
**required** · [`examples/base/expand-engine/main.go:167-176`](../../../examples/base/expand-engine/main.go#L167-L176)

```go
func required(w io.Writer) error {
	e := &expand.Engine{Lookup: mapLookup(map[string]string{"TOKEN": ""})}
	_, err := e.Expand("${TOKEN:?set TOKEN in .env.local}", nil)
	var re *expand.RequiredError
	if !errors.As(err, &re) {
		return fmt.Errorf("expected a RequiredError, got %v", err)
	}
	fmt.Fprintf(w, "error: %v\nkey=%s empty=%v\n", err, re.Key, re.Empty)
	return nil
}
```
<!-- /ds:block hash=9fdce6 -->

## Try it

```sh
cd examples/base/expand-engine && go run .
```

```text
${URL}                             => postgres://db.internal:5432/app
${EMPTY-unset} ${EMPTY:-empty}     =>  empty
${HOST+set} [${EMPTY:+nonempty}]   => set []
${MISSING:-${HOST:-x}}             => db.internal
price: $$5 for $HOST               => price: $5 for db.internal
resolved: secret-for-db/password []
split: "vault" "db/password" true
parsed: name=PORT default-if-empty=true arg=5432
run off: a $(echo $WHO) b [$(fail)]
run on:  a ECHO WORLD b
run fail: expand: command "fail" failed: exit status 1 (cmd="fail")
A => "a-b-", SELF => "", default depth 32
error: expand: required variable TOKEN is not set or is empty: set TOKEN in .env.local
key=TOKEN empty=true
```

## Pitfalls

- **The engine does not cut cycles by itself.** It records names on the Stack, but only your `Lookup` can decide that a re-entry reads as unset (or as an earlier definition, as dotenv does). A Lookup that ignores the Stack still terminates, through `MaxDepth`, but `A=$A$A` then branches at every level: up to 2^32 expansions at the default depth.
- **When the depth bound is hit, the rest is left verbatim.** A chain deeper than `MaxDepth` returns its remaining `$X` text unexpanded, not an error.
- **Unset is not an error.** `${VAR}` with nothing defining `VAR` expands to `""`, as in Compose. Use `${VAR:?message}` when a missing value must stop the program.
- **`-` and `:-` differ on empty values.** `${EMPTY-unset}` gives `""` because `EMPTY` is set; only `${EMPTY:-empty}` treats empty like unset.
- **Scheme names are matched without case, and the operators win.** `${GoPass:x}` reaches the resolver as scheme `gopass`; `${VAR:-d}` is never a scheme because a scheme's colon cannot be followed by `-`, `+` or `?`.
- **An unbalanced `${` or `$(` is kept as text.** `${A` and `$(echo` are emitted as written rather than guessed at; a `$` followed by anything that cannot start a name (`$5`, `$ `) is a literal dollar.
- **A resolved scheme value is not expanded again.** What `Resolve` returns is inserted as is; a `Var` with `Literal: true` is too. Return `Literal` for values that must not be interpreted, such as dotenv's single-quoted entries.
- **Every callback error aborts.** An error from `Lookup` or `Resolve` stops `Expand` and is returned as is; an error from `Run` (a command that fails, exits non-zero or is refused) stops it with a `*expand.CommandError` naming the command and unwrapping to the runner's error. A command that may fail harmlessly must succeed in its own right (`$(cmd || true)`) or have the runner return a fallback value.

## Related

- [shell](shell.md): the runner the formats plug into `Run`, usable on its own; [value](value.md): this engine with the formats' rules for one string.
- [Plugins](plugin.md): `Lookuper`, `Resolver` and `CommandRunner` are how a format fills these callbacks from installed plugins.
- [dotenv README § Variable expansion](../../../dotenv/README.md#variable-expansion) and [§ Interpolation](../../../dotenv/README.md#interpolation): the rules dotenv adds (single quotes, which definition a self-reference sees).
- [jsonc README § Expansion and plugins](../../../jsonc/README.md#expansion-and-plugins).
- [Root README § expand](../../../README.md#expand).
- Example: [`examples/base/expand-engine`](../../../examples/base/expand-engine).
- API: [expand on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/expand).
