// Package jsonccmd is the `textdatactl jsonc` verb tree: get, set, unset,
// restore, delete, diff, to-json, fmt and validate over the textdata jsonc
// library (and its format and schema packages).
//
// Design rules (shared with dotenv/cli/dotenvcmd, keep them or the commands
// stop being embeddable):
//
//   - constructor-per-command (newXCmd(app)): no init() registration and no
//     package-level state, so a host can mount the tree twice or under
//     another root without cross-talk;
//   - all output flows through the app's clikit/outfmt Printer, so --json
//     always prints exactly one {ok,data|error} envelope;
//   - mutating verbs go Open → edit → Save (atomic, via jsonc.Doc.Save) and
//     skip the Save when the result is byte-identical, so a no-op edit never
//     touches the file;
//   - exit codes: 0 ok, 1 the operation failed (or diff found differences),
//     2 the command line was malformed.
//
// Files are positional arguments (`get <file> <path>`), and paths use the
// jsonc library's grammar: the dotted form `a."b.c"[2]` or a JSON Pointer
// `/a/b.c/2`. The empty path "" is the whole document.
package jsonccmd

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/jsonc"
)

// Exit codes, the contract with shells and CI. Frozen API.
const (
	// ExitOK: the operation succeeded (for diff: the files mean the same).
	ExitOK = 0
	// ExitFailure: the operation ran and failed (missing path, syntax error
	// in the file), or diff found differences, as diff(1) does.
	ExitFailure = 1
	// ExitUsage: the invocation was malformed (unknown flag, missing operand,
	// a path or JSON value that does not parse).
	ExitUsage = 2
)

// Error codes this CLI adds to the clikit/outfmt set for failures the jsonc
// library names. Scripts branch on them, so they are frozen API.
const (
	// CodeSyntax: the file is not valid JSONC (or not valid JSON under
	// --strict); the message carries file:line:col.
	CodeSyntax outfmt.ErrCode = "syntax"
	// CodeType: the value at the path has the wrong kind for the verb (unset
	// on an array element, delete of the root).
	CodeType outfmt.ErrCode = "type"
	// CodeExists: restore of a key that is live again.
	CodeExists outfmt.ErrCode = "exists"
	// CodeLayout: unset of a member that does not occupy lines of its own.
	CodeLayout outfmt.ErrCode = "layout"
	// CodeStrict: the verb needs comments, which --strict forbids.
	CodeStrict outfmt.ErrCode = "strict"
	// CodeSchema: the validate --schema file is not a valid schema.
	CodeSchema outfmt.ErrCode = "schema"
)

// Flag names, defined once so a rename cannot leave a stale literal behind.
const (
	flagJSON      = "json"
	flagStrict    = "strict"
	flagDryRun    = "dry-run"
	flagJSONValue = "json-value"
)

// DefaultProgram is the name the CLI prints for itself when the host sets
// none: the words a user types to reach the verbs.
const DefaultProgram = "textdatactl jsonc"

// commandUse is the namespace name when the tree is grafted with Command.
const commandUse = "jsonc"

// ExitError is how a verb's failure leaves the cobra tree: the exit code
// plus whether the failure was already reported through the Printer. Execute
// maps it to its return value; a host that grafts Command into its own tree
// reads Code with errors.As.
type ExitError struct {
	// Code is ExitFailure or ExitUsage.
	Code int
	// Quiet is true when the failure was already printed (human line or JSON
	// envelope); the host must not print Err again.
	Quiet bool
	// Err is the cause; nil for a bare exit code (diff found differences).
	Err error
}

// Error implements error.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap returns the cause.
func (e *ExitError) Unwrap() error { return e.Err }

// app is the per-invocation state every verb closes over. One per Execute or
// Command call, never shared.
type app struct {
	jsonOut bool
	strict  bool
	printer *outfmt.Printer
}

// Option configures Execute and Command. Functional options so a new
// setting never breaks a caller.
type Option func(*config)

type config struct {
	program string
}

// WithProgramName sets the name the CLI prints for itself: the "<name>: "
// prefix on failure lines, the "Run '<name> --help'" hint, the help Usage
// line and --version. Use the words a user types to reach the verbs. Empty
// keeps DefaultProgram. The --help examples are fixed text and keep naming
// "textdatactl jsonc".
func WithProgramName(name string) Option {
	return func(c *config) { c.program = name }
}

func newConfig(opts []Option) config {
	var c config
	for _, o := range opts {
		o(&c)
	}
	if c.program == "" {
		c.program = DefaultProgram
	}
	return c
}

// Execute parses args, runs the verb and returns the process exit code. It
// never calls os.Exit, so the whole CLI runs in-process in tests.
func Execute(args []string, stdout, stderr io.Writer, opts ...Option) int {
	cfg := newConfig(opts)
	a := &app{printer: &outfmt.Printer{Out: stdout, Err: stderr, Program: cfg.program}}
	root := newRootCmd(a)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	if xe, ok := errors.AsType[*ExitError](err); ok {
		if !xe.Quiet && xe.Err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", a.printer.ProgramName(), xe.Err)
		}
		return xe.Code
	}
	// Cobra's own errors (unknown verb or flag, wrong operand count): nobody
	// has printed anything yet because the root silences errors. Under
	// --json the usage failure is an envelope like every other failure.
	if a.jsonOut || jsonRequested(args) {
		a.printer.JSON = true
		if a.printer.Fail(outfmt.CodeUsage, err.Error()) == nil {
			return ExitUsage
		}
	}
	prog := a.printer.ProgramName()
	_, _ = fmt.Fprintf(stderr, "%s: %v\nRun '%s --help' for usage.\n", prog, err, prog)
	return ExitUsage
}

// Command returns the verb tree as a cobra command named "jsonc", for a host
// that grafts it into its own tree instead of delegating to Execute. Output
// goes to the command's OutOrStdout/ErrOrStderr. A verb failure comes back
// as an *ExitError (Quiet: already printed); cobra's own usage errors come
// back unprinted, for the host to report.
func Command(opts ...Option) *cobra.Command {
	cfg := newConfig(opts)
	a := &app{printer: &outfmt.Printer{Program: cfg.program}}
	c := newRootCmd(a)
	c.Use = commandUse
	return c
}

// jsonRequested reports whether args ask for --json before any "--". Cobra
// may reject the command line before it binds the flag, and the failure must
// still be an envelope when the caller asked for one. Last one wins.
func jsonRequested(args []string) bool {
	requested := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--"+flagJSON {
			requested = true
		}
		if v, ok := strings.CutPrefix(arg, "--"+flagJSON+"="); ok {
			on, err := strconv.ParseBool(v)
			requested = err == nil && on
		}
	}
	return requested
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use: commandUse,
		// The display name carries the program name (it may hold spaces,
		// "textdatactl jsonc", which Use cannot).
		Annotations: map[string]string{cobra.CommandDisplayNameAnnotation: a.printer.ProgramName()},
		Short:       "Comment-preserving JSONC control: get, set, unset, diff without disturbing a byte you did not touch",
		Long: "jsonc edits JSON and JSONC files through the textdata jsonc library: every byte\n" +
			"outside the value you touch survives verbatim, comments and layout included.\n" +
			"Paths are dotted (server.port, a.\"b.c\"[2]) or JSON Pointers (/server/port).",
		Example: "  textdatactl jsonc get config.jsonc server.port\n" +
			"  textdatactl jsonc set config.jsonc server.port 9090 --json-value --dry-run\n" +
			"  textdatactl jsonc diff base.jsonc prod.jsonc --json\n" +
			"  textdatactl jsonc validate config.jsonc --schema config.schema.json",
		Version:       versionString(debug.ReadBuildInfo),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(c *cobra.Command, _ []string) error { return c.Help() },
		PersistentPreRun: func(c *cobra.Command, _ []string) {
			a.printer.JSON = a.jsonOut
			// Grafted via Command: write where the host's tree writes.
			if a.printer.Out == nil {
				a.printer.Out = c.OutOrStdout()
			}
			if a.printer.Err == nil {
				a.printer.Err = c.ErrOrStderr()
			}
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(&a.jsonOut, flagJSON, false, "emit a machine-readable {ok,data|error} envelope")
	root.PersistentFlags().BoolVar(&a.strict, flagStrict, false, "read files as plain JSON: comments and trailing commas are errors")
	root.AddCommand(
		newGetCmd(a),
		newSetCmd(a),
		newUnsetCmd(a),
		newRestoreCmd(a),
		newDeleteCmd(a),
		newDiffCmd(a),
		newToJSONCmd(a),
		newFmtCmd(a),
		newValidateCmd(a),
	)
	return root
}

// options are the jsonc options every verb opens files with.
func (a *app) options() []jsonc.Option {
	if a.strict {
		return []jsonc.Option{jsonc.Strict()}
	}
	return nil
}

// cliModulePath is this module, whose version --version reports whether it
// is the main module or embedded in a host binary.
const cliModulePath = "github.com/ubgo/textdata/jsonc/cli"

// devVersion is reported when no module version is stamped.
const devVersion = "devel"

// develStamp is what the toolchain records for a module built from a local
// directory (a go.work workspace).
const develStamp = "(devel)"

// versionString reads this module's stamped version: Main when jsonc/cli is
// the main module, else the matching dependency (its Replace when replaced).
// The reader is a parameter so every arm is testable.
func versionString(read func() (*debug.BuildInfo, bool)) string {
	info, ok := read()
	if !ok || info == nil {
		return devVersion
	}
	mod := &info.Main
	if mod.Path != cliModulePath {
		mod = nil
		for _, dep := range info.Deps {
			if dep.Path == cliModulePath {
				mod = dep
				if dep.Replace != nil {
					mod = dep.Replace
				}
				break
			}
		}
	}
	if mod == nil || mod.Version == "" || mod.Version == develStamp {
		return devVersion
	}
	return mod.Version
}
