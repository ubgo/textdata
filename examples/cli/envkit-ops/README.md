# envkit-ops

Drives the operations API behind `dotenvctl` (`github.com/ubgo/textdata/dotenv/cli/envkit`) from Go, on a `.env` family in a temp directory: `Inventories` lists the family, `SelectFile` picks the keys a deploy would push, `Set` previews with `DryRun` and then writes next to an anchor, `Unset` and `Restore` comment a key out and back, `Diff` compares two files, `BuildMatrix` with `DriftRows` shows drift against the `.env.example` contract, and `ChildEnv` builds a child process environment where the file wins.

Guide: [envkit: the operations API in Go](../../../docs/guides/cli/envkit.md).

## Run it

```sh
cd examples/cli/envkit-ops && go run .
```

Expected output:

```text
.env.stag    env=stag    contract=false keys=3 placeholders=1
.env.prod    env=prod    contract=false keys=5 placeholders=0
.env.example env=example contract=true  keys=3 placeholders=0
select TOKEN=t-1
select REGION=eu
select DB_URL=postgres://db.prod/app
named key missing: envkit: key "NOPE" not found
preview: [+SENTRY_ENV=staging] written: false
restore: [-# SENTRY_ENV=staging +SENTRY_ENV=staging]
unset absent key is ErrKeyNotFound: true
diff + DEPLOY_REGION
diff + DEPLOY_TOKEN
diff - SENTRY_ENV
diff ~ API_KEY: __FILL_ME__ -> k-123
diff ~ DB_URL: postgres://db.stag/app -> postgres://db.prod/app
diff ~ HOST: db.stag -> db.prod
columns: [stag prod]
drift API_KEY       stag=placeholder prod=present
drift DEPLOY_REGION stag=missing prod=present
drift DEPLOY_TOKEN  stag=missing prod=present
drift SENTRY_DSN    stag=disabled prod=missing
drift SENTRY_ENV    stag=present prod=missing
contract gaps: true [SENTRY_DSN]
child env: [API_KEY=k-123 DB_URL=postgres://db.prod/app DEPLOY_REGION=eu DEPLOY_TOKEN=t-1 HOST=db.prod TERM=xterm]
```

`main_test.go` runs the program and checks this output.
