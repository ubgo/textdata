# verb-tour

Runs every `textdatactl` verb once through `ctl.Execute` (the same entry point as the binary) on files in a temp directory: the jsonc verbs `get`, `set`, `unset`, `restore`, `delete`, `diff`, `to-json`, `fmt --check` and `validate`; the dotenv verbs `list`, `keys`, `unset`, `restore`, `envs`, `matrix --contract` and `diff`; and `version --json`. Each command line is printed with its output and exit code, as a shell session would show it. Exit code 1 from `diff`, `fmt --check`, `validate` and `matrix --contract` is the expected "found something" answer.

Guides: [jsonc CLI](../../../docs/guides/cli/jsonc-cli.md), [dotenvctl command reference](../../../docs/guides/cli/dotenvctl.md), [Multi-environment](../../../docs/guides/cli/multi-env.md), [textdatactl](../../../docs/guides/getting-started/textdatactl.md).

## Run it

```sh
cd examples/cli/verb-tour && go run .
```

Expected output:

```text
$ textdatactl jsonc get config.jsonc server.port
8080
exit=0
$ textdatactl jsonc set config.jsonc server.port 9090 --json-value
exit=0
$ textdatactl jsonc unset config.jsonc server.debug --dry-run
-    "debug": true
+    // "debug": true
exit=0
$ textdatactl jsonc unset config.jsonc server.debug
exit=0
$ textdatactl jsonc restore config.jsonc server.debug
exit=0
$ textdatactl jsonc delete config.jsonc tags[1] --dry-run
-  "tags": ["a", "b"],
+  "tags": ["a"],
exit=0
$ textdatactl jsonc diff config.jsonc next.jsonc
- server.debug: true
~ server.host: "localhost" -> "0.0.0.0"
~ server.port: 9090 -> 8080
+ tags[2]: "c"
exit=1
$ textdatactl jsonc to-json config.jsonc
{
  "server": {
    "host": "localhost",
    "port": 9090,
    "debug": true
  },
  "tags": ["a", "b"]
}
exit=0
$ textdatactl jsonc fmt next.jsonc --check
next.jsonc
exit=1
$ textdatactl jsonc validate config.jsonc --forbid-trailing-commas
config.jsonc:8:21: trailing comma after the last member (trailing-comma)
exit=1
$ textdatactl dotenv list --expand
HOST   localhost
URL    http://localhost:8080
DEBUG  true
exit=0
$ textdatactl dotenv keys
HOST
URL
DEBUG
exit=0
$ textdatactl dotenv unset DEBUG --dry-run
-DEBUG=true
+# DEBUG=true
exit=0
$ textdatactl dotenv unset DEBUG
exit=0
$ textdatactl dotenv restore DEBUG
exit=0
$ textdatactl dotenv envs
ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default             .env          3     1         0          0
prod                .env.prod     2     0         0          0
example (contract)  .env.example  3     0         0          0
exit=0
$ textdatactl dotenv matrix --contract .env.example
KEY         default  prod
DEBUG       ✓        —
HOST        ✓        ✓
OLD         #        —
SENTRY_DSN  —        —
URL         ✓        ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
contract: default is missing 1 key(s): [SENTRY_DSN]
contract: prod is missing 1 key(s): [SENTRY_DSN]
exit=1
$ textdatactl dotenv diff .env .env.prod --expand
- DEBUG=true
~ HOST: localhost -> example.com
~ URL: http://localhost:8080 -> https://example.com
exit=1
$ textdatactl version --json
ok envelope: true
exit=0
```

`main_test.go` runs the program and checks this output.
