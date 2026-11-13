# textdatactl: the command line

`textdatactl` is one binary for every textdata format: `textdatactl dotenv …` reads and edits `.env` files, `textdatactl jsonc …` reads and edits JSON and JSONC files, and `textdatactl version` reports what you are running. Every edit keeps the comments and layout you did not touch, exactly as the Go libraries do. Use it in shell scripts, CI steps and Makefiles, or by hand when you want `set` without opening an editor.

## Before you start

Install with Go (needs Go 1.26.4 or later, the version `cmd/textdatactl` declares):

```sh
go install github.com/ubgo/textdata/cmd/textdatactl@latest
```

Or download a prebuilt binary from the [`textdatactl/v0.1.4` release](https://github.com/ubgo/textdata/releases/tag/textdatactl%2Fv0.1.4): archives for `darwin_amd64`, `darwin_arm64`, `linux_amd64`, `linux_arm64` (`.tar.gz`) and `windows_amd64` (`.zip`), each holding the single `textdatactl` binary, plus `checksums.txt`:

```sh
curl -fsSLO https://github.com/ubgo/textdata/releases/download/textdatactl/v0.1.4/textdatactl_v0.1.4_linux_amd64.tar.gz
curl -fsSLO https://github.com/ubgo/textdata/releases/download/textdatactl/v0.1.4/checksums.txt
sha256sum -c checksums.txt --ignore-missing   # textdatactl_v0.1.4_linux_amd64.tar.gz: OK
tar xzf textdatactl_v0.1.4_linux_amd64.tar.gz && sudo mv textdatactl /usr/local/bin/
```

The release tag is `textdatactl/v0.1.4` (the binary's tag); the Go module is tagged `cmd/textdatactl/v0.1.4`, which is what `go install …@latest` resolves. To build from a clone instead: `cd cmd/textdatactl && go build -o textdatactl .`.

The examples below run in a folder holding the `.env` and `config.jsonc` from [`examples/getting-started/first-read`](../../../examples/getting-started/first-read) (copy its `app.env` to `.env` first: `cp app.env .env`). The in-process example for Step 4 is [`examples/getting-started/embed-cli`](../../../examples/getting-started/embed-cli).

To see every verb of both formats and `version --json` run once with real output and exit codes, run [`examples/cli/verb-tour`](../../../examples/cli/verb-tour).

## Step 1 — Check what you are running

```sh
$ textdatactl version
textdatactl v0.1.4
  source:      ldflags
  commit:      861b434792ec70e0b4760162bb65f21df68e3db2
  built:       2026-11-13T05:17:57Z
  go:          go1.27.1
  platform:    linux/amd64
$ textdatactl --version
textdatactl version v0.1.4
```

That is the `linux_amd64` release archive. A binary from `go install` reports `source: module` and the module version; one built from a clone reports a pseudo-version such as `v0.1.5-0.20261113071213-e24c1e3cc8bb+dirty`. `textdatactl version --json` prints the same facts in the JSON envelope:

```json
{"ok":true,"data":{"binary":"textdatactl","version":"v0.1.5-0.20261113071213-e24c1e3cc8bb+dirty","source":"module","commit":"e24c1e3cc8bb8e048e5ed14e1200aedd182409e5","build_time":"2026-11-13T07:12:13Z","modified":true,"go_version":"go1.27.1","platform":"linux/amd64"}}
```

## Step 2 — Read and edit a .env file

`textdatactl dotenv` takes every verb of the standalone `dotenvctl` (`get`, `set`, `unset`, `restore`, `list`, `keys`, `diff`, `matrix`, `envs`, `run`, `plugins`, `github`, `vercel`). It works on `.env` in the current directory; `-f` picks another file.

```sh
$ textdatactl dotenv get DATABASE_URL --expand
postgres://localhost:5432/app
$ textdatactl dotenv set DB_PORT=6543 LOG_LEVEL=info
$ textdatactl dotenv keys
DB_HOST
DB_PORT
DATABASE_URL
LOG_LEVEL
$ textdatactl dotenv -f .env get DB_PORT --json
{"ok":true,"data":{"key":"DB_PORT","value":"6543","expanded":false}}
$ textdatactl dotenv unset DB_HOST
$ textdatactl dotenv list --disabled
DB_PORT       6543
DATABASE_URL  postgres://${DB_HOST}:${DB_PORT}/app
LOG_LEVEL     info

# disabled
# DB_HOST=localhost
$ textdatactl dotenv restore DB_HOST
$ textdatactl dotenv run -- sh -c 'echo "$DATABASE_URL"'
postgres://localhost:6543/app
```

`run` starts the command with the file's expanded values in its environment; textdatactl's own environment, and your shell's, are never changed. The full verb reference is in the [dotenv CLI README](../../../dotenv/cli/README.md).

## Step 3 — Read and edit a JSONC file

`textdatactl jsonc <verb> <file> <path> …` takes dotted paths (`server.port`, `a."b.c"[2]`) or JSON Pointers (`/features/1`). Verbs: `get`, `set`, `unset`, `restore`, `delete`, `fmt`, `validate`, `diff`, `to-json`.

```sh
$ textdatactl jsonc get config.jsonc server.port
8080
$ textdatactl jsonc get config.jsonc /features/1
billing
$ textdatactl jsonc set config.jsonc server.port 9090 --json-value --dry-run
-    "port": 8080,
+    "port": 9090,
$ textdatactl jsonc set config.jsonc server.port 9090 --json-value
$ textdatactl jsonc set config.jsonc server.host api.local
$ textdatactl jsonc get config.jsonc server --json
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"api.local","port":9090}}}
$ textdatactl jsonc to-json config.jsonc
{
  "server": {
    "host": "api.local",
    "port": 9090
  },
  "features": ["search", "billing"]
}
```

After those edits `config.jsonc` still holds both comments and its trailing commas; only `"port"` and `"host"` changed. `--strict` reads a file as plain JSON:

```sh
$ echo '{"a":1,}' > strict.json
$ textdatactl jsonc get strict.json a --strict
textdatactl jsonc: strict.json:1:7: trailing comma is not allowed in strict JSON
$ textdatactl jsonc get strict.json a --strict --json
{"ok":false,"error":{"code":"syntax","message":"strict.json:1:7: trailing comma is not allowed in strict JSON"}}
```

See the [jsonc CLI README](../../../jsonc/cli/README.md) for every verb and flag.

## Step 4 — Run textdatactl inside your own program

Package `github.com/ubgo/textdata/cmd/textdatactl/ctl` is the whole command tree. `ctl.Execute(args, stdout, stderr)` runs it in-process and returns the exit code instead of exiting, and `ctl.Formats()` lists the mounted formats. Use it to test scripts that call textdatactl, or to ship the CLI inside a larger tool.

<!-- ds:block id=listformats-pccwjeek -->
**listFormats** · [`examples/getting-started/embed-cli/main.go:52-56`](../../../examples/getting-started/embed-cli/main.go#L52-L56)

```go
func listFormats(w io.Writer) {
	for _, f := range ctl.Formats() {
		fmt.Fprintf(w, "format %-6s %s\n", f.Name, f.Short)
	}
}
```
<!-- /ds:block hash=fef708 -->

<!-- ds:block id=runverbs-ymcdztp4 -->
**runVerbs** · [`examples/getting-started/embed-cli/main.go:62-75`](../../../examples/getting-started/embed-cli/main.go#L62-L75)

```go
func runVerbs(w io.Writer, config string) error {
	for _, args := range [][]string{
		{"jsonc", "set", config, "server.port", "9090", "--json-value"},
		{"jsonc", "get", config, "server.port"},
		{"jsonc", "get", config, "server", "--json"},
	} {
		if code := ctl.Execute(args, w, w); code != ctl.ExitOK {
			return fmt.Errorf("%v: exit %d", args[:2], code)
		}
	}
	code := ctl.Execute([]string{"jsonc", "get", config, "missing"}, io.Discard, io.Discard)
	fmt.Fprintf(w, "missing path: exit %d (failure=%v)\n", code, code == ctl.ExitFailure)
	return nil
}
```
<!-- /ds:block hash=69c104 -->

## Try it

```sh
cd examples/getting-started/embed-cli && go run .
```

```text
format dotenv Comment-preserving .env control (get, set, diff, matrix, run, github, vercel)
format jsonc  Comment-preserving JSON/JSONC control (get, set, unset, restore, delete, diff, to-json, fmt, validate)
9090
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"0.0.0.0","port":9090}}}
missing path: exit 1 (failure=true)
```

## Pitfalls

- **`jsonc set` writes a string unless you say otherwise.** `textdatactl jsonc set config.jsonc server.port 9090` writes `"port": "9090"`. Add `--json-value` to parse the value as JSON (a number, `true`, `null`, an object or array, or a quoted string).
- **Exit codes are a contract.** `0` success, `1` failure (a missing key or path, a syntax error, a refused save), `2` usage (wrong arguments, an unknown format or verb). Scripts can branch on them; `--json` puts the reason in `error.code` (`not_found`, `syntax`, …).
- **With `--json`, errors go to stdout.** The `{ok:false,error}` envelope is printed on stdout so one parser reads every outcome; without `--json`, errors go to stderr.
- **`dotenv get` is raw by default.** It prints the value as written (`postgres://${DB_HOST}:…`); add `--expand` to resolve references. `run` always passes expanded values.
- **`dotenv` verbs work on `./.env`.** Without `-f`, a script run from another directory reads (or `set` creates) a `.env` there. Pass `-f path/.env` in scripts.
- **Install completion for `textdatactl`, not `dotenvctl`.** `source <(textdatactl completion bash)` completes the formats and every `dotenv` and `jsonc` verb; a `dotenvctl` script does not complete `textdatactl`.
- **`go install` needs a recent Go.** `cmd/textdatactl` declares Go 1.26.4; with an older toolchain use a release archive.

## Related

- [Install and read your first file](install-and-first-read.md) and [Edit and save](edit-and-save.md): the same operations from Go.
- [cmd/textdatactl README](../../../cmd/textdatactl/README.md), [dotenv CLI README](../../../dotenv/cli/README.md), [jsonc CLI README](../../../jsonc/cli/README.md).
- Example: [`examples/getting-started/embed-cli`](../../../examples/getting-started/embed-cli).
- API: [ctl on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/cmd/textdatactl/ctl).
