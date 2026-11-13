package plugin

import (
	"fmt"
	"strings"
)

// Capability names one ability a plugin was detected as having.
//
// Optional interfaces fail silently: a method with a slightly wrong signature
// compiles, installs, and never runs. Reporting what was actually detected
// turns that hour of debugging into a one-line fix.
type Capability string

// The capabilities this package defines. A format may define more of its own
// (dotenv: "EntryObserver") and report them with Check.
const (
	CapLookup           Capability = "Lookuper"
	CapResolver         Capability = "Resolver"
	CapValueTransformer Capability = "ValueTransformer"
	CapCommandRunner    Capability = "CommandRunner"
	CapSaveGuard        Capability = "SaveGuard"
	CapExpandObserver   Capability = "ExpandObserver"
	CapValidator        Capability = "Validator"
	CapIncluder         Capability = "Includer"
)

// Capabilities is the fixed reporting order of this package's capabilities, so
// diagnostics read the same on every run.
var Capabilities = []Capability{
	CapLookup, CapResolver, CapValueTransformer, CapCommandRunner,
	CapSaveGuard, CapExpandObserver, CapValidator, CapIncluder,
}

// Has reports whether p implements the interface named by c. Unknown
// capabilities report false.
func Has(p Plugin, c Capability) bool {
	switch c {
	case CapLookup:
		_, ok := p.(Lookuper)
		return ok
	case CapResolver:
		_, ok := p.(Resolver)
		return ok
	case CapValueTransformer:
		_, ok := p.(ValueTransformer)
		return ok
	case CapCommandRunner:
		_, ok := p.(CommandRunner)
		return ok
	case CapSaveGuard:
		_, ok := p.(SaveGuard)
		return ok
	case CapExpandObserver:
		_, ok := p.(ExpandObserver)
		return ok
	case CapValidator:
		_, ok := p.(Validator)
		return ok
	case CapIncluder:
		_, ok := p.(Includer)
		return ok
	default:
		return false
	}
}

// Check detects a capability; formats use it for abilities this package does
// not define, or to widen one (dotenv also counts its own SaveGuard(*File)).
type Check struct {
	Cap Capability
	Has func(Plugin) bool
}

// Info is one installed plugin and the capabilities it was recognised as
// having.
type Info struct {
	Name         string
	Capabilities []Capability
}

// String renders as "vault: Lookuper, Resolver", or "tracer: (none)".
func (p Info) String() string {
	if len(p.Capabilities) == 0 {
		return p.Name + ": (none)"
	}
	names := make([]string, len(p.Capabilities))
	for i, c := range p.Capabilities {
		names[i] = string(c)
	}
	return p.Name + ": " + strings.Join(names, ", ")
}

// Registry keeps installed plugins in install order and answers which one
// holds a capability. Formats keep their own pre-filtered slices for the hot
// path; the registry is for naming and reporting.
//
// The zero Registry is ready to use. A nil plugin is ignored.
type Registry struct {
	plugins []Plugin
	// checks are the capabilities detected, in reporting order. nil means
	// this package's Capabilities with Has.
	checks []Check
}

// NewRegistry returns a registry that detects this package's capabilities
// with Has, overridden or extended by checks: a check whose Cap is already
// known replaces that detection, a new Cap is appended to the reporting order.
func NewRegistry(checks ...Check) *Registry {
	r := &Registry{}
	for _, c := range Capabilities {
		c := c
		r.checks = append(r.checks, Check{Cap: c, Has: func(p Plugin) bool { return Has(p, c) }})
	}
	for _, extra := range checks {
		replaced := false
		for i := range r.checks {
			if r.checks[i].Cap == extra.Cap {
				r.checks[i] = extra
				replaced = true
			}
		}
		if !replaced {
			r.checks = append(r.checks, extra)
		}
	}
	return r
}

// Add installs plugins in order.
func (r *Registry) Add(ps ...Plugin) {
	for _, p := range ps {
		if p != nil {
			r.plugins = append(r.plugins, p)
		}
	}
}

// Plugins returns the installed plugins in install order.
func (r *Registry) Plugins() []Plugin { return r.plugins }

// HasCap reports whether p holds c under this registry's checks.
func (r *Registry) HasCap(p Plugin, c Capability) bool {
	if r.checks == nil {
		return Has(p, c)
	}
	for _, ch := range r.checks {
		if ch.Cap == c {
			return ch.Has(p)
		}
	}
	return false
}

// NameFor returns the name of the i-th installed plugin holding c, or
// "unknown". Formats keep filtered slices whose indices do not line up with
// install order; this maps an index in such a slice back to the plugin, so an
// error names the one that failed.
func (r *Registry) NameFor(c Capability, i int) string {
	seen := 0
	for _, p := range r.plugins {
		if !r.HasCap(p, c) {
			continue
		}
		if seen == i {
			return p.Name()
		}
		seen++
	}
	return "unknown"
}

// Info reports every installed plugin and its detected capabilities.
func (r *Registry) Info() []Info {
	checks := r.checks
	if checks == nil {
		checks = NewRegistry().checks
	}
	out := make([]Info, 0, len(r.plugins))
	for _, p := range r.plugins {
		info := Info{Name: p.Name()}
		for _, ch := range checks {
			if ch.Has(p) {
				info.Capabilities = append(info.Capabilities, ch.Cap)
			}
		}
		out = append(out, info)
	}
	return out
}

// Filter returns the installed plugins implementing T, in install order.
func Filter[T any](r *Registry) []T {
	var out []T
	for _, p := range r.plugins {
		if v, ok := p.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

// Error is a plugin failure that names the plugin:
// `dotenv: plugin "vault": lookup DB_PASSWORD: connection refused`.
type Error struct {
	// Prefix is the format or package raising it ("dotenv", "jsonc").
	Prefix string
	// Plugin is the failing plugin's Name.
	Plugin string
	// Op says what was being done ("lookup DB_PASSWORD", "guard save f").
	Op string
	// Err is the plugin's error.
	Err error
}

// Errorf wraps err from plugin name while doing op.
func Errorf(prefix, name, op string, err error) *Error {
	return &Error{Prefix: prefix, Plugin: name, Op: op, Err: err}
}

func (e *Error) Error() string {
	s := fmt.Sprintf("plugin %q: %s: %v", e.Plugin, e.Op, e.Err)
	if e.Prefix != "" {
		s = e.Prefix + ": " + s
	}
	return s
}

// Unwrap returns the plugin's error.
func (e *Error) Unwrap() error { return e.Err }
