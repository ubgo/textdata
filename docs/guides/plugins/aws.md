# aws plugin

`github.com/ubgo/textdata/plugins/aws` reads secrets from AWS Secrets Manager and SSM Parameter Store. A file says `DB_PASSWORD=${aws:sm/prod/db#password}` or `DB_URL=${aws:ssm//app/prod/db_url}`; the expanded read calls `GetSecretValue` or `GetParameter` (decrypted) and the file keeps the reference. One instance is a `Resolver` for `${aws:...}` and, when you map names, a `Lookuper` for plain `${NAME}`. It speaks the AWS JSON protocol over `net/http` and signs with its own Signature Version 4 code, checked against AWS's published SigV4 test suite: no AWS SDK in your dependency tree.

## Before you start

```sh
go get github.com/ubgo/textdata/plugins/aws@latest
```

You need a region and credentials allowed to call `secretsmanager:GetSecretValue` and/or `ssm:GetParameter` (plus `kms:Decrypt` for encrypted values). The runnable program for this page is [`examples/plugins/aws-httptest`](../../../examples/plugins/aws-httptest); an `httptest` server answers like both services and checks each request's SigV4 scope.

## Step 1 — Know the reference grammar

```text
ref = "sm/" secret-id [ "?stage=" label | "?version=" id ] [ "#" key ]
    | "ssm/" param-name [ "#" key ]
```

| Reference | Reads |
|---|---|
| `${aws:sm/prod/api-key}` | the whole value of secret `prod/api-key` |
| `${aws:sm/prod/db#password}` | key `password` of a secret whose value is a JSON object |
| `${aws:sm/prod/db?stage=AWSPREVIOUS#password}` | the same key of the previous version (or `?version=<VersionId>`) |
| `${aws:sm/arn:aws:secretsmanager:...:secret:prod/db-AbCdEf}` | a secret by full ARN |
| `${aws:ssm//app/prod/db_url}` | parameter `/app/prod/db_url`, decrypted |
| `${aws:ssm/plain}` | parameter `plain` (no leading slash) |
| `${aws:ssm//app/db:3}` | parameter version 3 (AWS's own `:version` / `:label` suffix) |

A key whose value is not a string comes back as its JSON text (`5432`, `true`). A binary secret (`SecretBinary`) is returned decoded.

## Step 2 — Build the plugin

`aws.New` needs a region and a credentials source; it sends no request. `WithCredentialsSource` takes a function asked before each request, which is the seam for credentials the plugin does not read itself. `WithEndpoint` sends every request to one URL (LocalStack, a VPC endpoint, a test server) while still signing for the region. `WithLookupPrefix` answers plain names from one place, here parameters under `/app/prod/`:

<!-- ds:block id=aws-newplugin-ax28y3p7 -->
**newPlugin** · [`examples/plugins/aws-httptest/main.go:191-202`](../../../examples/plugins/aws-httptest/main.go#L191-L202)

```go
func newPlugin(endpoint string, asked *int) (*aws.Plugin, error) {
	source := func() (aws.Credentials, error) {
		*asked++
		return aws.Credentials{AccessKeyID: demoKeyID, SecretAccessKey: demoSecret, SessionToken: demoToken}, nil
	}
	return aws.New(
		aws.WithRegion(demoRegion),
		aws.WithCredentialsSource(source),
		aws.WithEndpoint(endpoint), // omit to call the regional AWS endpoints
		aws.WithLookupPrefix("ssm//app/prod/"),
	)
}
```
<!-- /ds:block hash=50e58c -->

| Option | Default | Effect |
|---|---|---|
| `WithRegion(r)` | none (required) | signing region and regional endpoint (`.amazonaws.com.cn` for `cn-` regions) |
| `WithCredentials(c)` | none | fixed `aws.Credentials{AccessKeyID, SecretAccessKey, SessionToken}` |
| `WithCredentialsSource(fn)` | none | `func() (Credentials, error)`, asked once per request |
| `FromEnv()` | off | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, region from `AWS_REGION` else `AWS_DEFAULT_REGION`; once, in `New` |
| `FromSharedCredentials(file, profile)` | off | one profile of `~/.aws/credentials` (or `file`); `""` means default file / `default` profile; once, in `New` |
| `WithEndpoint(url)` | regional AWS endpoints | one endpoint for both services |
| `WithLookup(name, ref)`, `WithLookupPrefix(p)` | none | Lookuper: `${name}` reads `ref`; `${NAME}` reads `p+NAME` |
| `WithHTTPClient(c)`, `WithTimeout(d)` | default client, 10s | transport and per-request bound |
| `WithName(n)`, `WithScheme(s)` | `aws`, `aws` | two instances side by side: `${aws:...}` and `${aws-eu:...}` |

Options apply in order and a later one wins, so `FromEnv(), WithRegion("eu-west-1")` keeps the environment's keys but pins the region.

## Step 3 — Check the signature in a fake

Every request is a signed `POST` with an `X-Amz-Target` header naming the operation. The fake checks that the SigV4 credential scope names the region and the service, and refuses anything else the way AWS does:

<!-- ds:block id=aws-signedfor-jpc597t6 -->
**fakeAWS.signedFor** · [`examples/plugins/aws-httptest/main.go:135-144`](../../../examples/plugins/aws-httptest/main.go#L135-L144)

```go
func (f *fakeAWS) signedFor(w http.ResponseWriter, auth, service string) bool {
	if !strings.Contains(auth, "/"+demoRegion+"/"+service+"/aws4_request") {
		writeError(w, http.StatusForbidden, "InvalidSignatureException")
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes[service]++
	return true
}
```
<!-- /ds:block hash=e0b408 -->

## Step 4 — Read through dotenv and JSONC

The same instance installs in both formats. Two keys of `prod/db` share one request, and the JSONC read of `db_url` comes from the cache:

<!-- ds:block id=aws-readdotenv-c8y6j8u7 -->
**readDotenv** · [`examples/plugins/aws-httptest/main.go:207-217`](../../../examples/plugins/aws-httptest/main.go#L207-L217)

```go
func readDotenv(w io.Writer, p *aws.Plugin) error {
	f := dotenv.Parse(envSource, dotenv.WithPlugin(p))
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-12s %s\n", key+":", v)
	}
	return nil
}
```
<!-- /ds:block hash=827477 -->

## Step 5 — Supply credentials another way

`FromSharedCredentials` reads one profile once, in `New`; `New` fails with `aws.ErrNoCredentials` when no source was configured at all:

<!-- ds:block id=aws-credentialsources-dkp46znp -->
**credentialSources** · [`examples/plugins/aws-httptest/main.go:238-249`](../../../examples/plugins/aws-httptest/main.go#L238-L249)

```go
func credentialSources(w io.Writer, dir string) error {
	file := filepath.Join(dir, "credentials")
	ini := "[ci]\naws_access_key_id = " + demoKeyID + "\naws_secret_access_key = " + demoSecret + "\n"
	if err := os.WriteFile(file, []byte(ini), 0o600); err != nil {
		return err
	}
	_, err := aws.New(aws.WithRegion(demoRegion), aws.FromSharedCredentials(file, "ci"))
	fmt.Fprintf(w, "%-12s ok=%t\n", "shared file:", err == nil)
	_, err = aws.New(aws.WithRegion(demoRegion))
	fmt.Fprintf(w, "%-12s %t\n", "no creds:", errors.Is(err, aws.ErrNoCredentials))
	return nil
}
```
<!-- /ds:block hash=eb814e -->

## Try it

```sh
cd examples/plugins/aws-httptest && go run .
```

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

## Pitfalls

- **Web identity, SSO, `credential_process` and instance metadata (IMDS) credentials are not supported yet**, and neither is STS `AssumeRole`. Fetch them yourself (for example with the AWS SDK in your own program, or from your platform's metadata) and hand them over through `WithCredentialsSource`, which is asked before every request and so can serve rotating temporary credentials; cache inside the source when fetching is expensive.
- **The region is not read from `~/.aws/config`.** Use `WithRegion` or `FromEnv` (`AWS_REGION`, then `AWS_DEFAULT_REGION`).
- **Hierarchical parameter names keep their slash.** `ssm//app/prod/db_url` reads `/app/prod/db_url`; `ssm/app/prod/db_url` reads `app/prod/db_url`, a different parameter.
- **`#key` needs a JSON object.** On a plain-string secret it fails with `aws.ErrInvalidRef`; leave the key out to read the whole value. A missing key wraps `plugin.ErrNotFound` and lists the key names, never the values.
- **Errors map by AWS error code first, then by status.** `ResourceNotFoundException` / `ParameterNotFound` is `plugin.ErrNotFound`; access denied, bad or expired credentials, signature errors and KMS refusals (`DecryptionFailure`, `InvalidKeyId`) are `plugin.ErrDenied`; throttling, timeouts and 5xx are `plugin.ErrUnavailable` (`TestAPIErrorMapping`).
- **Credentials never print.** `Credentials.String` and `%#v` show only the access key id (`TestCredentialsNeverPrinted`), and no error carries a secret value.
- **The cache holds each value for the plugin's lifetime.** `Purge()` forgets it; failures are never cached. The cache key ignores `#key`, so several keys of one secret cost one request.
- **The plugin is read-only.** It calls only `GetSecretValue` and `GetParameter`.

## Related

- [Value plugins overview](overview.md), [vault plugin](vault.md), [write a value plugin](write-a-value-plugin.md)
- Package reference: [plugins/aws README](../../../plugins/aws/README.md)
- Example: [`examples/plugins/aws-httptest`](../../../examples/plugins/aws-httptest); package [`Example`](../../../plugins/aws/example_test.go); cross-format test: [`integration/aws`](../../../integration/aws)
- API: [pkg.go.dev/github.com/ubgo/textdata/plugins/aws](https://pkg.go.dev/github.com/ubgo/textdata/plugins/aws)
