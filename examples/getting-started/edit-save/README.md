# edit-save

Writes a `.env` and a `config.jsonc` into a temp directory, edits them (`Set`, `SetAfter`, `Unset` in dotenv; `Set` and `Append` in jsonc), saves, and prints both files: every comment, blank line and layout choice the edits did not touch is unchanged. Nothing in the repository is modified.

Guide: [edit-and-save](../../../docs/guides/getting-started/edit-and-save.md).

## Run

```sh
cd examples/getting-started/edit-save && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
--- .env
# Database settings for local development
# DB_HOST=localhost
DB_PORT=6543
DB_NAME=app

export LOG_LEVEL=info # quoted, with an inline comment
--- config.jsonc
{
  // Where the API listens.
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
    "tls": true,
  },
  /* Features switched on in this environment. */
  "features": ["search", "billing", "audit"],
}
```
