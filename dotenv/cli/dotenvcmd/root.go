// Package dotenvcmd wires the dotenvctl verb tree.
//
// Design rules (from the ubgo/dotenv repo docsi/cli/PLAN.md — keep them or the commands stop being
// embeddable):
//
//   - constructor-per-command (newXCmd(app)): no init() registration and no
//     package-level state, so a host application can mount the tree twice or
//     under another root without cross-talk;
//   - all output flows through the app's outfmt.Printer — a verb never prints
//     to os.Stdout directly, which is what makes tests and --json airtight;
//   - mutating verbs go Open → edit → Save and skip the Save when the render
//     is byte-identical, extending the library's no-op invariant to the CLI.
package dotenvcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/providerkit"
	githubplugin "github.com/ubgo/textdata/dotenv/cli/providers/github"
	vercelplugin "github.com/ubgo/textdata/dotenv/cli/providers/vercel"
)

// ds:def id=exitcodes-msf66qpb owner=@maintainers stability=stable

// Exit codes — the CLI's contract with shells and CI. Closed set; scripts
// branch on these, so the values are frozen API.
const (
	// ExitOK: the operation succeeded (for diff: the files are identical).
	ExitOK = 0

	// ExitFailure: the operation ran and failed — missing key, required-var
	// error, diff found differences. Mirrors diff(1)'s "1 = differences".
	ExitFailure = 1

	// ExitUsage: the invocation itself was malformed (unknown flag, missing
	// operand). Mirrors the BSD EX_USAGE convention of 2-for-usage that
	// grep/diff follow.
	ExitUsage = 2
)

// Flag names shared across verbs — defined once so a rename cannot leave a
// stale literal behind in one verb's lookup.
const (
	flagFile        = "file"
	flagJSON        = "json"
	flagExpand      = "expand"
	flagDryRun      = "dry-run"
	flagDelete      = "delete"
	flagAfter       = "after"
	flagBefore      = "before"
	flagDisabled    = "disabled"
	flagInherited   = "inherited"
	flagValues      = "values"
	flagOnlyDrift   = "only-drift"
	flagContract    = "contract"
	flagPlaceholder = "placeholder"
)

// program is this CLI's own name, used when the host sets none: the shared
// clikit/outfmt default names the umbrella textdatactl, and dotenvctl must keep
// printing "dotenvctl: …" exactly as it always has.
const program = "dotenvctl"

// defaultEnvFile is where every verb looks when -f/--file is not given: the
// conventional ./.env of the working directory.
const defaultEnvFile = ".env"

// exitError carries an exit code through cobra's error return so Execute can
// map operation failures (1) apart from usage errors (2) without stringly
// inspection. Every RunE failure a verb REPORTS deliberately is one of these;
// anything else bubbling out of cobra is by definition a usage problem.
type exitError struct {
	code int
	// quiet suppresses Execute's fallback printing — set when the verb already
	// emitted its own failure envelope/message via the Printer.
	quiet bool
	err   error
}

// Error implements error.
func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

// failf builds the standard failure path: it emits the failure through the
// printer (both modes) and returns the quiet exit error, so verbs fail in one
// line and cannot forget the envelope. The exit code follows the error code:
// outfmt.CodeUsage (a malformed invocation a verb detects itself, such as
// conflicting flags) exits ExitUsage like cobra's own usage errors; every
// other code is an operation failure, ExitFailure.
func (a *app) failf(code outfmt.ErrCode, format string, args ...any) error {
	exit := ExitFailure
	if code == outfmt.CodeUsage {
		exit = ExitUsage
	}
	msg := fmt.Sprintf(format, args...)
	if err := a.printer.Fail(code, msg); err != nil {
		return &exitError{code: exit, err: err}
	}
	return &exitError{code: exit, quiet: true, err: errors.New(msg)}
}

// exitWithCode wraps an already-reported failure with a specific exit code —
// used by `run`, whose contract is to pass the child's exit code through.
func exitWithCode(code int) error {
	return &exitError{code: code, quiet: true}
}

// app is the per-invocation state every verb constructor closes over: flag
// values bound at parse time plus the single output seam. One app per Execute
// call — never shared, never global.
type app struct {
	// file is the target .env path (-f/--file).
	file string

	// jsonOut mirrors --json; verbs read it only through printer.JSON.
	jsonOut bool

	printer *outfmt.Printer

	// errOut receives usage/cobra errors, kept separate from printer.Out so
	// --json stdout stays a single clean envelope.
	errOut io.Writer

	// runner / interactiveFn / askFn are the plugin-facing seams with real
	// defaults set in Execute; tests replace them to drive plugins end-to-end
	// without subprocesses or a terminal.
	runner        providerkit.Runner
	interactiveFn func() bool
	askFn         func(prompt string) bool

	// stdin is where confirmation answers are read from. Injectable because
	// reading os.Stdin directly made the prompt's accept path untestable —
	// the branch that decides whether a secret gets written.
	stdin io.Reader

	// callbackArgs reach exec plugins as DOTENVCTL_BIN_ARGS (WithCallbackArgs).
	callbackArgs []string
}

// ExecuteOption configures one Execute call. Functional options, like the
// library's dotenv.Option, so Execute keeps its three-argument form: every
// existing call compiles unchanged and a new setting never breaks a caller.
type ExecuteOption func(*executeConfig)

// executeConfig is what ExecuteOptions write into. Its zero value is the
// standalone dotenvctl binary's behaviour.
type executeConfig struct {
	// program names the CLI in the lines it prints about itself; empty means
	// "dotenvctl".
	program string

	// callbackArgs are the arguments that follow the host executable to reach
	// the dotenv verbs; exec plugins receive them as DOTENVCTL_BIN_ARGS.
	callbackArgs []string
}

// WithProgramName sets the name the CLI prints for itself when a host
// program embeds it: the "<name>: " prefix on failure lines, the
// "Run '<name> --help' for usage." hint, the help Usage line and the
// --version line. Use the words a user types to reach the verbs, e.g.
// "myapp env" for a host that mounts them under "myapp env". An empty name
// keeps the default, "dotenvctl".
//
// The name also replaces dotenvctl in the --help examples, and its first
// word names the completion script (`completion bash` emits a script for
// "myapp", which completes through `myapp __complete env …`; the host must
// route that to Execute as `__complete …`). It does not reach the JSON
// envelope, which carries no program name, or exec plugins, which are still
// found as dotenvctl-<name> binaries on PATH.
func WithProgramName(name string) ExecuteOption {
	return func(c *executeConfig) { c.program = name }
}

// WithCallbackArgs declares the arguments that must follow the host
// executable to reach the dotenv verbs, for hosts that mount them under a
// namespace: textdatactl passes "dotenv", a host serving them as "myapp env"
// passes "env". Exec plugins receive the words, space-separated, in
// DOTENVCTL_BIN_ARGS and call the host back with
// `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get KEY`
// (unquoted, so it splits into words and vanishes when empty). Each argument
// must therefore be a single word without whitespace. With no call the
// variable is empty: the standalone dotenvctl serves the verbs at its root.
func WithCallbackArgs(args ...string) ExecuteOption {
	return func(c *executeConfig) { c.callbackArgs = slices.Clone(args) }
}

// Execute parses args, runs the selected verb, and returns the process exit
// code. It never calls os.Exit itself — main does — so tests drive the full
// CLI in-process with buffers for both streams. opts are optional; with none
// the CLI behaves exactly as the dotenvctl binary.
func Execute(args []string, stdout, stderr io.Writer, opts ...ExecuteOption) int {
	var cfg executeConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	name := cfg.program
	if name == "" {
		name = program
	}
	a := &app{printer: &outfmt.Printer{Out: stdout, Err: stderr, Program: name}, errOut: stderr, callbackArgs: cfg.callbackArgs}
	return executeApp(a, args, stdout, stderr)
}

// executeApp is Execute with the app supplied — the test entry point that
// lets a suite pre-seed the plugin seams (fake runner, scripted prompts).
func executeApp(a *app, args []string, stdout, stderr io.Writer) int {
	if a.runner == nil {
		a.runner = providerkit.ExecRunner{}
	}
	if a.interactiveFn == nil {
		a.interactiveFn = stdinIsTerminal
	}
	if a.askFn == nil {
		a.askFn = a.askOnTerminal
	}
	if a.stdin == nil {
		a.stdin = os.Stdin
	}

	root := newRootCmd(a)

	// Exec-plugin dispatch runs BEFORE cobra: an unknown first token backed
	// by a dotenvctl-<name> binary on PATH is that plugin's invocation, argv
	// and exit code passed through verbatim (PLUGINS_SPEC §6). Built-ins can
	// never be shadowed — hasBuiltin wins first.
	if code, handled := a.execDispatch(root, args, stdout, stderr); handled {
		return code
	}

	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		if xe, ok := errors.AsType[*exitError](err); ok {
			if !xe.quiet && xe.err != nil {
				// Terminal stderr — a failed print of the failure message has
				// no further fallback; the exit code still tells the story.
				_, _ = fmt.Fprintf(stderr, "%s: %v\n", a.printer.ProgramName(), xe.err)
			}
			return xe.code
		}
		// Plugin/kit failures carry their own code via providerkit.CmdError —
		// the same 0/1/2 contract, produced on the other side of the seam.
		if pe, ok := errors.AsType[*providerkit.CmdError](err); ok {
			if !pe.Quiet && pe.Err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: %v\n", a.printer.ProgramName(), pe.Err)
			}
			return pe.Code
		}
		// Cobra's own errors (unknown flag/command, bad args) reach here — and
		// because the root sets SilenceErrors+SilenceUsage, NOBODY has printed
		// anything yet. Silent exit-2 was a real failure mode: `list --env x`
		// before the alias existed produced no output at all, reading exactly
		// like "no secrets". Errors must be loud.
		if a.jsonOut || jsonRequested(args) {
			// Under --json the usage failure is an envelope like every other
			// failure, so a script piping into jq always gets one object.
			// PersistentPreRun never ran for a usage error, so the mode is
			// set here.
			a.printer.JSON = true
			if a.printer.Fail(outfmt.CodeUsage, err.Error()) == nil {
				return ExitUsage
			}
		}
		prog := a.printer.ProgramName()
		_, _ = fmt.Fprintf(stderr, "%s: %v\nRun '%s --help' for usage.\n", prog, err, prog)
		return ExitUsage
	}
	return ExitOK
}

// jsonRequested reports whether args ask for --json before any "--"
// terminator. It exists for usage errors: cobra may reject the command line
// (unknown verb or flag) before it binds --json, and the failure must still
// be an envelope when the caller asked for one. The last occurrence wins, as
// in flag parsing; a malformed value such as --json=maybe counts as off.
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

// newRootCmd assembles the verb tree. Persistent flags bind into the shared
// app so every verb sees the same -f/--json without redeclaring them.
func newRootCmd(a *app) *cobra.Command {
	if a.printer.Program == "" {
		a.printer.Program = program
	}
	root := &cobra.Command{
		Use: program,
		// The display name, not Use, carries the program name: cobra builds
		// the help Usage lines, the --version line and its own "unknown
		// command ... for X" text from it, and it may hold spaces ("myapp
		// env"), which Use cannot — cobra takes Use's first word as the name.
		Annotations: map[string]string{cobra.CommandDisplayNameAnnotation: a.printer.ProgramName()},
		Short:       "Comment-preserving .env control — get, set, diff, run without disturbing a byte you didn't touch",
		Long: "dotenvctl edits .env files through the textdata dotenv library: every byte outside\n" +
			"the entry you touch survives verbatim — comments, blank lines, ordering, and\n" +
			"quoting style included. It never modifies its own process environment.",
		Version:       versionString(debug.ReadBuildInfo),
		SilenceUsage:  true,
		SilenceErrors: true,
		// PersistentPreRun rather than doing it in Execute: the flag values
		// exist only after cobra parses, and this is the first hook after.
		PersistentPreRun: func(*cobra.Command, []string) {
			a.printer.JSON = a.jsonOut
		},
	}

	root.PersistentFlags().StringVarP(&a.file, flagFile, "f", defaultEnvFile, "path to the .env file")
	root.PersistentFlags().BoolVar(&a.jsonOut, flagJSON, false, "emit a machine-readable {ok,data|error} envelope")

	root.AddCommand(
		newGetCmd(a),
		newSetCmd(a),
		newUnsetCmd(a),
		newRestoreCmd(a),
		newListCmd(a),
		newKeysCmd(a),
		newEnvsCmd(a),
		newMatrixCmd(a),
		newDiffCmd(a),
		newRunCmd(a),
	)

	// Plugins mount as namespaces via the one Deps channel (PLUGINS_SPEC §1).
	// Deps are built lazily-enough here: flag values (a.file, a.jsonOut) bind
	// before RunE fires, and Source parses only when a verb asks.
	deps := a.pluginDeps()
	root.AddCommand(githubplugin.New().Command(deps))
	root.AddCommand(vercelplugin.New().Command(deps))
	root.AddCommand(newPluginsCmd(a, root))

	if name := a.printer.ProgramName(); name != program {
		hostedName(root, name)
	}
	return root
}

// exampleProgram matches "dotenvctl" where a command line starts in help
// text: at the start of a line after indentation, followed by a space or the
// line end. It leaves the dotenvctl-<name> plugin binary names alone.
var exampleProgram = regexp.MustCompile(`(?m)^(\s*)` + program + `( |$)`)

// hostedName makes a hosted tree speak the host's name: every verb's
// examples (provider namespaces included) and the root's long text show
// name instead of dotenvctl, and the root command's cobra name becomes the
// host binary's (name's first word), so `completion` emits a script for the
// binary the user actually runs. That script calls
// `<binary> __complete <namespace> …`; the host routes it here (textdatactl
// does, see cmd/textdatactl/ctl).
func hostedName(root *cobra.Command, name string) {
	if bin, _, _ := strings.Cut(name, " "); bin != "" {
		root.Use = bin
	}
	replacement := "${1}" + strings.ReplaceAll(name, "$", "$$") + "${2}"
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Example = exampleProgram.ReplaceAllString(c.Example, replacement)
		c.Long = exampleProgram.ReplaceAllString(c.Long, replacement)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// cliModulePath is this module's import path: the module whose version
// --version and DOTENVCTL_VERSION report, whether dotenvctl is the main
// module or a dependency of a host binary that embeds dotenvcmd.
const cliModulePath = "github.com/ubgo/textdata/dotenv/cli"

// devVersion is reported when no module version is stamped (a source build,
// or the module replaced by a local directory).
const devVersion = "devel"

// versionString reads the version of THIS module (cliModulePath) stamped by
// the Go toolchain — no hand-maintained constant to forget at release time.
// When dotenvctl is the main module that is Main.Version; when a host binary
// embeds dotenvcmd, Main is the host, so the version comes from the matching
// entry in Deps (its Replace, when replaced). devVersion when nothing is
// stamped. The reader is a parameter so every arm is testable; production
// always passes debug.ReadBuildInfo.
func versionString(read func() (*debug.BuildInfo, bool)) string {
	info, ok := read()
	if !ok || info == nil {
		return devVersion
	}
	mod := &info.Main
	if mod.Path != cliModulePath && mod.Path != "" {
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
	// "(devel)" is what the toolchain stamps on a workspace or local build of
	// a dependency; report it like an unstamped build.
	if mod == nil || mod.Version == "" || mod.Version == develStamp {
		return devVersion
	}
	return mod.Version
}

// develStamp is the version the Go toolchain records for a module built from
// a local directory (a go.work workspace).
const develStamp = "(devel)"
