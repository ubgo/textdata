# Contributing to textdata

Thanks for your interest in improving **textdata**. This guide covers how to get set up, the guarantees that must not break, and what a good pull request looks like.

By participating, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting started

```sh
git clone https://github.com/ubgo/textdata.git
cd textdata
task test       # quick: every module in go.work
task check      # the full gate: gofmt, dependency check, vet, race tests, short fuzz of every target
task fuzz:list  # every fuzz target
```

You need Go 1.26.4 or newer for the whole workspace (the libraries themselves build on Go 1.22) and [Task](https://taskfile.dev). The repository holds several Go modules: the shared base at the root, one module per format (`dotenv/`, `jsonc/`), `clikit/`, the format CLIs, the value plugins under `plugins/`, the push providers under `dotenv/cli/providers/`, and `cmd/textdatactl`. A `go.work` at the root stitches them together, so a change to the base is visible to every module immediately. `examples/` and `integration/` are test-only modules and are never released.

## The guarantees (non-negotiable)

Every one of these is pinned by fuzz or property tests through `texttest`. A change that trips one is wrong, not the test.

- `Parse(x).Render() == x`, byte for byte, for every input, valid or not.
- An edit changes only the bytes it touches.
- Nothing touches the process environment, and no command runs unless the caller turned that on.
- Errors carry `file:line:col`.
- The base and the format modules use the Go standard library only.

## The dependency rules

- Formats and value plugins import the base; the base never imports a format.
- Formats never import each other or a plugin. A value plugin is written once against `plugin` and works in every format.
- No module requires a sibling it only uses in tests. `task deps:check` (part of `task check`) and `integration/deps_test.go` enforce this.
- **The base is public API.** Go's minimal version selection can pair an older format with a newer base, so base changes are additive only.

## Ways to contribute

- **Report a bug**: open an issue with the bug template; include the smallest input file that shows it, the module and version, and what you expected.
- **Request a feature**: open an issue with the feature template; describe the problem first, then the change you have in mind.
- **Send a pull request**: for anything non-trivial, open an issue first so we can agree on the approach before you write code.

## Branches and commits

- Branch off `main` with a short descriptive name (`fix/...`, `feat/...`, `docs/...`).
- Keep commits focused: one logical change per commit, with a message that says what changed and why.
- A bug fix comes with a test that fails before the fix.

## Pull request checklist

- [ ] The change is scoped and described (link the issue it closes).
- [ ] `task check` passes.
- [ ] New behaviour has a test; parser changes keep the fuzz targets green.
- [ ] Docs are updated: the module README, the guide under `docs/`, and `ds check` is clean when a documented block changed.
- [ ] The module's `CHANGELOG.md` has an entry under `[Unreleased]` for user-facing changes.
- [ ] No unrelated files or formatting churn.

## Changelog

Each module keeps its own `CHANGELOG.md`, following [Keep a Changelog](https://keepachangelog.com/). Add user-facing changes under `[Unreleased]` in the module you changed.

## Releasing

Releases are manual and PATCH-only by default; no CI runs on push. Each module has its own path-prefixed tag (`v0.1.4` for the base, `dotenv/v0.1.4`, `plugins/vault/v0.1.4`, …), tagged in dependency order: the base and `clikit` first, then the formats, value plugins and providers, then the format CLIs, then `cmd/textdatactl`. A version the checksum database has seen can never be reused, so a bad release is fixed by releasing the next PATCH, never by re-tagging.

## Questions

Open an [issue](https://github.com/ubgo/textdata/issues). We're happy to help you land your first contribution.
