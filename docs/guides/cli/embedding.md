# Embed the CLIs in your own program

Every textdata CLI is a Go package you can run in-process: `dotenvcmd.Execute` is the whole `dotenvctl`, `jsonccmd.Execute` the whole `textdatactl jsonc`, and `ctl.Execute` the `textdatactl` umbrella. Each takes the arguments, a stdout and a stderr writer, returns the exit code instead of calling `os.Exit`, and can print your program's name instead of its own. Embed them when your tool should offer `myapp env set KEY=VALUE` or `myapp jsonc get …` without shipping a second binary, or when a Go test should drive the real CLI without building one.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv/cli@latest           # dotenvcmd
go get github.com/ubgo/textdata/jsonc/cli@latest            # jsonccmd
go get github.com/ubgo/textdata/cmd/textdatactl@latest      # ctl, the umbrella
```

The two runnable programs for this page are [`examples/cli/embed-dotenvctl`](../../../examples/cli/embed-dotenvctl) and [`examples/cli/embed-jsonc`](../../../examples/cli/embed-jsonc). These modules use cobra; the format libraries themselves (`dotenv`, `jsonc`) stay free of third-party dependencies.

| Package | Entry points | Grafting into a cobra tree |
|---|---|---|
| `dotenv/cli/dotenvcmd` | `Execute(args, stdout, stderr, opts...)`, `WithProgramName`, `WithCallbackArgs` | delegate from a command with flag parsing off (Step 4) |
| `jsonc/cli/jsonccmd` | `Execute(args, stdout, stderr, opts...)`, `Command(opts...)`, `WithProgramName`, `ExitError` | `Command` returns a `*cobra.Command` named `jsonc` (Step 5) |
| `cmd/textdatactl/ctl` | `Execute(args, stdout, stderr)`, `Formats()`, `Format` | `Formats` lists each format's `Execute` (Step 6) |

All three share the exit-code contract: `0` ok, `1` the operation failed (or a diff found differences), `2` the command line was malformed. Each package exports `ExitOK`, `ExitFailure` and `ExitUsage`.

## Step 1 — run a dotenv verb in-process

`args` are what a user types after `dotenvctl`, without the program name. Output lands in the writers you pass, exactly as on a terminal, and `--json` works the same way:

<!-- ds:block id=runverb-hyve6qvu -->
**runVerb** · [`examples/cli/embed-dotenvctl/main.go:74-76`](../../../examples/cli/embed-dotenvctl/main.go#L74-L76)

```go
func runVerb(w io.Writer, args ...string) int {
	return dotenvcmd.Execute(args, w, w)
}
```
<!-- /ds:block hash=aa5f14 -->

`runVerb(w, "-f", envFile, "get", "URL", "--expand")` prints `http://localhost:8080` and returns `0`; a missing key returns `1`.

## Step 2 — print your own program name

Embedded, the CLI still calls itself `dotenvctl` in its failure lines, and a user who never installed `dotenvctl` would be told to run `dotenvctl --help`. `WithProgramName` takes the words your users type:

<!-- ds:block id=runashost-f5w6cdqm -->
**runAsHost** · [`examples/cli/embed-dotenvctl/main.go:84-86`](../../../examples/cli/embed-dotenvctl/main.go#L84-L86)

```go
func runAsHost(w io.Writer, args ...string) int {
	return dotenvcmd.Execute(args, w, w, dotenvcmd.WithProgramName(hostProgram+" "+hostEnvVerb))
}
```
<!-- /ds:block hash=679332 -->

The name reaches the `<name>: ` failure prefix, the `Run '<name> --help' for usage.` hint, the help Usage line, the examples inside `--help` (`myapp env get DATABASE_URL`) and the `--version` line; its first word names the `completion` script. It does not reach the JSON envelope (which carries no program name), or exec plugins, which are still found as `dotenvctl-<name>` on PATH.

## Step 3 — check the exit code

`Execute` never exits. Pass its return value to `os.Exit` in `main`, or branch on it as the example's `run` does. The `dotenvctl` binary itself is one line, `os.Exit(dotenvcmd.Execute(os.Args[1:], os.Stdout, os.Stderr))` ([`dotenv/cli/cmd/dotenvctl`](../../../dotenv/cli/cmd/dotenvctl)), and `textdatactl` the same over `ctl.Execute`. Under `--json` every outcome, usage errors included, is one envelope on stdout, so a host can decode it instead of parsing text.

## Step 4 — mount the dotenv verbs in a cobra tree

`dotenvcmd` has no cobra command to graft: it owns its flag parsing, exec-plugin dispatch and error printing, and those must see the raw arguments. Mount it by delegation instead: a subcommand with `DisableFlagParsing` hands its args to `Execute`:

<!-- ds:block id=envcommand-whd27a9r -->
**envCommand** · [`examples/cli/embed-dotenvctl/main.go:94-108`](../../../examples/cli/embed-dotenvctl/main.go#L94-L108)

```go
func envCommand() *cobra.Command {
	return &cobra.Command{
		Use:                hostEnvVerb + " <verb> [args]",
		Short:              "Edit .env files (the dotenvctl verbs)",
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			code := dotenvcmd.Execute(args, c.OutOrStdout(), c.ErrOrStderr(),
				dotenvcmd.WithProgramName(hostProgram+" "+hostEnvVerb))
			if code != dotenvcmd.ExitOK {
				return exitCodeError(code)
			}
			return nil
		},
	}
}
```
<!-- /ds:block hash=638b17 -->

The host root runs it and turns the carried code back into an exit code:

<!-- ds:block id=runhosttree-3jzzrrdy -->
**runHostTree** · [`examples/cli/embed-dotenvctl/main.go:122-136`](../../../examples/cli/embed-dotenvctl/main.go#L122-L136)

```go
func runHostTree(w io.Writer, args ...string) int {
	root := &cobra.Command{Use: hostProgram, SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(envCommand())
	root.SetArgs(args)
	root.SetOut(w)
	root.SetErr(w)
	if err := root.Execute(); err != nil {
		if code, ok := err.(exitCodeError); ok {
			return int(code)
		}
		fmt.Fprintf(w, "%s: %v\n", hostProgram, err)
		return dotenvcmd.ExitUsage
	}
	return dotenvcmd.ExitOK
}
```
<!-- /ds:block hash=3494f4 -->

This is how `textdatactl` mounts `dotenv`: everything after the namespace goes to the format's `Execute` with the program name `textdatactl dotenv`.

## Step 5 — run or graft the jsonc verbs

`jsonccmd.Execute` works like `dotenvcmd.Execute`; its default program name is `textdatactl jsonc`:

<!-- ds:block id=runjsonc-bqrk46h2 -->
**runJSONC** · [`examples/cli/embed-jsonc/main.go:87-89`](../../../examples/cli/embed-jsonc/main.go#L87-L89)

```go
func runJSONC(w io.Writer, args ...string) int {
	return jsonccmd.Execute(args, w, w, jsonccmd.WithProgramName(hostProgram+" jsonc"))
}
```
<!-- /ds:block hash=93b471 -->

Unlike dotenvcmd, jsonccmd can also hand you its verb tree: `Command` returns a `*cobra.Command` named `jsonc` that writes to the host command's `OutOrStdout`/`ErrOrStderr`. A verb failure comes back as `*jsonccmd.ExitError` with `Quiet` set (already printed); cobra's own usage errors come back unprinted, for the host to report:

<!-- ds:block id=rungrafted-sefus95z -->
**runGrafted** · [`examples/cli/embed-jsonc/main.go:96-115`](../../../examples/cli/embed-jsonc/main.go#L96-L115)

```go
func runGrafted(w io.Writer, args ...string) int {
	root := &cobra.Command{Use: hostProgram, SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(jsonccmd.Command(jsonccmd.WithProgramName(hostProgram + " jsonc")))
	root.SetArgs(args)
	root.SetOut(w)
	root.SetErr(w)
	err := root.Execute()
	if err == nil {
		return jsonccmd.ExitOK
	}
	var xe *jsonccmd.ExitError
	if errors.As(err, &xe) {
		if !xe.Quiet && xe.Err != nil {
			fmt.Fprintf(w, "%s: %v\n", hostProgram, xe.Err)
		}
		return xe.Code
	}
	fmt.Fprintf(w, "%s: %v\n", hostProgram, err)
	return jsonccmd.ExitUsage
}
```
<!-- /ds:block hash=36c918 -->

## Step 6 — embed the whole umbrella

`ctl.Execute` is all of `textdatactl`: `ctl.Execute([]string{"jsonc", "get", file, "server.host"}, w, w)`. `ctl.Formats` lists the mounted format CLIs, each with its namespace, a one-line help text and an `Execute` that takes the program name to print, so a host can build its own umbrella over the same formats:

<!-- ds:block id=formatnames-tbfqmvgp -->
**formatNames** · [`examples/cli/embed-jsonc/main.go:122-128`](../../../examples/cli/embed-jsonc/main.go#L122-L128)

```go
func formatNames() []string {
	var names []string
	for _, f := range ctl.Formats() {
		names = append(names, f.Name)
	}
	return names
}
```
<!-- /ds:block hash=db2a42 -->

## Try it

```console
$ cd examples/cli/embed-dotenvctl && go run .
http://localhost:8080
exit: 0
dotenvctl: key "PORT" not found in missing.env
exit: 1
myapp env: unknown command "nosuchverb" for "myapp env"
Run 'myapp env --help' for usage.
exit: 2
{"ok":true,"data":{"results":[{"key":"PORT","created":true}],"changed":true,"dry_run":false}}
exit: 0
myapp env: key "PORT" not found in missing.env
exit: 1
```

```console
$ cd examples/cli/embed-jsonc && go run .
exit: 0
{
  // where the API listens
  "server": {"host": "localhost", "port": 9090}
}
9090
exit: 0
myapp jsonc: missing.jsonc: no such file
exit: 1
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"localhost","port":9090}}}
exit: 0
myapp jsonc: missing.jsonc: no such file
exit: 1
myapp: unknown command "nosuchverb" for "myapp jsonc"
exit: 2
formats: [dotenv jsonc]
localhost
exit: 0
textdatactl dotenv: key "PORT" not found in missing.env
exit: 1
```

The `nosuchverb` line in the second run is the host's own message: through `Command`, cobra's usage error comes back unprinted and `runGrafted` reports it under `myapp`.

## Pitfalls

- **`Execute` reads the real process for some things.** `dotenvcmd.Execute` dispatches exec plugins from the process PATH, `run` starts children with the process environment as their base, and confirmation prompts read `os.Stdin`. Only stdout and stderr are injectable.
- **Exec plugins call the host back through its own executable.** A plugin's `DOTENVCTL_BIN` is `os.Executable()`, which is your program, and plugins call `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f FILE get KEY`. If your program mounts the verbs under a namespace (`myapp env`), declare it with `dotenvcmd.WithCallbackArgs("env")` so the call reaches the verbs, as `textdatactl` does with `WithCallbackArgs("dotenv")`; without it the call reaches your root and fails. Serving the verbs at the top level, as [`examples/cli/exec-plugin`](../../../examples/cli/exec-plugin) does, needs no option. See [exec plugins](exec-plugins.md).
- **`--version` reports the CLI module's version, not yours.** An embedded `dotenvcmd` prints the version of `github.com/ubgo/textdata/dotenv/cli` it was built with (and `jsonccmd` that of `jsonc/cli`), after your program name; a workspace or `replace` build prints `devel`.
- **Do not graft dotenvcmd with cobra flag parsing on.** The host would reject `-f` and `--json` before dotenvctl sees them (`unknown shorthand flag: 'f'`); keep `DisableFlagParsing: true`.
- **With `jsonccmd.Command`, print usage errors yourself.** Only verb failures are printed by the tree; an unknown verb or flag comes back as a plain error, unprinted.
- **Completion goes through your binary.** With `WithProgramName("myapp env")`, `myapp env completion bash` prints a script for `myapp`, which asks `myapp __complete env …` for candidates. Route that to `dotenvcmd.Execute` as `__complete …` (strip `env`), the way `textdatactl` does for `dotenv` and `jsonc`; if your root is a cobra tree with its own `completion`, your script already calls it and only the routing is needed.
- **One `Execute` per call, no shared state.** Each call builds a fresh command tree with no package-level state, so one call's flags cannot leak into the next; the process-wide PATH, environment and working directory are shared.

## Related

- [dotenvctl command reference](dotenvctl.md), [jsonc CLI](jsonc-cli.md), [textdatactl](../getting-started/textdatactl.md)
- [Exec plugins](exec-plugins.md)
- [`dotenvcmd` example functions](../../../dotenv/cli/dotenvcmd/example_test.go) (`ExampleExecute`, `ExampleExecute_programName`)
- READMEs: [`dotenv/cli`](../../../dotenv/cli/README.md), [`jsonc/cli`](../../../jsonc/cli/README.md#embedding), [`cmd/textdatactl`](../../../cmd/textdatactl/README.md)
- API: [`dotenvcmd`](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/dotenvcmd), [`jsonccmd`](https://pkg.go.dev/github.com/ubgo/textdata/jsonc/cli/jsonccmd), [`ctl`](https://pkg.go.dev/github.com/ubgo/textdata/cmd/textdatactl/ctl)
