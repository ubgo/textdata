package plugin

import (
	"errors"
	"reflect"
	"testing"
)

type named string

func (n named) Name() string { return string(n) }

type everything struct{ named }

func (everything) Lookup(string) (string, bool, error)            { return "", false, nil }
func (everything) Schemes() []string                              { return []string{"x"} }
func (everything) Resolve(string, string) (string, error)         { return "", nil }
func (everything) RunCommand(string) (string, error)              { return "", nil }
func (everything) TransformValue(string, string) (string, error)  { return "", nil }
func (everything) GuardSave(Document) error                       { return nil }
func (everything) ObserveExpand(string, string, string)           {}
func (everything) Validate(Document) error                        { return nil }
func (everything) Include(string, string) ([]byte, string, error) { return nil, "", nil }

type lookOnly struct{ named }

func (lookOnly) Lookup(string) (string, bool, error) { return "", false, nil }

// special is a format-defined ability.
type special interface{ Special() }

type withSpecial struct{ named }

func (withSpecial) Special() {}

func TestHas(t *testing.T) {
	t.Parallel()
	all := everything{"all"}
	for _, c := range Capabilities {
		if !Has(all, c) {
			t.Errorf("Has(everything, %s) = false", c)
		}
		if Has(named("none"), c) {
			t.Errorf("Has(named, %s) = true", c)
		}
	}
	if Has(all, Capability("invented")) {
		t.Error("unknown capability reported")
	}
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	var zero Registry
	zero.Add(lookOnly{"a"}, nil, lookOnly{"b"})
	if zero.NameFor(CapLookup, 1) != "b" || zero.NameFor(CapLookup, 2) != "unknown" || zero.NameFor(Capability("x"), 0) != "unknown" {
		t.Error("zero registry NameFor")
	}
	if got := zero.Info(); len(got) != 2 || got[0].String() != "a: Lookuper" {
		t.Errorf("zero Info = %v", got)
	}

	r := NewRegistry(
		Check{Cap: "Special", Has: func(p Plugin) bool { _, ok := p.(special); return ok }},
		// Widen SaveGuard: a format counting its own guard type too.
		Check{Cap: CapSaveGuard, Has: func(p Plugin) bool { return Has(p, CapSaveGuard) || p.Name() == "legacy-guard" }},
	)
	r.Add(named("tracer"), everything{"all"}, withSpecial{"sp"}, named("legacy-guard"))
	want := []string{
		"tracer: (none)",
		"all: Lookuper, Resolver, ValueTransformer, CommandRunner, SaveGuard, ExpandObserver, Validator, Includer",
		"sp: Special",
		"legacy-guard: SaveGuard",
	}
	var got []string
	for _, i := range r.Info() {
		got = append(got, i.String())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Info =\n%q\nwant\n%q", got, want)
	}
	if r.NameFor(CapSaveGuard, 1) != "legacy-guard" || r.NameFor("Special", 0) != "sp" {
		t.Error("NameFor with checks")
	}
	if !r.HasCap(withSpecial{"s"}, "Special") || r.HasCap(withSpecial{"s"}, "Nope") {
		t.Error("HasCap")
	}
	if len(r.Plugins()) != 4 {
		t.Error("Plugins")
	}
	if l := Filter[Lookuper](r); len(l) != 1 {
		t.Errorf("Filter[Lookuper] = %d", len(l))
	}
}

func TestError(t *testing.T) {
	t.Parallel()
	err := Errorf("dotenv", "vault", "lookup DB_PASSWORD", ErrUnavailable)
	if got, want := err.Error(), `dotenv: plugin "vault": lookup DB_PASSWORD: plugin unavailable`; got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Error("errors.Is")
	}
	if got := (&Error{Plugin: "p", Op: "op", Err: ErrDenied}).Error(); got != `plugin "p": op: access denied` {
		t.Errorf("no prefix = %q", got)
	}
}
