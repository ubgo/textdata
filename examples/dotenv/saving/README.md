# dotenv/saving

Save and SaveAs, the error for a parsed file with no path, which mode each case gets, line endings and the trailing newline, and a save through a symlink.

Guide: [docs/guides/dotenv/saving.md](../../../docs/guides/dotenv/saving.md).

## Run it

```sh
cd examples/dotenv/saving && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/saving/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== Save
"# app\nPORT=9090\n", files in dir: 1
== no path
dotenv: no path set; use Open, or set Path before Save
saved with mode -rw-------
== modes
.env.locked  -r--------
.env.copy    -r--------
.env.shared  -rw-r--r--
== line endings
CRLF:       "A=1\r\nB=3\r\nC=4\r\n"
mixed:      "A=x\r\nB=2\r\n"
no newline: "A=1\nB=2"
scratch:    "A=1\n"
== SaveAs
.env.prod: "DOMAIN=acme.io\n"
.env.qa: "DOMAIN=qa.acme.io\n"
Path still: .env.staging
empty path: dotenv: SaveAs needs a path
== symlink
still a link: true, target now "A=2\n"
```
