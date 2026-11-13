# Base packages

The shared base module `github.com/ubgo/textdata` (stdlib only) holds what every format uses. You need these guides when you write a plugin, build a new format, or want one of the pieces on its own (an atomic file write, a `${VAR}` engine, `file:line:col` errors). Using `dotenv` or `jsonc` does not require reading them.

| Guide | Package | What you learn | Example |
|---|---|---|---|
| [plugin.md](plugin.md) | `plugin` | the plugin abilities (Lookuper, Resolver, CommandRunner, ValueTransformer, SaveGuard, Validator, Includer), capability detection, the registry, named errors, the `Document` view | [`registry`](../../../examples/base/registry/) |
| [expand.md](expand.md) | `expand` | the `${VAR}` engine, the six operators, `${scheme:ref}`, `$(cmd)`, the cycle guard, `RequiredError` | [`expand-engine`](../../../examples/base/expand-engine/) |
| [atomicfile.md](atomicfile.md) | `atomicfile` | temp file, fsync, rename; keep the mode; write through a symlink | [`atomic-write`](../../../examples/base/atomic-write/) |
| [pos-and-path.md](pos-and-path.md) | `pos`, `path` | dotted paths and JSON Pointer, byte offset to line and column, `file:line:col` error lists | [`paths`](../../../examples/base/paths/) |
| [texttest.md](texttest.md) | `texttest` | the round-trip and untouched-bytes property harness for a new format, with fuzzing | [`property-harness`](../../../examples/base/property-harness/) |

Related: [write a value plugin](../plugins/write-a-value-plugin.md), [learning path](../../README.md).
