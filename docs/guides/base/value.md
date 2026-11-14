# Expand one value: `value.Expand`

Package `github.com/ubgo/textdata/value` expands a single string with the same engine and rules as dotenv and jsonc: `$VAR`, `${VAR}`, the `${VAR:-default}` family, nesting, `$$`, `${scheme:ref}` through value plugins, and `$(command)` when you turn it on. Use it when the value does not live in a `.env` or JSONC file: a YAML field, a flag, a database row, a setting such as `token: "$(gopass show -o work/api)"` or `token: "${gopass:work/api}"` that should behave exactly as it would in a `.env`.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

`value` is in the base module (standard library only). The value plugins are separate modules, so a program that uses one adds it, for example `go get github.com/ubgo/textdata/plugins/gopass@latest`. The runnable programs are [`examples/value/env`](../../../examples/value/env), [`examples/value/commands`](../../../examples/value/commands) (needs a Unix shell), [`examples/value/secrets`](../../../examples/value/secrets) and [`examples/value/strict`](../../../examples/value/strict).

`value.Expand(ctx, s, opts...)` reads and runs nothing unless an option asks for it:

| Option | What it adds |
|---|---|
| `WithEnv()` | variables from the process environment (`os.LookupEnv`) |
| `WithLookup(fn)` | variables from your function: a map, a config section |
| `WithCommands()` | `$(command)` through [`shell.Run`](shell.md), bounded by `ctx` |
| `WithRunner(r)` | `$(command)` through your `plugin.CommandRunner` instead of the shell |
| `WithPlugins(ps...)` | value plugins: Lookupers, Resolvers for `${scheme:ref}`, a CommandRunner, ValueTransformers, ExpandObservers |
| `WithStrict()` | every unresolved reference is an error instead of `""` or the text as written (Step 7) |

## Step 1 — Expand from the environment

With `WithEnv()`, `$NAME` and `${NAME}` read the process environment. Without it, the environment is never read and every name is unset (`""`), so a value expands the same on every machine until you choose otherwise.

<!-- ds:block id=value-fromenv-t3q8w5kd -->
**fromEnv** · [`examples/value/env/main.go:61-74`](../../../examples/value/env/main.go#L61-L74)

```go
func fromEnv(w io.Writer) error {
	ctx := context.Background()
	in := "postgres://${DEMO_USER}@$DEMO_HOST/app"
	with, err := value.Expand(ctx, in, value.WithEnv())
	if err != nil {
		return err
	}
	without, err := value.Expand(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with env:    %s\nwithout env: %s\n", with, without)
	return nil
}
```
<!-- /ds:block hash=8871e0 -->

## Step 2 — Defaults and required values

The operators are the Docker Compose ones the formats use: `${VAR:-d}` for a default when unset or empty, `${VAR-d}` when unset only, `${VAR:+a}` and `${VAR+a}` for an alternate, `${VAR:?msg}` and `${VAR?msg}` to fail, and `$$` for a literal dollar. A default may hold another reference. The full table is in [expand § Step 2](expand.md#step-2--use-the-operators-nesting-and-).

<!-- ds:block id=value-defaults-m6x2c9vb -->
**defaultsAndRequired** · [`examples/value/env/main.go:80-95`](../../../examples/value/env/main.go#L80-L95)

```go
func defaultsAndRequired(w io.Writer) error {
	for _, in := range []string{
		"${DEMO_PORT:-5432}",
		"${DEMO_REGION:-${DEMO_HOST}}",
		"${DEMO_USER:+user is set}",
		"cost: $$5",
		"${DEMO_USER:?DEMO_USER must be set}",
	} {
		out, err := value.Expand(context.Background(), in, value.WithEnv())
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-36s => %s\n", in, out)
	}
	return nil
}
```
<!-- /ds:block hash=58ee43 -->

Sources are asked in the order you give them and the first one that has a name wins, so `WithLookup(overrides), WithEnv()` lets a map override the environment. A value a source returns is expanded in turn; a reference back to a name being expanded reads as unset, so a chain always ends.

<!-- ds:block id=value-layered-j9n4r7fs -->
**layeredSources** · [`examples/value/env/main.go:101-116`](../../../examples/value/env/main.go#L101-L116)

```go
func layeredSources(w io.Writer) error {
	overrides := map[string]string{
		"DEMO_HOST": "localhost",
		"DSN":       "postgres://${DEMO_USER}@${DEMO_HOST}/app",
	}
	lookup := func(name string) (string, bool) {
		v, ok := overrides[name]
		return v, ok
	}
	out, err := value.Expand(context.Background(), "$DSN", value.WithLookup(lookup), value.WithEnv())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "layered:     %s\n", out)
	return nil
}
```
<!-- /ds:block hash=eadf37 -->

## Step 3 — Turn on commands, deliberately

`$(command)` is kept as written until you pass `WithCommands()` or `WithRunner(r)`. That is the no-surprise rule: expanding a value must never run a command because the value happened to contain one. Turn commands on only for values from a source you trust as much as your code, such as your own config file, never for input from a request or another user.

<!-- ds:block id=value-offbydefault-w8d3h6zq -->
**offByDefault** · [`examples/value/commands/main.go:46-53`](../../../examples/value/commands/main.go#L46-L53)

```go
func offByDefault(w io.Writer) error {
	out, err := value.Expand(context.Background(), "token=$(echo never-runs)", value.WithEnv())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "off:      %s\n", out)
	return nil
}
```
<!-- /ds:block hash=c7c4bf -->

With [`WithCommands()`](ds:block?id=withcommands-jhgmdcyp), each command runs through `shell.Run` with the context you passed to `Expand`: `sh -c` on Unix-likes, PowerShell on Windows (see [shell § Step 1](shell.md#step-1--run-a-command)). The command's own references expand first (`$(printf '%s' $WHO)` sees `WHO`), and the trailing newline is trimmed. A failing command is an `*expand.CommandError` that unwraps to the shell's error.

<!-- ds:block id=value-commandson-q2v7m4xe -->
**commandsOn** · [`examples/value/commands/main.go:59-75`](../../../examples/value/commands/main.go#L59-L75)

```go
func commandsOn(w io.Writer) error {
	who := func(name string) (string, bool) { return "ada", name == "WHO" }
	out, err := value.Expand(context.Background(), "hello $(printf '%s\\n' $WHO)!", value.WithLookup(who), value.WithCommands())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "on:       %s\n", out)

	_, err = value.Expand(context.Background(), "$(echo 'no such entry' >&2; exit 2)", value.WithCommands())
	var ce *expand.CommandError
	var ee *shell.ExitError
	if !errors.As(err, &ce) || !errors.As(err, &ee) {
		return fmt.Errorf("want *expand.CommandError wrapping *shell.ExitError, got %v", err)
	}
	fmt.Fprintf(w, "failed:   code=%d\n          %v\n", ee.Code, err)
	return nil
}
```
<!-- /ds:block hash=5b24b7 -->

The context bounds every command. For a limit or another shell that holds no matter which context is passed, hand a `shell.Runner` to `WithRunner`; `Expand` still passes its context to it, so cancelling stops the command.

<!-- ds:block id=value-bounded-f6k9p2ns -->
**boundedCommands** · [`examples/value/commands/main.go:81-91`](../../../examples/value/commands/main.go#L81-L91)

```go
func boundedCommands(w io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	_, err := value.Expand(ctx, "$(sleep 30)", value.WithCommands())
	fmt.Fprintf(w, "deadline: ErrTimeout=%t\n", errors.Is(err, shell.ErrTimeout))

	r := shell.Runner{Timeout: shortTimeout}
	_, err = value.Expand(context.Background(), "$(sleep 30)", value.WithRunner(r))
	fmt.Fprintf(w, "runner:   ErrTimeout=%t\n", errors.Is(err, shell.ErrTimeout))
	return nil
}
```
<!-- /ds:block hash=ad5a8e -->

## Step 4 — Allow-list commands with `WithRunner`

Any `plugin.CommandRunner` (one method, `RunCommand(cmd string) (string, error)`) can stand in for the shell. Use it to allow only known commands, to run them in a sandbox, or to fake them in tests. Whatever the runner returns as an error becomes an `*expand.CommandError` naming the command.

<!-- ds:block id=value-allowlist-c4t8j1wr -->
**allowListed** · [`examples/value/commands/main.go:111-121`](../../../examples/value/commands/main.go#L111-L121)

```go
func allowListed(w io.Writer) error {
	runner := allowList{"gopass show -o work/api": "fake-api-token"}
	out, err := value.Expand(context.Background(), "$(gopass show -o work/api)", value.WithRunner(runner))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "allowed:  %s\n", out)
	_, err = value.Expand(context.Background(), "$(rm -rf ~)", value.WithRunner(runner))
	fmt.Fprintf(w, "refused:  %v\n", err)
	return nil
}
```
<!-- /ds:block hash=b3076c -->

## Step 5 — Read secrets with value plugins

`WithPlugins` installs the same plugins the formats use. Each one owns a scheme: `${gopass:path}` (the password-store plugin), `${file:path}`, `${vault:path#field}`, `${aws:sm/…}` and `${aws:ssm/…}`. A scheme no installed plugin owns is read as an ordinary name, so installing a plugin never changes how other values read. In production only the construction changes; [the plugin guides](../plugins/README.md) cover each backend's options.

<!-- ds:block id=value-newplugins-u7h3x9ma -->
**newPlugins** · [`examples/value/secrets/main.go:91-109`](../../../examples/value/secrets/main.go#L91-L109)

```go
func newPlugins(dir, vaultAddr, awsEndpoint string) ([]plugin.Plugin, error) {
	gp := gopassplugin.New(gopassplugin.WithRunner(fakeGopass))
	fp := fileplugin.New(dir) // confined to dir by default
	vp, err := vault.New(vault.WithAddress(vaultAddr), vault.WithToken(fakeVaultToken))
	if err != nil {
		return nil, err
	}
	ap, err := aws.New(
		aws.WithRegion(fakeRegion),
		aws.WithCredentialsSource(func() (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: fakeKeyID, SecretAccessKey: fakeSecretKey}, nil
		}),
		aws.WithEndpoint(awsEndpoint),
	)
	if err != nil {
		return nil, err
	}
	return []plugin.Plugin{gp, fp, vp, ap}, nil
}
```
<!-- /ds:block hash=2042c7 -->

<!-- ds:block id=value-expandsecrets-s2p6d4kc -->
**expandSecrets** · [`examples/value/secrets/main.go:115-131`](../../../examples/value/secrets/main.go#L115-L131)

```go
func expandSecrets(w io.Writer, plugins []plugin.Plugin) error {
	for _, in := range []string{
		"${gopass:work/api}",
		"${file:api.key}",
		"${vault:secret/data/app#password}",
		"${aws:ssm//app/prod/db_url}",
		"Bearer ${gopass:work/api}",
		"[${nobody:thing}]", // no plugin owns the scheme: an ordinary, unset name
	} {
		out, err := value.Expand(context.Background(), in, value.WithPlugins(plugins...))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-34s => %s\n", in, out)
	}
	return nil
}
```
<!-- /ds:block hash=74c4cb -->

## Step 6 — Handle errors

`Expand` returns the error types the formats return, so one set of `errors.As` checks covers a `.env`, a JSONC file and a single value:

| Error | When |
|---|---|
| `*expand.RequiredError` | `${VAR:?msg}` or `${VAR?msg}` was not satisfied |
| `*expand.CommandError` | a `$(command)` failed; it unwraps to `*shell.ExitError`, `shell.ErrTimeout`, `shell.ErrNoShell` or your runner's error |
| `*plugin.Error` | a plugin failed; it names the plugin and unwraps to its error, such as `plugin.ErrNotFound` or `plugin.ErrUnavailable` |
| `value.ErrUnsupportedPlugin` (wrapped) | a plugin passed to `WithPlugins` has no ability a single value can use (a SaveGuard or Validator alone) |
| `*expand.UnsetError`, `*expand.UnknownSchemeError`, `*expand.CommandOffError` | only with `WithStrict`: an unset name, an unclaimed `${scheme:ref}`, a `$(command)` with commands off (Step 7); all match `expand.ErrStrict` |

<!-- ds:block id=value-requirederror-b5y1k8hp -->
**requiredError** · [`examples/value/env/main.go:122-130`](../../../examples/value/env/main.go#L122-L130)

```go
func requiredError(w io.Writer) error {
	_, err := value.Expand(context.Background(), "${DEMO_API_TOKEN:?set DEMO_API_TOKEN first}", value.WithEnv())
	var re *expand.RequiredError
	if !errors.As(err, &re) {
		return fmt.Errorf("want *expand.RequiredError, got %v", err)
	}
	fmt.Fprintf(w, "required:    key=%s\n             %v\n", re.Key, err)
	return nil
}
```
<!-- /ds:block hash=a54d29 -->

<!-- ds:block id=value-pluginerrors-g8r5n1vt -->
**pluginErrors** · [`examples/value/secrets/main.go:137-145`](../../../examples/value/secrets/main.go#L137-L145)

```go
func pluginErrors(w io.Writer, plugins []plugin.Plugin) error {
	_, err := value.Expand(context.Background(), "${gopass:work/missing}", value.WithPlugins(plugins...))
	var pe *plugin.Error
	if !errors.As(err, &pe) {
		return fmt.Errorf("want *plugin.Error, got %v", err)
	}
	fmt.Fprintf(w, "missing: plugin=%s not found=%t\n", pe.Plugin, errors.Is(err, plugin.ErrNotFound))
	return nil
}
```
<!-- /ds:block hash=ae2c0d -->

## Step 7 — Make unresolved references errors with `WithStrict`

By default a reference nothing answers reads as `""`, as in Compose, and a `$(command)` with commands off stays as written. A caller that needs certainty, such as a tool reading an API token from its config, would then send an empty token or the literal text `$(gopass show x)`. [`WithStrict()`](ds:block?id=withstrict-puc4zm2j) turns each of those into an error. Everything that resolves reads exactly as before: a defined name, a name set to `""`, `${VAR:-d}` and `${VAR-d}` (their default), and `${VAR:+a}` (`""` when unset is what it asks for).

<!-- ds:block id=value-strictresolves-k4w9p2td -->
**strictResolves** · [`examples/value/strict/main.go:52-65`](../../../examples/value/strict/main.go#L52-L65)

```go
func strictResolves(w io.Writer) error {
	for _, in := range []string{
		"Bearer ${API_TOKEN}",
		"[${EMPTY}]",
		"${REGION:-eu-west-1}",
	} {
		out, err := value.Expand(context.Background(), in, value.WithStrict(), value.WithLookup(lookup))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-24s => %s\n", in, out)
	}
	return nil
}
```
<!-- /ds:block hash=6c63bd -->

What it refuses, each as its own type that matches [`expand.ErrStrict`](ds:block?id=errstrict-wtxc9pe2):

| Reference | Error |
|---|---|
| a name no source defines (`$TOKEN`, `${TOKEN}`, a typo) | [`*expand.UnsetError`](ds:block?id=unseterror-fzc9dngm) with `Name` |
| a `${scheme:ref}` no installed plugin claims (`${vault:x}` without the vault plugin, `${gopas:x}`) | [`*expand.UnknownSchemeError`](ds:block?id=unknownschemeerror-fg4srxup) with `Scheme` and `Ref` |
| a `$(command)` without `WithCommands` or `WithRunner` | [`*expand.CommandOffError`](ds:block?id=commandofferror-j9abapdg) with `Cmd` |

`${VAR:?msg}` keeps its own `*expand.RequiredError`, which does not match `ErrStrict`. Each error carries `Offset`, the byte offset in `s` of the reference that failed, and `Via`, the variable whose value held it when the reference was reached through another one; never a value ([pinned in the engine](ds:block?id=teststrictrefuses-8m4ujeu7&assert=true) and [through `value`](ds:block?id=teststrict-uemkh2yq&assert=true)). An unclaimed `${scheme:ref}` is treated as one name in strict mode: `${vault:db-pass}` is refused, never read as the name `vault:db` with the default `pass` as it is without `WithStrict`.

<!-- ds:block id=value-strictrefuses-r7n3c5hm -->
**strictRefuses** · [`examples/value/strict/main.go:72-100`](../../../examples/value/strict/main.go#L72-L100)

```go
func strictRefuses(w io.Writer) error {
	for _, in := range []string{
		"Bearer ${API_TOKN}",         // a typo: unset
		"${gopas:work/api}",          // a scheme no plugin claims
		"$(gopass show -o work/api)", // commands are off
		"${API_SECRET:?set it}",      // required: its own error
	} {
		_, err := value.Expand(context.Background(), in, value.WithStrict(), value.WithLookup(lookup))
		var (
			unset   *expand.UnsetError
			scheme  *expand.UnknownSchemeError
			cmdOff  *expand.CommandOffError
			require *expand.RequiredError
		)
		kind := "none"
		switch {
		case errors.As(err, &unset):
			kind = "unset " + unset.Name
		case errors.As(err, &scheme):
			kind = "unknown scheme " + scheme.Scheme
		case errors.As(err, &cmdOff):
			kind = "command off"
		case errors.As(err, &require):
			kind = "required " + require.Key
		}
		fmt.Fprintf(w, "%-28s strict=%-5t %s\n  %v\n", in, errors.Is(err, expand.ErrStrict), kind, err)
	}
	return nil
}
```
<!-- /ds:block hash=9328ca -->

The formats have the same switch, named `WithStrictExpand()` there because jsonc's `Strict()` already means "parse as plain JSON": [dotenv expansion](../dotenv/expansion.md), [jsonc expansion](../jsonc/expansion-and-plugins.md). A format of your own sets `expand.Engine.Strict` ([expand](expand.md)).

## Try it

```sh
cd examples/value/env && go run .
```

```text
with env:    postgres://ada@db.internal/app
without env: postgres://@/app
${DEMO_PORT:-5432}                   => 5432
${DEMO_REGION:-${DEMO_HOST}}         => db.internal
${DEMO_USER:+user is set}            => user is set
cost: $$5                            => cost: $5
${DEMO_USER:?DEMO_USER must be set}  => ada
layered:     postgres://ada@localhost/app
required:    key=DEMO_API_TOKEN
             expand: required variable DEMO_API_TOKEN is not set or is empty: set DEMO_API_TOKEN first
```

```sh
cd examples/value/commands && go run .
```

```text
off:      token=$(echo never-runs)
on:       hello ada!
failed:   code=2
          expand: command "echo 'no such entry' >&2; exit 2" failed: shell: command "echo 'no such entry' >&2; exit 2" exited with code 2: no such entry
deadline: ErrTimeout=true
runner:   ErrTimeout=true
allowed:  fake-api-token
refused:  expand: command "rm -rf ~" failed: command not allowed
```

```sh
cd examples/value/secrets && go run .
```

```text
${gopass:work/api}                 => fake-gopass-token
${file:api.key}                    => fake-file-key
${vault:secret/data/app#password}  => fake-vault-password
${aws:ssm//app/prod/db_url}        => postgres://db.internal/app
Bearer ${gopass:work/api}          => Bearer fake-gopass-token
[${nobody:thing}]                  => []
missing: plugin=gopass not found=true
```

```sh
cd examples/value/strict && go run .
```

```text
Bearer ${API_TOKEN}      => Bearer fake-token
[${EMPTY}]               => []
${REGION:-eu-west-1}     => eu-west-1
Bearer ${API_TOKN}           strict=true  unset API_TOKN
  expand: variable API_TOKN is not set (offset 7)
${gopas:work/api}            strict=true  unknown scheme gopas
  expand: no resolver for scheme "gopas" in ${gopas:work/api} (offset 0)
$(gopass show -o work/api)   strict=true  command off
  expand: command substitution is off: $(gopass show -o work/api) (offset 0)
${API_SECRET:?set it}        strict=false required API_SECRET
  expand: required variable API_SECRET is not set or is empty: set it
```

## Pitfalls

- **Unset is not an error.** `${TOKEN}` with nothing defining `TOKEN` is `""`. Write `${TOKEN:?set TOKEN}` when one missing value must stop the program, or pass `WithStrict()` to refuse every unresolved reference.
- **Without a source, nothing has a value.** `Expand(ctx, "$HOME")` is `""`: pass `WithEnv()` to read the environment.
- **A `-` after a colon is an operator, not part of a scheme.** `${nobody:owns-this}` is the name `nobody:owns` with the default `this`, exactly as in the formats. Scheme references such as `${vault:a/b}` are not affected, because a scheme's colon is never followed by `-`, `+` or `?`.
- **Command output is trimmed of trailing newlines only.** A command printing `"  x  \n"` gives `"  x  "`.
- **The runner you give wins.** `WithRunner` and a CommandRunner plugin replace the shell; the last one given is used. `WithCommands` after them does not bring the shell back.
- **A `$(command)` on Windows runs in PowerShell.** A value written for `sh` (`$(printf '%s' x)`) needs `WithRunner(shell.Runner{Shell: []string{"bash", "-c"}})` there; see [shell § Pitfalls](shell.md#pitfalls).
- **Transformers and observers see path `""`.** A single value has no key or path; a `ValueTransformer` that picks values by path sees `""`.

## Related

- [shell](shell.md): the command runner on its own.
- [expand](expand.md): the engine underneath, for a format of your own.
- [Value plugins](../plugins/README.md): gopass, file, vault, aws, and writing your own.
- [dotenv expansion](../dotenv/expansion.md) and [jsonc expansion](../jsonc/expansion-and-plugins.md): the same rules over whole files.
- Examples: [`examples/value/env`](../../../examples/value/env), [`examples/value/commands`](../../../examples/value/commands), [`examples/value/secrets`](../../../examples/value/secrets), [`examples/value/strict`](../../../examples/value/strict).
- API: [value on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/value).
