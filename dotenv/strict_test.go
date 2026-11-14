package dotenv

import (
	"errors"
	"strings"
	"testing"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/pos"
)

// TestStrictExpand: WithStrictExpand turns each unresolved reference into a
// typed error at the entry's file:line:col; resolved forms read as before.
// ds:def id=teststrictexpand-59jbhqpv owner=@maintainers stability=stable
func TestStrictExpand(t *testing.T) {
	t.Parallel()
	src := "# header\nHOST=db\nEMPTY=\nURL=postgres://${HOST}/x\nDEF=${NOPE:-d}\nLIT='${NOPE}'\n" +
		"BAD=x${NOPE}\nQ=\"${vault:db}\"\nCMD=$(gopass show x)\nVIA=${BAD}\n"
	f := Parse(src, WithStrictExpand())
	f.Path = "app.env"

	for key, want := range map[string]string{"URL": "postgres://db/x", "DEF": "d", "LIT": "${NOPE}", "EMPTY": ""} {
		got, _, err := f.GetExpanded(key)
		if err != nil || got != want {
			t.Errorf("GetExpanded(%s) = %q, %v; want %q", key, got, err, want)
		}
	}

	tests := []struct {
		key  string
		msg  string
		want error
	}{
		{"BAD", "app.env:7:5: expand BAD: expand: variable NOPE is not set (offset 1)", &expand.UnsetError{}},
		{"Q", `app.env:8:3: expand Q: expand: no resolver for scheme "vault" in ${vault:db} (offset 0)`, &expand.UnknownSchemeError{}},
		{"CMD", "app.env:9:5: expand CMD: expand: command substitution is off: $(gopass show x) (offset 0)", &expand.CommandOffError{}},
		{"VIA", "app.env:10:5: expand VIA: expand: variable NOPE is not set (offset 0, in the value of BAD)", &expand.UnsetError{}},
	}
	for _, tt := range tests {
		_, _, err := f.GetExpanded(tt.key)
		var pe *pos.Error
		if !errors.As(err, &pe) || !errors.Is(err, expand.ErrStrict) {
			t.Fatalf("GetExpanded(%s) err = %v; want a *pos.Error matching expand.ErrStrict", tt.key, err)
		}
		if err.Error() != tt.msg {
			t.Errorf("GetExpanded(%s) err =\n %s\nwant\n %s", tt.key, err, tt.msg)
		}
	}

	if _, err := f.ExpandedMap(); !errors.Is(err, expand.ErrStrict) {
		t.Errorf("ExpandedMap err = %v, want a strict error", err)
	}
}

// TestStrictExpandKeepsRequiredError: ${VAR:?msg} keeps dotenv's own
// *RequiredError, unwrapped, as without strict.
func TestStrictExpandKeepsRequiredError(t *testing.T) {
	t.Parallel()
	_, _, err := Parse("A=${NOPE:?set it}\n", WithStrictExpand()).GetExpanded("A")
	var re *RequiredError
	if !errors.As(err, &re) || errors.Is(err, expand.ErrStrict) {
		t.Fatalf("err = %v, want *RequiredError", err)
	}
}

// TestStrictExpandLookupAndCommands: a Lookuper answer and an enabled runner
// both satisfy strict mode.
func TestStrictExpandLookupAndCommands(t *testing.T) {
	t.Parallel()
	f := Parse("A=$FROM_ENV\nB=$(x)\n", WithStrictExpand(),
		WithLookup(func(n string) (string, bool) { return "env:" + n, n == "FROM_ENV" }),
		WithCommandRunner(func(cmd string) (string, error) { return "ran:" + cmd, nil }))
	m, err := f.ExpandedMap()
	if err != nil || m["A"] != "env:FROM_ENV" || m["B"] != "ran:x" {
		t.Fatalf("ExpandedMap = %v, %v", m, err)
	}
}

// TestStrictExpandOffByDefault: without the option nothing changes.
func TestStrictExpandOffByDefault(t *testing.T) {
	t.Parallel()
	m, err := Parse("A=[${NOPE}][${vault:x}][$(id)]\n").ExpandedMap()
	if err != nil || m["A"] != "[][][$(id)]" {
		t.Fatalf("ExpandedMap = %v, %v", m, err)
	}
}

// TestStrictExpandNoValueInError: the value of a neighbouring secret never
// reaches the error text.
func TestStrictExpandNoValueInError(t *testing.T) {
	t.Parallel()
	_, _, err := Parse("SECRET=hunter2\nA=$SECRET${NOPE}\n", WithStrictExpand()).GetExpanded("A")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("err = %v", err)
	}
}
