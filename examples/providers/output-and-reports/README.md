# Output, error codes and reports

Shows `outfmt.Printer` in human and JSON mode, `Fail` on the right stream, `FailWith` for a part-way failure, `textdiff.Lines` for a dry-run preview, and `report.DiffHTML` / `report.MatrixHTML` with values masked and revealed.

Guide: [output-and-reports.md](../../../docs/guides/providers/output-and-reports.md).

Report pages are written to a temp directory and removed; the program prints their properties, not their bytes, because each page embeds its generation time.

## Run

```sh
cd examples/providers/output-and-reports && go run .
go test ./providers/output-and-reports/   # from examples/: compares the output with testdata/output.txt
```

## Expected output

```text
postgres://db/app
{"ok":true,"data":{"key":"DB_URL","value":"postgres://db/app"}}
human: out="" err="mytool: key \"API_KEY\" not found\n"
{"ok":false,"error":{"code":"not_found","message":"key \"API_KEY\" not found"}}
{"ok":false,"data":{"pushed":["DB_URL"]},"error":{"code":"exec","message":"push REDIS_URL: backend unavailable"}}
-DB_HOST=localhost
+DB_HOST=db.prod
no-op is nil: true
[-A=1 -B=2 -C=3 +A=9 +B=2 +C=8]
diff-revealed-false.html: secret=false mask=true banner=false
diff-revealed-true.html: secret=true mask=false banner=true
matrix.html: doctype=true script=false external=false
```
