# Vercel provider with a fake vercel CLI

Mounts the real `vercelplugin` under a root named `dotenvctl` with a fake `providerkit.Runner` standing in for `vercel`. Each indented `[vercel …] stdin="…"` line is a call the plugin made. Covers the default `development` target, a `--sensitive` push to two targets, a prefix-stripped push, `list` (including an empty target), `prune`, the push-only `--sensitive` flag and a `--json` push that fails part-way.

Guide: [vercel.md](../../../docs/guides/providers/vercel.md).

No `vercel` binary, network or Vercel account. The test clears `VERCEL_TOKEN`; the banner reads `unlinked directory` because the example's directory has no `.vercel/project.json`.

## Run

```sh
cd examples/providers/vercel && go run .
go test ./providers/vercel/   # from examples/: compares the output with testdata/output.txt
```

## Expected output

```text
$ dotenvctl vercel push --dry-run
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: development
keys   : 3
API_KEY: skipped-placeholder
EMPTY: skipped-empty
DB_URL: would-push
DB_HOST: would-push
VERCEL_ANALYTICS_ID: would-push
exit 0

$ dotenvctl vercel push DB_URL --target production,preview --sensitive --yes
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: production, preview
keys   : 1
    [vercel env add DB_URL production --force --sensitive] stdin="postgres://db.internal/app"
    [vercel env add DB_URL preview --force --sensitive] stdin="postgres://db.internal/app"
DB_URL: pushed
exit 0

$ dotenvctl vercel push --prefix VERCEL_ --strip-prefix --target preview --dry-run
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: preview
keys   : 1
ANALYTICS_ID: would-push
exit 0

$ dotenvctl vercel list --target production
    [vercel env ls production] stdin=""
API_KEY
DB_URL
OLD_FLAG
exit 0

$ dotenvctl vercel list --target preview
    [vercel env ls preview] stdin=""
no environment variables found for target(s) preview (each target is listed separately — try another --target: production, preview or development)
exit 0

$ dotenvctl vercel prune --target production --dry-run
    [vercel env ls production] stdin=""
    [vercel whoami] stdin=""
project: unlinked directory (vercel CLI will resolve or prompt)
account: acme-bot (stored vercel auth)
targets: production
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: would-delete
exit 0

$ dotenvctl vercel list --sensitive
dotenvctl: unknown flag: --sensitive
exit 2

$ dotenvctl --json vercel push --target production --yes
    [vercel whoami] stdin=""
    [vercel env add DB_URL production --force] stdin="postgres://db.internal/app"
    [vercel env add DB_HOST production --force] stdin="db.internal"
{"ok":false,"data":{"meta":{"account":"acme-bot (stored vercel auth)","project":"unlinked directory (vercel CLI will resolve or prompt)","targets":"production"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"EMPTY","action":"skipped-empty"},{"name":"DB_URL","action":"pushed"}],"dry_run":false},"error":{"code":"exec","message":"push DB_HOST: target production: exit status 1: Error: simulated refusal"}}
exit 1
```
