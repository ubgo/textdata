# integration (test-only)

Module `github.com/ubgo/textdata/integration`. Nothing imports it and it is never tagged; it exists only in the workspace (`go.work`).

It holds two kinds of test that would otherwise force a library module to require a sibling it does not use:

- **Cross-format tests:** one value plugin instance installed in a dotenv file and a jsonc file at once (`gopass/`, `file/`, `extends/`, `vault/`, `aws/`), plus the vault and aws examples that show a plugin inside a format. Each plugin module keeps only tests that need the base.
- **The require table:** `deps_test.go` pins the exact direct requires of every module in `go.work`. The formats and value plugins require only the base, cobra appears only in CLI modules, and no module requires this one. Adding a require means changing the table in the same commit.

`task deps:check` (part of `task check`) adds the other half: every direct require must be imported by the module's non-test code.

```sh
cd integration && go test ./...
```
