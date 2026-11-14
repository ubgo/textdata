# dotenv: strict expansion

`dotenv.WithStrictExpand`: resolvable values read as usual; a typo'd variable, a `${vault:...}` with no vault plugin and a `$(command)` with commands off each fail with the value's `file:line:col` and a typed expand error.

Guide: [dotenv expansion](../../../docs/guides/dotenv/expansion.md).

## Run

```sh
cd examples/dotenv/strict-env && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
DSN      postgres://db.internal/app
REPLICA  .env:4:9: expand REPLICA: expand: variable DB_HOTS is not set (offset 11)
TOKEN    .env:5:7: expand TOKEN: expand: no resolver for scheme "vault" in ${vault:secret/app#token} (offset 0)
STAMP    .env:6:7: expand STAMP: expand: command substitution is off: $(date +%s) (offset 0)
```
