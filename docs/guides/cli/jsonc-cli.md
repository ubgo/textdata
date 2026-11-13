# jsonc CLI (textdatactl jsonc)

`textdatactl jsonc` reads and edits JSON and JSONC files from the shell without disturbing a byte you did not touch: comments, layout and key order survive every edit. It has nine verbs (`get`, `set`, `unset`, `restore`, `delete`, `diff`, `to-json`, `fmt`, `validate`), one `--json` envelope for scripts, a `--strict` switch for plain JSON, and the same exit codes as every textdata CLI. Use it to change a setting in `tsconfig.json` or a VS Code `settings.json` from a script, to gate CI on formatting or a JSON Schema, or to turn a commented config into plain JSON for a tool that cannot read comments.

## Before you start

The jsonc verbs ship in the umbrella binary (there is no standalone jsonc binary):

```sh
go install github.com/ubgo/textdata/cmd/textdatactl@latest
textdatactl jsonc --help
```

Installing and versioning `textdatactl` itself is covered in [textdatactl](../getting-started/textdatactl.md). To run the verbs from Go instead, see [embedding the CLIs](embedding.md) and [`examples/cli/embed-jsonc`](../../../examples/cli/embed-jsonc).

Every jsonc verb also runs once, in process, in [`examples/cli/verb-tour`](../../../examples/cli/verb-tour) (its README shows the full session with exit codes). The command lines it runs are the ones you would type after `textdatactl`:

<!-- ds:block id=jsonctour-mjjstg7x -->
**jsoncTour** · [`examples/cli/verb-tour/main.go:88-99`](../../../examples/cli/verb-tour/main.go#L88-L99)

```go
func jsoncTour(w io.Writer) {
	textdatactl(w, "jsonc", "get", configFile, "server.port")
	textdatactl(w, "jsonc", "set", configFile, "server.port", "9090", "--json-value")
	textdatactl(w, "jsonc", "unset", configFile, "server.debug", "--dry-run")
	textdatactl(w, "jsonc", "unset", configFile, "server.debug")
	textdatactl(w, "jsonc", "restore", configFile, "server.debug")
	textdatactl(w, "jsonc", "delete", configFile, "tags[1]", "--dry-run")
	textdatactl(w, "jsonc", "diff", configFile, nextFile)
	textdatactl(w, "jsonc", "to-json", configFile)
	textdatactl(w, "jsonc", "fmt", nextFile, "--check")
	textdatactl(w, "jsonc", "validate", configFile, "--forbid-trailing-commas")
}
```
<!-- /ds:block hash=fbd42c -->

Every output on this page comes from a real run. The commands run in the order shown, starting from these two files:

```jsonc
// config.jsonc
{
  // where the API listens
  "server": {
    "host": "localhost",
    "port": 8080,
    "debug": true,
  },
  "hosts": ["a.example", "b.example"],
}
```

```jsonc
// prod.jsonc
{
  "server": {"host": "api.example", "port": 443},
  "hosts": ["a.example", "b.example"],
  "region": "eu",
}
```

## Step 1 — address a value with a path

Files are positional arguments (`get <file> <path>`). Paths use the jsonc library's grammar: dotted (`server.port`, `hosts[1]`, `a."b.c"`) or a JSON Pointer (`/server/port`, `/hosts/1`). The empty path `""` is the whole document. The grammar in full: [read and paths](../jsonc/read-and-paths.md).

### get — print a value

A string prints bare, so `v=$(textdatactl jsonc get …)` reads the string itself; anything else prints as indented plain JSON with number literals exactly as written.

```console
$ textdatactl jsonc get config.jsonc server.port
8080
$ textdatactl jsonc get config.jsonc /server/host
localhost
$ textdatactl jsonc get config.jsonc server
{
  "host": "localhost",
  "port": 8080,
  "debug": true
}
$ textdatactl jsonc get config.jsonc hosts[1] --json
{"ok":true,"data":{"path":"hosts[1]","kind":"string","value":"b.example"}}
$ textdatactl jsonc get config.jsonc server.nope; echo "exit=$?"
textdatactl jsonc: config.jsonc: jsonc: path not found: "server.nope"
exit=1
$ textdatactl jsonc get config.jsonc "server.[" ; echo "exit=$?"
textdatactl jsonc: config.jsonc: jsonc: path: invalid syntax: "server.[" at byte 7: empty segment
exit=2
$ textdatactl jsonc get missing.jsonc a; echo "exit=$?"
textdatactl jsonc: missing.jsonc: no such file
exit=1
```

The `--json` payload is `{path, kind, value}`: `kind` is `object`, `array`, `string`, `number`, `bool` or `null`, and `value` is raw JSON. A malformed path is a usage error (exit `2`); a missing path or file is an operation failure (exit `1`).

## Step 2 — edit values

### set — replace or create a value

The value is a string unless `--json-value` says to parse it as one JSON value (number, bool, null, object, array or quoted string), which is then written exactly as typed. Missing parent objects are created, and so is a missing file (mode `0600`).

```console
$ textdatactl jsonc set config.jsonc server.port 9090 --dry-run
-    "port": 8080,
+    "port": "9090",
$ textdatactl jsonc set config.jsonc server.port 9090 --json-value --dry-run
-    "port": 8080,
+    "port": 9090,
$ textdatactl jsonc set config.jsonc server.port 9090 --json-value
$ textdatactl jsonc set config.jsonc server.port 9090 --json-value; echo "exit=$?"
no changes
exit=0
$ textdatactl jsonc set config.jsonc server.tls.cert /etc/cert.pem --json
{"ok":true,"data":{"file":"config.jsonc","path":"server.tls.cert","created":true,"changed":true,"dry_run":false}}
$ textdatactl jsonc set config.jsonc features '{"beta": true}' --json-value
$ textdatactl jsonc set config.jsonc x '{bad' --json-value; echo "exit=$?"
textdatactl jsonc: --json-value: "{bad" is not one JSON value
exit=2
$ cat config.jsonc
{
  // where the API listens
  "server": {
    "host": "localhost",
    "port": 9090,
    "debug": true,
    "tls": {
      "cert": "/etc/cert.pem"
    },
  },
  "hosts": ["a.example", "b.example"],
  "features": {"beta": true},
}
```

The comment, the trailing commas and the one-line `hosts` array survived; the new `tls` object follows the file's own style. A successful write prints nothing; an edit that leaves the bytes identical prints `no changes` and does not touch the file.

### unset and restore — comment a member out and back

`unset` comments an object member out (`// "debug": true,`) so it no longer counts but stays in the file; `restore` brings it back byte for byte.

```console
$ textdatactl jsonc unset config.jsonc server.debug --dry-run
-    "debug": true,
+    // "debug": true,
$ textdatactl jsonc unset config.jsonc server.debug
$ grep debug config.jsonc
    // "debug": true,
$ textdatactl jsonc get config.jsonc server.debug; echo "exit=$?"
textdatactl jsonc: config.jsonc: jsonc: path not found: "server.debug"
exit=1
$ textdatactl jsonc restore config.jsonc server.debug
$ textdatactl jsonc restore config.jsonc server.debug; echo "exit=$?"
textdatactl jsonc: config.jsonc: jsonc: key already exists: "server.debug"
exit=1
$ textdatactl jsonc unset config.jsonc hosts[0]; echo "exit=$?"
textdatactl jsonc: config.jsonc: jsonc: wrong value type: "hosts[0]" is an array element; Disable works on object members
exit=1
$ textdatactl jsonc unset config.jsonc features.beta --json; echo "exit=$?"
{"ok":false,"error":{"code":"layout","message":"config.jsonc: jsonc: value does not occupy its own lines: \"features.beta\""}}
exit=1
```

### delete — remove a member or array element

`delete` removes the value with its attached comments and comma.

```console
$ textdatactl jsonc delete config.jsonc server.tls --dry-run
-    "tls": {
-      "cert": "/etc/cert.pem"
-    },
$ textdatactl jsonc delete config.jsonc server.tls
$ textdatactl jsonc delete config.jsonc hosts[1] --dry-run
-  "hosts": ["a.example", "b.example"],
+  "hosts": ["a.example"],
$ textdatactl jsonc delete config.jsonc "" ; echo "exit=$?"
textdatactl jsonc: config.jsonc: jsonc: wrong value type: cannot delete the root value
exit=1
```

`set`, `unset`, `restore` and `delete` all take `--dry-run`, which prints the would-be line diff and writes nothing. Their `--json` payload is `{file, path, created, changed, dry_run, diff}` (`created` only for `set`, `diff` only for a dry run). Writes are atomic (temp file and rename) and keep the file's mode.

## Step 3 — compare and convert

### diff — changes by path

`diff` compares meaning: comments, layout and key order are invisible, and changes are listed by path. Exit `0` same, `1` different, like `diff(1)`.

```console
$ textdatactl jsonc diff config.jsonc prod.jsonc; echo "exit=$?"
- features: {"beta":true}
+ region: "eu"
- server.debug: true
~ server.host: "localhost" -> "api.example"
~ server.port: 9090 -> 443
exit=1
$ cp config.jsonc same.jsonc
$ textdatactl jsonc diff config.jsonc same.jsonc; echo "exit=$?"
exit=0
$ textdatactl jsonc diff config.jsonc prod.jsonc --json
{"ok":true,"data":{"equal":false,"changes":[{"kind":"removed","path":"features","pointer":"/features","old":{"beta":true},"old_pos":{"file":"config.jsonc","line":9,"col":15}},{"kind":"added","path":"region","pointer":"/region","new":"eu","new_pos":{"file":"prod.jsonc","line":4,"col":13}},{"kind":"removed","path":"server.debug","pointer":"/server/debug","old":true,"old_pos":{"file":"config.jsonc","line":6,"col":14}},{"kind":"changed","path":"server.host","pointer":"/server/host","old":"localhost","new":"api.example","old_pos":{"file":"config.jsonc","line":4,"col":13},"new_pos":{"file":"prod.jsonc","line":2,"col":22}},{"kind":"changed","path":"server.port","pointer":"/server/port","old":9090,"new":443,"old_pos":{"file":"config.jsonc","line":5,"col":13},"new_pos":{"file":"prod.jsonc","line":2,"col":45}}]}}
```

Each change carries both the dotted path and the JSON Pointer, and the `file:line:col` of each side. The library behind it: [jsonc diff](../jsonc-tools/diff.md).

### to-json — plain JSON for tools that cannot read comments

```console
$ textdatactl jsonc to-json config.jsonc
{
  "server": {
    "host": "localhost",
    "port": 9090,
    "debug": true
  },
  "hosts": ["a.example", "b.example"],
  "features": {"beta": true}
}
$ textdatactl jsonc to-json prod.jsonc --json
{"ok":true,"data":{"value":{"server":{"host":"api.example","port":443},"hosts":["a.example","b.example"],"region":"eu"}}}
```

Comments, trailing commas and a BOM are dropped; every other byte, layout included, stays as written. Inside the envelope the value is compacted. An empty file prints `null`.

## Step 4 — format and validate in CI

### fmt — one consistent style, comments kept

Like gofmt, `fmt` is silent on success and writes only when the file changes. `--check` writes nothing, prints the file name and exits `1` when the file would change.

```console
$ textdatactl jsonc fmt prod.jsonc --check; echo "exit=$?"
prod.jsonc
exit=1
$ textdatactl jsonc fmt prod.jsonc --dry-run
-  "server": {"host": "api.example", "port": 443},
-  "hosts": ["a.example", "b.example"],
-  "region": "eu",
+  "server": {
+    "host": "api.example",
+    "port": 443
+  },
+  "hosts": [
+    "a.example",
+    "b.example"
+  ],
+  "region": "eu"
$ textdatactl jsonc fmt prod.jsonc --tabs --trailing-commas && cat -A prod.jsonc | head -4
{$
^I"server": {$
^I^I"host": "api.example",$
^I^I"port": 443,$
$ textdatactl jsonc fmt prod.jsonc; textdatactl jsonc fmt prod.jsonc --check; echo "exit=$?"
exit=0
$ textdatactl jsonc fmt prod.jsonc --check --dry-run; echo "exit=$?"
textdatactl jsonc: --check and --dry-run are mutually exclusive
exit=2
$ textdatactl jsonc fmt prod.jsonc --indent 0; echo "exit=$?"
textdatactl jsonc: --indent must be at least 1, got 0
exit=2
```

| Flag | Meaning |
|---|---|
| `--indent N` | spaces per level (default 2, at least 1) |
| `--tabs` | indent with tabs (overrides `--indent`) |
| `--trailing-commas` | a comma after the last member or element (ignored with `--strict`) |
| `--dry-run` | print the would-be diff, write nothing |
| `--check` | write nothing; print the file name and exit `1` when it would change |

The `--json` payload is `{file, changed, written, check, dry_run, diff}`. The style options in Go: [jsonc format](../jsonc-tools/format.md).

### validate — syntax, lint and JSON Schema

`validate` reports syntax errors, lint findings (duplicate keys, mixed indentation, comments other tools lose; trailing commas with `--forbid-trailing-commas`) and, with `--schema FILE`, JSON Schema violations. Every issue prints as `file:line:col: message (rule)`, sorted by position. Any error exits `1`; schema warnings (unknown keys under `additionalProperties: false`, unsupported keywords) print with `warning:` and do not fail.

With this schema (`schema.json`) and a deliberately wrong `bad.jsonc`:

```json
{
  "type": "object",
  "properties": {
    "server": {
      "type": "object",
      "properties": {"port": {"type": "integer", "maximum": 65535}, "host": {"type": "string"}},
      "required": ["host"]
    },
    "region": {"enum": ["eu", "us"]}
  },
  "additionalProperties": false
}
```

```jsonc
{
  "server": {"port": 70000},
  "region": "asia",
  "extra": 1,
}
```

```console
$ textdatactl jsonc validate config.jsonc; echo "exit=$?"
exit=0
$ textdatactl jsonc validate bad.jsonc --schema schema.json; echo "exit=$?"
bad.jsonc:2:13: server: missing required key "host" (required)
bad.jsonc:2:22: server.port: must be <= 65535 (maximum)
bad.jsonc:3:13: region: must be one of "eu", "us"; got "asia" (enum)
bad.jsonc:4:3: warning: extra: unknown key "extra" (additionalProperties)
exit=1
$ textdatactl jsonc validate bad.jsonc --forbid-trailing-commas; echo "exit=$?"
bad.jsonc:4:13: trailing comma after the last member (trailing-comma)
exit=1
$ textdatactl jsonc validate prod.jsonc --schema schema.json --json; echo "exit=$?"
{"ok":true,"data":{"file":"prod.jsonc","valid":true,"errors":[],"warnings":[{"file":"prod.jsonc","line":6,"col":3,"source":"schema","rule":"additionalProperties","path":"hosts","message":"hosts: unknown key \"hosts\""}]}}
exit=0
$ textdatactl jsonc validate broken.jsonc; echo "exit=$?"      # a missing comma
broken.jsonc:3:3: missing comma before '"' (syntax)
exit=1
```

The `--json` payload is `{file, valid, errors[], warnings[]}`, each issue `{file, line, col, source, rule, path, message}` with `source` one of `syntax`, `lint`, `schema`. A failed validation is still `"ok":true` (the verb ran); read `valid`. A schema file that is missing exits `1` with code `io`, one that is not a valid schema with code `schema`. The schema subset and the Go side: [jsonc schema](../jsonc-tools/schema.md) and [lint and validate](../jsonc/lint-validate.md).

## Step 5 — script it: --json, --strict and exit codes

Two global flags work on every verb, before or after it:

| Flag | Meaning |
|---|---|
| `--json` | print exactly one `{ok, data}` or `{ok:false, error:{code, message}}` envelope on stdout |
| `--strict` | read files as plain JSON: comments and trailing commas are syntax errors, and `unset` (which needs a comment) is refused |
| `-v`, `--version` | print the version and exit |

```console
$ textdatactl jsonc get config.jsonc server.nope --json; echo "exit=$?"
{"ok":false,"error":{"code":"not_found","message":"config.jsonc: jsonc: path not found: \"server.nope\""}}
exit=1
$ textdatactl jsonc nosuch --json; echo "exit=$?"
{"ok":false,"error":{"code":"usage","message":"unknown command \"nosuch\" for \"textdatactl jsonc\""}}
exit=2
$ textdatactl jsonc --strict get config.jsonc server.port; echo "exit=$?"
textdatactl jsonc: config.jsonc:2:3: comments are not allowed in strict JSON
exit=1
$ textdatactl jsonc get prod.jsonc region --strict; echo "exit=$?"
eu
exit=0
$ textdatactl jsonc unset plain.json a --strict --json; echo "exit=$?"
{"ok":false,"error":{"code":"strict","message":"plain.json: jsonc: not allowed in strict mode"}}
exit=1
$ textdatactl jsonc validate config.jsonc --strict --json; echo "exit=$?"
{"ok":true,"data":{"file":"config.jsonc","valid":false,"errors":[{"file":"config.jsonc","line":2,"col":3,"source":"syntax","rule":"syntax","message":"comments are not allowed in strict JSON"}],"warnings":[]}}
exit=1
```

Exit codes: `0` ok (for `diff`: the files mean the same), `1` the operation failed, `diff` found differences, `fmt --check` would change the file, or `validate` found errors; `2` the command line was malformed (unknown verb or flag, wrong operand count, a path or `--json-value` that does not parse, a bad `fmt` flag). The constants are `jsonccmd.ExitOK`, `ExitFailure` and `ExitUsage`.

Error codes in the envelope: `not_found`, `io` and `usage` from [`clikit/outfmt`](../../../clikit/README.md#outfmt), plus the jsonc ones, each frozen API:

| Code | When |
|---|---|
| `syntax` | the file is not valid JSONC (or not valid JSON under `--strict`); the message carries `file:line:col` |
| `type` | the value at the path has the wrong kind for the verb (`unset` on an array element, `delete` of the root) |
| `exists` | `restore` of a key that is live again |
| `layout` | `unset` of a member that does not sit on lines of its own |
| `strict` | the verb needs comments, which `--strict` forbids |
| `schema` | the `validate --schema` file is not a valid schema |

## Try it

From a clone, build the umbrella and run the steps above in a scratch directory with the two files from [Before you start](#before-you-start):

```sh
cd cmd/textdatactl && go build -o /usr/local/bin/textdatactl .
```

Or drive the same verbs from Go:

```console
$ cd examples/cli/embed-jsonc && go run . | head -6
exit: 0
{
  // where the API listens
  "server": {"host": "localhost", "port": 9090}
}
9090
```

## Pitfalls

- **Without `--json-value` every value is a string.** `set config.jsonc server.port 9090` writes `"9090"`, as the dry run above shows. Every `set` example in `--help` that writes a number, bool or object carries `--json-value`.
- **`unset` needs a member on its own lines.** A member inside a one-line object (`"features": {"beta": true}`) cannot be commented out and fails with `layout`; `delete` it, or `fmt` the file first.
- **`unset` and `restore` work on object members only.** Array elements fail with `type`; use `delete`.
- **A missing file is an error for every verb but `set`.** `set` creates the file; `get`, `unset`, `diff` and the rest exit `1` with `no such file`.
- **`validate` with `--json` returns `"ok":true` on invalid files.** The envelope says the verb ran; read `data.valid` or the exit code.
- **A broken file stops the schema check.** When the file has syntax errors, `validate --schema` reports only those; fix them and run again for schema issues.
- **`--trailing-commas` does nothing under `--strict`.** Strict JSON has no trailing commas, so the flag is ignored rather than rejected.
- **The program name in messages follows the host.** Run through `textdatactl` the prefix is `textdatactl jsonc:`; a host that embeds the verbs with `WithProgramName` prints its own name, while the `--help` examples keep saying `textdatactl jsonc`.

## Related

- [Embedding the CLIs](embedding.md) and [`examples/cli/embed-jsonc`](../../../examples/cli/embed-jsonc)
- [dotenvctl command reference](dotenvctl.md): the same contract for `.env` files
- jsonc library guides: [read and paths](../jsonc/read-and-paths.md), [edit](../jsonc/edit.md), [disable and restore](../jsonc/disable-restore.md), [lint and validate](../jsonc/lint-validate.md); tools: [format](../jsonc-tools/format.md), [diff](../jsonc-tools/diff.md), [schema](../jsonc-tools/schema.md)
- [`jsonc/cli` README](../../../jsonc/cli/README.md) and [`jsonccmd` on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/jsonc/cli/jsonccmd)
