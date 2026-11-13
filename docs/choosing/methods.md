# Which method for the main workflows

Both formats offer a one-call read, a document you can edit, raw and expanded getters, several ways to add or remove a key, and three ways to write. They look alike but differ in what they promise about missing files, expansion and the bytes on disk. Each table below says which call to use, and the bullets under it say what changes if you pick another.

## Read: `Read`, `Open` or `Parse`

| Call | dotenv | jsonc | Missing file | Use it when |
|---|---|---|---|---|
| `Read(path)` | `map[string]string`, values **expanded** | `map[string]any`, `$extends` merged when an includer is installed, expanded only with `jsonc.WithExpand()` | dotenv: error `file does not exist`; jsonc: empty map, no error | you only consume values at startup |
| `Open(path)` | `*dotenv.File` | `*jsonc.Doc` | empty document, `Existed()` is false, `Save` creates the file | you will read and maybe edit and save |
| `Parse(src)` / `ParseReader(r)` | from a string / reader | from bytes / reader | - | the content is not a file (embedded, stdin, a test), or you set the path later |

- If you only need values, call `Read`. In dotenv that is the expanded view; in jsonc pass `jsonc.WithExpand()` to get the same.
- If you will edit, call `Open`, never `Read` then write the map back: the map has no comments, and in dotenv its values are already expanded, so saving them bakes `${DOMAIN}` into a literal.
- If the file must exist, check it: `dotenv.Read` fails for you, but `dotenv.Open`, `jsonc.Open` and `jsonc.Read` all treat a missing file as empty. Test `Existed()` after `Open`.
- If you parsed from memory and want to save, give the document a path: `f.Path = "…"` in dotenv, `doc.SetPath("…")` in jsonc, or use `SaveAs`.

Guides: [dotenv reading](../guides/dotenv/reading.md), [jsonc read and paths](../guides/jsonc/read-and-paths.md#step-8--read-a-file-in-one-call). Examples: [`examples/dotenv/reading`](../../examples/dotenv/reading), [`examples/jsonc/read-config`](../../examples/jsonc/read-config).

## Get a value: raw, typed, decoded or expanded

| You want | dotenv | jsonc |
|---|---|---|
| The value as written, `${VAR}` unresolved | `f.Get(key)` → `(string, bool)`; `f.Map()` | `doc.Get(path)` → `any`; `GetString`, `GetInt`, `GetFloat`, `GetBool` |
| The resolved value | `f.GetExpanded(key)` → `(string, bool, error)`; `f.ExpandedMap()` | `doc.GetExpanded(path)` → `(any, error)`; or `doc.Expanded()` for a whole resolved copy |
| A Go struct | `ExpandedMap()` then `strconv` or a struct-tag binder ([typed config](../guides/dotenv/typed-config.md)) | `doc.Decode(&cfg)` (raw) or `expanded.Decode(&cfg)` |
| Plain JSON bytes for another tool | - | `doc.JSON()` |

- If the value may hold `${…}`, read it with the expanded getter; `Get` and `GetString` return the reference text.
- If a missing variable must stop the program, write `${NAME:?message}` in the file and handle the `RequiredError` the expanded getter returns.
- If you need typed jsonc values, use `GetInt`/`GetBool`/…: they return `jsonc.ErrType` on a mismatch, while `Get` hands you a `float64` for every number.

Guides: [dotenv expansion](../guides/dotenv/expansion.md#step-1--get-the-raw-or-the-expanded-value), [jsonc typed values](../guides/jsonc/read-and-paths.md#step-1--read-typed-values), [jsonc decode](../guides/jsonc/read-and-paths.md#step-7--decode-into-a-struct), [jsonc expansion](../guides/jsonc/expansion-and-plugins.md#step-1--expand-through-a-lookup). Examples: [`examples/dotenv/expansion`](../../examples/dotenv/expansion), [`examples/dotenv/typed-config`](../../examples/dotenv/typed-config), [`examples/jsonc/expand`](../../examples/jsonc/expand).

## Add or change a key: `Set` or `SetAfter` / `SetBefore`

| Call | Existing key | New key goes | Anchor missing |
|---|---|---|---|
| `Set` | updated in place, everything else untouched | dotenv: end of file; jsonc: end of its object, parents created | - |
| `SetAfter(anchor, …)` / `SetBefore` | updated in place, **not moved** | next to the anchor | dotenv: `dotenv.ErrAnchorNotFound`; jsonc: an error wrapping `jsonc.ErrNotFound` |
| dotenv `Append`, `InsertAfter`, `InsertBefore` | never looked up: a duplicate is added | where you say | - |

- If you only care about the value, use `Set`; it returns whether the key was created.
- If a new key should sit next to its group (`DB_USER` after `DB_HOST`), use `SetAfter`; a key that already exists stays where it is, so running the same code twice does not shuffle the file.
- If you are generating a file from scratch, build it with `NewPair`, `NewComment`, `NewBlank` and `Append` in dotenv, or `generate.New` in jsonc ([generate](../guides/jsonc-tools/generate.md)).
- If the value is a JSON fragment (an object or array as text), use `jsonc.Raw(…)` so it is inserted verbatim instead of quoted.

Guides: [dotenv editing](../guides/dotenv/editing.md#step-2--place-a-new-key-next-to-a-related-one), [jsonc edit](../guides/jsonc/edit.md#step-2--place-a-new-key-next-to-a-sibling). Examples: [`examples/dotenv/editing`](../../examples/dotenv/editing), [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config).

## Remove a key: comment out or delete

| You want | dotenv | jsonc |
|---|---|---|
| Turn it off, keep it visible and reversible | `f.Unset(key, false)` → `# KEY=value` | `doc.Disable(path)` → `// "key": value` |
| Turn it back on | `f.Restore(key)` | `doc.Restore(path)` |
| Remove it for good | `f.Unset(key, true)` | `doc.Delete(path)`; array elements with `Remove(path, i)` |

- If a teammate might want the setting back, comment it out; `Restore` removes exactly the marker that was added.
- If the key is a secret that must leave the file, delete it: a commented-out line still holds the value.
- If the jsonc member shares a line with other members (`{"a": 1, "b": 2}`), `Disable` fails with "value does not occupy its own lines"; reformat first ([format](../guides/jsonc-tools/format.md)) or `Delete`.

Guides: [dotenv on and off](../guides/dotenv/editing.md#step-3--turn-a-setting-off-and-back-on), [jsonc disable and restore](../guides/jsonc/disable-restore.md), [jsonc delete](../guides/jsonc/edit.md#step-3--delete-a-member). Examples: [`examples/dotenv/editing`](../../examples/dotenv/editing), [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config).

## Write: `Save`, `SaveAs` or `Render`

| Call | Writes to | Mode | Runs SaveGuards |
|---|---|---|---|
| `Save()` | the path from `Open` (or `f.Path` / `SetPath`) | keeps the existing file's mode; a new dotenv file is 0600 | yes |
| `SaveAs(path)` | another path; the document's own path is unchanged | an existing destination keeps its mode, a new one gets the document's | yes |
| `Render()` / `Bytes()` | nowhere: returns the bytes | - | no |

- If you edit a file in place, `Save`. It writes a temp file, fsyncs, renames, and writes through a symlink to its target.
- If you derive several files from one (staging to prod), `SaveAs` each variant; the source document keeps its path ([clone between environments](../guides/dotenv/editing.md#step-7--derive-another-environment-with-clone)).
- If you need the bytes for a diff, a test, an HTTP response or your own writer, `Render`; nothing is checked and nothing is written.

Guides: [dotenv saving](../guides/dotenv/saving.md), [jsonc save](../guides/jsonc/save.md). Examples: [`examples/dotenv/saving`](../../examples/dotenv/saving), [`examples/dotenv/clone-env`](../../examples/dotenv/clone-env), [`examples/jsonc/edit-config`](../../examples/jsonc/edit-config).

## Pitfalls

- **Expanded values are for reading.** Writing an expanded value back (`f.Set(k, expanded)`) replaces `${HOST}` with today's host; edit the raw value.
- **`jsonc.Open` on a misspelt path is not an error.** You get an empty document; check `Existed()` when the file must be there.
- **A failing `$(cmd)` is an error** in both formats: a non-zero exit or a runner error returns an `*expand.CommandError` naming the command, never an empty value (see [value plugins](value-plugins.md)).
- **`Render` does not run SaveGuards.** A guard that blocks a private key from being saved does not stop you writing `Render()` output yourself.

## Related

- [Which format and which modules](format-and-modules.md), [Options for your kind of app](app-setups.md)
- [Getting started: edit and save](../guides/getting-started/edit-and-save.md), [`examples/getting-started/edit-save`](../../examples/getting-started/edit-save)
