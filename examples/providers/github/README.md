# GitHub provider with a fake gh

Mounts the real `githubplugin` under a root named `dotenvctl`, exactly as `dotenvctl` does, with a fake `providerkit.Runner` standing in for `gh`. Each indented `[gh …] stdin="…"` line is a call the plugin made, showing that values travel on stdin. Covers a dry run with the target from the current repository, a `--json` push to an environment, `env-create`, `list`, `prune --keep`, the `--org`/`--repo` usage error, the secret-name check and the `GH_TOKEN` label.

Guide: [github.md](../../../docs/guides/providers/github.md).

No `gh` binary, network or GitHub account. The last scenario sets `GH_TOKEN` to a dummy value for one run and restores it; the fake never sees it. The test clears `GH_TOKEN` and `GITHUB_TOKEN` so the `account:` lines are deterministic.

## Run

```sh
cd examples/providers/github && go run .
go test ./providers/github/   # from examples/: compares the output with testdata/output.txt
```

## Expected output

```text
$ dotenvctl github push --prefix=GITHUB_SECRET_ --strip-prefix --dry-run
    [gh repo view --json nameWithOwner --jq .nameWithOwner] stdin=""
    [gh api user --jq .login] stdin=""
target : acme/api (from cwd git remote)
account: octocat (stored gh auth)
keys   : 2
API_KEY: skipped-placeholder
DB_URL: would-push
GHCR_PAT: would-push
exit 0

$ dotenvctl --json github push --prefix=GITHUB_SECRET_ --strip-prefix --repo acme/api --env prod --yes
    [gh api user --jq .login] stdin=""
    [gh secret set DB_URL --repo acme/api --env prod] stdin="postgres://db.internal/app"
    [gh secret set GHCR_PAT --repo acme/api --env prod] stdin="ghp_example_only"
{"ok":true,"data":{"meta":{"account":"octocat (stored gh auth)","target":"acme/api (environment prod)"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"DB_URL","action":"pushed"},{"name":"GHCR_PAT","action":"pushed"}],"dry_run":false}}
exit 0

$ dotenvctl github env-create prod --repo acme/api --yes
target : acme/api
environment: prod (create or confirm)
    [gh api --method PUT repos/acme/api/environments/prod] stdin=""
prod: environment ready
exit 0

$ dotenvctl github list --repo acme/api --env prod
    [gh secret list --json name --repo acme/api --env prod] stdin=""
API_KEY
DB_URL
GHCR_PAT
OLD_TOKEN
VPS_DEPLOY_KEY
exit 0

$ dotenvctl github prune --prefix=GITHUB_SECRET_ --strip-prefix --repo acme/api --keep VPS_DEPLOY_KEY --dry-run
    [gh secret list --json name --repo acme/api] stdin=""
    [gh api user --jq .login] stdin=""
target : acme/api
account: octocat (stored gh auth)
keys   : 1
API_KEY: kept-skipped
VPS_DEPLOY_KEY: kept-keep-flag
OLD_TOKEN: would-delete
exit 0

$ dotenvctl github push --org acme --repo acme/api --yes
dotenvctl: --org is mutually exclusive with --repo/--environment
exit 2

$ dotenvctl github push LOG_LEVEL app.name --repo acme/api --yes
dotenvctl: key(s) [app.name] are not valid GitHub secret names ([A-Za-z_][A-Za-z0-9_]*) — rename them or select others; refusing a partial push
exit 2

# with GH_TOKEN set in the environment
$ dotenvctl github push DB_HOST --repo acme/api --dry-run
    [gh api user --jq .login] stdin=""
target : acme/api
account: octocat (via GH_TOKEN)
keys   : 1
DB_HOST: would-push
exit 0
```
