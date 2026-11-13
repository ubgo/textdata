# dotenv/syntax

The four quoting styles, multi-line values, comments, the `:` delimiter, key names, duplicates, `encrypted:` values and the two escape modes, each printed as Get and GetExpanded.

Guide: [docs/guides/dotenv/syntax.md](../../../docs/guides/dotenv/syntax.md).

## Run it

```sh
cd examples/dotenv/syntax && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/syntax/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== quoting
NAME: Get="world" GetExpanded="world"
BARE: Get="hello $NAME" GetExpanded="hello world"
DOUBLE: Get="hello $NAME  " GetExpanded="hello world  "
SINGLE: Get="hello $NAME" GetExpanded="hello $NAME"
BACKTICK: Get="it's \"hi\" $NAME" GetExpanded="it's \"hi\" $NAME"
EXPORTED: Get="yes" GetExpanded="yes"
SPACED: Get="padded value" GetExpanded="padded value"
EMPTY: Get="" GetExpanded=""
== multi-line
CERT: Get="line one\nline two" GetExpanded="line one\nline two"
PEM: Get="-----BEGIN KEY-----\nab'c\"d\n-----END KEY-----" GetExpanded="-----BEGIN KEY-----\nab'c\"d\n-----END KEY-----"
unterminated keys: [A B] round trip: true
== comments
PASSWORD: Get="a#b" GetExpanded="a#b"
PORT: Get="8080" GetExpanded="8080"
QUOTED: Get="a # b" GetExpanded="a # b"
AFTER: Get="kept" GetExpanded="kept"
disabled: [{DISABLED off}]
== colon delimiter
HOST: Get="db.internal" GetExpanded="db.internal"
PORT: Get="5432" GetExpanded="5432"
URL: Get="http://example.com:8080" GetExpanded="http://example.com:8080"
http: Get="//example.com" GetExpanded="//example.com"
== key names
pair      key="2FA_SECRET" raw="2FA_SECRET=a"
pair      key="spring.datasource.url" raw="spring.datasource.url=b"
pair      key="MY-SERVICE-TOKEN" raw="MY-SERVICE-TOKEN=c"
pair      key="SERVICES[0]" raw="SERVICES[0]=d"
other     key="" raw="HAS KEY=e"
other     key="" raw="=no key"
inherited key="HOME" raw="HOME"
== duplicates
Get: info Count: 2 Pairs: [{LEVEL debug} {LEVEL info}]
== encrypted values
BARE: Get="encrypted:ab$cd" GetExpanded="encrypted:ab"
SINGLE: Get="encrypted:ab$cd" GetExpanded="encrypted:ab$cd"
== escape modes
mode: extended
  NEWLINE="a\nb"
  WINPATH="C:\new\temp"
  QUOTE="say \"hi\""
  SINGLE="a\\nb"
mode: compose
  NEWLINE="a\\nb"
  WINPATH="C:\\new\\temp"
  QUOTE="say \"hi\""
  SINGLE="a\\nb"
```
