# secrets

`${scheme:ref}` in `value.Expand` through the value plugins: the password-store plugin (gopass) with a fake runner, the file plugin over a temp directory, and the vault and aws plugins against local `httptest` servers, plus the `*plugin.Error` a missing entry returns. Every store is a fake with made-up values.

Guide: [value](../../../docs/guides/base/value.md).

## Run

```sh
cd examples/value/secrets && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
${gopass:work/api}                 => fake-gopass-token
${file:api.key}                    => fake-file-key
${vault:secret/data/app#password}  => fake-vault-password
${aws:ssm//app/prod/db_url}        => postgres://db.internal/app
Bearer ${gopass:work/api}          => Bearer fake-gopass-token
[${nobody:thing}]                  => []
missing: plugin=gopass not found=true
```
