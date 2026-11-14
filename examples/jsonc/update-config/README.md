# jsonc: update a shared config

`jsonc.Update`: six workers each add their own entry and bump a shared counter at once, and every edit survives with the comments intact. `Update` holds the cross-process lock across open, edit and save.

Guide: [jsonc save](../../../docs/guides/jsonc/save.md).

## Run

```sh
cd examples/jsonc/update-config && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
runs: 6
agents: 6
comment kept: true
```
