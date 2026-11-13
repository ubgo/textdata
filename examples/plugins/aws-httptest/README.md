# aws against an httptest server

Installs the aws value plugin (`github.com/ubgo/textdata/plugins/aws`) in a dotenv file and a JSONC document. An `httptest` server answers like AWS Secrets Manager (`GetSecretValue`) and SSM Parameter Store (`GetParameter`), and refuses any request whose Signature Version 4 `Authorization` header is not scoped to the demo key, region and service. No AWS account, SDK or network is involved.

It shows:

- Secrets Manager references: one key of a JSON secret (`#password`), a non-string key returned as JSON text (`#port` → `5432`), a version stage (`?stage=AWSPREVIOUS`) and a whole value.
- A Parameter Store reference with a hierarchical name (`ssm//app/prod/db_url`, note the double slash) and `WithLookupPrefix` answering plain `${FEATURE_FLAG}`.
- `WithCredentialsSource`, asked once per request (never per reference: the cache absorbs repeats), with a session token.
- `plugin.ErrNotFound` for a missing secret, `FromSharedCredentials` reading one profile of a credentials file, and `aws.ErrNoCredentials` from `New` when none were given.

## Run

```sh
cd examples/plugins/aws-httptest && go run .
```

Expected output:

```text
DB_PASSWORD: pw-current
DB_PORT:     5432
PREVIOUS:    pw-previous
API_KEY:     plain-api-key
DB_URL:      postgres://db.internal/app
FEATURE:     on
jsonc:       postgres://db.internal/app
missing:     not found=true
requests:    secretsmanager=4 ssm=2 (credentials asked 6 times)
shared file: ok=true
no creds:    true
```

Eight reads reached the server six times: `prod/db#password` and `prod/db#port` share one request, and the JSONC read came from the cache.

Guide: [aws plugin](../../../docs/guides/plugins/aws.md).
