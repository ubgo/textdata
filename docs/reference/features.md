# Feature index

Every feature of textdata, one row each, grouped by area. Each row names the package that provides it, the guide step that teaches it, and a runnable program under [`examples/`](../../examples) that calls it: `cd examples/<area>/<name> && go run .` runs it, and its `main_test.go` checks the output. Start from the [docs index](../README.md) for the learning path, or from [Choosing](../choosing/README.md) when you are deciding between options.

Packages are Go import paths under `github.com/ubgo/textdata/`. Most features are reachable from the library and, where a CLI verb exists, from `textdatactl` as well.

## Areas

- [Base: plugin](#base-plugin)
- [Base: expand](#base-expand)
- [Base: atomicfile, pos, path, texttest](#base-atomicfile-pos-path-texttest)
- [dotenv: read](#dotenv-read)
- [dotenv: syntax](#dotenv-syntax)
- [dotenv: edit](#dotenv-edit)
- [dotenv: expand](#dotenv-expand)
- [dotenv: plugins](#dotenv-plugins)
- [dotenv: save](#dotenv-save)
- [jsonc: parse](#jsonc-parse)
- [jsonc: read](#jsonc-read)
- [jsonc: edit](#jsonc-edit)
- [jsonc: lint, extends, expand, save](#jsonc-lint-extends-expand-save)
- [jsonc tools: format, diff, generate, schema](#jsonc-tools-format-diff-generate-schema)
- [jsonc CLI (`textdatactl jsonc`)](#jsonc-cli-textdatactl-jsonc)
- [dotenv CLI (`dotenvctl`, `textdatactl dotenv`)](#dotenv-cli-dotenvctl-textdatactl-dotenv)
- [Push providers](#push-providers)
- [clikit](#clikit)
- [textdatactl](#textdatactl)
- [Value plugins](#value-plugins)

## Base: plugin

| Feature | Package | Guide | Example |
|---|---|---|---|
| Plugin interfaces (Plugin, Lookuper, Resolver, CommandRunner, ValueTransformer, SaveGuard, ExpandObserver, Validator, Includer) | `plugin` | [base/plugin § Step 1](../guides/base/plugin.md#step-1--write-a-plugin-with-the-abilities-it-needs) | [`examples/base/registry`](../../examples/base/registry) |
| Capability detection (`Capability`, `Has`, `Info`) | `plugin` | [base/plugin § Step 2](../guides/base/plugin.md#step-2--check-what-was-detected) | [`examples/base/registry`](../../examples/base/registry) |
| Plugin registry (`NewRegistry`, `Filter`, `Check`) | `plugin` | [base/plugin § Step 3](../guides/base/plugin.md#step-3--keep-plugins-in-a-registry) | [`examples/base/registry`](../../examples/base/registry) |
| Plugin errors (`Error`, `Errorf`, `ErrUnavailable`, `ErrUnsupportedScheme`, `ErrDenied`) | `plugin` | [base/plugin § Step 4](../guides/base/plugin.md#step-4--name-the-plugin-in-errors) | [`examples/base/registry`](../../examples/base/registry) |
| `Document` read-only view for guards and validators | `plugin` | [base/plugin § Step 1](../guides/base/plugin.md#step-1--write-a-plugin-with-the-abilities-it-needs) | [`examples/base/registry`](../../examples/base/registry) |

## Base: expand

| Feature | Package | Guide | Example |
|---|---|---|---|
| `${VAR}` engine with operators (`-`, `:-`, `+`, `:+`, `?`, `:?`), nesting, `$$` | `expand` | [base/expand § Step 2](../guides/base/expand.md#step-2--use-the-operators-nesting-and-) | [`examples/base/expand-engine`](../../examples/base/expand-engine) |
| `${scheme:ref}` resolver hook (`SplitScheme`, `ParseRef`) | `expand` | [base/expand § Step 3](../guides/base/expand.md#step-3--answer-schemeref-with-a-resolver) | [`examples/base/expand-engine`](../../examples/base/expand-engine) |
| `$(cmd)` substitution hook (`Engine.Run`) | `expand` | [base/expand § Step 4](../guides/base/expand.md#step-4--turn-on-cmd-with-a-runner) | [`examples/base/expand-engine`](../../examples/base/expand-engine) |
| Cycle guard and depth bound (`Stack`, `DefaultMaxDepth`) | `expand` | [base/expand § Step 5](../guides/base/expand.md#step-5--stop-cycles-with-the-stack) | [`examples/base/expand-engine`](../../examples/base/expand-engine) |
| `RequiredError` for `${VAR:?e}` | `expand` | [base/expand § Step 6](../guides/base/expand.md#step-6--fail-loudly-with-varmsg) | [`examples/base/expand-engine`](../../examples/base/expand-engine) |

## Base: atomicfile, pos, path, texttest

| Feature | Package | Guide | Example |
|---|---|---|---|
| Atomic write: temp file, fsync, rename, keep mode, write through symlinks (`Write`, `Options`, `Resolve`) | `atomicfile` | [base/atomicfile § Step 1](../guides/base/atomicfile.md#step-1--write-a-new-file) | [`examples/base/atomic-write`](../../examples/base/atomic-write) |
| Positions and `file:line:col` errors (`Pos`, `Index`, `Error`, `ErrorList`, `First`) | `pos` | [base/pos-and-path § Step 3](../guides/base/pos-and-path.md#step-3--map-byte-offsets-to-line-and-column) | [`examples/base/paths`](../../examples/base/paths) |
| Paths `a."b.c"[2]` and JSON Pointer (`Parse`, `ParsePointer`, `ParseAny`, `Path.String`) | `path` | [base/pos-and-path § Step 1](../guides/base/pos-and-path.md#step-1--parse-dotted-paths-and-json-pointers) | [`examples/base/paths`](../../examples/base/paths) |
| Round-trip and untouched-bytes property harness (`RoundTrip`, `CheckUntouched`, `Untouched`, `DiffWindow`, `Seed`, `Corpus`) | `texttest` | [base/texttest § Step 2](../guides/base/texttest.md#step-2--round-trip-the-corpus) | [`examples/base/property-harness`](../../examples/base/property-harness) |

## dotenv: read

| Feature | Package | Guide | Example |
|---|---|---|---|
| Read a file to a map (`Read`) | `dotenv` | [dotenv/reading § Step 1](../guides/dotenv/reading.md#step-1--read-the-values-in-one-call) | [`examples/dotenv/reading`](../../examples/dotenv/reading) |
| Open a file, missing file is empty (`Open`, `Existed`, `Path`, `Mode`) | `dotenv` | [dotenv/reading § Step 2](../guides/dotenv/reading.md#step-2--open-a-file-you-may-edit) | [`examples/dotenv/reading`](../../examples/dotenv/reading) |
| Parse a string or reader (`Parse`, `ParseReader`) | `dotenv` | [dotenv/reading § Step 3](../guides/dotenv/reading.md#step-3--parse-a-string-or-a-reader) | [`examples/dotenv/reading`](../../examples/dotenv/reading) |
| Typed lookups (`Get`, `Has`, `Count`, `Keys`, `Pairs`, `Map`) | `dotenv` | [dotenv/reading § Step 4](../guides/dotenv/reading.md#step-4--query-the-file) | [`examples/dotenv/reading`](../../examples/dotenv/reading) |
| Entries and kinds (`Entries`, `Entry`, `Kind`) | `dotenv` | [dotenv/entries § Step 2](../guides/dotenv/entries.md#step-2--read-an-entrys-fields) | [`examples/dotenv/entries`](../../examples/dotenv/entries) |
| Disabled settings (`Disabled`) | `dotenv` | [dotenv/editing § Step 3](../guides/dotenv/editing.md#step-3--turn-a-setting-off-and-back-on) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Inherited (name-only) declarations (`Inherited`) | `dotenv` | [dotenv/editing § Step 4](../guides/dotenv/editing.md#step-4--handle-inherited-declarations) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Typed configuration with the standard library | `dotenv` | [dotenv/typed-config § Step 2](../guides/dotenv/typed-config.md#step-2--bind-them-with-the-standard-library) | [`examples/dotenv/typed-config`](../../examples/dotenv/typed-config) |

## dotenv: syntax

| Feature | Package | Guide | Example |
|---|---|---|---|
| Quoting rules (single, double, backtick) | `dotenv` | [dotenv/syntax § Step 1](../guides/dotenv/syntax.md#step-1--pick-a-quoting-style) | [`examples/dotenv/syntax`](../../examples/dotenv/syntax) |
| Multiline values | `dotenv` | [dotenv/syntax § Step 2](../guides/dotenv/syntax.md#step-2--write-a-multi-line-value) | [`examples/dotenv/syntax`](../../examples/dotenv/syntax) |
| Comments (full line and inline) | `dotenv` | [dotenv/syntax § Step 3](../guides/dotenv/syntax.md#step-3--know-where-a-value-ends-comments) | [`examples/dotenv/syntax`](../../examples/dotenv/syntax) |
| Escape sequences (`WithEscapes`, `EscapeMode`) | `dotenv` | [dotenv/syntax § Step 8](../guides/dotenv/syntax.md#step-8--choose-an-escape-mode) | [`examples/dotenv/syntax`](../../examples/dotenv/syntax) |
| `export` prefix, `:` delimiter, key rules, duplicates | `dotenv` | [dotenv/syntax § Step 4](../guides/dotenv/syntax.md#step-4--use-the--delimiter) | [`examples/dotenv/syntax`](../../examples/dotenv/syntax) |
| Encrypted values left as written | `dotenv` | [dotenv/syntax § Step 7](../guides/dotenv/syntax.md#step-7--keep-encrypted-values-literal) | [`examples/dotenv/syntax`](../../examples/dotenv/syntax) |

## dotenv: edit

| Feature | Package | Guide | Example |
|---|---|---|---|
| Set a value in place or append (`Set`) | `dotenv` | [dotenv/editing § Step 1](../guides/dotenv/editing.md#step-1--change-or-add-a-value-with-set) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Place a new key next to an anchor (`SetAfter`, `SetBefore`, `ErrAnchorNotFound`) | `dotenv` | [dotenv/editing § Step 2](../guides/dotenv/editing.md#step-2--place-a-new-key-next-to-a-related-one) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Comment out or delete a key (`Unset`) | `dotenv` | [dotenv/editing § Step 3](../guides/dotenv/editing.md#step-3--turn-a-setting-off-and-back-on) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Turn a disabled setting back on (`Restore`) | `dotenv` | [dotenv/editing § Step 3](../guides/dotenv/editing.md#step-3--turn-a-setting-off-and-back-on) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Generate a file from entries (`NewPair`, `NewComment`, `NewBlank`, `Append`, `InsertAfter`, `InsertBefore`) | `dotenv` | [dotenv/editing § Step 5](../guides/dotenv/editing.md#step-5--generate-a-file-from-entries) | [`examples/dotenv/editing`](../../examples/dotenv/editing) |
| Clone between environments (`Clone`) | `dotenv` | [dotenv/editing § Step 7](../guides/dotenv/editing.md#step-7--derive-another-environment-with-clone) | [`examples/dotenv/clone-env`](../../examples/dotenv/clone-env) |
| Render byte-exact (`Render`, `Bytes`) | `dotenv` | [dotenv/saving § Step 3](../guides/dotenv/saving.md#step-3--keep-line-endings-and-the-trailing-newline) | [`examples/dotenv/saving`](../../examples/dotenv/saving) |

## dotenv: expand

| Feature | Package | Guide | Example |
|---|---|---|---|
| Expanded value (`GetExpanded`, `ExpandedMap`) | `dotenv` | [dotenv/expansion § Step 1](../guides/dotenv/expansion.md#step-1--get-the-raw-or-the-expanded-value) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion) |
| Expansion operators, nesting, `$$`, self-reference rule | `dotenv` | [dotenv/expansion § Step 2](../guides/dotenv/expansion.md#step-2--use-the--operators) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion) |
| Required variables (`RequiredError`) | `dotenv` | [dotenv/expansion § Step 6](../guides/dotenv/expansion.md#step-6--fail-loudly-with-requirederror) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion) |
| Values from outside the file (`WithLookup`) | `dotenv` | [dotenv/plugins § Step 1](../guides/dotenv/plugins.md#step-1--install-one-function-hooks) | [`examples/dotenv/plugin-hooks`](../../examples/dotenv/plugin-hooks) |
| Command substitution, off by default (`WithCommandSubstitution`, `WithCommandRunner`) | `dotenv` | [dotenv/expansion § Step 7](../guides/dotenv/expansion.md#step-7--run-commands-opt-in) | [`examples/dotenv/expansion`](../../examples/dotenv/expansion) |

## dotenv: plugins

| Feature | Package | Guide | Example |
|---|---|---|---|
| Install plugins (`WithPlugin`, `Plugins`) | `dotenv` | [dotenv/plugins § Step 3](../guides/dotenv/plugins.md#step-3--write-a-plugin-type) | [`examples/dotenv/write-a-plugin`](../../examples/dotenv/write-a-plugin) |
| The seven hooks (Lookuper, Resolver, CommandRunner, ValueTransformer, SaveGuard, EntryObserver, ExpandObserver) | `dotenv` | [dotenv/plugins § Step 4](../guides/dotenv/plugins.md#step-4--watch-with-observers) | [`examples/dotenv/plugin-hooks`](../../examples/dotenv/plugin-hooks) |
| Hooks change reads, never writes (the firewall), plugin ordering | `dotenv` | [dotenv/plugins § Step 6](../guides/dotenv/plugins.md#step-6--rely-on-the-firewall) | [`examples/dotenv/plugin-hooks`](../../examples/dotenv/plugin-hooks) |
| Value transform on read (`WithValueTransform`) | `dotenv` | [dotenv/plugins § Step 1](../guides/dotenv/plugins.md#step-1--install-one-function-hooks) | [`examples/dotenv/plugin-hooks`](../../examples/dotenv/plugin-hooks) |
| Veto a save (`WithSaveGuard`, `SaveGuard`) | `dotenv` | [dotenv/plugins § Step 1](../guides/dotenv/plugins.md#step-1--install-one-function-hooks) | [`examples/dotenv/plugin-hooks`](../../examples/dotenv/plugin-hooks) |
| Writing a plugin | `dotenv` | [dotenv/plugins § Step 3](../guides/dotenv/plugins.md#step-3--write-a-plugin-type) | [`examples/dotenv/write-a-plugin`](../../examples/dotenv/write-a-plugin) |

## dotenv: save

| Feature | Package | Guide | Example |
|---|---|---|---|
| Atomic save (`Save`, `SaveAs`), keeps mode and symlinks | `dotenv` | [dotenv/saving § Step 1](../guides/dotenv/saving.md#step-1--save-to-the-file-you-opened) | [`examples/dotenv/saving`](../../examples/dotenv/saving) |
| Line endings and trailing newline preserved | `dotenv` | [dotenv/saving § Step 3](../guides/dotenv/saving.md#step-3--keep-line-endings-and-the-trailing-newline) | [`examples/dotenv/saving`](../../examples/dotenv/saving) |

## jsonc: parse

| Feature | Package | Guide | Example |
|---|---|---|---|
| Tolerant parse with comments attached (`Parse`, `ParseReader`, `Open`) | `jsonc` | [jsonc/parse-and-strict § Step 1](../guides/jsonc/parse-and-strict.md#step-1--parse-a-commented-file) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Strict JSON mode (`Strict`, `IsStrict`) | `jsonc` | [jsonc/parse-and-strict § Step 2](../guides/jsonc/parse-and-strict.md#step-2--parse-plain-json-with-strict) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Positioned syntax errors (`WithFileName`, `Err`, `ErrInvalid`) | `jsonc` | [jsonc/parse-and-strict § Step 3](../guides/jsonc/parse-and-strict.md#step-3--handle-syntax-errors) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Byte-exact render (`Render`, `Bytes`) | `jsonc` | [jsonc/parse-and-strict § Step 1](../guides/jsonc/parse-and-strict.md#step-1--parse-a-commented-file) | [`examples/jsonc/merge-layers`](../../examples/jsonc/merge-layers) |

## jsonc: read

| Feature | Package | Guide | Example |
|---|---|---|---|
| Get values (`Get`, `GetString`, `GetInt`, `GetFloat`, `GetBool`) | `jsonc` | [jsonc/read-and-paths § Step 1](../guides/jsonc/read-and-paths.md#step-1--read-typed-values) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Inspect structure (`Has`, `Keys`, `Len`, `Kind`) | `jsonc` | [jsonc/read-and-paths § Step 2](../guides/jsonc/read-and-paths.md#step-2--inspect-the-structure) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Comments of a value (`Comment`) | `jsonc` | [jsonc/read-and-paths § Step 3](../guides/jsonc/read-and-paths.md#step-3--read-the-comment-of-a-value) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Plain JSON out (`JSON`) and decode into a struct (`Decode`) | `jsonc` | [jsonc/read-and-paths § Step 7](../guides/jsonc/read-and-paths.md#step-7--decode-into-a-struct) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Read a file to a map (`Read`) | `jsonc` | [jsonc/read-and-paths § Step 8](../guides/jsonc/read-and-paths.md#step-8--read-a-file-in-one-call) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Dotted paths and JSON Pointer | `jsonc` | [jsonc/read-and-paths § Step 4](../guides/jsonc/read-and-paths.md#step-4--address-values-with-paths) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |
| Positions of values and keys (`Pos`, `KeyPos`) | `jsonc` | [jsonc/read-and-paths § Step 5](../guides/jsonc/read-and-paths.md#step-5--report-positions) | [`examples/jsonc/read-config`](../../examples/jsonc/read-config) |

## jsonc: edit

| Feature | Package | Guide | Example |
|---|---|---|---|
| Set with parent creation (`Set`) | `jsonc` | [jsonc/edit § Step 1](../guides/jsonc/edit.md#step-1--set-values) | [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config) |
| Place a new member (`SetAfter`, `SetBefore`) | `jsonc` | [jsonc/edit § Step 2](../guides/jsonc/edit.md#step-2--place-a-new-key-next-to-a-sibling) | [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config) |
| Delete a member or element (`Delete`) | `jsonc` | [jsonc/edit § Step 3](../guides/jsonc/edit.md#step-3--delete-a-member) | [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config) |
| Arrays (`Append`, `Insert`, `Remove`) | `jsonc` | [jsonc/edit § Step 4](../guides/jsonc/edit.md#step-4--edit-arrays) | [`examples/jsonc/arrays-comments`](../../examples/jsonc/arrays-comments) |
| Comments (`SetComment`, `RemoveComment`) | `jsonc` | [jsonc/edit § Step 6](../guides/jsonc/edit.md#step-6--set-and-remove-comments) | [`examples/jsonc/arrays-comments`](../../examples/jsonc/arrays-comments) |
| Comment out and bring back (`Disable`, `Restore`) | `jsonc` | [jsonc/disable-restore § Step 1](../guides/jsonc/disable-restore.md#step-1--disable-a-member) | [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config) |
| Merge and clone (`Merge`, `Clone`) | `jsonc` | [jsonc/merge-clone § Step 1](../guides/jsonc/merge-clone.md#step-1--clone-the-base-and-merge-the-override) | [`examples/jsonc/merge-layers`](../../examples/jsonc/merge-layers) |

## jsonc: lint, extends, expand, save

| Feature | Package | Guide | Example |
|---|---|---|---|
| Lint (`Lint`, `LintRule`, `AllLintRules`, `LintOnly`, `ForbidTrailingCommas`, `Issue`) | `jsonc` | [jsonc/lint-validate § Step 1](../guides/jsonc/lint-validate.md#step-1--run-every-rule) | [`examples/jsonc/lint`](../../examples/jsonc/lint) |
| Validate through installed Validator plugins (`Validate`) | `jsonc` | [jsonc/lint-validate § Step 4](../guides/jsonc/lint-validate.md#step-4--run-validate) | [`examples/jsonc/lint`](../../examples/jsonc/lint) |
| `$extends` (`Extended`, `Extends`, `ExtendsKey`, `MaxExtendsDepth`, extends errors) | `jsonc` | [jsonc/extends § Step 2](../guides/jsonc/extends.md#step-2--build-the-merged-view) | [`examples/jsonc/extends`](../../examples/jsonc/extends) |
| Expansion (`WithExpand`, `Expanded`, `GetExpanded`, `ExpandRequested`, `WithLookup`) | `jsonc` | [jsonc/expansion-and-plugins § Step 1](../guides/jsonc/expansion-and-plugins.md#step-1--expand-through-a-lookup) | [`examples/jsonc/expand`](../../examples/jsonc/expand) |
| Command substitution, off by default (`WithCommandSubstitution`) | `jsonc` | [jsonc/expansion-and-plugins § Step 4](../guides/jsonc/expansion-and-plugins.md#step-4--turn-on-command-substitution-deliberately) | [`examples/jsonc/expand`](../../examples/jsonc/expand) |
| Plugins (`WithPlugin`, `Plugins`) | `jsonc` | [jsonc/expansion-and-plugins § Step 3](../guides/jsonc/expansion-and-plugins.md#step-3--install-value-plugins) | [`examples/jsonc/expand`](../../examples/jsonc/expand) |
| Atomic save with SaveGuards (`Save`, `SaveAs`, `SetPath`) | `jsonc` | [jsonc/save § Step 1](../guides/jsonc/save.md#step-1--save-edits-in-place) | [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config) |

## jsonc tools: format, diff, generate, schema

| Feature | Package | Guide | Example |
|---|---|---|---|
| Re-lay a file out keeping comments (`format.Format`, `Style`, `DefaultStyle`) | `jsonc/format` | [jsonc-tools/format § Step 2](../guides/jsonc-tools/format.md#step-2--choose-a-style) | [`examples/jsonc-tools/format`](../../examples/jsonc-tools/format) |
| Minify (`format.Minify`) | `jsonc/format` | [jsonc-tools/format § Step 4](../guides/jsonc-tools/format.md#step-4--minify-to-plain-json) | [`examples/jsonc-tools/format`](../../examples/jsonc-tools/format) |
| Semantic diff by path (`diff.Diff`, `Change`, `Kind`) | `jsonc/diff` | [jsonc-tools/diff § Step 1](../guides/jsonc-tools/diff.md#step-1--list-the-changes) | [`examples/jsonc-tools/diff`](../../examples/jsonc-tools/diff) |
| Value equality (`diff.Equal`) | `jsonc/diff` | [jsonc-tools/diff § Step 4](../guides/jsonc-tools/diff.md#step-4--compare-values-with-the-same-rule) | [`examples/jsonc-tools/diff`](../../examples/jsonc-tools/diff) |
| Build a file by hand (`generate.New`, `Object`, `Array`, item options) | `jsonc/generate` | [jsonc-tools/generate § Step 4](../guides/jsonc-tools/generate.md#step-4--build-a-file-by-hand) | [`examples/jsonc-tools/generate-from-struct`](../../examples/jsonc-tools/generate-from-struct) |
| Generate from a config struct (`generate.FromStruct`) | `jsonc/generate` | [jsonc-tools/generate § Step 2](../guides/jsonc-tools/generate.md#step-2--generate-and-write-the-file) | [`examples/jsonc-tools/generate-from-struct`](../../examples/jsonc-tools/generate-from-struct) |
| Generator options (`Indent`, `Tabs`, `Spaces`, `CRLF`, `TrailingCommas`, `Comments`, `FinalNewline`, `SchemaURL`) | `jsonc/generate` | [jsonc-tools/generate § Step 5](../guides/jsonc-tools/generate.md#step-5--pick-the-options) | [`examples/jsonc-tools/generate-from-struct`](../../examples/jsonc-tools/generate-from-struct) |
| Schema from a struct (`schema.FromStruct`) | `jsonc/schema` | [jsonc-tools/schema § Step 1](../guides/jsonc-tools/schema.md#step-1--build-the-schema-from-your-config-struct) | [`examples/jsonc-tools/schema-plugin`](../../examples/jsonc-tools/schema-plugin) |
| Validate a document (`schema.Validate`, `Result`, `Issue`, `OnWarning`) | `jsonc/schema` | [jsonc-tools/schema § Step 2](../guides/jsonc-tools/schema.md#step-2--validate-a-file) | [`examples/jsonc-tools/schema-plugin`](../../examples/jsonc-tools/schema-plugin) |
| Load or parse a schema (`schema.Parse`, `schema.Load`) | `jsonc/schema` | [jsonc-tools/schema § Step 3](../guides/jsonc-tools/schema.md#step-3--load-or-parse-a-schema-written-by-hand) | [`examples/jsonc-tools/schema-plugin`](../../examples/jsonc-tools/schema-plugin) |
| Schema as a Validator and SaveGuard plugin (`schema.Plugin`) | `jsonc/schema` | [jsonc-tools/schema § Step 4](../guides/jsonc-tools/schema.md#step-4--run-it-as-a-plugin) | [`examples/jsonc-tools/schema-plugin`](../../examples/jsonc-tools/schema-plugin) |

## jsonc CLI (`textdatactl jsonc`)

| Feature | Package | Guide | Example |
|---|---|---|---|
| `get` | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 1](../guides/cli/jsonc-cli.md#step-1--address-a-value-with-a-path) | [`examples/cli/embed-jsonc`](../../examples/cli/embed-jsonc) |
| `set` (`--json-value`, `--dry-run`) | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 2](../guides/cli/jsonc-cli.md#step-2--edit-values) | [`examples/cli/embed-jsonc`](../../examples/cli/embed-jsonc) |
| `unset`, `restore`, `delete` | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 2](../guides/cli/jsonc-cli.md#step-2--edit-values) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `diff` | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 3](../guides/cli/jsonc-cli.md#step-3--compare-and-convert) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `to-json` | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 3](../guides/cli/jsonc-cli.md#step-3--compare-and-convert) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `fmt` (`--check`, `--indent`, `--tabs`, `--trailing-commas`) | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 4](../guides/cli/jsonc-cli.md#step-4--format-and-validate-in-ci) | [`examples/cli/embed-jsonc`](../../examples/cli/embed-jsonc) |
| `validate` (`--schema`, `--forbid-trailing-commas`) | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 4](../guides/cli/jsonc-cli.md#step-4--format-and-validate-in-ci) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `--json` envelope, `--strict`, exit codes | `jsonc/cli/jsonccmd` | [cli/jsonc-cli § Step 5](../guides/cli/jsonc-cli.md#step-5--script-it---json---strict-and-exit-codes) | [`examples/cli/embed-jsonc`](../../examples/cli/embed-jsonc) |
| Embed the verbs (`Command`, `Execute`, `WithProgramName`) | `jsonc/cli/jsonccmd` | [cli/embedding § Step 5](../guides/cli/embedding.md#step-5--run-or-graft-the-jsonc-verbs) | [`examples/cli/embed-jsonc`](../../examples/cli/embed-jsonc) |

## dotenv CLI (`dotenvctl`, `textdatactl dotenv`)

| Feature | Package | Guide | Example |
|---|---|---|---|
| `get` (`--expand`) | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 2](../guides/cli/dotenvctl.md#step-2--read-values) | [`examples/cli/embed-dotenvctl`](../../examples/cli/embed-dotenvctl) |
| `set` (`--after`, `--before`, `--dry-run`) | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 3](../guides/cli/dotenvctl.md#step-3--edit-values) | [`examples/cli/embed-dotenvctl`](../../examples/cli/embed-dotenvctl) |
| `unset` (`--delete`, `--dry-run`) | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 3](../guides/cli/dotenvctl.md#step-3--edit-values) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `restore` | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 3](../guides/cli/dotenvctl.md#step-3--edit-values) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `list` (`--expand`, `--disabled`, `--inherited`), `keys` | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 2](../guides/cli/dotenvctl.md#step-2--read-values) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `run -- COMMAND` (file values in a child process) | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 4](../guides/cli/dotenvctl.md#step-4--run-a-command-with-the-files-values) | [`examples/cli/run-child`](../../examples/cli/run-child) |
| `envs` (discover the .env family) | `dotenv/cli/dotenvcmd` | [cli/multi-env § Step 1](../guides/cli/multi-env.md#step-1--envs-discover-the-family) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `matrix` (`--contract`, `--only-drift`, `--values`, `--reveal`, `--placeholder`, `--dir`) | `dotenv/cli/dotenvcmd` | [cli/multi-env § Step 2](../guides/cli/multi-env.md#step-2--matrix-the-drift-table) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `diff` (`--expand`, `--reveal`) | `dotenv/cli/dotenvcmd` | [cli/multi-env § Step 5](../guides/cli/multi-env.md#step-5--diff-two-files) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| Output formats (`--json`, `--format human|json|html`, `-o`) | `dotenv/cli/dotenvcmd` | [cli/dotenvctl § Step 1](../guides/cli/dotenvctl.md#step-1--learn-the-global-flags-the-envelope-and-the-exit-codes) | [`examples/cli/embed-dotenvctl`](../../examples/cli/embed-dotenvctl) |
| Exec plugins (`dotenvctl-<name>` on PATH, `plugins`, `DOTENVCTL_*` context) | `dotenv/cli/dotenvcmd` | [cli/exec-plugins § Step 4](../guides/cli/exec-plugins.md#step-4--list-what-dotenvctl-found) | [`examples/cli/exec-plugin`](../../examples/cli/exec-plugin) |
| Embed the CLI (`dotenvcmd.Execute`, `WithProgramName`) | `dotenv/cli/dotenvcmd` | [cli/embedding § Step 1](../guides/cli/embedding.md#step-1--run-a-dotenv-verb-in-process) | [`examples/cli/embed-dotenvctl`](../../examples/cli/embed-dotenvctl) |
| Operations API: select, edit, diff (`envkit.Select`, `SelectFile`, `Set`, `Unset`, `Restore`, `Diff`, `DiffMaps`) | `dotenv/cli/envkit` | [cli/envkit § Step 2](../guides/cli/envkit.md#step-2--select-the-keys-you-will-act-on) | [`examples/cli/envkit-ops`](../../examples/cli/envkit-ops) |
| Operations API: matrix and drift (`BuildMatrix`, `Inventories`, `DriftRows`, `SparseColumn`, `DisambiguateColumns`) | `dotenv/cli/envkit` | [cli/envkit § Step 5](../guides/cli/envkit.md#step-5--build-the-drift-matrix) | [`examples/cli/envkit-ops`](../../examples/cli/envkit-ops) |
| Operations API: run a child (`Run`, `ChildEnv`, `MergeEnv`) | `dotenv/cli/envkit` | [cli/envkit § Step 6](../guides/cli/envkit.md#step-6--build-a-child-process-environment) | [`examples/cli/envkit-ops`](../../examples/cli/envkit-ops) |
| Discover a .env family (`discover.Scan`, `Classify`, `envkit.Discover`, `EnvNameForPath`) | `dotenv/cli/discover` | [cli/envkit § Step 1](../guides/cli/envkit.md#step-1--discover-the-family) | [`examples/cli/envkit-ops`](../../examples/cli/envkit-ops) |

## Push providers

| Feature | Package | Guide | Example |
|---|---|---|---|
| GitHub: `push`, `list`, `prune` for repo, environment and org secrets (`--repo`, `--environment`, `--org`) | `dotenv/cli/providers/github` | [providers/github § Step 1](../guides/providers/github.md#step-1--preview-a-push) | [`examples/providers/github`](../../examples/providers/github) |
| GitHub: `env-create` (`--yes`) | `dotenv/cli/providers/github` | [providers/github § Step 4](../guides/providers/github.md#step-4--create-the-environment-first) | [`examples/providers/github`](../../examples/providers/github) |
| GitHub: authentication and token override | `dotenv/cli/providers/github` | [providers/github § Step 6](../guides/providers/github.md#step-6--act-as-another-identity) | [`examples/providers/github`](../../examples/providers/github) |
| GitHub: mount in your own binary (`githubplugin.New`) | `dotenv/cli/providers/github` | [providers/github § Step 7](../guides/providers/github.md#step-7--mount-it-in-your-own-binary) | [`examples/providers/github`](../../examples/providers/github) |
| Vercel: `push`, `list`, `prune` (`--target`, `--sensitive`) | `dotenv/cli/providers/vercel` | [providers/vercel § Step 2](../guides/providers/vercel.md#step-2--push-to-the-environments-you-mean) | [`examples/providers/vercel`](../../examples/providers/vercel) |
| Vercel: mount in your own binary (`vercelplugin.New`) | `dotenv/cli/providers/vercel` | [providers/vercel § Step 7](../guides/providers/vercel.md#step-7--mount-it-in-your-own-binary) | [`examples/providers/vercel`](../../examples/providers/vercel) |
| Selection flags (`--prefix`, `--exclude-prefix`, `--include`, `--exclude`, `--strip-prefix`, `--expand`, `--include-placeholders`, `--include-empty`) | `clikit/providerkit` | [providers/selection § Step 2](../guides/providers/selection.md#step-2--keep-one-audience-and-rename-it) | [`examples/providers/selection`](../../examples/providers/selection) |
| Prune stale remote secrets (`--keep`, `--yes`, `--dry-run`) | `clikit/providerkit` | [providers/github § Step 5](../guides/providers/github.md#step-5--prune-what-your-file-no-longer-defines) | [`examples/providers/github`](../../examples/providers/github) |

## clikit

| Feature | Package | Guide | Example |
|---|---|---|---|
| Output envelope and error codes (`outfmt.Printer`, `ErrCode`) | `clikit/outfmt` | [providers/output-and-reports § Step 1](../guides/providers/output-and-reports.md#step-1--print-through-a-printer) | [`examples/providers/output-and-reports`](../../examples/providers/output-and-reports) |
| HTML reports (`report.MatrixHTML`, `DiffHTML`, `MaskGlyph`) | `clikit/report` | [providers/output-and-reports § Step 5](../guides/providers/output-and-reports.md#step-5--render-an-html-report) | [`examples/providers/output-and-reports`](../../examples/providers/output-and-reports) |
| Line diff (`textdiff.Lines`) | `clikit/textdiff` | [providers/output-and-reports § Step 4](../guides/providers/output-and-reports.md#step-4--preview-an-edit) | [`examples/providers/output-and-reports`](../../examples/providers/output-and-reports) |
| Key selection grammar and placeholders (`selection.Options`, `Selection`, `PlaceholderMatcher`) | `clikit/selection` | [providers/selection § Step 6](../guides/providers/selection.md#step-6--use-your-own-placeholder-convention) | [`examples/providers/selection`](../../examples/providers/selection) |
| Build a push provider (`providerkit.Plugin`, `NewPushCmd`, `NewListCmd`, `NewPruneCmd`, `Confirm`, `Runner`) | `clikit/providerkit` | [providers/write-a-provider § Step 2](../guides/providers/write-a-provider.md#step-2--return-a-command-tree-from-command) | [`examples/providers/fake-provider`](../../examples/providers/fake-provider) |

## textdatactl

| Feature | Package | Guide | Example |
|---|---|---|---|
| `textdatactl dotenv …` (every dotenvctl verb) | `cmd/textdatactl` | [getting-started/textdatactl § Step 2](../guides/getting-started/textdatactl.md#step-2--read-and-edit-a-env-file) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| `textdatactl jsonc …` | `cmd/textdatactl` | [getting-started/textdatactl § Step 3](../guides/getting-started/textdatactl.md#step-3--read-and-edit-a-jsonc-file) | [`examples/getting-started/embed-cli`](../../examples/getting-started/embed-cli) |
| `textdatactl version` (`--json`) | `cmd/textdatactl` | [getting-started/textdatactl § Step 1](../guides/getting-started/textdatactl.md#step-1--check-what-you-are-running) | [`examples/cli/verb-tour`](../../examples/cli/verb-tour) |
| Embed the umbrella (`ctl.Execute`, `ctl.Formats`) | `cmd/textdatactl/ctl` | [getting-started/textdatactl § Step 4](../guides/getting-started/textdatactl.md#step-4--run-textdatactl-inside-your-own-program) | [`examples/getting-started/embed-cli`](../../examples/getting-started/embed-cli) |

## Value plugins

| Feature | Package | Guide | Example |
|---|---|---|---|
| gopass: `${gopass:path}` resolver (`gopassplugin.New`, `WithRunner`, `WithBinary`, `WithTimeout`, `WithCache`) | `plugins/gopass` | [plugins/gopass § Step 1](../guides/plugins/gopass.md#step-1--build-the-plugin) | [`examples/plugins/gopass-fake-runner`](../../examples/plugins/gopass-fake-runner) |
| gopass: `${NAME}` lookups (`WithPrefix`, `WithMapping`) | `plugins/gopass` | [plugins/gopass § Step 1](../guides/plugins/gopass.md#step-1--build-the-plugin) | [`examples/plugins/gopass-fake-runner`](../../examples/plugins/gopass-fake-runner) |
| file: `${file:path}` resolver with confinement (`fileplugin.New`, `WithRoot`, `Unconfined`, `WithMaxSize`, `WithTrimNewline`) | `plugins/file` | [plugins/file § Step 1](../guides/plugins/file.md#step-1--install-it-confined-the-default) | [`examples/plugins/file-confined`](../../examples/plugins/file-confined) |
| extends: disk includer (`extends.New`, `WithRoot`, `AllowAbsolute`, `WithMaxSize`) | `plugins/extends` | [plugins/extends § Step 4](../guides/plugins/extends.md#step-4--read-from-disk-confined) | [`examples/plugins/extends-fs`](../../examples/plugins/extends-fs) |
| extends: fs.FS includer (`extends.NewFS`) | `plugins/extends` | [plugins/extends § Step 1](../guides/plugins/extends.md#step-1--open-a-document-with-the-fsfs-includer) | [`examples/plugins/extends-fs`](../../examples/plugins/extends-fs) |
| vault: KV v1/v2 `${vault:path?version=N#field}` (`vault.New`, `WithAddress`, `WithToken`, `WithNamespace`, `FromEnv`) | `plugins/vault` | [plugins/vault § Step 1](../guides/plugins/vault.md#step-1--know-the-reference-grammar) | [`examples/plugins/vault-httptest`](../../examples/plugins/vault-httptest) |
| vault: `${NAME}` lookups (`WithLookup`, `WithLookupPrefix`) | `plugins/vault` | [plugins/vault § Step 3](../guides/plugins/vault.md#step-3--build-the-plugin) | [`examples/plugins/vault-httptest`](../../examples/plugins/vault-httptest) |
| aws: Secrets Manager and SSM `${aws:sm/…}`, `${aws:ssm/…}` (`aws.New`, `WithRegion`, `WithEndpoint`) | `plugins/aws` | [plugins/aws § Step 1](../guides/plugins/aws.md#step-1--know-the-reference-grammar) | [`examples/plugins/aws-httptest`](../../examples/plugins/aws-httptest) |
| aws: credentials (`WithCredentials`, `FromEnv`, `FromSharedCredentials`, `WithCredentialsSource`) | `plugins/aws` | [plugins/aws § Step 5](../guides/plugins/aws.md#step-5--supply-credentials-another-way) | [`examples/plugins/aws-httptest`](../../examples/plugins/aws-httptest) |
| aws: `${NAME}` lookups (`WithLookup`, `WithLookupPrefix`) | `plugins/aws` | [plugins/aws § Step 2](../guides/plugins/aws.md#step-2--build-the-plugin) | [`examples/plugins/aws-httptest`](../../examples/plugins/aws-httptest) |
