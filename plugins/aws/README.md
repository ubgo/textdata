# aws

`github.com/ubgo/textdata/plugins/aws` reads secrets from AWS Secrets Manager and SSM Parameter Store for every textdata format (dotenv, jsonc, and any later one). One plugin instance is a `plugin.Resolver` for `${aws:...}` references and a `plugin.Lookuper` for plain `${NAME}` references, sharing one HTTP client and one cache.

Stdlib only, no AWS SDK: it speaks the AWS JSON protocol over `net/http` and signs requests with its own Signature Version 4 implementation, which is tested byte for byte against the AWS SigV4 test suite (`testdata/sigv4`). It only reads (`GetSecretValue`, `GetParameter`); it never writes to AWS.

## Install

```sh
go get github.com/ubgo/textdata/plugins/aws
```

## Example

```go
p, err := aws.New(
	aws.WithRegion("eu-west-1"),
	aws.FromSharedCredentials("", "deploy"),   // or WithCredentials, WithCredentialsSource, FromEnv
	aws.WithLookupPrefix("ssm//app/prod/"),    // ${NAME} -> parameter /app/prod/NAME
)
if err != nil { return err }

f := dotenv.Parse(src, dotenv.WithPlugin(p))
doc, err := jsonc.Parse(data, jsonc.WithPlugin(p)) // the same instance works in every format
```

```dotenv
DB_PASSWORD=${aws:sm/prod/db#password}
API_KEY=${aws:sm/prod/api-key}
DB_URL=${aws:ssm//app/prod/db_url}
PREVIOUS=${aws:sm/prod/db?stage=AWSPREVIOUS#password}
LOG_LEVEL=${LOG_LEVEL}
```

The resolved value is used in memory only; the file always keeps (and writes back) the reference.

## Reference grammar

```
ref      = "sm/" secret-id [ "?" sm-query ] [ "#" key ]
         | "ssm/" param-name [ "#" key ]
sm-query = "stage=" label | "version=" version-id
```

- `sm/<secret-id>` reads a Secrets Manager secret by name or full ARN (`sm/arn:aws:secretsmanager:...:secret:prod/db-AbCdEf`). `?stage=` or `?version=` selects a version.
- `ssm/<param-name>` reads a Parameter Store parameter with decryption on. The name is written exactly as AWS spells it, so a hierarchical name keeps its leading slash: `ssm//app/prod/db_url` reads `/app/prod/db_url`, `ssm/plain` reads `plain`. A `:version` or `:label` suffix (`ssm//app/db:3`) is passed to AWS, which interprets it.
- `#key` reads one key of a value that is a JSON object (the usual Secrets Manager key/value secret); a non-string value is returned as its compact JSON text. Without `#key` the whole value is returned. A binary secret (`SecretBinary`) is returned as its bytes.
- Names may not contain `?`, `#`, whitespace or control bytes (AWS names never do); the key may contain anything.

## Options

| Option | Does |
|---|---|
| `WithRegion(r)` | required (or via `FromEnv`); requests go to `secretsmanager.<r>.amazonaws.com` and `ssm.<r>.amazonaws.com` (`.amazonaws.com.cn` for `cn-` regions) |
| `WithCredentials(c)` | fixed `Credentials{AccessKeyID, SecretAccessKey, SessionToken}` |
| `WithCredentialsSource(fn)` | asked before each request; the hook for any credential kind the plugin does not read itself (web identity, SSO, instance metadata) |
| `FromEnv()` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, and `AWS_REGION` (else `AWS_DEFAULT_REGION`), once, in `New` |
| `FromSharedCredentials(file, profile)` | one profile of a shared credentials file, once, in `New`; `""` means `~/.aws/credentials` and `default` |
| `WithEndpoint(url)` | send every request to one endpoint (LocalStack, a VPC endpoint, tests); still signed for the region |
| `WithHTTPClient(c)`, `WithTimeout(d)` | client and per-request timeout (default 10s) |
| `WithName(n)`, `WithScheme(s)` | run instances for two regions or accounts side by side (`${aws-eu:...}`) |
| `WithLookup(name, ref)` | `${name}` reads `ref` |
| `WithLookupPrefix(prefix)` | `${NAME}` reads `prefix+NAME`: `sm/prod/app#` (one key of one JSON secret) or `ssm//app/prod/` (one parameter per name); tried in order |

Options apply in order; a later one wins, so `FromEnv(), WithRegion("x")` overrides the environment's region. `Purge()` empties the cache. `Read(Ref)` and `ParseRef` are exported for callers that want a value without a document.

## Guarantees

- **No implicit environment or files.** The environment is read only through `FromEnv()`, the shared credentials file only through `FromSharedCredentials`, both once, in `New`.
- **Errors by meaning.** Access denied, bad, expired or unsigned credentials, and KMS refusals (`DecryptionFailure`, `InvalidKeyId`) wrap `plugin.ErrDenied`; network failures, timeouts, throttling and 5xx wrap `plugin.ErrUnavailable`; `ResourceNotFoundException`, `ParameterNotFound` and a missing `#key` wrap `plugin.ErrNotFound`; a malformed reference wraps `aws.ErrInvalidRef`; missing credentials wrap `aws.ErrNoCredentials`. AWS being down is never a silent "unset".
- **No secret in an error.** Errors repeat the reference, the status, the AWS error code and AWS's message (truncated), never a response body or a value; a value that is not JSON does not repeat the JSON parser's message, which can quote bytes. `Credentials` print only the access key id, also under `%v` and `%#v`.
- **Lookuper contract.** An unmapped name, or a mapped reference that does not exist, answers `ok=false`; denied and unavailable are errors.
- **Per-plugin cache.** Each value is fetched once per plugin (several keys of one secret cost one request); failures are not cached; instances never share a cache. Safe for concurrent use.

## Not included

Web identity (`AWS_WEB_IDENTITY_TOKEN_FILE`), SSO, `credential_process` and instance metadata credentials are not read by the plugin; pass them in through `WithCredentialsSource`. The region is not read from `~/.aws/config`.
