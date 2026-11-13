// Package gopassplugin resolves ${gopass:path} references from a gopass
// password store, in every textdata format.
//
// Installing it is what turns it on: its documented job is to run the gopass
// binary (`gopass show -o <ref>`), and it does so only when a format asks it
// to resolve a reference it owns. Nothing runs at install time.
//
// It holds two abilities sharing one runner and one cache:
//
//   - plugin.Resolver for the scheme "gopass": ${gopass:personal/db/password}.
//   - plugin.Lookuper, off until WithPrefix or WithMapping is given: maps a
//     plain ${NAME} that the file itself does not define to a store path.
//
// Secret safety: the value is read from stdout only, has one trailing newline
// trimmed, and never appears in an error. argv carries only the reference.
// Stdin is not connected, so gopass cannot stop to ask a question.
package gopassplugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/textdata/plugin"
)

// Scheme is the reference scheme this plugin owns: ${gopass:ref}.
const Scheme = "gopass"

// pluginName is what Name returns, so a failure names this plugin.
const pluginName = "gopass"

// DefaultBinary is the program run when WithBinary is not given; looked up on
// PATH by the default runner.
const DefaultBinary = "gopass"

// DefaultTimeout bounds one gopass call when WithTimeout is not given. It is
// generous because gpg may wait for a pinentry dialog.
const DefaultTimeout = 30 * time.Second

// exitNotFound is the exit code gopass uses for "entry is not in the password
// store" (gopass's ExitNotFound).
const exitNotFound = 11

// notFoundText is the stable part of gopass's not-found message, a second
// signal for wrappers that do not keep the exit code.
const notFoundText = "not in the password store"

// maxDetail caps how much of gopass's stderr an error repeats.
const maxDetail = 200

// Runner runs the gopass binary with args and returns what it wrote to stdout
// and stderr. err is the process error: an error with an ExitCode() int method
// (as *exec.ExitError has) carries the exit status, exec.ErrNotFound means the
// binary is missing. Inject one with WithRunner so tests never call real
// gopass.
type Runner func(ctx context.Context, binary string, args []string) (stdout, stderr []byte, err error)

// ExecRunner is the default Runner: os/exec with stdin unconnected and the
// process environment inherited (gopass and gpg need HOME and the agent
// socket).
func ExecRunner(ctx context.Context, binary string, args []string) ([]byte, []byte, error) {
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.Bytes(), errOut.Bytes(), err
}

// Option configures a Plugin.
type Option func(*Plugin)

// WithRunner replaces how gopass is run. A nil runner keeps the default.
func WithRunner(r Runner) Option {
	return func(p *Plugin) {
		if r != nil {
			p.run = r
		}
	}
}

// WithBinary sets the gopass program (a name looked up on PATH, or a path).
func WithBinary(binary string) Option {
	return func(p *Plugin) {
		if binary != "" {
			p.binary = binary
		}
	}
}

// WithContext sets the parent context of every gopass call, so a caller can
// cancel resolution. The Resolver and Lookuper interfaces carry no context,
// which is why it is fixed at construction. Default context.Background().
func WithContext(ctx context.Context) Option {
	return func(p *Plugin) {
		if ctx != nil {
			p.ctx = ctx
		}
	}
}

// WithTimeout bounds each gopass call. Zero or negative means no timeout
// beyond the parent context. Default DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(p *Plugin) { p.timeout = d }
}

// WithCache turns the per-plugin cache on or off (default on). With it on, each
// reference is fetched once for the life of the Plugin: a document expanded
// several times decrypts each secret once. Only successes are cached; a
// not-found or a failure is asked again next time.
func WithCache(enabled bool) Option {
	return func(p *Plugin) { p.cache = enabled }
}

// WithPrefix turns the Lookuper on: an undefined ${NAME} is looked up at
// prefix+NAME (for example prefix "personal/app/" maps ${DB_PASSWORD} to
// personal/app/DB_PASSWORD). The prefix is used exactly as given. Every
// undefined name the file references then costs one gopass call, so prefer
// WithMapping when the set of names is known.
func WithPrefix(prefix string) Option {
	return func(p *Plugin) {
		p.prefix = prefix
		p.prefixSet = true
	}
}

// WithMapping turns the Lookuper on for exactly the names in m: ${NAME} is
// looked up at m[NAME]. A mapped name wins over WithPrefix. The map is copied.
func WithMapping(m map[string]string) Option {
	return func(p *Plugin) {
		if p.mapping == nil {
			p.mapping = make(map[string]string, len(m))
		}
		for k, v := range m {
			p.mapping[k] = v
		}
	}
}

// Plugin is the gopass plugin. Safe for concurrent use.
type Plugin struct {
	run       Runner
	binary    string
	ctx       context.Context
	timeout   time.Duration
	cache     bool
	prefix    string
	prefixSet bool
	mapping   map[string]string

	mu     sync.Mutex
	cached map[string]string
}

// New returns a gopass plugin. Install it with the format's WithPlugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{
		run:     ExecRunner,
		binary:  DefaultBinary,
		ctx:     context.Background(),
		timeout: DefaultTimeout,
		cache:   true,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements plugin.Plugin.
func (p *Plugin) Name() string { return pluginName }

// Schemes implements plugin.Resolver.
func (p *Plugin) Schemes() []string { return []string{Scheme} }

// Resolve implements plugin.Resolver: the first line of the gopass entry ref
// (`gopass show -o ref`). A missing entry wraps plugin.ErrNotFound; a missing
// binary, a timeout or any other store failure wraps plugin.ErrUnavailable; a
// malformed ref is an error without running anything.
func (p *Plugin) Resolve(scheme, ref string) (string, error) {
	if !strings.EqualFold(scheme, Scheme) {
		return "", fmt.Errorf("gopass: scheme %q: %w", scheme, plugin.ErrUnsupportedScheme)
	}
	return p.Show(ref)
}

// Lookup implements plugin.Lookuper. Without WithPrefix or WithMapping it
// answers ok=false for every name and runs nothing. A name with no entry in the
// store is ok=false, so the next Lookuper is tried; a store failure is an
// error, never a silent "unset".
func (p *Plugin) Lookup(name string) (string, bool, error) {
	ref, ok := p.refFor(name)
	if !ok {
		return "", false, nil
	}
	v, err := p.Show(ref)
	if errors.Is(err, plugin.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// refFor maps a variable name to a store path. Names that could read as a
// flag or walk the store (leading '-', '/', "..", control characters) are
// never mapped by prefix.
func (p *Plugin) refFor(name string) (string, bool) {
	if ref, ok := p.mapping[name]; ok {
		return ref, true
	}
	if !p.prefixSet || name == "" {
		return "", false
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") || name[0] == '-' || hasControl(name) {
		return "", false
	}
	return p.prefix + name, true
}

// Show returns the first line of the entry at ref, through the cache. It is
// what Resolve and Lookup use; exported for callers that want a value without
// a document.
func (p *Plugin) Show(ref string) (string, error) {
	if err := checkRef(ref); err != nil {
		return "", err
	}
	if p.cache {
		p.mu.Lock()
		v, ok := p.cached[ref]
		p.mu.Unlock()
		if ok {
			return v, nil
		}
	}
	v, err := p.show(ref)
	if err != nil {
		return "", err
	}
	if p.cache {
		p.mu.Lock()
		if p.cached == nil {
			p.cached = make(map[string]string)
		}
		p.cached[ref] = v
		p.mu.Unlock()
	}
	return v, nil
}

// ClearCache forgets every cached value, for a long-running caller that wants
// rotated secrets picked up.
func (p *Plugin) ClearCache() {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()
}

// show runs gopass once and classifies the outcome.
func (p *Plugin) show(ref string) (string, error) {
	ctx := p.ctx
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	stdout, stderr, err := p.run(ctx, p.binary, []string{"show", "-o", ref})
	if err == nil {
		return trimNewline(string(stdout)), nil
	}
	detail := scrub(stderr, stdout)
	if errors.Is(err, exec.ErrNotFound) {
		return "", fmt.Errorf("gopass: %s: binary %q not found: %w", ref, p.binary, plugin.ErrUnavailable)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("gopass: %s: %v: %w", ref, ctxErr, plugin.ErrUnavailable)
	}
	var coded interface{ ExitCode() int }
	if (errors.As(err, &coded) && coded.ExitCode() == exitNotFound) || strings.Contains(detail, notFoundText) {
		return "", fmt.Errorf("gopass: %s: entry not in the password store: %w", ref, plugin.ErrNotFound)
	}
	msg := err.Error()
	if detail != "" {
		msg += ": " + detail
	}
	return "", fmt.Errorf("gopass: %s: %s: %w", ref, msg, plugin.ErrUnavailable)
}

// checkRef refuses refs that are empty, could be read as a flag by gopass, or
// carry control characters. Nothing is run for them.
func checkRef(ref string) error {
	switch {
	case ref == "":
		return errors.New("gopass: empty reference")
	case ref[0] == '-':
		return fmt.Errorf("gopass: reference %q starts with '-'", ref)
	case hasControl(ref):
		return fmt.Errorf("gopass: reference %q contains a control character", ref)
	}
	return nil
}

func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// trimNewline removes one trailing "\n" or "\r\n" (gopass ends its output with
// one); every other byte of the value is kept.
func trimNewline(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	return strings.TrimSuffix(s, "\n")
}

// scrub returns the first line of stderr, capped at maxDetail bytes, with any
// line of stdout removed from it: gopass never prints the secret to stderr,
// and this keeps it that way even for a wrapper that does.
func scrub(stderr, stdout []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n")
	for _, s := range strings.Split(string(stdout), "\n") {
		if s = strings.TrimSpace(s); s != "" {
			line = strings.ReplaceAll(line, s, "[redacted]")
		}
	}
	if len(line) > maxDetail {
		line = line[:maxDetail] + "..."
	}
	return strings.TrimSpace(line)
}

// Compile-time checks: the abilities a format detects by type assertion.
var (
	_ plugin.Plugin   = (*Plugin)(nil)
	_ plugin.Resolver = (*Plugin)(nil)
	_ plugin.Lookuper = (*Plugin)(nil)
)
