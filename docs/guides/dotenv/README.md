# dotenv guides

Step-by-step guides for the `github.com/ubgo/textdata/dotenv` library: a comment- and byte-preserving `.env` parser and editor. Each guide shows complete snippets from a runnable program under [`examples/dotenv/`](../../../examples/dotenv/), its real output, and the traps found in the code and tests. The module's [README](../../../dotenv/README.md) is the one-page overview; the CLI is documented in [`dotenv/cli/README.md`](../../../dotenv/cli/README.md).

Read them in this order the first time:

| Guide | What you learn | Example |
|---|---|---|
| [reading.md](reading.md) | `Read`, `Open`, `Parse`, `ParseReader`; `Get`, `Has`, `Count`, `Keys`, `Pairs`, `Map`; `Existed`, `Mode`, `Path` | [`reading`](../../../examples/dotenv/reading/) |
| [syntax.md](syntax.md) | quoting, multi-line values, comments, `:` delimiter, key names, duplicates, `encrypted:` values, escape modes | [`syntax`](../../../examples/dotenv/syntax/) |
| [entries.md](entries.md) | the `Entry` type, the six `Kind`s, walking `Entries()` | [`entries`](../../../examples/dotenv/entries/) |
| [expansion.md](expansion.md) | `GetExpanded`, `ExpandedMap`, the `${…}` operators, `$$`, self-references, `RequiredError`, `$(command)` | [`expansion`](../../../examples/dotenv/expansion/) |
| [editing.md](editing.md) | `Set`, `SetAfter`, `SetBefore`, `Unset`, `Restore`, `Disabled`, `Inherited`, constructors, `Append`, `Insert*`, `Clone` | [`editing`](../../../examples/dotenv/editing/), [`clone-env`](../../../examples/dotenv/clone-env/) |
| [saving.md](saving.md) | `Save`, `SaveAs`, modes, line endings, symlinks | [`saving`](../../../examples/dotenv/saving/) |
| [plugins.md](plugins.md) | the seven hooks, function options, `${scheme:ref}` resolvers, writing a plugin type, ordering, the firewall | [`plugin-hooks`](../../../examples/dotenv/plugin-hooks/), [`write-a-plugin`](../../../examples/dotenv/write-a-plugin/) |
| [typed-config.md](typed-config.md) | binding the expanded strings to a typed struct | [`typed-config`](../../../examples/dotenv/typed-config/) |

Start from your goal:

- A program only needs its configuration: [reading.md › Step 1](reading.md#step-1--read-the-values-in-one-call), then [typed-config.md](typed-config.md).
- A value does not come back the way it was written: [syntax.md](syntax.md), then [expansion.md](expansion.md).
- A tool must change a file a person edits: [editing.md](editing.md) and [saving.md](saving.md).
- Secrets come from a store or arrive encrypted: [plugins.md](plugins.md), and the ready-made value plugins under [`plugins/`](../../../plugins/).
- One environment's file is derived from another: [editing.md › Step 7](editing.md#step-7--derive-another-environment-with-clone).
