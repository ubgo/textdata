# dotenv: update a shared .env

`dotenv.Update`: five workers each set their own key and bump a shared counter at once, and every edit survives with the comment intact. `Update` holds the cross-process lock across open, edit and save.

Guide: [dotenv saving](../../../docs/guides/dotenv/saving.md).

## Run

```sh
cd examples/dotenv/update-env && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
DEPLOYS=5
keys: 6
comment kept: true
```
