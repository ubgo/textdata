# strict

`value.WithStrict`: references that resolve, a name set to `""` and `${VAR:-default}` read as usual; an unset name, a `${scheme:ref}` no plugin claims and a `$(command)` with commands off are typed errors (`*expand.UnsetError`, `*expand.UnknownSchemeError`, `*expand.CommandOffError`) that match `expand.ErrStrict` and name the reference and its offset, never a value. `${VAR:?msg}` keeps its `*expand.RequiredError`.

Guide: [value](../../../docs/guides/base/value.md).

## Run

```sh
cd examples/value/strict && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
Bearer ${API_TOKEN}      => Bearer fake-token
[${EMPTY}]               => []
${REGION:-eu-west-1}     => eu-west-1
Bearer ${API_TOKN}           strict=true  unset API_TOKN
  expand: variable API_TOKN is not set (offset 7)
${gopas:work/api}            strict=true  unknown scheme gopas
  expand: no resolver for scheme "gopas" in ${gopas:work/api} (offset 0)
$(gopass show -o work/api)   strict=true  command off
  expand: command substitution is off: $(gopass show -o work/api) (offset 0)
${API_SECRET:?set it}        strict=false required API_SECRET
  expand: required variable API_SECRET is not set or is empty: set it
```
