// Command output-and-reports shows the three output helpers every textdata
// CLI shares: the outfmt Printer (human lines or one {ok,data|error} JSON
// envelope, with a closed set of error codes), textdiff (the -old/+new lines
// a --dry-run preview prints) and report (self-contained HTML pages, values
// masked unless revealed).
//
// Use them when you write your own verb or provider and want it to behave
// like dotenvctl and textdatactl: same streams, same envelope, same codes.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/report"
	"github.com/ubgo/textdata/clikit/textdiff"
	"github.com/ubgo/textdata/dotenv/cli/envkit"
)

// programName prefixes human failure lines.
const programName = "mytool"

// reportFileMode is 0600: a revealed report contains secrets.
const reportFileMode = 0o600

// secretValue must never appear in a masked report.
const secretValue = "postgres://db.prod/app?sslmode=verify-full"

// bannerText is the marker a revealed page carries.
const bannerText = "CONTAINS SECRETS"

// The two environments the reports compare, and the files they come from.
const (
	envStag  = "stag"
	envProd  = "prod"
	fileStag = ".env." + envStag
	fileProd = ".env." + envProd
)

// getPayload is one verb's --json data. A named struct, never a bare map,
// so the wire shape is greppable at the call site.
//
//ds:def id=getpayload-98mr2y9u
type getPayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// pushedPayload is the partial record a verb reports when it fails part-way.
type pushedPayload struct {
	Pushed []string `json:"pushed"`
}

func main() {
	dir, err := os.MkdirTemp("", "reports-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "output-and-reports:", err)
		os.Exit(1)
	}
}

// run walks the three helpers in guide order.
func run(w io.Writer, dir string) error {
	if err := printBothModes(w); err != nil {
		return err
	}
	if err := failBothModes(w); err != nil {
		return err
	}
	if err := failPartWay(w); err != nil {
		return err
	}
	previewEdit(w)
	return writeReports(w, dir)
}

// printBothModes runs one verb in each mode. Human mode prints lines and OK
// is silent; JSON mode prints exactly one envelope and Human is silent, so a
// verb calls both unconditionally and never checks the mode itself.
//
//ds:def id=printbothmodes-9ees84ht
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

// failBothModes reports a failure. Human mode writes "<program>: <message>"
// to Err, never Out, so `v=$(mytool get KEY)` captures a value or nothing;
// JSON mode writes the failure envelope to Out, because the envelope is the
// output a script parses. Scripts branch on the code, never the message.
//
//ds:def id=failbothmodes-fyqv3g4y
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

// failPartWay is for an operation that stopped after doing some work: one
// ok:false envelope carries both the error and what already happened, so a
// script never has to parse two objects. In human mode it is plain Fail; the
// caller prints the human form of the partial record itself first.
//
//ds:def id=failpartway-7bh3zda3
func failPartWay(w io.Writer) error {
	p := &outfmt.Printer{Out: w, JSON: true}
	return p.FailWith(outfmt.CodeExec, "push REDIS_URL: backend unavailable",
		pushedPayload{Pushed: []string{"DB_URL"}})
}

// previewEdit prints what a --dry-run shows for one edit: the changed region
// as -old/+new lines. Lines trims the common prefix and suffix; it is not a
// general diff, so two separate edits come back as one block spanning both.
//
//ds:def id=previewedit-58aanwta
func previewEdit(w io.Writer) {
	before := "# db\nDB_HOST=localhost\nDB_PORT=5432\n"
	after := "# db\nDB_HOST=db.prod\nDB_PORT=5432\n"
	for _, line := range textdiff.Lines(before, after) {
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, "no-op is nil:", textdiff.Lines(before, before) == nil)
	fmt.Fprintln(w, textdiff.Lines("A=1\nB=2\nC=3", "A=9\nB=2\nC=8"))
}

// writeReports renders the same diff twice. The masked page never receives
// the values (leave the value fields empty unless Revealed), so it is safe to
// attach to a ticket; the revealed page shows them under a banner.
//
//ds:def id=writereports-6k27b2uy
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

// writeMatrix renders a drift matrix: one row per key, one cell per env.
// State is the CSS class and Symbol the glyph; with Revealed false no value
// is ever shown, only the state of each cell.
//
//ds:def id=writematrix-4fdg4qba
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

// cell builds a matrix cell from envkit's closed set of states, so the CSS
// class and the glyph come from the same table the CLI draws with.
func cell(state envkit.CellState) report.MatrixCell {
	return report.MatrixCell{State: string(state), Symbol: envkit.StateSymbols[state]}
}
