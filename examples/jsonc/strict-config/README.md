# jsonc: strict expansion

`jsonc.WithStrictExpand`: a config whose references resolve decodes into a typed struct; one with a typo'd variable fails with the string's `file:line:col` wrapping `*expand.UnsetError`.

Guide: [jsonc expansion and plugins](../../../docs/guides/jsonc/expansion-and-plugins.md).

## Run

```sh
cd examples/jsonc/strict-config && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
good: {Default:claude Token:fake-token Region:eu-west-1}
typo: at=3:12 name=API_TOKN strict=true
  typo.jsonc:3:12: expand "token": expand: variable API_TOKN is not set (offset 0)
```
