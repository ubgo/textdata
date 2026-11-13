// Command expand-engine drives the format-neutral ${VAR} engine from package
// expand directly: operators, nesting, $$, a ${scheme:ref} resolver, a fake
// $(cmd) runner, the cycle guard and RequiredError. Formats such as dotenv and
// jsonc wrap this same engine; using it bare shows what a format adds on top.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ubgo/textdata/expand"
)

// vaultScheme is the one scheme the demo resolver owns; every other scheme
// falls through to an ordinary variable lookup.
const vaultScheme = "vault"

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order and prints what each returns.
func run(w io.Writer) error {
	steps := []func(io.Writer) error{operators, schemes, commands, cycles, required}
	for _, step := range steps {
		if err := step(w); err != nil {
			return err
		}
	}
	return nil
}

// mapLookup is the smallest useful Lookup: values come from a map and expand
// in turn. A name already on the Stack reads as unset, which is what stops a
// cycle such as A=$B, B=$A; the engine itself only bounds depth.
//
//ds:def id=maplookup-bhxv9kp2
func mapLookup(vars map[string]string) func(string, *expand.Stack) (expand.Var, error) {
	return func(name string, st *expand.Stack) (expand.Var, error) {
		if _, busy := st.At(name); busy {
			return expand.Var{}, nil
		}
		v, ok := vars[name]
		return expand.Var{Value: v, Set: ok}, nil
	}
}

// operators shows the six ${VAR<op>arg} forms, a nested default and $$.
//
//ds:def id=operators-re38k7u7
func operators(w io.Writer) error {
	e := &expand.Engine{Lookup: mapLookup(map[string]string{
		"HOST":  "db.internal",
		"EMPTY": "",
		"URL":   "postgres://${HOST}:${PORT:-5432}/app",
	})}
	for _, in := range []string{
		"${URL}",                         // nested references expand in turn
		"${EMPTY-unset} ${EMPTY:-empty}", // '-' only when unset, ':-' also when empty
		"${HOST+set} [${EMPTY:+nonempty}]",
		"${MISSING:-${HOST:-x}}", // a default may itself hold a reference
		"price: $$5 for $HOST",   // $$ is a literal dollar
	} {
		out, err := e.Expand(in, nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-34s => %s\n", in, out)
	}
	return nil
}

// schemes resolves ${vault:ref} through Resolve. ok=false hands a token back
// to Lookup, so a scheme nobody owns reads as an ordinary (here unset) name.
//
//ds:def id=schemes-74khhwz9
func schemes(w io.Writer) error {
	e := &expand.Engine{
		Lookup: mapLookup(nil),
		Resolve: func(scheme, ref string) (string, bool, error) {
			if scheme != vaultScheme {
				return "", false, nil
			}
			return "secret-for-" + ref, true, nil
		},
	}
	out, err := e.Expand("${vault:db/password} [${other:thing}]", nil)
	if err != nil {
		return err
	}
	scheme, ref, ok := expand.SplitScheme("vault:db/password")
	fmt.Fprintf(w, "resolved: %s\nsplit: %q %q %v\n", out, scheme, ref, ok)
	r := expand.ParseRef("PORT:-5432")
	fmt.Fprintf(w, "parsed: name=%s default-if-empty=%v arg=%s\n", r.Name, r.Op == expand.OpDefaultEmpty, r.Arg)
	return nil
}

// commands wires a fake runner into Run. Without Run, $(...) is kept as
// written; a failing command is a *expand.CommandError naming the command.
//
//ds:def id=commands-7z2aksvq
func commands(w io.Writer) error {
	vars := map[string]string{"WHO": "world"}
	off := &expand.Engine{Lookup: mapLookup(vars)}
	on := &expand.Engine{
		Lookup: mapLookup(vars),
		Run: func(cmd string) (string, error) {
			if strings.HasPrefix(cmd, "fail") {
				return "", errors.New("exit status 1")
			}
			return strings.ToUpper(cmd), nil // stands in for a real shell
		},
	}
	in := "a $(echo $WHO) b [$(fail)]"
	offOut, err := off.Expand(in, nil)
	if err != nil {
		return err
	}
	onOut, err := on.Expand("a $(echo $WHO) b", nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "run off: %s\nrun on:  %s\n", offOut, onOut)
	_, err = on.Expand(in, nil)
	var ce *expand.CommandError
	if !errors.As(err, &ce) {
		return fmt.Errorf("want a *expand.CommandError, got %v", err)
	}
	fmt.Fprintf(w, "run fail: %v (cmd=%q)\n", err, ce.Cmd)
	return nil
}

// cycles expands a self-referencing set of values. The Stack carries the
// names being expanded; Enter pushes the entry being expanded so a reference
// back to it is seen by mapLookup as a cycle. MaxDepth backstops deep chains.
//
//ds:def id=cycles-fh85r3fe
func cycles(w io.Writer) error {
	e := &expand.Engine{Lookup: mapLookup(map[string]string{
		"A": "a-$B", "B": "b-$A", "SELF": "$SELF$SELF$SELF",
	})}
	st := expand.NewStack()
	leave := st.Enter("A", 0) // we are expanding A's own value
	a, err := e.Expand("a-$B", st)
	leave()
	if err != nil {
		return err
	}
	self, err := e.Expand("$SELF", nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "A => %q, SELF => %q, default depth %d\n", a, self, expand.DefaultMaxDepth)
	return nil
}

// required shows ${VAR:?msg}: an unset or empty variable is an error, never
// "". errors.As recovers the key, message and which form was used.
//
//ds:def id=required-q5qdggzy
func required(w io.Writer) error {
	e := &expand.Engine{Lookup: mapLookup(map[string]string{"TOKEN": ""})}
	_, err := e.Expand("${TOKEN:?set TOKEN in .env.local}", nil)
	var re *expand.RequiredError
	if !errors.As(err, &re) {
		return fmt.Errorf("expected a RequiredError, got %v", err)
	}
	fmt.Fprintf(w, "error: %v\nkey=%s empty=%v\n", err, re.Key, re.Empty)
	return nil
}
