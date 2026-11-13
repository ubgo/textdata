# file plugin, confined

Reads `${file:...}` references with the file value plugin (`github.com/ubgo/textdata/plugins/file`) and shows its guard rails. Everything happens in a temporary folder the program creates and removes.

It shows:

- One confined plugin installed in a dotenv file and a JSONC document; `${file:./key.pem}` reads the file next to them.
- `${file:../shared.txt}` refused with `plugin.ErrDenied`, because confinement to the base folder is on by default; an absolute path is refused too.
- `WithMaxSize` failing with `fileplugin.ErrTooLarge` instead of truncating.
- `WithTrimNewline(false)` keeping the file's last newline.
- `WithRoot("..")` moving the boundary one folder up, and `Unconfined()` removing it.

## Run

```sh
cd examples/plugins/file-confined && go run .
```

Expected output:

```text
dotenv TLS_KEY: pem-body
jsonc tls.key:  pem-body
../shared.txt:  denied=true
over 4 bytes:   too large=true
untrimmed:      "pem-body\n"
WithRoot(..):   shared-value
confined abs:   denied=true
Unconfined abs: shared-value
```

Guide: [file plugin](../../../docs/guides/plugins/file.md).
