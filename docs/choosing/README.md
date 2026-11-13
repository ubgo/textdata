# Choosing

Decision guides for the questions people ask before they write code: which format and modules, which method, which secret backend, which options for their kind of app, and whether to use the library, the CLI or both. Each page has a decision table, "if X then Y" rules, the pitfalls that bite, and a guide and a runnable example for every option.

| Question | Page |
|---|---|
| `.env` or JSONC or strict JSON? Which `go get` lines, so I pull in nothing I do not use? | [Which format and which modules](format-and-modules.md) |
| `Read`, `Open` or `Parse`? `Get`, `Decode` or `GetExpanded`? `Set` or `SetAfter`? Comment out or delete? `Save`, `SaveAs` or `Render`? | [Which method for the main workflows](methods.md) |
| Lookuper or Resolver? gopass, file, Vault, AWS, `$extends` or my own plugin? | [Which value plugin or secret backend](value-plugins.md) |
| Command substitution on or off? File confinement, SaveGuards, schema validation? What does a service, a CLI tool, a CI job or a deploy step need? | [Options for your kind of app](app-setups.md) |
| Library, `textdatactl`, `dotenvctl`, embedding the CLI, a push provider or an exec plugin? | [Library, CLI or embedding the CLI](library-cli-or-embed.md) |

## Related

- [Docs index and learning path](../README.md)
- [Feature index](../reference/features.md): every feature with its guide step and example
