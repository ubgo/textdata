# jsonc guides

Step-by-step guides for `github.com/ubgo/textdata/jsonc`: a comment- and byte-preserving parser and editor for JSONC, and for plain JSON with `jsonc.Strict()`. Each guide shows complete snippets from a runnable program under [`examples/jsonc/`](../../../examples/jsonc/), its real output, and its pitfalls. The module's [README](../../../jsonc/README.md) is the one-page overview; the tools (format, diff, generate, schema) are in [jsonc tools](../jsonc-tools/README.md).

Read them in this order the first time:

| Guide | What you learn | Example |
|---|---|---|
| [parse-and-strict.md](parse-and-strict.md) | `Parse`, `ParseReader`, `Open`, `Strict`, positioned syntax errors, `Render` | [`read-config`](../../../examples/jsonc/read-config/) |
| [read-and-paths.md](read-and-paths.md) | `Get*`, `Has`, `Keys`, `Len`, `Kind`, `Comment`, paths and JSON Pointer, `Pos`/`KeyPos`, `JSON`, `Decode`, `Read` | [`read-config`](../../../examples/jsonc/read-config/) |
| [edit.md](edit.md) | `Set`, `SetAfter`, `SetBefore`, `Delete`, `Append`, `Insert`, `Remove`, `Raw`, `SetComment`, `RemoveComment` | [`edit-config`](../../../examples/jsonc/edit-config/), [`arrays-comments`](../../../examples/jsonc/arrays-comments/) |
| [disable-restore.md](disable-restore.md) | comment a member out with `Disable` and bring it back with `Restore` | [`edit-config`](../../../examples/jsonc/edit-config/) |
| [merge-clone.md](merge-clone.md) | layered config with `Clone` and `Merge` | [`merge-layers`](../../../examples/jsonc/merge-layers/) |
| [extends.md](extends.md) | `"$extends"`, includers, `Extended`, extends errors | [`extends`](../../../examples/jsonc/extends/) |
| [expansion-and-plugins.md](expansion-and-plugins.md) | `WithExpand`, `WithLookup`, `Expanded`, `GetExpanded`, value plugins, `$(cmd)` | [`expand`](../../../examples/jsonc/expand/) |
| [lint-validate.md](lint-validate.md) | `Lint`, lint rules, `Validator` plugins, `Validate` | [`lint`](../../../examples/jsonc/lint/) |
| [save.md](save.md) | `Save`, `SaveAs`, `SetPath`, modes, symlinks, SaveGuards | [`edit-config`](../../../examples/jsonc/edit-config/) |

Start from your goal:

- Read a config into a struct: [read-and-paths.md › Step 7](read-and-paths.md#step-7--decode-into-a-struct).
- Change one value in a file a person maintains: [edit.md](edit.md), then [save.md](save.md).
- A base config plus per-environment overrides: [extends.md](extends.md) or [merge-clone.md](merge-clone.md).
- Secrets from a store: [expansion-and-plugins.md](expansion-and-plugins.md) and [Which value plugin](../../choosing/value-plugins.md).
