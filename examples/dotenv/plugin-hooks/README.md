# dotenv/plugin-hooks

The function options (WithLookup, WithValueTransform, WithSaveGuard), a Resolver for `${scheme:ref}`, an EntryObserver and ExpandObserver for dead-key detection, plugin ordering, and the firewall that keeps saved bytes unchanged.

Guide: [docs/guides/dotenv/plugins.md](../../../docs/guides/dotenv/plugins.md).

## Run it

```sh
cd examples/dotenv/plugin-hooks && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/plugin-hooks/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== function options
ZONE: eu-west-1-a PASS: hunter2 Get(PASS): encrypted:c2VjcmV0 err: <nil>
save: dotenv: plugin "save-guard": guard save never-written.env: read-only
== Resolver
PASS=pa55
OTHER=[]
LIT=${mem:db/password}
dotenv: plugin "mem-store": resolve mem:nope: not found: mem:nope
== observers
disabled: [DEBUG] never referenced: [PORT URL]
== ordering
A: t2(t1(second)) B: t2(t1(last-runner))
== firewall
reads as: SECRET render unchanged: true saved unchanged: true
```
