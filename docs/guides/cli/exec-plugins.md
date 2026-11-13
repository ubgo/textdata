# Exec plugins

Any executable named `dotenvctl-<name>` on your PATH becomes a dotenvctl subcommand, git-style: `dotenvctl acme push` runs `dotenvctl-acme push`, in any language. dotenvctl passes the plugin a few `DOTENVCTL_*` context variables, streams its output through, and returns its exit code. Write one when you need a verb dotenvctl does not have (export to your shell, push to a secret store with no built-in provider, a team-specific check) without forking the CLI or writing Go.

## Before you start

You need `dotenvctl` (or `textdatactl dotenv`, see the pitfall about callbacks below) and a directory on your PATH you can write to:

```sh
go install github.com/ubgo/textdata/dotenv/cli/cmd/dotenvctl@latest
```

The runnable program for this page is [`examples/cli/exec-plugin`](../../../examples/cli/exec-plugin): it writes a `dotenvctl-hello` script into a temp directory, makes that directory the whole PATH, lists it, and runs it. POSIX only, because the plugin is a `/bin/sh` script.

## Step 1 — write the plugin

The file name is the whole registration: `dotenvctl-hello` serves `dotenvctl hello`. This one prints its arguments and the context it was given, then calls the host back for a value:

<!-- ds:block id=helloscript-jcm2k9nu -->
**helloScript** · [`examples/cli/exec-plugin/main.go:62-70`](../../../examples/cli/exec-plugin/main.go#L62-L70)

```go
`#!/bin/sh
echo "hello, args: [$*]"
echo "file: ${DOTENVCTL_FILE##*/}"
echo "json: $DOTENVCTL_JSON"
echo "placeholder: $DOTENVCTL_PLACEHOLDER"
echo "version set: ${DOTENVCTL_VERSION:+yes}"
printf 'callback: '
"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get GREETING
`
```
<!-- /ds:block hash=cc6920 -->

It must be executable; the example writes it, and a plugin with a built-in's name that will be shadowed, with mode `0755`:

<!-- ds:block id=installplugins-hxu85kcd -->
**installPlugins** · [`examples/cli/exec-plugin/main.go:135-143`](../../../examples/cli/exec-plugin/main.go#L135-L143)

```go
func installPlugins(dir string) error {
	scripts := map[string]string{pluginName: helloScript, shadowedName: shadowedScript}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, pluginPrefix+name), []byte(body), execMode); err != nil {
			return err
		}
	}
	return nil
}
```
<!-- /ds:block hash=f8d498 -->

## Step 2 — read the context variables

dotenvctl adds these to the plugin's environment. They are facts only; secret values are never passed in the environment. The names are exported Go constants in `dotenvcmd`:

<!-- ds:block id=pluginenv-efwe4fby -->
**EnvPluginFile** · [`dotenv/cli/dotenvcmd/execplugin.go:25-42`](../../../dotenv/cli/dotenvcmd/execplugin.go#L25-L42)

```go
const (
	// EnvPluginFile is the absolute path of the -f file the user chose.
	EnvPluginFile = "DOTENVCTL_FILE"
	// EnvPluginJSON is "1" when the user passed --json, else "0".
	EnvPluginJSON = "DOTENVCTL_JSON"
	// EnvPluginPlaceholder is the active placeholder regexp.
	EnvPluginPlaceholder = "DOTENVCTL_PLACEHOLDER"
	// EnvPluginBin is the absolute path of the running host executable — the
	// callback handle, always followed by EnvPluginBinArgs:
	// `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" list --json`.
	EnvPluginBin = "DOTENVCTL_BIN"
	// EnvPluginBinArgs holds the space-separated words that must follow
	// DOTENVCTL_BIN to reach the dotenv verbs: empty for dotenvctl, "dotenv"
	// under textdatactl (WithCallbackArgs). Expand it unquoted.
	EnvPluginBinArgs = "DOTENVCTL_BIN_ARGS"
	// EnvPluginVersion is the host version, so plugins can gate on features.
	EnvPluginVersion = "DOTENVCTL_VERSION"
)
```
<!-- /ds:block hash=22e0c8 -->

| Variable | Value |
|---|---|
| `DOTENVCTL_FILE` | absolute path of the `-f` file (default `.env` in the current directory) |
| `DOTENVCTL_JSON` | `1` when `--json` was given before the plugin name, else `0` |
| `DOTENVCTL_PLACEHOLDER` | the placeholder pattern, always the default `^__[A-Z0-9_]+__$` |
| `DOTENVCTL_BIN` | absolute path of the running host executable, the callback handle; empty if the host cannot name itself |
| `DOTENVCTL_BIN_ARGS` | the words that must follow `DOTENVCTL_BIN` to reach the verbs, space-separated: empty under `dotenvctl`, `dotenv` under `textdatactl dotenv`, whatever an embedding host declares with `WithCallbackArgs` |
| `DOTENVCTL_VERSION` | the host's version, the string `--version` prints after the name |

## Step 3 — call the host back for values

To read values, run `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" <verb>`. Leave `${DOTENVCTL_BIN_ARGS:-}` unquoted: it splits into the namespace words under `textdatactl dotenv` (`textdatactl dotenv -f … get KEY`) and disappears under `dotenvctl`, so one plugin line works under both; the `:-` keeps `set -u` scripts working on older hosts that do not set it. The plugin then inherits dotenvctl's parsing, expansion, required-variable errors and selection flags, instead of parsing `.env` itself and getting quoting or `${VAR}` wrong. For many keys, call `list --expand --json` once and parse the envelope rather than one `get` per key.

`DOTENVCTL_BIN` is the host's own executable. When the host is a Go program that embeds `dotenvcmd`, that is the program, which must then serve the call. The example does it the simplest way, with an environment flag it sets only while the plugin runs:

<!-- ds:block id=serveashost-3adxb4gf -->
**serveAsHost** · [`examples/cli/exec-plugin/main.go:96-101`](../../../examples/cli/exec-plugin/main.go#L96-L101)

```go
func serveAsHost() (int, bool) {
	if os.Getenv(hostModeEnv) != hostModeOn {
		return 0, false
	}
	return dotenvcmd.Execute(os.Args[1:], os.Stdout, os.Stderr), true
}
```
<!-- /ds:block hash=1cf7e0 -->

## Step 4 — list what dotenvctl found

`dotenvctl plugins` scans PATH for `dotenvctl-*` files with an execute bit and lists each name once (the first PATH hit wins, as with any command lookup). Built-in verbs always win: a plugin named after one is listed as `shadowed by built-in` and never runs. The rule is one function, also used at dispatch time; cobra's implicit `help` and `completion` count as built-ins:

<!-- ds:block id=hasbuiltin-ck96jzwc -->
**hasBuiltin** · [`dotenv/cli/dotenvcmd/execplugin.go:146-154`](../../../dotenv/cli/dotenvcmd/execplugin.go#L146-L154)

```go
func hasBuiltin(root *cobra.Command, name string) bool {
	for _, c := range root.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	// Cobra's implicit verbs are built-ins too.
	return slices.Contains(cobraImplicitCommands, name)
}
```
<!-- /ds:block hash=f3b04e -->

The example reads the `--json` form:

<!-- ds:block id=listplugins-xgpent5s -->
**listPlugins** · [`examples/cli/exec-plugin/main.go:177-194`](../../../examples/cli/exec-plugin/main.go#L177-L194)

```go
func listPlugins(w io.Writer) error {
	var out bytes.Buffer
	if code := dotenvcmd.Execute([]string{"plugins", "--json"}, &out, w); code != dotenvcmd.ExitOK {
		return fmt.Errorf("plugins: exit %d", code)
	}
	var env pluginsEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		return err
	}
	for _, p := range env.Data.Plugins {
		status := "active"
		if p.Shadowed {
			status = "shadowed by built-in"
		}
		fmt.Fprintf(w, "plugin %s: %s (%s)\n", p.Name, status, filepath.Base(p.Path))
	}
	return nil
}
```
<!-- /ds:block hash=e44445 -->

From the shell, with the plugin installed in `/usr/local/bin` and a stray `dotenvctl-github` next to it, the human table looks like this:

```console
$ dotenvctl plugins
NAME    PATH                             STATUS
github  /usr/local/bin/dotenvctl-github  shadowed by built-in
hello   /usr/local/bin/dotenvctl-hello   active
$ dotenvctl plugins --json
{"ok":true,"data":{"plugins":[{"name":"github","path":"/usr/local/bin/dotenvctl-github","shadowed":true},{"name":"hello","path":"/usr/local/bin/dotenvctl-hello","shadowed":false}]}}
```

## Step 5 — run it

Dispatch happens before dotenvctl parses its own command line: when the first non-flag word names no built-in and a `dotenvctl-<word>` exists on PATH, the rest of the arguments go to the plugin verbatim and its exit code becomes dotenvctl's. Only `-f`/`--file` and `--json` placed before the plugin name are read by the host:

<!-- ds:block id=runplugin-3ch8m563 -->
**runPlugin** · [`examples/cli/exec-plugin/main.go:202-209`](../../../examples/cli/exec-plugin/main.go#L202-L209)

```go
func runPlugin(w io.Writer, args ...string) int {
	if err := os.Setenv(hostModeEnv, hostModeOn); err != nil {
		fmt.Fprintln(w, err)
		return dotenvcmd.ExitFailure
	}
	defer func() { _ = os.Unsetenv(hostModeEnv) }()
	return dotenvcmd.Execute(args, w, w)
}
```
<!-- /ds:block hash=57a2c8 -->

From the shell:

```console
$ dotenvctl -f .env.prod --json hello a b
hello, args: [a b]
file: .env.prod
json: 1
…
$ dotenvctl hello -f .env.prod
hello, args: [-f .env.prod]
file: .env
json: 0
…
```

In the second run `-f .env.prod` came after the plugin name, so it went to the script and `DOTENVCTL_FILE` stayed `.env`.

## Try it

```console
$ cd examples/cli/exec-plugin && go run .
plugin github: shadowed by built-in (dotenvctl-github)
plugin hello: active (dotenvctl-hello)
hello, args: [world]
file: .env
json: 0
placeholder: ^__[A-Z0-9_]+__$
version set: yes
callback: hi from the file
exit: 0
hello, args: []
file: .env
json: 1
placeholder: ^__[A-Z0-9_]+__$
version set: yes
callback: hi from the file
exit: 0
```

`callback:` is the value the script got by running `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get GREETING`, served by the example binary itself.

## A complete plugin: dotenvctl-export

This plugin prints the selected keys as shell `export` lines, so `eval "$(dotenvctl export)"` loads a file into your current shell. It uses the whole contract: it reads the context variables, calls the host back for names and values (so selection flags, expansion and required variables behave as in the built-in verbs), honours the placeholder pattern, and passes host failures on with the right exit code. Save it as `dotenvctl-export`, `chmod +x` it and put it on your PATH (needs `bash` 4 or later):

```bash
#!/usr/bin/env bash
# dotenvctl-export: print the selected keys as shell `export` lines.
# Usage: dotenvctl [-f FILE] export [selection flags]
set -euo pipefail

if [ -z "${DOTENVCTL_BIN:-}" ]; then
  echo "dotenvctl-export: run me as 'dotenvctl export'" >&2
  exit 2
fi

# Ask the host for the names, so selection flags work exactly as in `keys`.
# On failure dotenvctl has already printed its message on stderr.
if ! names=$("$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" keys "$@"); then
  exit 1
fi

while IFS= read -r key; do
  [ -n "$key" ] || continue
  # Ask the host for each value, expanded with dotenvctl's own rules.
  if ! value=$("$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get "$key" --expand); then
    exit 1
  fi
  if [[ $value =~ $DOTENVCTL_PLACEHOLDER ]]; then
    echo "skipped $key (placeholder)" >&2
    continue
  fi
  # Single-quote the value; an embedded ' becomes '\''.
  printf "export %s='%s'\n" "$key" "${value//\'/\'\\\'\'}"
done <<< "$names"
```

With this `.env`:

```env
DB_HOST=db.internal
DB_URL=postgres://${DB_HOST}/app
DB_PASS=__YOU__
GREETING="it's here"
```

```console
$ dotenvctl plugins
NAME    PATH                             STATUS
export  /usr/local/bin/dotenvctl-export  active
$ dotenvctl export
export DB_HOST='db.internal'
export DB_URL='postgres://db.internal/app'
skipped DB_PASS (placeholder)
export GREETING='it'\''s here'
$ dotenvctl export --prefix DB_ 2>/dev/null
export DB_HOST='db.internal'
export DB_URL='postgres://db.internal/app'
$ eval "$(dotenvctl export --prefix DB_URL)"; echo "$DB_URL"
postgres://db.internal/app
$ dotenvctl export --include NOPE; echo "exit=$?"
dotenvctl: envkit: --include key "NOPE" not found
exit=1
$ dotenvctl -f req.env export; echo "exit=$?"      # req.env holds NEED=${MISSING:?set MISSING first}
dotenvctl: dotenv: required variable MISSING is not set or is empty: set MISSING first
exit=1
$ dotenvctl-export; echo "exit=$?"
dotenvctl-export: run me as 'dotenvctl export'
exit=2
```

To push to a real backend instead of printing, replace the `printf` with your store's CLI and feed the value on stdin, never as an argument (`printf '%s' "$value" | acme-cli secret set "$key"`), the same rule the built-in providers follow. To write a provider in Go instead, with the shared flags, guards and confirm prompt ready-made, see [write a provider](../providers/write-a-provider.md).

## Pitfalls

- **Only flags before the plugin name are the host's.** `dotenvctl -f .env.prod export` sets `DOTENVCTL_FILE`; `dotenvctl export -f .env.prod` passes `-f .env.prod` to the script and `DOTENVCTL_FILE` stays `.env`. The same goes for `--json` and `DOTENVCTL_JSON`.
- **No execute bit, no plugin.** A script without `chmod +x` is neither listed nor dispatched; `dotenvctl hello` then fails as an unknown command with exit `2`. On Windows any matching file counts.
- **A built-in name can never be a plugin name.** `dotenvctl-github`, `dotenvctl-keys`, `dotenvctl-help` and `dotenvctl-completion` never run; `plugins` shows them as `shadowed by built-in`.
- **Test the host's exit status, not its output.** `dotenvctl get NOPE` prints its message on stderr and exits `1`; captured with `$(…)` the value is empty, which a real empty value also is.
- **`DOTENVCTL_PLACEHOLDER` is always the default pattern.** `matrix --placeholder` does not change it; a team with its own pattern must pass it to the plugin another way.
- **Always put `${DOTENVCTL_BIN_ARGS:-}` after `DOTENVCTL_BIN`.** Under `textdatactl dotenv`, `DOTENVCTL_BIN` is the `textdatactl` binary, which needs the `dotenv` namespace; a plugin that calls `"$DOTENVCTL_BIN" -f "$DOTENVCTL_FILE" get KEY` without it fails there with `textdatactl: unknown shorthand flag: 'f' in -f` and exit `2`. Quoting it (`"$DOTENVCTL_BIN_ARGS"`) passes an empty argument under `dotenvctl`, which is an unknown command.
- **A plugin is as trustworthy as anything else on your PATH.** dotenvctl adds discovery (`plugins`) and the no-shadowing rule, not a sandbox.
- **Plugins are POSIX-friendly, not portable by default.** On Windows the executable's extension is stripped from the name, and a shell script needs a shell to run it.

## Related

- [dotenvctl command reference § plugins](dotenvctl.md#plugins--discover-exec-plugins)
- [Embedding the CLI](embedding.md): when your own program is the host
- [Recipes § load a file into your shell](recipes.md#load-a-file-into-your-shell)
- [Write a provider in Go](../providers/write-a-provider.md) and the [GitHub](../providers/github.md) and [Vercel](../providers/vercel.md) providers
- Example: [`examples/cli/exec-plugin`](../../../examples/cli/exec-plugin); API: [`dotenvcmd` on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/dotenv/cli/dotenvcmd#pkg-constants)
