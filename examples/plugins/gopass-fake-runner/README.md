# gopass with a fake runner

Installs the gopass value plugin (`github.com/ubgo/textdata/plugins/gopass`) in a dotenv file and a JSONC document at the same time. A fake `Runner` answers `gopass show -o <ref>` from a map, so the program needs no gopass binary, no password store and no gpg.

It shows:

- `${gopass:personal/app/db}` resolved through the Resolver, while `Get` and `Render` keep the reference.
- `WithMapping` turning on the Lookuper for one plain name (`${DB_USER}`).
- A missing entry failing with `plugin.ErrNotFound` instead of reading as empty.
- One plugin instance shared by both formats: the cache means the second read does not run gopass again.

## Run

```sh
cd examples/plugins/gopass-fake-runner && go run .
```

Expected output:

```text
dotenv raw:      ${gopass:personal/app/db}
dotenv expanded: hush-from-gopass (user app)
missing entry:   not found=true
jsonc expanded:  hush-from-gopass
jsonc unchanged: true
gopass calls:    3
```

Three calls: the password and the user once each, and the missing entry (failures are never cached). The JSONC read of the password came from the cache.

Guide: [gopass plugin](../../../docs/guides/plugins/gopass.md).
