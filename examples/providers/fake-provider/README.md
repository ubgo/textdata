# A push provider over an in-memory store

Builds a complete provider on `clikit/providerkit`: a store implementing `SecretWriter`, `SecretLister`, `SecretDeleter` and `EmptyListHinter`, a `Plugin` whose `Command` generates `push`, `list` and `prune`, a gate that prints a banner and calls `Confirm`, `Meta` for the `--json` payload, and a host that mounts it with a global `--json` flag and maps `*providerkit.CmdError` to the exit code. It then runs the confirm refusal, dry runs, a push, list, prune with `--keep`, the empty-list hint and a push that fails part-way in both output modes.

Guide: [write-a-provider.md](../../../docs/guides/providers/write-a-provider.md).

The store is a Go map; values come from a temp `.env` through `envkit.SelectFile`. No network, account or backend CLI.

## Run

```sh
cd examples/providers/fake-provider && go run .
go test ./providers/fake-provider/   # from examples/: compares the output with testdata/output.txt
```

## Expected output

```text
$ memctl mem push
target : memory (2 stored)
keys   : 2
memctl: refusing to write without --yes in non-interactive mode
exit 2

$ memctl mem push --dry-run
target : memory (2 stored)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: would-push
REDIS_URL: would-push
exit 0

$ memctl mem push --yes
target : memory (2 stored)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: pushed
REDIS_URL: pushed
exit 0

$ memctl mem list
API_KEY
DB_URL
OLD_FLAG
REDIS_URL
exit 0

$ memctl mem prune --dry-run
target : memory (4 stored)
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: would-delete
exit 0

$ memctl mem prune --keep OLD_FLAG --dry-run
nothing to prune
API_KEY: kept-skipped
OLD_FLAG: kept-keep-flag
exit 0

$ memctl mem prune --yes
target : memory (4 stored)
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: deleted
exit 0

$ memctl --json mem push --prefix DB_ --yes
{"ok":true,"data":{"meta":{"target":"memory"},"results":[{"name":"DB_URL","action":"pushed"}],"dry_run":false}}
exit 0

$ memctl mem list
no secrets in the memory store (did you push yet?)
exit 0

$ memctl mem push --yes
target : memory (0 stored)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: pushed
memctl: push REDIS_URL: backend unavailable
exit 1

$ memctl --json mem push --yes
{"ok":false,"data":{"meta":{"target":"memory"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"DB_URL","action":"pushed"}],"dry_run":false},"error":{"code":"exec","message":"push REDIS_URL: backend unavailable"}}
exit 1
```
