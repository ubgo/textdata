package expand

import (
	"errors"
	"strings"
	"testing"
)

// mapEngine is an Engine over a fixed map: values expand, a cycle back to a
// name on the stack reads as unset (the simplest self-reference rule).
func mapEngine(vars map[string]string) *Engine {
	return &Engine{Lookup: func(name string, st *Stack) (Var, error) {
		if _, on := st.At(name); on {
			return Var{}, nil
		}
		v, ok := vars[name]
		return Var{Value: v, Set: ok}, nil
	}}
}

func TestExpandForms(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"A": "a", "EMPTY": "", "NESTED": "<${A}>", "HOST": "db", "PORT": "5432"}
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"$A", "a"},
		{"${A}", "a"},
		{"x${A}y$A.z", "xaya.z"},
		{"$$A", "$A"},
		{"cost $", "cost $"},
		{"$1 $- $", "$1 $- $"},
		{"${UNSET}", ""},
		{"${UNSET-d}", "d"},
		{"${EMPTY-d}", ""},
		{"${EMPTY:-d}", "d"},
		{"${A:-d}", "a"},
		{"${A+alt}", "alt"},
		{"${UNSET+alt}", ""},
		{"${EMPTY+alt}", "alt"},
		{"${EMPTY:+alt}", ""},
		{"${A:+alt}", "alt"},
		{"${UNSET:-${A:-x}}", "a"},
		{"${UNSET:-${NOPE:-x}}", "x"},
		{"${NESTED}", "<a>"},
		{"${unclosed", "${unclosed"},
		{"$(unclosed", "$(unclosed"},
		{"$(echo hi)", "$(echo hi)"}, // Run is nil: verbatim
		{"postgres://${HOST}:${PORT}/x", "postgres://db:5432/x"},
	}
	e := mapEngine(vars)
	for _, tt := range tests {
		got, err := e.Expand(tt.in, nil)
		if err != nil || got != tt.want {
			t.Errorf("Expand(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestRequired(t *testing.T) {
	t.Parallel()
	e := mapEngine(map[string]string{"EMPTY": ""})
	tests := []struct {
		in    string
		msg   string
		empty bool
		text  string
	}{
		{"${X?boom}", "boom", false, "expand: required variable X is not set: boom"},
		{"${X:?}", "", true, "expand: required variable X is not set or is empty"},
		{"${EMPTY:?need it}", "need it", true, "expand: required variable EMPTY is not set or is empty: need it"},
	}
	for _, tt := range tests {
		_, err := e.Expand(tt.in, nil)
		var re *RequiredError
		if !errors.As(err, &re) || re.Message != tt.msg || re.Empty != tt.empty || err.Error() != tt.text {
			t.Errorf("Expand(%q) err = %v", tt.in, err)
		}
	}
	if got, err := e.Expand("${EMPTY?x}", nil); err != nil || got != "" {
		t.Errorf("? on a set-but-empty var = %q, %v", got, err)
	}

	custom := errors.New("custom")
	e.Required = func(name, msg string, empty bool) error { return custom }
	if _, err := e.Expand("${X?}", nil); err != custom {
		t.Errorf("custom Required = %v", err)
	}
}

func TestCyclesTerminate(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"A": "$A$A$A", "B": "${C}", "C": "${B}x", "D": "${D:-dflt}"}
	e := mapEngine(vars)
	for in, want := range map[string]string{"$A": "", "$B": "x", "$D": "dflt"} {
		got, err := e.Expand(in, nil)
		if err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	// Without any cycle rule, the depth bound still stops a deep chain.
	deep := &Engine{MaxDepth: 4, Lookup: func(name string, _ *Stack) (Var, error) {
		return Var{Value: "+$X", Set: true}, nil
	}}
	got, err := deep.Expand("$X", nil)
	if err != nil || got != "++++$X" {
		t.Errorf("depth-bounded = %q, %v", got, err)
	}
}

func TestStackSeesPositions(t *testing.T) {
	t.Parallel()
	// Lookup sees the At pushed for the name being expanded.
	var seen []int
	e := &Engine{Lookup: func(name string, st *Stack) (Var, error) {
		if at, ok := st.At(name); ok {
			seen = append(seen, at)
			return Var{}, nil
		}
		return Var{Value: "${" + name + "}", Set: true, At: 7}, nil
	}}
	st := NewStack()
	leave := st.Enter("OUTER", 3)
	if _, err := e.Expand("${N}", st); err != nil {
		t.Fatal(err)
	}
	leave()
	if len(seen) != 1 || seen[0] != 7 {
		t.Errorf("seen = %v, want [7]", seen)
	}
	if _, ok := st.At("OUTER"); ok {
		t.Error("leave did not pop")
	}
	// Re-entering restores the previous position.
	l1 := st.Enter("X", 1)
	l2 := st.Enter("X", 2)
	l2()
	if at, _ := st.At("X"); at != 1 {
		t.Errorf("after inner leave At = %d, want 1", at)
	}
	l1()
}

func TestLiteralAndLookupErrors(t *testing.T) {
	t.Parallel()
	e := &Engine{Lookup: func(name string, _ *Stack) (Var, error) {
		switch name {
		case "BAD":
			return Var{}, errors.New("store down")
		case "X":
			return Var{}, nil
		}
		return Var{Value: "${BAD}", Set: true, Literal: true}, nil
	}}
	if got, err := e.Expand("$LIT", nil); err != nil || got != "${BAD}" {
		t.Errorf("literal = %q, %v", got, err)
	}
	if _, err := e.Expand("$BAD", nil); err == nil || err.Error() != "store down" {
		t.Errorf("lookup error = %v", err)
	}
	if _, err := e.Expand("${X:-$BAD}", nil); err == nil {
		t.Error("error inside a default was swallowed")
	}
}

func TestCommands(t *testing.T) {
	t.Parallel()
	var ran []string
	e := mapEngine(map[string]string{"WHO": "world"})
	e.Run = func(cmd string) (string, error) {
		ran = append(ran, cmd)
		if cmd == "fail" {
			return "", errors.New("exit 1")
		}
		return strings.ToUpper(cmd), nil
	}
	got, err := e.Expand("a $(echo $WHO) b c $(x(y)z)", nil)
	if err != nil || got != "a ECHO WORLD b c X(Y)Z" {
		t.Errorf("Expand = %q, %v", got, err)
	}
	if _, err := e.Expand("b $(fail) c", nil); err == nil {
		t.Error("a failing command expanded without an error")
	}
	if strings.Join(ran, "|") != "echo world|x(y)z|fail" {
		t.Errorf("ran = %q", ran)
	}
	e.Lookup = func(string, *Stack) (Var, error) { return Var{}, errors.New("down") }
	if _, err := e.Expand("$(echo $X)", nil); err == nil {
		t.Error("lookup error inside a command was swallowed")
	}
}

func TestSchemes(t *testing.T) {
	t.Parallel()
	e := mapEngine(map[string]string{"gopass:x": "from-file"})
	// Without a resolver a scheme-shaped token is an ordinary name.
	if got, _ := e.Expand("${gopass:x}", nil); got != "from-file" {
		t.Errorf("no resolver = %q", got)
	}
	var observed []string
	e.Observe = func(name, v string) { observed = append(observed, name+"="+v) }
	e.Resolve = func(scheme, ref string) (string, bool, error) {
		switch scheme {
		case "gopass":
			return "secret(" + ref + ")", true, nil
		case "broken":
			return "", true, errors.New("unavailable")
		}
		return "", false, nil
	}
	tests := map[string]string{
		"${gopass:personal/db}": "secret(personal/db)",
		"${GoPass:a-b}":         "secret(a-b)",
		"${other:thing}":        "", // not claimed: ordinary (unset) name
		"${gopass:x}":           "secret(x)",
	}
	for in, want := range tests {
		if got, err := e.Expand(in, nil); err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := e.Expand("${broken:x}", nil); err == nil {
		t.Error("resolver error swallowed")
	}
	if !strings.Contains(strings.Join(observed, "|"), "gopass:personal/db=secret(personal/db)") {
		t.Errorf("observed = %q", observed)
	}
}

func TestSplitSchemeAndParseRef(t *testing.T) {
	t.Parallel()
	for tok, want := range map[string]bool{
		"gopass:a": true, "s3+x.y-z:k": true, "a:": true,
		"VAR:-d": false, "VAR:+a": false, "VAR:?e": false, ":x": false, "1a:x": false, "a b:x": false, "plain": false,
	} {
		if _, _, ok := SplitScheme(tok); ok != want {
			t.Errorf("SplitScheme(%q) ok = %v, want %v", tok, ok, want)
		}
	}
	tests := []struct {
		tok  string
		want Ref
	}{
		{"VAR", Ref{Name: "VAR"}},
		{"VAR:-d", Ref{"VAR", OpDefaultEmpty, "d"}},
		{"VAR-d", Ref{"VAR", OpDefault, "d"}},
		{"VAR:+a", Ref{"VAR", OpAlternateSet, "a"}},
		{"VAR+a", Ref{"VAR", OpAlternate, "a"}},
		{"VAR:?e", Ref{"VAR", OpRequiredSet, "e"}},
		{"VAR?e", Ref{"VAR", OpRequired, "e"}},
		{"VAR:-a-b", Ref{"VAR", OpDefaultEmpty, "a-b"}},
	}
	for _, tt := range tests {
		if got := ParseRef(tt.tok); got != tt.want {
			t.Errorf("ParseRef(%q) = %+v, want %+v", tt.tok, got, tt.want)
		}
	}
	if MatchBrace("{a${b}c}") != 7 || MatchBrace("{x") != -1 || MatchParen("(a(b)c)") != 6 || MatchParen("(") != -1 {
		t.Error("Match*")
	}
}

func FuzzExpandTerminates(f *testing.F) {
	for _, s := range []string{"$A", "${A:-${B:-$C}}", "$$", "$(x)", "${gopass:x}", "${", "$A$A$A"} {
		f.Add(s, "$A$B", "${A}x")
	}
	f.Fuzz(func(t *testing.T, s, a, b string) {
		e := mapEngine(map[string]string{"A": a, "B": b, "C": s})
		e.Run = func(cmd string) (string, error) { return cmd, nil }
		e.Resolve = func(scheme, ref string) (string, bool, error) { return ref, scheme == "gopass", nil }
		_, _ = e.Expand(s, nil) // must return, not hang or panic
	})
}

// TestCommandFailureIsAnError pins that a $(command) whose runner fails is an
// error naming the command, never a silent "": a refused or crashed command
// must not turn into an empty password.
func TestCommandFailureIsAnError(t *testing.T) {
	cause := errors.New("exit status 1")
	e := &Engine{
		Lookup: func(string, *Stack) (Var, error) { return Var{}, nil },
		Run: func(cmd string) (string, error) {
			if cmd == "ok" {
				return "fine", nil
			}
			return "", cause
		},
	}
	got, err := e.Expand("a-$(ok)-$(boom now)-b", nil)
	if err == nil {
		t.Fatalf("Expand = %q, nil error; want a *CommandError", got)
	}
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("error %v (%T) is not a *CommandError", err, err)
	}
	if ce.Cmd != "boom now" {
		t.Errorf("Cmd = %q, want %q", ce.Cmd, "boom now")
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, runner error) = false")
	}
	if want := `expand: command "boom now" failed: exit status 1`; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if got != "" {
		t.Errorf("value on error = %q, want empty", got)
	}
}
