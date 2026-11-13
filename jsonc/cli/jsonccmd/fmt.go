package jsonccmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/textdiff"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/format"
)

// fmtPayload is the --json data shape for `fmt`.
type fmtPayload struct {
	File string `json:"file"`
	// Changed is true when the file is not in the requested style (for
	// --check and --dry-run: would change).
	Changed bool `json:"changed"`
	// Written is true only when the file was rewritten.
	Written bool `json:"written"`
	Check   bool `json:"check"`
	DryRun  bool `json:"dry_run"`
	// Diff is the would-be line diff of a dry run.
	Diff []string `json:"diff,omitempty"`
}

// newFmtCmd re-lays a file out in one style through jsonc/format, keeping
// every comment and the meaning. Like gofmt it is silent on success; --check
// lists the file and exits 1 when it is not formatted, for CI.
func newFmtCmd(a *app) *cobra.Command {
	var check, dryRun, tabs, trailing bool
	indent := format.DefaultStyle().Indent
	c := &cobra.Command{
		Use:   "fmt <file>",
		Short: "Format a file in one consistent style, keeping comments",
		Example: "  textdatactl jsonc fmt config.jsonc\n" +
			"  textdatactl jsonc fmt config.jsonc --check            # CI: exit 1 and list the file if it would change\n" +
			"  textdatactl jsonc fmt config.jsonc --indent 4 --dry-run\n" +
			"  textdatactl jsonc fmt config.jsonc --tabs --trailing-commas",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file := args[0]
			if indent < 1 {
				return a.usagef("--indent must be at least 1, got %d", indent)
			}
			if check && dryRun {
				return a.usagef("--check and --%s are mutually exclusive", flagDryRun)
			}
			d, err := a.open(file)
			if err != nil {
				return err
			}
			style := format.DefaultStyle()
			style.Indent, style.Tabs, style.TrailingCommas = indent, tabs, trailing
			out, err := format.Format(d, style)
			if err != nil {
				return a.docError(file, err)
			}
			before := d.Render()
			res := fmtPayload{File: file, Changed: string(out) != string(before), Check: check, DryRun: dryRun}
			switch {
			case check:
				if res.Changed {
					a.printer.Human(file)
				}
			case dryRun:
				res.Diff = textdiff.Lines(string(before), string(out))
				for _, line := range res.Diff {
					a.printer.Human(line)
				}
			case res.Changed:
				if err := a.saveBytes(file, out); err != nil {
					return err
				}
				res.Written = true
			}
			if err := a.printer.OK(res); err != nil {
				return err
			}
			if check && res.Changed {
				// Reported above; the exit code is the CI contract.
				return &ExitError{Code: ExitFailure, Quiet: true}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "write nothing; exit 1 and print the file name when it would change")
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	c.Flags().IntVar(&indent, "indent", indent, "spaces per indentation level")
	c.Flags().BoolVar(&tabs, "tabs", false, "indent with tabs (overrides --indent)")
	c.Flags().BoolVar(&trailing, "trailing-commas", false, "put a comma after the last member or element (ignored with --strict)")
	return c
}

// saveBytes writes new content to an existing file through jsonc's atomic
// save. SaveAs keeps the destination's own mode and writes through a
// symlink, exactly as Doc.Save does for an opened file.
func (a *app) saveBytes(file string, content []byte) error {
	nd, err := jsonc.Parse(content, a.options()...)
	if err != nil {
		// Format promises output that re-parses; this guards that promise.
		return a.failf(CodeSyntax, "%s: formatted output does not parse: %v", file, err)
	}
	if err := nd.SaveAs(file); err != nil {
		return a.failf(outfmt.CodeIO, "%v", err)
	}
	return nil
}
