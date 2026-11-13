# expand-engine

Drives the base `expand` engine directly: the six `${VAR<op>arg}` operators, nesting and `$$`, a `${scheme:ref}` resolver with `SplitScheme` and `ParseRef`, a fake `$(cmd)` runner, the cycle guard through `Stack`, and `RequiredError`. No real commands run.

Guide: [expand](../../../docs/guides/base/expand.md).

## Run

```sh
cd examples/base/expand-engine && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
${URL}                             => postgres://db.internal:5432/app
${EMPTY-unset} ${EMPTY:-empty}     =>  empty
${HOST+set} [${EMPTY:+nonempty}]   => set []
${MISSING:-${HOST:-x}}             => db.internal
price: $$5 for $HOST               => price: $5 for db.internal
resolved: secret-for-db/password []
split: "vault" "db/password" true
parsed: name=PORT default-if-empty=true arg=5432
run off: a $(echo $WHO) b [$(fail)]
run on:  a ECHO WORLD b
run fail: expand: command "fail" failed: exit status 1 (cmd="fail")
A => "a-b-", SELF => "", default depth 32
error: expand: required variable TOKEN is not set or is empty: set TOKEN in .env.local
key=TOKEN empty=true
```
