# clikit

Shared CLI plumbing for the textdata command-line tools: one output envelope, one set of error codes, one HTML report renderer, one dry-run diff, one selection grammar and one provider-plugin seam, so every format's CLI and every provider behaves the same.

Module `github.com/ubgo/textdata/clikit`. It imports no file format: it works on flat key/value selections and plain report structs, and each format's CLI module resolves its own files into them. Its only third-party dependency is [cobra](https://github.com/spf13/cobra) (for `providerkit`).

```sh
go get github.com/ubgo/textdata/clikit
```

Status: released; the current tag is `clikit/v0.1.4`.

**Who uses it:** [`dotenv/cli`](../dotenv/cli/README.md) (`dotenvctl`, and `textdatactl dotenv` through it), [`jsonc/cli`](../jsonc/cli) (`textdatactl jsonc`), and the provider plugins [`dotenv/cli/providers/github`](../dotenv/cli/providers/github/README.md) and [`dotenv/cli/providers/vercel`](../dotenv/cli/providers/vercel/README.md). These packages moved here from `github.com/ubgo/dotenv/cli` (`outfmt`, `report`, `textdiff`, `providerkit`, and the selection grammar that was part of `envkit`); the code is the same, and the default program name is now `textdatactl` (`dotenvctl` sets its own name, so the standalone binary prints what it always did).

| Package | What |
|---|---|
| [`outfmt`](#outfmt) | the single output seam: human lines or the `{ok,data|error}` JSON envelope, and the closed set of error codes |
| [`report`](#report) | self-contained HTML pages for a drift matrix and a diff, values masked by default |
| [`textdiff`](#textdiff) | the minimal line diff a `--dry-run` preview prints |
| [`selection`](#selection) | the selection grammar (`--prefix`, `--exclude-prefix`, `--include`, `--exclude`, `--strip-prefix`, placeholder and empty guards) as plain types |
| [`providerkit`](#providerkit) | the provider-plugin seam: `Plugin.Command(Deps)`, generated `push` / `list` / `prune` verbs, the confirm gate, the injectable `Runner` |

## outfmt

Every byte a verb emits goes through a `Printer`, chosen once per run: human mode prints lines (`Human`, `Humanf`) and `OK` is silent; JSON mode prints exactly one envelope per invocation and `Human` is silent, so `--json` output stays parseable. The envelope is stable API for scripts:

```
{"ok":true,"data":<verb-specific>}
{"ok":false,"error":{"code":"<code>","message":"..."}}
{"ok":false,"data":<partial record>,"error":{...}}   (FailWith: failed part-way)
```

In human mode success goes to `Out` and failures to `Err` (`<program>: <message>`; `os.Stderr` when `Err` is nil), so `v=$(dotenvctl get KEY)` captures a value or nothing, never an error message. `Program` sets the prefix; empty means `DefaultProgram` (`"textdatactl"`).

<!-- snippet: clikit/outfmt/example_test.go#api_outfmt_Printer -->
```go
// ExamplePrinter prints the same verb in both modes. Human mode prints lines
// and OK is silent; JSON mode prints exactly one envelope per call and Human
// is silent, so --json output stays parseable.
func ExamplePrinter() {
	for _, jsonMode := range []bool{false, true} {
		p := &outfmt.Printer{Out: os.Stdout, JSON: jsonMode}
		p.Human("DB_URL")
		p.Humanf("%d keys", 1)
		if err := p.OK(apiOutfmtPayload{Keys: []string{"DB_URL"}}); err != nil {
			fmt.Println("write:", err)
		}
	}
	// Output:
	// DB_URL
	// 1 keys
	// {"ok":true,"data":{"keys":["DB_URL"]}}
}
```

### Error codes

`ErrCode` is a closed set; scripts branch on these strings, so adding one is additive API and renaming one is a breaking change.

| Code | Meaning |
|---|---|
| `not_found` | the named key (or disabled or inherited entry) does not exist in the file |
| `io` | the file could not be read or written |
| `required` | a `${VAR:?msg}` / `${VAR?msg}` reference failed during `--expand` |
| `usage` | arguments did not parse (bad `KEY=VALUE`, missing operand) |
| `exec` | the child process of `run` (or a backend CLI) could not be started or failed; a child that starts and exits non-zero passes its exit code through instead |
| `diff` | reserved for `diff` when the files differ; `diff` emits `ok:true` with the differences, and the code names the cause of exit 1 |

Exit codes are the same contract in every textdata CLI: `0` ok, `1` the operation failed, `2` usage.

## report

`MatrixHTML(Matrix)` and `DiffHTML(Diff)` render one self-contained HTML page each: inline CSS, no external requests, no JavaScript, light and dark through `prefers-color-scheme`. The security property is structural: with `Revealed` false the caller never fills the value fields and the templates render `MaskGlyph` (`••••••`) for every value cell, so a masked page contains no secret bytes and is safe to attach to a ticket. `Revealed` true shows the values and stamps a "contains secrets" banner.

<!-- snippet: clikit/report/example_test.go#api_report_DiffHTML -->
```go
// ExampleDiffHTML renders a diff page. With Revealed false every value cell
// is report.MaskGlyph, even if a value was passed in; with Revealed true the
// values appear and the page carries a "contains secrets" banner.
func ExampleDiffHTML() {
	d := report.Diff{
		FileA:   ".env.stag",
		FileB:   ".env.prod",
		Changed: []report.DiffChange{{Key: "DB_URL", From: "postgres://stag", To: apiReportSecret}},
	}
	for _, reveal := range []bool{false, true} {
		d.Revealed = reveal
		page, err := report.DiffHTML(d)
		if err != nil {
			fmt.Println("render:", err)
			return
		}
		fmt.Printf("revealed=%t secret=%t glyph=%t banner=%t\n", reveal,
			bytes.Contains(page, []byte(apiReportSecret)),
			bytes.Contains(page, []byte(report.MaskGlyph)),
			bytes.Contains(page, []byte("CONTAINS SECRETS")))
	}
	// Output:
	// revealed=false secret=false glyph=true banner=false
	// revealed=true secret=true glyph=false banner=true
}
```

## textdiff

`Lines(before, after)` trims the common prefix and suffix of two renderings and returns the changed region as `-old` / `+new` lines (nil when identical). Textdata edits are local by construction, so this is all a preview needs; it is not a general diff.

<!-- snippet: clikit/textdiff/example_test.go#api_textdiff_Lines -->
```go
// ExampleLines previews an edit as "-old"/"+new" lines for the one region
// that changed. Identical input returns nil, which callers read as "no-op".
// It trims the common prefix and suffix; it is not a general diff.
func ExampleLines() {
	before := "# db\nDB_HOST=localhost\nDB_PORT=5432\n"
	after := "# db\nDB_HOST=db.prod\nDB_PORT=5432\n"

	for _, l := range textdiff.Lines(before, after) {
		fmt.Println(l)
	}
	fmt.Println(textdiff.Lines(before, before) == nil)

	// Two separate edits come back as one block spanning both.
	fmt.Println(textdiff.Lines("A=1\nB=2\nC=3", "A=9\nB=2\nC=8"))
	// Output:
	// -DB_HOST=localhost
	// +DB_HOST=db.prod
	// true
	// [-A=1 -B=2 -C=3 +A=9 +B=2 +C=8]
}
```

## selection

The selection grammar every textdata CLI shares, with no file format in it. `Options` says which keys (`Keys`, `Prefix`, `ExcludePrefix`, `IncludeKeys`, `ExcludeKeys`), under which names (`StripPrefix`), whether references are expanded (`Expand`), and which guards apply (`IncludePlaceholders`, `IncludeEmpty`, `PlaceholderPattern`, default `DefaultPlaceholderPattern` = `^__[A-Z0-9_]+__$`). A format's operations package (for dotenv, `envkit.SelectFile`) applies it to a file and returns a `Selection{Pairs, Skipped}`; `Skipped` carries a `SkipReason` (`placeholder`, `empty`), so a caller can tell "nothing matched" from "everything was a placeholder".

Resolution order, pinned by the format's tests: explicit `Keys` as given, otherwise all active keys → `Prefix` keeps → `ExcludePrefix` drops → `IncludeKeys` force-add → `ExcludeKeys` drop → `StripPrefix` renames → the guards classify what is left. A key the user named that does not exist is an error, never a silent skip. The resolution itself still lives in each format's operations package; it moves here as `Apply` when a second format needs it.

## providerkit

The seam between a CLI's core and its provider plugins. The mandatory contract is one method, `Plugin.Command(Deps) *cobra.Command`: a plugin is a cobra namespace, mountable under any root (no `init()`, no package state). `Deps` carries everything a plugin may use from the host: the `Printer`, a `Source` that returns the resolved selection (plugins never parse files), the `Runner` that executes backend CLIs (`ExecRunner{}` in production, a fake in tests), and the `Interactive` / `Ask` confirm hooks.

The rest is opt-in machinery that keeps providers uniform: implement `SecretWriter` (`Set`), `SecretLister` (`Names`) and `SecretDeleter` (`Delete`) over your backend, and `NewPushCmd`, `NewListCmd` and `NewPruneCmd` generate the standard verbs with the shared selection flags, the placeholder and empty guards, `--dry-run`, `--yes` and the confirm gate (`Confirm`: non-interactive runs refuse to write without `--yes`). `VerbConfig.Gate` adds a banner (target, account) before writes and `VerbConfig.Meta` adds resolution context to the `--json` payload. Per-key results use the closed `Action` set (`pushed`, `would-push`, `skipped-placeholder`, `skipped-empty`, `deleted`, `would-delete`, `kept-skipped`, `kept-keep-flag`); `prune` spares guard-skipped keys and `--keep` names, because delete has no undo. `Fail` prints the standard failure and returns a `*CmdError` carrying the exit code.

<!-- snippet: clikit/providerkit/example_test.go#api_providerkit_store -->
```go
// apiKitStore is an in-memory secret store implementing all three capability
// interfaces — the shape a real backend adapter takes, minus the network.
type apiKitStore struct {
	values map[string]string
}

// Set implements providerkit.SecretWriter.
func (s *apiKitStore) Set(_ context.Context, name, value string) error {
	s.values[name] = value
	return nil
}

// Names implements providerkit.SecretLister: names only, never values.
func (s *apiKitStore) Names(context.Context) ([]string, error) {
	names := make([]string, 0, len(s.values))
	for n := range s.values {
		names = append(names, n)
	}
	slices.Sort(names)
	return names, nil
}

// Delete implements providerkit.SecretDeleter.
func (s *apiKitStore) Delete(_ context.Context, name string) error {
	delete(s.values, name)
	return nil
}
```

<!-- snippet: clikit/providerkit/example_test.go#api_providerkit_NewPushCmd -->
```go
// ExampleNewPushCmd generates a standard `push` verb over your store. The kit
// owns selection, the placeholder/empty guards, the confirm gate and the
// output; your store only writes. Non-interactive runs need --yes.
func ExampleNewPushCmd() {
	store := &apiKitStore{values: map[string]string{}}
	src := &selection.Selection{
		Pairs:   []selection.Pair{{Key: "DB_URL", Value: "postgres://db/app"}},
		Skipped: []selection.Skip{{Name: "API_KEY", Reason: selection.SkipPlaceholder}},
	}
	push := func() *cobra.Command {
		return providerkit.NewPushCmd(apiKitDeps(src), store, providerkit.VerbConfig{})
	}

	fmt.Println("exit:", apiKitRun(push()))
	fmt.Println("exit:", apiKitRun(push(), "--dry-run"))
	fmt.Println("exit:", apiKitRun(push(), "--yes"))
	fmt.Println("stored:", store.values)
	// Output:
	// textdatactl: refusing to write without --yes in non-interactive mode
	// exit: 2
	// API_KEY: skipped-placeholder
	// DB_URL: would-push
	// exit: 0
	// API_KEY: skipped-placeholder
	// DB_URL: pushed
	// exit: 0
	// stored: map[DB_URL:postgres://db/app]
}
```

[`ExampleNewPruneCmd`](providerkit/example_test.go), [`ExampleNewListCmd`](providerkit/example_test.go) and [`ExampleVerbConfig`](providerkit/example_test.go) cover the other verbs; [`dotenv/cli/providers/github`](../dotenv/cli/providers/github/README.md) and [`dotenv/cli/providers/vercel`](../dotenv/cli/providers/vercel/README.md) are complete providers built on it.

## Testing

```sh
go test -race ./...   # from this directory
task check            # from the repository root: the full gate
```

Every Go block on this page is copied from a runnable `Example` (compiled and output-checked by `go test`); the `snippet:` marker above each block names the file and region.
