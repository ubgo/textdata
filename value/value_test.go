package value

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/plugin"
)

func vars(m map[string]string) Option {
	return WithLookup(func(name string) (string, bool) {
		v, ok := m[name]
		return v, ok
	})
}

func mustExpand(t *testing.T, s string, opts ...Option) string {
	t.Helper()
	out, err := Expand(context.Background(), s, opts...)
	if err != nil {
		t.Fatalf("Expand(%q): %v", s, err)
	}
	return out
}

// TestEnv: WithEnv reads the process environment; without it nothing does.
func TestEnv(t *testing.T) {
	t.Setenv("TEXTDATA_VALUE_USER", "ada")
	if got := mustExpand(t, "hi ${TEXTDATA_VALUE_USER} $TEXTDATA_VALUE_USER", WithEnv()); got != "hi ada ada" {
		t.Errorf("with env = %q", got)
	}
	if got := mustExpand(t, "hi [${TEXTDATA_VALUE_USER}]"); got != "hi []" {
		t.Errorf("without env = %q, want the environment unread", got)
	}
}

// TestDefaults covers the operator family through the shared engine.
func TestDefaults(t *testing.T) {
	m := vars(map[string]string{"EMPTY": "", "HOST": "db"})
	tests := map[string]string{
		"${PORT:-5432}":     "5432",
		"${EMPTY-unset}":    "",
		"${EMPTY:-empty}":   "empty",
		"${HOST:+set}":      "set",
		"${NOPE:+set}":      "",
		"price $$5":         "price $5",
		"${A:-${HOST:-x}}":  "db",
		"tail $":            "tail $",
		"${unbalanced":      "${unbalanced",
		"$HOST.internal:80": "db.internal:80",
	}
	for in, want := range tests {
		if got := mustExpand(t, in, m); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
}

// TestRequired: ${VAR:?msg} is an *expand.RequiredError, the formats' type.
func TestRequired(t *testing.T) {
	_, err := Expand(context.Background(), "${TOKEN:?set TOKEN}", vars(map[string]string{"TOKEN": ""}))
	var re *expand.RequiredError
	if !errors.As(err, &re) || re.Key != "TOKEN" || !re.Empty || re.Message != "set TOKEN" {
		t.Fatalf("err = %v, want *expand.RequiredError", err)
	}
}

// TestNested: a source value is expanded in turn; a self-reference ends.
func TestNested(t *testing.T) {
	m := vars(map[string]string{
		"URL":  "postgres://${HOST}:${PORT:-5432}/app",
		"HOST": "db",
		"SELF": "a$SELF",
		"A":    "x$B", "B": "y$A",
	})
	if got := mustExpand(t, "$URL", m); got != "postgres://db:5432/app" {
		t.Errorf("nested = %q", got)
	}
	if got := mustExpand(t, "$SELF|$A", m); got != "a|xy" {
		t.Errorf("cycles = %q", got)
	}
}

// TestCommandsOffByDefault is the no-surprise rule: $(…) stays verbatim and
// nothing runs unless commands were asked for, even with every other option.
func TestCommandsOffByDefault(t *testing.T) {
	marker := t.TempDir() + "/ran"
	in := "a $(touch " + marker + ") b"
	got := mustExpand(t, in, WithEnv(), vars(nil), WithPlugins(fakeStore{}))
	if got != in {
		t.Errorf("Expand = %q, want the text kept", got)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the command ran")
	}
}

// TestWithRunner: a custom runner turns commands on and gets the command
// after its own references expanded; its error is *expand.CommandError.
func TestWithRunner(t *testing.T) {
	var seen []string
	allow := runnerFunc(func(cmd string) (string, error) {
		seen = append(seen, cmd)
		if cmd != "whoami ada" {
			return "", errors.New("not allowed")
		}
		return "ADA", nil
	})
	m := vars(map[string]string{"U": "ada"})
	if got := mustExpand(t, "user=$(whoami $U)", m, WithRunner(allow)); got != "user=ADA" {
		t.Errorf("Expand = %q", got)
	}
	_, err := Expand(context.Background(), "$(rm -rf x)", WithRunner(allow))
	var ce *expand.CommandError
	if !errors.As(err, &ce) || ce.Cmd != "rm -rf x" || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v, want *expand.CommandError", err)
	}
	// A CommandRunner plugin is the same as WithRunner.
	if got := mustExpand(t, "$(whoami ada)", WithPlugins(namedRunner{allow})); got != "ADA" {
		t.Errorf("plugin runner = %q", got)
	}
}

// TestContextRunner: a runner with Run(ctx, cmd) receives Expand's context.
func TestContextRunner(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "from-ctx")
	r := ctxRunner(func(ctx context.Context, _ string) (string, error) {
		v, _ := ctx.Value(key{}).(string)
		return v, nil
	})
	out, err := Expand(ctx, "$(anything)", WithRunner(r))
	if err != nil || out != "from-ctx" {
		t.Fatalf("Expand = %q, %v", out, err)
	}
}

// TestPluginScheme: ${scheme:ref} goes to the owning Resolver, matched without
// case; an unowned scheme is an ordinary (here unset, or looked-up) name.
func TestPluginScheme(t *testing.T) {
	p := WithPlugins(fakeStore{})
	if got := mustExpand(t, "${store:db/pass} ${STORE:x}", p); got != "secret(db/pass) secret(x)" {
		t.Errorf("scheme = %q", got)
	}
	if got := mustExpand(t, "[${other:thing}]", p); got != "[]" {
		t.Errorf("unknown scheme = %q, want an unset ordinary reference", got)
	}
	if got := mustExpand(t, "${other:thing}", p, vars(map[string]string{"other:thing": "as-name"})); got != "as-name" {
		t.Errorf("unknown scheme = %q, want it read as a name", got)
	}
	if got := mustExpand(t, "${MISSING:-d}", p); got != "d" {
		t.Errorf("operator = %q, want the operator, not a scheme", got)
	}
}

// TestPluginErrors: a failing resolver or lookuper is a *plugin.Error naming
// the plugin, and unwraps to the plugin's sentinel.
func TestPluginErrors(t *testing.T) {
	_, err := Expand(context.Background(), "${store:missing}", WithPlugins(fakeStore{}))
	var pe *plugin.Error
	if !errors.As(err, &pe) || pe.Plugin != "fake-store" || !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if want := `value: plugin "fake-store": resolve store:missing: not found`; err.Error() != want {
		t.Errorf("Error() = %q", err.Error())
	}
	_, err = Expand(context.Background(), "$DOWN", WithPlugins(fakeStore{}))
	if !errors.Is(err, plugin.ErrUnavailable) {
		t.Errorf("lookup err = %v", err)
	}
}

// TestSourceOrder: sources are asked in option order; the first answer wins.
func TestSourceOrder(t *testing.T) {
	t.Setenv("TEXTDATA_VALUE_X", "env")
	m := vars(map[string]string{"TEXTDATA_VALUE_X": "map"})
	if got := mustExpand(t, "$TEXTDATA_VALUE_X", m, WithEnv()); got != "map" {
		t.Errorf("map first = %q", got)
	}
	if got := mustExpand(t, "$TEXTDATA_VALUE_X", WithEnv(), m); got != "env" {
		t.Errorf("env first = %q", got)
	}
	if got := mustExpand(t, "$PLUGINVAR", WithPlugins(fakeStore{})); got != "from-plugin" {
		t.Errorf("plugin lookuper = %q", got)
	}
}

// TestTransformAndObserve: transformers see the final value; observers see
// every resolution.
func TestTransformAndObserve(t *testing.T) {
	var obs []string
	p := WithPlugins(upper{}, observer(func(path, name, v string) { obs = append(obs, path+"|"+name+"="+v) }))
	if got := mustExpand(t, "${A}-${store:k}", vars(map[string]string{"A": "a"}), p, WithPlugins(fakeStore{})); got != "A-SECRET(K)" {
		t.Errorf("Expand = %q", got)
	}
	if strings.Join(obs, ",") != "|A=a,|store:k=secret(k)" {
		t.Errorf("observed %q", obs)
	}
}

// TestUnsupportedPlugin: a plugin with no ability Expand uses is an error,
// never silently installed.
func TestUnsupportedPlugin(t *testing.T) {
	_, err := Expand(context.Background(), "x", WithPlugins(guardOnly{}))
	if !errors.Is(err, ErrUnsupportedPlugin) || !strings.Contains(err.Error(), "SaveGuard") {
		t.Fatalf("err = %v", err)
	}
}

// TestNilOptionsAndContext: nil options, a nil plugin, a nil lookup and a
// nil context are ignored rather than panicking.
func TestNilOptionsAndContext(t *testing.T) {
	var noCtx context.Context
	out, err := Expand(noCtx, "x$Y", nil, WithLookup(nil), WithPlugins(nil))
	if err != nil || out != "x" {
		t.Fatalf("Expand = %q, %v", out, err)
	}
}

type runnerFunc func(string) (string, error)

func (f runnerFunc) RunCommand(cmd string) (string, error) { return f(cmd) }

type ctxRunner func(context.Context, string) (string, error)

func (f ctxRunner) RunCommand(cmd string) (string, error) { return f(context.Background(), cmd) }
func (f ctxRunner) Run(ctx context.Context, cmd string) (string, error) {
	return f(ctx, cmd)
}

type namedRunner struct{ runnerFunc }

func (namedRunner) Name() string { return "allow-list" }

// fakeStore is a secret store: Resolver for "store", Lookuper for PLUGINVAR,
// unavailable for DOWN.
type fakeStore struct{}

func (fakeStore) Name() string      { return "fake-store" }
func (fakeStore) Schemes() []string { return []string{"store"} }
func (fakeStore) Resolve(_, ref string) (string, error) {
	if ref == "missing" {
		return "", plugin.ErrNotFound
	}
	return "secret(" + ref + ")", nil
}
func (fakeStore) Lookup(name string) (string, bool, error) {
	switch name {
	case "PLUGINVAR":
		return "from-plugin", true, nil
	case "DOWN":
		return "", false, plugin.ErrUnavailable
	}
	return "", false, nil
}

type upper struct{}

func (upper) Name() string { return "upper" }
func (upper) TransformValue(_, v string) (string, error) {
	return strings.ToUpper(v), nil
}

type observer func(path, name, resolved string)

func (observer) Name() string                         { return "observer" }
func (f observer) ObserveExpand(path, name, v string) { f(path, name, v) }

type guardOnly struct{}

func (guardOnly) Name() string                    { return "guard" }
func (guardOnly) GuardSave(plugin.Document) error { return nil }
