package jsonccmd

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
	"github.com/ubgo/textdata/pos"
)

// Where an issue comes from. Closed set; wire API in validate --json.
const (
	sourceSyntax = "syntax"
	sourceLint   = "lint"
	sourceSchema = "schema"
)

// ruleSyntax is the rule name of a syntax error.
const ruleSyntax = "syntax"

// issue is one validate finding on the wire.
type issue struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
	// Source is syntax, lint or schema.
	Source string `json:"source"`
	// Rule is the lint rule or the schema keyword that failed.
	Rule string `json:"rule"`
	// Path is the value's dotted path, for schema issues.
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`

	offset int
}

// String renders `file:line:col: message (rule)`.
func (i issue) String() string {
	return (&pos.Error{File: i.File, Pos: pos.Pos{Line: i.Line, Col: i.Col}, Msg: fmt.Sprintf("%s (%s)", i.Message, i.Rule)}).Error()
}

// validatePayload is the --json data shape for `validate`.
type validatePayload struct {
	File string `json:"file"`
	// Valid is true when there are no errors (warnings do not count).
	Valid    bool    `json:"valid"`
	Errors   []issue `json:"errors"`
	Warnings []issue `json:"warnings"`
}

// newValidateCmd reports syntax errors, lint findings and, with --schema,
// JSON Schema violations. Errors exit 1; schema warnings (unknown keys under
// additionalProperties, unsupported keywords) are printed but do not fail.
func newValidateCmd(a *app) *cobra.Command {
	var schemaFile string
	var forbidTrailing bool
	c := &cobra.Command{
		Use:   "validate <file>",
		Short: "Lint a file and check it against a JSON Schema",
		Example: "  textdatactl jsonc validate config.jsonc\n" +
			"  textdatactl jsonc validate config.jsonc --schema config.schema.json\n" +
			"  textdatactl jsonc validate config.json --strict --forbid-trailing-commas\n" +
			"  textdatactl jsonc validate config.jsonc --schema s.json --json | jq '.data.errors[].message'",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file := args[0]
			var sch *schema.Schema
			if schemaFile != "" {
				var err error
				if sch, err = schema.Load(schemaFile); err != nil {
					if errors.Is(err, schema.ErrSchema) || pos.First(err) != nil {
						return a.failf(CodeSchema, "%v", err)
					}
					return a.failf(outfmt.CodeIO, "%v", err)
				}
			}
			d, err := jsonc.Open(file, a.options()...)
			if d == nil {
				return a.failf(outfmt.CodeIO, "%v", err)
			}
			if !d.Existed() {
				return a.failf(outfmt.CodeIO, "%s: no such file", file)
			}

			res := validatePayload{File: file, Errors: []issue{}, Warnings: []issue{}}
			var l pos.ErrorList
			if errors.As(d.Err(), &l) {
				for _, e := range l {
					res.Errors = append(res.Errors, issue{File: file, Line: e.Pos.Line, Col: e.Pos.Col, offset: e.Pos.Offset, Source: sourceSyntax, Rule: ruleSyntax, Message: e.Msg})
				}
			}
			var lintOpts []jsonc.LintOption
			if forbidTrailing {
				lintOpts = append(lintOpts, jsonc.ForbidTrailingCommas())
			}
			// Lint checks the part that parsed, so it runs even on a
			// document with syntax errors.
			for _, li := range jsonc.Lint(d, lintOpts...) {
				res.Errors = append(res.Errors, issue{File: file, Line: li.Pos.Line, Col: li.Pos.Col, offset: li.Pos.Offset, Source: sourceLint, Rule: string(li.Rule), Message: li.Msg})
			}
			// A schema check of a broken document would only repeat the
			// syntax errors.
			if sch != nil && len(l) == 0 {
				r := schema.Validate(d, sch, schema.WithFileName(file))
				res.Errors = append(res.Errors, schemaIssues(file, r.Errors)...)
				res.Warnings = append(res.Warnings, schemaIssues(file, r.Warnings)...)
			}
			sortIssues(res.Errors)
			sortIssues(res.Warnings)
			res.Valid = len(res.Errors) == 0

			for _, i := range res.Errors {
				a.printer.Human(i.String())
			}
			for _, i := range res.Warnings {
				w := i
				w.Message = "warning: " + w.Message
				a.printer.Human(w.String())
			}
			if err := a.printer.OK(res); err != nil {
				return err
			}
			if !res.Valid {
				// Every issue is reported above; exit 1 is the contract.
				return &ExitError{Code: ExitFailure, Quiet: true}
			}
			return nil
		},
	}
	c.Flags().StringVar(&schemaFile, "schema", "", "JSON Schema file (draft 2020-12 subset) to validate against")
	c.Flags().BoolVar(&forbidTrailing, "forbid-trailing-commas", false, "report trailing commas as errors")
	return c
}

func schemaIssues(file string, in []schema.Issue) []issue {
	out := make([]issue, 0, len(in))
	for _, s := range in {
		msg := s.Message
		label := s.Path
		if label == "" {
			label = rootLabel
		}
		out = append(out, issue{File: file, Line: s.Pos.Line, Col: s.Pos.Col, offset: s.Pos.Offset, Source: sourceSchema, Rule: s.Keyword, Path: s.Path, Message: label + ": " + msg})
	}
	return out
}

// sortIssues orders by position; equal positions keep their source order
// (syntax, lint, schema).
func sortIssues(is []issue) {
	sort.SliceStable(is, func(i, j int) bool { return is[i].offset < is[j].offset })
}
