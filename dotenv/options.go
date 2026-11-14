package dotenv

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/shell"
)

// EscapeMode selects how backslash sequences inside double-quoted values are
// decoded.
//
// The two modes are genuinely incompatible, which is why this is a choice rather
// than a default someone has to work around: under Compose, `\n` in a value
// stays two characters; under Extended it becomes a newline. Whichever a parser
// picks, some .env in the wild is misread — so the caller picks, based on who
// wrote the file.
type EscapeMode int

// ds:def id=escapemodes-pqfu7w85 owner=@maintainers stability=stable

const (
	// EscapeExtended interprets \n, \r, \t, \" and \\ — what most .env
	// tooling outside Docker does, and what most application developers
	// expect. This is the default.
	EscapeExtended EscapeMode = iota

	// EscapeCompose interprets only \" and \\. Every other backslash sequence
	// is kept literally, which is how Docker Compose reads a .env. Use it for
	// files compose consumes, and for values holding Windows paths or regexes
	// where a backslash means itself.
	EscapeCompose
)

// String implements fmt.Stringer.
func (m EscapeMode) String() string {
	switch m {
	case EscapeExtended:
		return "extended"
	case EscapeCompose:
		return "compose"
	default:
		return "unknown"
	}
}

// options is the resolved configuration for a parse.
type options struct {
	escapes    EscapeMode
	allowShell bool
	// runCommand is the installed CommandRunner, or nil for the default
	// shell runner; read it through command, never directly.
	runCommand func(string) (string, error)
	// commandCtx bounds the default shell runner (WithCommandContext); nil is
	// context.Background(), today's unbounded behaviour.
	commandCtx context.Context
	// strictExpand makes unresolved references errors (WithStrictExpand).
	strictExpand bool

	// err is the first option error, such as a plugin with no ability dotenv
	// supports. Parse has no error return, so it is kept here and reported by
	// Open, ParseReader, Read, and every fallible method of the File.
	err error

	// plugins is every installed plugin, in install order, kept for reporting.
	plugins []Plugin

	// The remaining slices are the other pre-filtered capability subsets, for
	// the same reason as lookupers below.
	transformers []ValueTransformer
	resolvers    []Resolver
	// guards holds this package's SaveGuards and cross-format
	// plugin.SaveGuards (adapted by docGuard) in one install-ordered list.
	guards    []SaveGuard
	entryObs  []EntryObserver
	expandObs []ExpandObserver

	// lookupers is the pre-filtered subset implementing Lookuper.
	//
	// Capabilities are resolved ONCE at construction rather than asserted per
	// call: expansion runs this loop for every reference in every value, and a
	// type assertion there would be paid on the hottest path in the package. A
	// plugin without the capability then costs nothing at all.
	lookupers []Lookuper
}

// addPlugin installs p and records which capabilities it was detected as having.
func (o *options) addPlugin(p Plugin) {
	o.plugins = append(o.plugins, p)

	if v, ok := p.(Lookuper); ok {
		o.lookupers = append(o.lookupers, v)
	}
	if v, ok := p.(ValueTransformer); ok {
		o.transformers = append(o.transformers, v)
	}
	if v, ok := p.(Resolver); ok {
		o.resolvers = append(o.resolvers, v)
	}
	if v, ok := p.(SaveGuard); ok {
		o.guards = append(o.guards, v)
	} else if v, ok := p.(plugin.SaveGuard); ok {
		o.guards = append(o.guards, docGuard{v})
	}
	if v, ok := p.(EntryObserver); ok {
		o.entryObs = append(o.entryObs, v)
	}
	if v, ok := p.(ExpandObserver); ok {
		o.expandObs = append(o.expandObs, v)
	}
	if v, ok := p.(CommandRunner); ok {
		// A replacement rather than a chain: the last runner installed wins,
		// and installing one enables substitution.
		o.allowShell = true
		o.runCommand = v.RunCommand
	}
}

// pluginNameFor returns the name of the i-th plugin holding capability cap.
//
// The capability slices are filtered, so their indices do not line up with the
// install order; this walks it to find the matching plugin, so an error can name
// the one that actually failed.
func (o *options) pluginNameFor(cap Capability, i int) string {
	seen := 0
	for _, p := range o.plugins {
		if !hasCapability(p, cap) {
			continue
		}
		if seen == i {
			return p.Name()
		}
		seen++
	}
	return "unknown"
}

// ds:def id=hascapability-tcbcz8hy owner=@maintainers stability=stable

// hasCapability reports whether p implements the interface named by cap.
func hasCapability(p Plugin, cap Capability) bool {
	switch cap {
	case CapSaveGuard:
		_, ok := p.(SaveGuard)
		return ok || plugin.Has(p, CapSaveGuard)
	case CapEntryObserver:
		_, ok := p.(EntryObserver)
		return ok
	default:
		return plugin.Has(p, cap)
	}
}

// pluginInfo reports each plugin with the capabilities detected for it.
func (o *options) pluginInfo() []PluginInfo {
	// A fixed order so diagnostics read the same on every run.
	all := []Capability{
		CapLookup, CapResolver, CapValueTransformer, CapCommandRunner,
		CapSaveGuard, CapEntryObserver, CapExpandObserver,
	}

	out := make([]PluginInfo, 0, len(o.plugins))
	for _, p := range o.plugins {
		info := PluginInfo{Name: p.Name()}
		for _, c := range all {
			if hasCapability(p, c) {
				info.Capabilities = append(info.Capabilities, c)
			}
		}
		out = append(out, info)
	}
	return out
}

// Option customises parsing and expansion.
type Option func(*options)

// WithEscapes selects the escape-sequence dialect. Defaults to EscapeExtended.
func WithEscapes(m EscapeMode) Option {
	return func(o *options) { o.escapes = m }
}

// ds:def id=withcommandsubstitution-5kmt8q5c owner=@maintainers stability=stable

// WithCommandSubstitution enables $(command) expansion in GetExpanded.
//
// OFF by default, deliberately. With it on, merely READING a configuration file
// executes arbitrary shell — so a .env fetched from a repository, a container
// image, or a teammate becomes remote code execution. That is a decision the
// calling application must make knowingly about files it trusts, not something
// a parser should do silently.
//
// When enabled, commands run through shell.Run (package
// github.com/ubgo/textdata/shell): `sh -c` on Unix-likes; on Windows `pwsh`
// when on PATH, else `powershell.exe`. A .env written with sh syntax may need
// WithCommandRunner(shell.Runner{Shell: ...}.RunCommand) on Windows. Stdin is
// the null device, so a prompting command fails instead of hanging. A command
// that exits non-zero makes GetExpanded and ExpandedMap return an
// *expand.CommandError naming it, so a failed secret lookup never reads as an
// empty value. WithCommandContext bounds how long a command may run.
func WithCommandSubstitution(enabled bool) Option {
	return func(o *options) { o.allowShell = enabled }
}

// ds:def id=withcommandcontext-v6889xsb owner=@maintainers stability=stable

// WithCommandContext bounds $(command) substitution with ctx: when ctx is
// cancelled or its deadline passes, the running command is killed and
// expansion fails with an *expand.CommandError wrapping shell.ErrTimeout (a
// deadline) or context.Canceled. Use context.WithTimeout for a time limit.
//
// It applies to the default shell runner only; a runner installed with
// WithCommandRunner or a CommandRunner plugin is called as is. It does not
// turn substitution on. Without it, commands are unbounded, as before. The
// context is kept for the life of the File, so pass one that outlives the
// reads that should honour it.
func WithCommandContext(ctx context.Context) Option {
	return func(o *options) { o.commandCtx = ctx }
}

// command returns the $(command) runner: the installed CommandRunner, or the
// default shell runner bounded by WithCommandContext.
func (o *options) command() func(string) (string, error) {
	if o.runCommand != nil {
		return o.runCommand
	}
	if o.commandCtx == nil {
		return shellRun
	}
	ctx := o.commandCtx
	return func(cmd string) (string, error) { return shell.Run(ctx, cmd) }
}

// WithStrictExpand makes GetExpanded, ExpandedMap and Read refuse every
// reference that would otherwise read as "" or stay as written, for a caller
// that needs certainty:
//
//   - a name neither the file nor a Lookuper defines: *expand.UnsetError.
//     ${VAR:-d} and ${VAR-d} still apply their default, ${VAR:?msg} keeps its
//     *RequiredError, and a key set to an empty value is set;
//   - a ${scheme:ref} no installed Resolver claims: *expand.UnknownSchemeError;
//   - a $(command) while command substitution is off: *expand.CommandOffError
//     (without strict the text is kept as written).
//
// The error is a *pos.Error naming the file and the line:col where the
// entry's value starts, wrapping the typed error (variable name and byte
// offset inside the decoded value, never a value); every one matches
// expand.ErrStrict. Single-quoted and backtick values are literal and never
// expanded, so they never fail. Off by default.
// ds:def id=withstrictexpand-at8hy283 owner=@maintainers stability=stable
func WithStrictExpand() Option {
	return func(o *options) { o.strictExpand = true }
}

// WithCommandRunner overrides how $(command) is executed.
//
// Exists so tests can exercise substitution without spawning a shell, and so an
// application can supply a sandboxed or allow-listed runner instead of the
// default shell. Implies WithCommandSubstitution(true).
//
// Sugar for WithPlugin over a CommandRunner — a one-function hook should not
// require declaring a type.
func WithCommandRunner(run func(string) (string, error)) Option {
	return func(o *options) {
		if run == nil {
			o.allowShell = true
			return
		}
		o.addPlugin(funcRunner{name: "command-runner", fn: run})
	}
}

// WithValueTransform rewrites values after they are read.
//
// Sugar for WithPlugin over a ValueTransformer:
//
//	dotenv.Parse(src, dotenv.WithValueTransform(func(key, v string) (string, error) {
//	    s, ok := strings.CutPrefix(v, "encrypted:")
//	    if !ok { return v, nil }
//	    return decrypt(s)
//	}))
func WithValueTransform(fn func(key, value string) (string, error)) Option {
	return func(o *options) {
		if fn == nil {
			return
		}
		o.addPlugin(funcTransformer{name: "value-transform", fn: fn})
	}
}

// WithSaveGuard refuses a write when the file fails a check.
//
// Sugar for WithPlugin over a SaveGuard. The guard may reject, never rewrite:
// fn receives a copy of the file, so a Set on it changes nothing that is saved.
func WithSaveGuard(fn func(*File) error) Option {
	return func(o *options) {
		if fn == nil {
			return
		}
		o.addPlugin(funcGuard{name: "save-guard", fn: fn})
	}
}

// ErrUnsupportedPlugin reports a plugin passed to WithPlugin whose only
// abilities are ones dotenv never uses (an Includer, a Validator). Installing it
// would do nothing, so it is refused rather than silently ignored.
var ErrUnsupportedPlugin = errors.New("no ability dotenv supports")

// WithPlugin installs a plugin.
//
// The plugin's capabilities are detected once, here, by type assertion. A
// plugin implementing none is legal — it may exist only to be listed — but is
// usually a signature typo, which is what Plugins exists to make visible.
//
// A plugin whose only abilities are ones dotenv does not use (the extends
// Includer, a Validator) is refused: it is not installed, and Open,
// ParseReader and Read return an error wrapping ErrUnsupportedPlugin, as do
// GetExpanded, ExpandedMap, Save and SaveAs on a File from Parse. A plugin with
// at least one ability dotenv supports installs as usual.
func WithPlugin(plugins ...Plugin) Option {
	return func(o *options) {
		for _, p := range plugins {
			if p == nil {
				continue
			}
			if err := unsupportedPlugin(p); err != nil {
				if o.err == nil {
					o.err = err
				}
				continue
			}
			o.addPlugin(p)
		}
	}
}

// dotenvCapabilities are the abilities dotenv acts on, in reporting order.
var dotenvCapabilities = []Capability{
	CapLookup, CapResolver, CapValueTransformer, CapCommandRunner,
	CapSaveGuard, CapEntryObserver, CapExpandObserver,
}

// unsupportedPlugin returns an error when p holds known abilities but none
// that dotenv supports. A plugin with no known ability at all is left alone.
func unsupportedPlugin(p Plugin) error {
	for _, c := range dotenvCapabilities {
		if hasCapability(p, c) {
			return nil
		}
	}
	var other []string
	for _, c := range plugin.Capabilities {
		if plugin.Has(p, c) {
			other = append(other, string(c))
		}
	}
	if len(other) == 0 {
		return nil
	}
	return fmt.Errorf("dotenv: plugin %q: %w (%s)", p.Name(), ErrUnsupportedPlugin, strings.Join(other, ", "))
}

// WithLookup supplies values for references the file does not define.
//
// Sugar for WithPlugin over a Lookuper:
//
//	dotenv.Parse(src, dotenv.WithLookup(os.LookupEnv))
//
// That single line is how a caller opts into Compose's environment fallback
// without this package ever touching os.Environ itself.
func WithLookup(fn func(name string) (string, bool)) Option {
	return func(o *options) {
		if fn == nil {
			return
		}
		o.addPlugin(funcLookuper{name: "lookup", fn: fn})
	}
}

// newOptions applies opts over the defaults.
//
// runCommand starts nil (the default shell runner, see command) and is only
// ever replaced by a CommandRunner's method value.
//
// A caller CAN install a typed-nil plugin —
// WithPlugin((*MyPlugin)(nil)) satisfies the interface and panics on call. Only
// reflection distinguishes that from a real value, which is not worth carrying
// on every install for a mistake the compiler already makes obvious at the call
// site.
func newOptions(opts []Option) options {
	o := options{escapes: EscapeExtended}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
