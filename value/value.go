// Package value expands one string with the same engine and rules as the
// textdata formats: $VAR, ${VAR}, the ${VAR:-default} family, nested
// references, $$, ${scheme:ref} through plugins, and $(command) when the
// caller turns commands on.
//
// Why it exists: a program whose config is not a .env or JSONC file (a YAML
// field, a flag, a database row) often wants exactly dotenv's semantics for
// one secret-bearing value, such as `token: "$(gopass show -o work/api)"` or
// `token: "${gopass:work/api}"`. Without this package it has to wire
// expand.Engine and a command runner itself.
//
// Nothing is read or run unless asked for: without WithEnv or WithLookup no
// variable has a value, and without WithCommands or WithRunner a $(command)
// is left in the output as written. Errors are the types the formats return
// (*expand.RequiredError, *expand.CommandError, *plugin.Error), so a caller
// handles one set.
package value

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/shell"
)

// errPrefix names this package in plugin errors:
// `value: plugin "vault": lookup DB_PASSWORD: connection refused`.
const errPrefix = "value"

// Plugin names for the function sources, used in plugin errors and Info.
const (
	envSourceName    = "env"
	lookupSourceName = "lookup"
)

// ErrUnsupportedPlugin reports a plugin passed to WithPlugins with none of the
// abilities a single value can use (Lookuper, Resolver, CommandRunner,
// ValueTransformer, ExpandObserver). A SaveGuard or Validator alone has
// nothing to do here; failing loudly beats installing it silently.
var ErrUnsupportedPlugin = errors.New("value: plugin has no ability Expand supports")

// Option configures Expand.
type Option func(*options)

// options is the resolved configuration of one Expand call.
type options struct {
	reg plugin.Registry

	// lookupers is every variable source in option order (WithEnv,
	// WithLookup and plugin Lookupers alike); the first to answer wins.
	lookupers    []plugin.Lookuper
	resolvers    []plugin.Resolver
	transformers []plugin.ValueTransformer
	observers    []plugin.ExpandObserver

	// commands is true once WithCommands or WithRunner was given. runner is
	// nil for the default shell (shell.Run with Expand's context).
	commands bool
	runner   plugin.CommandRunner

	// strict makes unresolved references errors (expand.Engine.Strict).
	strict bool

	// err is the first option error, reported by Expand.
	err error
}

// WithEnv resolves $VAR and ${VAR} from the process environment
// (os.LookupEnv). Without it, the environment is never read.
func WithEnv() Option {
	return func(o *options) { o.add(funcLookuper{name: envSourceName, fn: os.LookupEnv}) }
}

// WithLookup resolves variables from fn, for example a map or a config
// section. Sources are asked in option order and the first to answer wins, so
// WithLookup(m), WithEnv() lets m override the environment. A nil fn is
// ignored.
func WithLookup(fn func(name string) (string, bool)) Option {
	return func(o *options) {
		if fn != nil {
			o.add(funcLookuper{name: lookupSourceName, fn: fn})
		}
	}
}

// ds:def id=withcommands-jhgmdcyp owner=@maintainers stability=stable

// WithCommands turns $(command) substitution on, run through shell.Run with
// the context given to Expand: `sh -c` on Unix-likes, PowerShell on Windows.
//
// Off unless given: with it on, expanding a value runs commands, so turn it on
// only for values from a source you trust. A failing command is an
// *expand.CommandError wrapping the shell's error (*shell.ExitError,
// shell.ErrTimeout, shell.ErrNoShell).
func WithCommands() Option {
	return func(o *options) { o.commands = true }
}

// WithRunner turns $(command) substitution on and runs commands with r
// instead of the default shell: an allow-listed runner, a fake in tests, or a
// shell.Runner with a timeout or another shell. The last runner given wins.
//
// When r also has a Run(ctx, cmd) method (shell.Runner does), Expand calls
// that with its own context, so cancelling the context stops the command. A
// nil r is the same as WithCommands.
func WithRunner(r plugin.CommandRunner) Option {
	return func(o *options) {
		o.commands = true
		o.runner = r
	}
}

// WithStrict makes every reference that would otherwise read as "" or stay as
// written an error, for a caller that needs certainty (a token taken from a
// config file must never silently become ""):
//
//   - an unset name: *expand.UnsetError. ${VAR:-d} and ${VAR-d} still apply
//     their default, ${VAR:?msg} keeps its *expand.RequiredError, ${VAR:+a}
//     still yields "" when unset, and a name set to "" is set;
//   - a ${scheme:ref} no installed Resolver claims (${vault:x} without the
//     vault plugin, the typo ${gopas:x}): *expand.UnknownSchemeError;
//   - a $(command) without WithCommands or WithRunner:
//     *expand.CommandOffError, instead of returning the command text.
//
// Each error names the variable, scheme or command and the byte offset of
// the reference in s, never a value, and matches expand.ErrStrict. Off unless
// given.
// ds:def id=withstrict-puc4zm2j owner=@maintainers stability=stable
func WithStrict() Option {
	return func(o *options) { o.strict = true }
}

// WithPlugins installs value plugins (for example the gopass, vault, aws and
// file plugins): Lookupers answer variables after any earlier source,
// Resolvers own their ${scheme:ref}, a CommandRunner acts as WithRunner,
// ValueTransformers rewrite the result (with path ""), and ExpandObservers see
// each resolution. A plugin with none of these abilities makes Expand return
// an error wrapping ErrUnsupportedPlugin. nil plugins are ignored.
func WithPlugins(ps ...plugin.Plugin) Option {
	return func(o *options) {
		for _, p := range ps {
			if p != nil {
				o.add(p)
			}
		}
	}
}

// add installs p and files it under every ability it has.
func (o *options) add(p plugin.Plugin) {
	o.reg.Add(p)
	known := false
	if v, ok := p.(plugin.Lookuper); ok {
		o.lookupers = append(o.lookupers, v)
		known = true
	}
	if v, ok := p.(plugin.Resolver); ok {
		o.resolvers = append(o.resolvers, v)
		known = true
	}
	if v, ok := p.(plugin.CommandRunner); ok {
		o.commands = true
		o.runner = v
		known = true
	}
	if v, ok := p.(plugin.ValueTransformer); ok {
		o.transformers = append(o.transformers, v)
		known = true
	}
	if v, ok := p.(plugin.ExpandObserver); ok {
		o.observers = append(o.observers, v)
		known = true
	}
	if !known && o.err == nil {
		o.err = unsupported(p, &o.reg)
	}
}

// unsupported builds the error for a plugin Expand cannot use, listing what
// it was detected as instead, so the fix is obvious.
func unsupported(p plugin.Plugin, reg *plugin.Registry) error {
	var other []string
	for _, c := range plugin.Capabilities {
		if reg.HasCap(p, c) {
			other = append(other, string(c))
		}
	}
	if len(other) == 0 {
		other = []string{"none"}
	}
	return fmt.Errorf("value: plugin %q: %w (%s)", p.Name(), ErrUnsupportedPlugin, strings.Join(other, ", "))
}

// Expand resolves every reference in s.
//
// Variables come from the sources given (WithEnv, WithLookup, plugin
// Lookupers), first answer wins; an unknown name is "", as in Compose, and
// ${VAR:?msg} turns that into an *expand.RequiredError (with WithStrict, every
// unresolved reference is an error). A value a source
// returns is expanded in turn, and a reference back to a name being expanded
// reads as unset, so every chain ends. ${scheme:ref} goes to the Resolver
// that owns the scheme; an unowned scheme is an ordinary reference.
// $(command) runs only with WithCommands, WithRunner or a CommandRunner
// plugin, bounded by ctx; otherwise it is kept as written. A source, resolver
// or transformer failure is a *plugin.Error naming the plugin.
func Expand(ctx context.Context, s string, opts ...Option) (string, error) {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.err != nil {
		return "", o.err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	eng := expand.Engine{Lookup: o.lookup, Strict: o.strict}
	if o.commands {
		eng.Run = o.run(ctx)
	}
	if len(o.resolvers) > 0 {
		eng.Resolve = o.resolve
	}
	if len(o.observers) > 0 {
		eng.Observe = func(name, resolved string) {
			for _, ob := range o.observers {
				ob.ObserveExpand("", name, resolved)
			}
		}
	}
	out, err := eng.Expand(s, nil)
	if err != nil {
		return "", err
	}
	for i, t := range o.transformers {
		v, err := t.TransformValue("", out)
		if err != nil {
			return "", plugin.Errorf(errPrefix, o.reg.NameFor(plugin.CapValueTransformer, i), "transform", err)
		}
		out = v
	}
	return out, nil
}

// contextRunner is a CommandRunner that can also take a context, as
// shell.Runner does. Expand prefers it so cancelling ctx stops the command.
type contextRunner interface {
	Run(ctx context.Context, cmd string) (string, error)
}

// run returns the engine's command callback: the installed runner (with ctx
// when it accepts one), or shell.Run with ctx.
func (o *options) run(ctx context.Context) func(string) (string, error) {
	switch r := o.runner.(type) {
	case nil:
		return func(cmd string) (string, error) { return shell.Run(ctx, cmd) }
	case contextRunner:
		return func(cmd string) (string, error) { return r.Run(ctx, cmd) }
	default:
		return r.RunCommand
	}
}

// lookup asks the sources in order; the first answer wins and an error
// aborts. A name already being expanded reads as unset: a source value that
// refers back to itself (PATH=$PATH:/x from a map) ends instead of looping.
func (o *options) lookup(name string, st *expand.Stack) (expand.Var, error) {
	if _, busy := st.At(name); busy {
		return expand.Var{}, nil
	}
	for i, l := range o.lookupers {
		v, ok, err := l.Lookup(name)
		if err != nil {
			return expand.Var{}, plugin.Errorf(errPrefix, o.reg.NameFor(plugin.CapLookup, i), "lookup "+name, err)
		}
		if ok {
			return expand.Var{Value: v, Set: true, At: expand.AtLookup}, nil
		}
	}
	return expand.Var{}, nil
}

// resolve hands ${scheme:ref} to the first Resolver owning the scheme;
// ok=false lets the engine read the token as an ordinary reference.
func (o *options) resolve(scheme, ref string) (string, bool, error) {
	for i, r := range o.resolvers {
		for _, s := range r.Schemes() {
			if !strings.EqualFold(s, scheme) {
				continue
			}
			v, err := r.Resolve(scheme, ref)
			if err != nil {
				return "", true, plugin.Errorf(errPrefix, o.reg.NameFor(plugin.CapResolver, i), "resolve "+scheme+":"+ref, err)
			}
			return v, true, nil
		}
	}
	return "", false, nil
}

// funcLookuper adapts a lookup function to a named plugin.Lookuper, so the
// function sources and plugin Lookupers share one ordered chain.
type funcLookuper struct {
	name string
	fn   func(string) (string, bool)
}

func (f funcLookuper) Name() string { return f.name }

func (f funcLookuper) Lookup(name string) (string, bool, error) {
	v, ok := f.fn(name)
	return v, ok, nil
}
