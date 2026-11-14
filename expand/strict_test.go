package expand

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// strictEngine is mapEngine with Strict on and a resolver owning "store".
func strictEngine(vars map[string]string) *Engine {
	e := mapEngine(vars)
	e.Strict = true
	e.Resolve = func(scheme, ref string) (string, bool, error) {
		if scheme != "store" {
			return "", false, nil
		}
		return "S(" + ref + ")", true, nil
	}
	return e
}

// TestStrictKeepsResolvedForms: every form that resolves, or whose operator
// says what unset means, reads exactly as it does outside strict mode.
func TestStrictKeepsResolvedForms(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"A": "a", "EMPTY": "", "NESTED": "<${A}>", "vault:kept": "defined"}
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"$A ${A}", "a a"},
		{"${EMPTY}", ""}, // set to "" is set
		{"${UNSET-d}", "d"},
		{"${UNSET:-d}", "d"},
		{"${EMPTY:-d}", "d"},
		{"${UNSET+alt}", ""},
		{"${UNSET:+alt}", ""},
		{"${UNSET:-${A}}", "a"},
		{"${NESTED}", "<a>"},
		{"$$UNSET", "$UNSET"},
		{"cost $", "cost $"},
		{"${unclosed", "${unclosed"},
		{"$(unclosed", "$(unclosed"},
		{"${store:db}", "S(db)"},
		{"${vault:kept}", "defined"}, // an unclaimed scheme token that a source defines
	}
	e := strictEngine(vars)
	for _, tt := range tests {
		got, err := e.Expand(tt.in, nil)
		if err != nil || got != tt.want {
			t.Errorf("strict Expand(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

// TestStrictRefuses pins each error strict mode adds, with the name and the
// offset of the outermost reference, and that each matches ErrStrict.
// ds:def id=teststrictrefuses-8m4ujeu7 owner=@maintainers stability=stable
func TestStrictRefuses(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"A": "a", "CHAIN": "x${MISSING}", "CMDV": "$(id)"}
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"bare", "$NOPE", &UnsetError{Name: "NOPE", Offset: 0}},
		{"braced", "ab ${NOPE}", &UnsetError{Name: "NOPE", Offset: 3}},
		{"after a resolved one", "$A-$NOPE", &UnsetError{Name: "NOPE", Offset: 3}},
		{"in a default", "${UNSET:-$NOPE}", &UnsetError{Name: "NOPE", Offset: 0}},
		{"in a value", "k=${CHAIN}", &UnsetError{Name: "MISSING", Offset: 2, Via: "CHAIN"}},
		{"unclaimed scheme", "t=${vault:db}", &UnknownSchemeError{Scheme: "vault", Ref: "db", Offset: 2}},
		{"typo scheme", "${gopas:work/api}", &UnknownSchemeError{Scheme: "gopas", Ref: "work/api", Offset: 0}},
		{"scheme ref with operator chars", "${vault:db-pass}", &UnknownSchemeError{Scheme: "vault", Ref: "db-pass", Offset: 0}},
		{"command off", "tok=$(gopass show x)", &CommandOffError{Cmd: "gopass show x", Offset: 4}},
		{"command off in a value", "${CMDV}", &CommandOffError{Cmd: "id", Offset: 0, Via: "CMDV"}},
	}
	e := strictEngine(vars)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.Expand(tt.in, nil)
			if err == nil {
				t.Fatalf("Expand(%q) = %q, nil; want %v", tt.in, got, tt.want)
			}
			if !errors.Is(err, ErrStrict) {
				t.Errorf("errors.Is(%v, ErrStrict) = false", err)
			}
			if !reflect.DeepEqual(err, tt.want) {
				t.Errorf("error = %#v, want %#v", err, tt.want)
			}
		})
	}
}

// TestStrictKeepsRequiredError: ${VAR:?msg} keeps its own error type in
// strict mode; it is not an *UnsetError.
func TestStrictKeepsRequiredError(t *testing.T) {
	t.Parallel()
	_, err := strictEngine(nil).Expand("${NOPE:?set it}", nil)
	var re *RequiredError
	if !errors.As(err, &re) || re.Message != "set it" {
		t.Fatalf("err = %v, want *RequiredError", err)
	}
	if errors.Is(err, ErrStrict) {
		t.Error("a RequiredError must not match ErrStrict")
	}
}

// TestStrictErrorsNeverCarryValues: a strict error message names the variable
// and position only; the values of the variables around it never appear.
func TestStrictErrorsNeverCarryValues(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t-value"
	vars := map[string]string{"TOKEN": secret, "CHAIN": secret + "${MISSING}"}
	for _, in := range []string{"$TOKEN ${NOPE}", "${CHAIN}"} {
		_, err := strictEngine(vars).Expand(in, nil)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("Expand(%q) err = %v; want an error without the value", in, err)
		}
	}
}

// TestNotStrictUnchanged: the zero Engine still reads every refused form the
// Compose way, so existing callers see no change.
func TestNotStrictUnchanged(t *testing.T) {
	t.Parallel()
	e := mapEngine(nil)
	e.Resolve = func(string, string) (string, bool, error) { return "", false, nil }
	for in, want := range map[string]string{"$NOPE": "", "${vault:db}": "", "${vault:db-pass}": "pass", "$(id)": "$(id)"} {
		got, err := e.Expand(in, nil)
		if err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
