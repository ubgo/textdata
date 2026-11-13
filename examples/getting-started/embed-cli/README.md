# embed-cli

Runs textdatactl in-process with `ctl.Execute` (same verbs, flags, exit codes and `--json` envelope as the binary) and lists the mounted formats with `ctl.Formats`. Works on a temp copy of a JSONC file.

Guide: [textdatactl](../../../docs/guides/getting-started/textdatactl.md).

## Run

```sh
cd examples/getting-started/embed-cli && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
format dotenv Comment-preserving .env control (get, set, diff, matrix, run, github, vercel)
format jsonc  Comment-preserving JSON/JSONC control (get, set, unset, restore, delete, diff, to-json, fmt, validate)
9090
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"0.0.0.0","port":9090}}}
missing path: exit 1 (failure=true)
```
