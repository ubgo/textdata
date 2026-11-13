# Output, error codes and reports

Three small `clikit` packages make every textdata CLI and provider print the same way. `outfmt` is the single output seam: human lines, or exactly one `{ok,data|error}` JSON envelope per run, with a closed set of error codes scripts can branch on. `textdiff` gives the `-old`/`+new` lines a `--dry-run` preview prints. `report` renders self-contained HTML pages for a drift matrix or a diff, with values masked unless the caller reveals them. Use them when you write your own verb, provider or host and want `--json`, exit codes, previews and reports to behave like `dotenvctl` and `textdatactl`.

## Before you start

```sh
go get github.com/ubgo/textdata/clikit@latest
```

All three are stdlib-only packages inside the `clikit` module. The runnable program for this page is [`examples/providers/output-and-reports`](../../../examples/providers/output-and-reports).

### The contract scripts rely on

[The envelope](ds:block?id=envelope-44255rwu), one JSON line per invocation:

```text
{"ok":true,"data":<verb-specific>}
{"ok":false,"error":{"code":"<code>","message":"..."}}
{"ok":false,"data":<partial record>,"error":{...}}      (failed part-way)
```

[The error codes](ds:block?id=errcodes-tjhb87tm) (`outfmt.ErrCode`), a closed set: adding one is additive API, renaming one is a breaking change.

| Code | Constant | Meaning |
|---|---|---|
| `not_found` | `CodeNotFound` | the named key (or disabled or inherited entry) does not exist in the file |
| `io` | `CodeIO` | the file could not be read or written |
| `required` | `CodeRequired` | a `${VAR:?msg}` / `${VAR?msg}` reference failed during `--expand` |
| `usage` | `CodeUsage` | arguments did not parse, or a write was refused without `--yes` |
| `exec` | `CodeExec` | a child process (`run`, or a backend CLI such as `gh`) could not be started or failed; a `run` child that starts and exits non-zero passes its own exit code through instead |
| `diff` | `CodeDiff` | reserved for `diff` when the files differ; `diff` itself emits `ok:true` with the differences |

Exit codes are the same in every textdata CLI: `0` ok, `1` the operation failed, `2` usage. `providerkit.Fail` maps `CodeUsage` to 2 and every other code to 1.

## Step 1 — Print through a Printer

Every byte a verb emits goes through one `outfmt.Printer`, chosen once per run. In human mode `Human`/`Humanf` print lines and `OK` is silent; in JSON mode `OK` prints the envelope and `Human` is silent. So a verb calls both, unconditionally, and never checks the mode. Pass `OK` a named struct, never a bare map, so the wire shape is visible where it is produced.

<!-- ds:block id=getpayload-98mr2y9u -->
**getPayload** · [`examples/providers/output-and-reports/main.go:48-51`](../../../examples/providers/output-and-reports/main.go#L48-L51)

```go
type getPayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
```
<!-- /ds:block hash=b8ea90 -->

<!-- ds:block id=printbothmodes-9ees84ht -->
**printBothModes** · [`examples/providers/output-and-reports/main.go:92-101`](../../../examples/providers/output-and-reports/main.go#L92-L101)

```go
func printBothModes(w io.Writer) error {
	for _, jsonMode := range []bool{false, true} {
		p := &outfmt.Printer{Out: w, Err: w, Program: programName, JSON: jsonMode}
		p.Human("postgres://db/app")
		if err := p.OK(getPayload{Key: "DB_URL", Value: "postgres://db/app"}); err != nil {
			return err
		}
	}
	return nil
}
```
<!-- /ds:block hash=f50d5a -->

## Step 2 — Fail on the right stream

`Fail(code, message)` in human mode writes `<program>: <message>` to `Err` (`os.Stderr` when nil), never to `Out`, so `v=$(mytool get KEY)` captures a value or nothing, never an error message. In JSON mode the failure envelope goes to `Out`, because the envelope is the output a script parses. `Program` sets the prefix; empty means `DefaultProgram` (`textdatactl`), and `ProgramName()` returns the effective one for failure lines you print yourself.

<!-- ds:block id=failbothmodes-fyqv3g4y -->
**failBothModes** · [`examples/providers/output-and-reports/main.go:109-119`](../../../examples/providers/output-and-reports/main.go#L109-L119)

```go
func failBothModes(w io.Writer) error {
	var out, errOut bytes.Buffer
	human := &outfmt.Printer{Out: &out, Err: &errOut, Program: programName}
	if err := human.Fail(outfmt.CodeNotFound, `key "API_KEY" not found`); err != nil {
		return err
	}
	fmt.Fprintf(w, "human: out=%q err=%q\n", out.String(), errOut.String())

	machine := &outfmt.Printer{Out: w, JSON: true}
	return machine.Fail(outfmt.CodeNotFound, `key "API_KEY" not found`)
}
```
<!-- /ds:block hash=7cc140 -->

## Step 3 — Report a failure part-way

When an operation did some work before failing, `FailWith(code, message, data)` prints one `ok:false` envelope that carries the partial record as `data`, so a script reads what happened and why it stopped from one object. In human mode it is plain `Fail`; print the human form of the partial record on `Out` yourself, first. The provider verbs (`push`, `prune`) do exactly this.

<!-- ds:block id=failpartway-7bh3zda3 -->
**failPartWay** · [`examples/providers/output-and-reports/main.go:127-131`](../../../examples/providers/output-and-reports/main.go#L127-L131)

```go
func failPartWay(w io.Writer) error {
	p := &outfmt.Printer{Out: w, JSON: true}
	return p.FailWith(outfmt.CodeExec, "push REDIS_URL: backend unavailable",
		pushedPayload{Pushed: []string{"DB_URL"}})
}
```
<!-- /ds:block hash=dcac36 -->

## Step 4 — Preview an edit

`textdiff.Lines(before, after)` returns the changed region of two renderings as `-old`/`+new` lines, or nil when they are identical (callers read nil as a true no-op). It trims the common prefix and suffix, which is all a preview of a textdata edit needs, because edits change one region by construction.

<!-- ds:block id=previewedit-58aanwta -->
**previewEdit** · [`examples/providers/output-and-reports/main.go:138-146`](../../../examples/providers/output-and-reports/main.go#L138-L146)

```go
func previewEdit(w io.Writer) {
	before := "# db\nDB_HOST=localhost\nDB_PORT=5432\n"
	after := "# db\nDB_HOST=db.prod\nDB_PORT=5432\n"
	for _, line := range textdiff.Lines(before, after) {
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, "no-op is nil:", textdiff.Lines(before, before) == nil)
	fmt.Fprintln(w, textdiff.Lines("A=1\nB=2\nC=3", "A=9\nB=2\nC=8"))
}
```
<!-- /ds:block hash=6a02cd -->

## Step 5 — Render an HTML report

`report.DiffHTML` and `report.MatrixHTML` return one self-contained page each: inline CSS, no JavaScript, no external requests, light and dark through `prefers-color-scheme`. The security property is structural: with `Revealed` false, leave the value fields empty and the templates print `report.MaskGlyph` (`••••••`, fixed width so not even value lengths leak) for every value cell, so a masked page holds no secret bytes and is safe to attach to a ticket. `Revealed` true shows the values and stamps a "contains secrets" banner.

<!-- ds:block id=writereports-6k27b2uy -->
**writeReports** · [`examples/providers/output-and-reports/main.go:153-175`](../../../examples/providers/output-and-reports/main.go#L153-L175)

```go
func writeReports(w io.Writer, dir string) error {
	for _, reveal := range []bool{false, true} {
		d := report.Diff{Program: programName, FileA: fileStag, FileB: fileProd, Revealed: reveal}
		change := report.DiffChange{Key: "DB_URL"}
		if reveal {
			change.From, change.To = "postgres://db.stag/app", secretValue
		}
		d.Changed = []report.DiffChange{change}
		page, err := report.DiffHTML(d)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, fmt.Sprintf("diff-revealed-%t.html", reveal))
		if err := os.WriteFile(path, page, reportFileMode); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: secret=%t mask=%t banner=%t\n", filepath.Base(path),
			bytes.Contains(page, []byte(secretValue)),
			bytes.Contains(page, []byte(report.MaskGlyph)),
			bytes.Contains(page, []byte(bannerText)))
	}
	return writeMatrix(w, dir)
}
```
<!-- /ds:block hash=abbef8 -->

A matrix has one row per key and one cell per environment; `State` becomes the cell's CSS class and `Symbol` its glyph.

<!-- ds:block id=writematrix-4fdg4qba -->
**writeMatrix** · [`examples/providers/output-and-reports/main.go:182-205`](../../../examples/providers/output-and-reports/main.go#L182-L205)

```go
func writeMatrix(w io.Writer, dir string) error {
	m := report.Matrix{
		Program: programName,
		Envs:    []string{envStag, envProd},
		Sources: []string{fileStag, fileProd},
		Rows: []report.MatrixRow{
			{Key: "DB_URL", Cells: []report.MatrixCell{cell(envkit.StatePresent), cell(envkit.StatePresent)}},
			{Key: "SENTRY_DSN", Cells: []report.MatrixCell{cell(envkit.StatePresent), cell(envkit.StateMissing)}},
		},
	}
	page, err := report.MatrixHTML(m)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "matrix.html")
	if err := os.WriteFile(path, page, reportFileMode); err != nil {
		return err
	}
	// The page embeds its generation time, so print properties, not bytes.
	fmt.Fprintf(w, "%s: doctype=%t script=%t external=%t\n", filepath.Base(path),
		bytes.HasPrefix(page, []byte("<!doctype html>")),
		bytes.Contains(page, []byte("<script")), bytes.Contains(page, []byte("src=\"http")))
	return nil
}
```
<!-- /ds:block hash=36db5d -->

From the command line, `dotenvctl matrix --format html -o matrix.html` and `dotenvctl diff .env.stag .env.prod --format html -o diff.html` write these pages (mode `0600`); add `--reveal` to show values.

## Try it

```sh
cd examples/providers/output-and-reports && go run .
```

```text
postgres://db/app
{"ok":true,"data":{"key":"DB_URL","value":"postgres://db/app"}}
human: out="" err="mytool: key \"API_KEY\" not found\n"
{"ok":false,"error":{"code":"not_found","message":"key \"API_KEY\" not found"}}
{"ok":false,"data":{"pushed":["DB_URL"]},"error":{"code":"exec","message":"push REDIS_URL: backend unavailable"}}
-DB_HOST=localhost
+DB_HOST=db.prod
no-op is nil: true
[-A=1 -B=2 -C=3 +A=9 +B=2 +C=8]
diff-revealed-false.html: secret=false mask=true banner=false
diff-revealed-true.html: secret=true mask=false banner=true
matrix.html: doctype=true script=false external=false
```

Line 1 is human mode, line 2 the same verb under JSON. Line 3 shows the failure went to `Err` and nothing to `Out`.

## Pitfalls

- **One envelope per invocation.** Call `OK`, `Fail` or `FailWith` exactly once per run in JSON mode; two calls print two objects and break `jq`. That is why a part-way failure uses `FailWith`, not `OK` then `Fail`.
- **Never write to cobra's writers directly.** A stray `fmt.Println` or `cmd.Println` pollutes `--json` output; everything goes through the `Printer`.
- **`Human` drops write errors on purpose** (a failed print to a terminal has no recovery); `OK`, `Fail` and `FailWith` return them because scripts depend on the envelope.
- **`Fail` prints; it does not exit.** Return an error carrying the exit code yourself, or use `providerkit.Fail`, which prints and returns a quiet `*providerkit.CmdError` (code 2 for `usage`, 1 otherwise). Do not print a quiet error a second time.
- **`diff` is not a failure code.** `dotenvctl diff` exits 1 when files differ but still prints `ok:true` with the differences; `diff` names the cause of that exit.
- **`textdiff` is not a general diff.** Two separate edits come back as one block spanning both (the last line of the output above); do not use it to apply patches.
- **A masked report only stays masked if you keep values out.** The templates print the glyph for every value cell when `Revealed` is false, but the package's rule is that the caller never fills `Value`, `From` or `To` in that case; keep secrets out of the input, not just out of the output.
- **Reports are not byte-stable.** Each page embeds its generation time, so compare properties (as the example does), not bytes, and do not commit regenerated pages expecting an empty diff.
- **Write revealed reports with mode `0600`.** They contain secrets; the example writes both with `0600`.

## Related

- [Write a push provider](write-a-provider.md): the kit verbs print through the `Printer` and use `FailWith` on a part-way failure
- [GitHub provider](github.md) and [Vercel provider](vercel.md): the envelope in real `--json` runs
- [Select which keys a command acts on](selection.md)
- Package reference: [clikit README § outfmt](../../../clikit/README.md#outfmt), [§ report](../../../clikit/README.md#report), [§ textdiff](../../../clikit/README.md#textdiff)
- Example: [`examples/providers/output-and-reports`](../../../examples/providers/output-and-reports); package examples: [`ExamplePrinter`](../../../clikit/outfmt/example_test.go), [`ExampleMatrixHTML`, `ExampleDiffHTML`](../../../clikit/report/example_test.go), [`ExampleLines`](../../../clikit/textdiff/example_test.go)
- API: [outfmt](https://pkg.go.dev/github.com/ubgo/textdata/clikit/outfmt), [report](https://pkg.go.dev/github.com/ubgo/textdata/clikit/report), [textdiff](https://pkg.go.dev/github.com/ubgo/textdata/clikit/textdiff) on pkg.go.dev
- [Feature index](../../reference/features.md)
