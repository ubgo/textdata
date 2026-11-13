// Command expansion shows how GetExpanded and ExpandedMap resolve ${VAR},
// $VAR, the Compose operators, $$, nesting, self-references and cycles, which
// names a reference can reach, and opt-in $(command) substitution with a fake
// runner. Nothing reads the process environment or runs a real command.
//
// Run it with `go run .`; nothing touches the disk.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ubgo/textdata/dotenv"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// step names one stage of the walk-through and the function that prints it.
type step struct {
	title string
	show  func(io.Writer) error
}

// run prints every step under a heading so the test can pin the whole output.
func run(w io.Writer) error {
	steps := []step{
		{"Get vs GetExpanded", getVsExpanded},
		{"operators", operators},
		{"dollars", dollars},
		{"nesting, self-reference, cycles", nesting},
		{"reachable names", names},
		{"ExpandedMap and RequiredError", requiredVars},
		{"command substitution", commands},
	}
	for _, s := range steps {
		fmt.Fprintln(w, "==", s.title)
		if err := s.show(w); err != nil {
			return err
		}
	}
	return nil
}

// expandAll prints GetExpanded for every active key of f, with %q so empty
// values are visible.
func expandAll(w io.Writer, f *dotenv.File) error {
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s=%q\n", key, v)
	}
	return nil
}

// getVsExpanded shows that Get returns the text on disk and GetExpanded
// resolves it against the same file. A reference to a key that does not exist
// is "", never an error, and a reference may point further down the file.
//
//ds:def id=getvsexpanded-f3wd8k2m
func getVsExpanded(w io.Writer) error {
	f := dotenv.Parse("URL=postgres://${HOST}:$PORT/app\nHOST=db\nPORT=5432\nMISSING=[${NOPE}]\n")
	raw, _ := f.Get("URL")
	url, ok, err := f.GetExpanded("URL")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Get=%s\nGetExpanded=%s ok=%v\n", raw, url, ok)
	missing, _, err := f.GetExpanded("MISSING")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "MISSING:", missing)
	return nil
}

// operators prints each Compose operator against a key that is set, one that
// is set but empty, and one that is absent. The colon forms treat empty like
// unset.
//
//ds:def id=operators-x9qm4t7c
func operators(w io.Writer) error {
	forms := []string{"${%s:-def}", "${%s-def}", "${%s:+alt}", "${%s+alt}"}
	var b strings.Builder
	b.WriteString("SET=value\nEMPTY=\n")
	for _, name := range []string{"SET", "EMPTY", "UNSET"} {
		for i, form := range forms {
			fmt.Fprintf(&b, "%s_%d=%s\n", name, i, fmt.Sprintf(form, name))
		}
	}
	f := dotenv.Parse(b.String())
	for _, key := range f.Keys()[2:] {
		raw, _ := f.Get(key)
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-14s -> %q\n", raw, v)
	}
	return nil
}

// dollars shows how the character after `$` decides its meaning: `$$` is one
// literal dollar, `$` before a non-name character is literal, and a backslash
// does not escape a dollar.
//
//ds:def id=dollars-k6vb3n9s
func dollars(w io.Writer) error {
	src := "NAME=world\n" +
		"COST=cost 5$$\n" +
		"TEMPLATE=$${NAME}\n" +
		"PRICE=$5 or 100$\n" +
		"UNCLOSED=${NAME\n" +
		"BACKSLASH=\"\\$NAME\"\n"
	return expandAll(w, dotenv.Parse(src))
}

// nesting shows chains, a default that is itself a reference, a key that
// extends its own earlier definition (Compose's top-to-bottom rule) and a
// two-key cycle, which ends instead of hanging.
//
//ds:def id=nesting-b8rj5w2x
func nesting(w io.Writer) error {
	src := "A=1\n" +
		"B=${A}0\n" +
		"C=${B}0\n" +
		"D=${NOPE:-${C}}\n" +
		"LIST=one\n" +
		"LIST=${LIST},two\n" +
		"CYCLE_A=a${CYCLE_B}\n" +
		"CYCLE_B=b${CYCLE_A}\n"
	return expandAll(w, dotenv.Parse(src))
}

// names shows which keys a reference can reach: a bare $name stops at the
// first non-name character, and inside braces a dash is the default operator,
// so a dashed key cannot be referenced.
//
//ds:def id=names-q2hc7m4z
func names(w io.Writer) error {
	src := "spring.url=jdbc\n" +
		"MY-KEY=dashed\n" +
		"BARE=$spring.url\n" +
		"BRACED=${spring.url}\n" +
		"DASHED=${MY-KEY}\n" +
		"LITERAL='${spring.url}'\n"
	f := dotenv.Parse(src)
	for _, key := range []string{"BARE", "BRACED", "DASHED", "LITERAL"} {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s=%q\n", key, v)
	}
	return nil
}

// requiredVars shows ${VAR:?message}: ExpandedMap returns the first failure in
// file order and no map, and errors.As reads the *RequiredError fields.
//
//ds:def id=requiredvars-n7tf2c5k
func requiredVars(w io.Writer) error {
	ok := dotenv.Parse("TOKEN=t-123\nAUTH=Bearer ${TOKEN:?set TOKEN}\n")
	m, err := ok.ExpandedMap()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "AUTH:", m["AUTH"])

	bad := dotenv.Parse("TOKEN=\nAUTH=Bearer ${TOKEN:?set TOKEN}\nDB=${DB_URL?}\n")
	m, err = bad.ExpandedMap()
	var req *dotenv.RequiredError
	if errors.As(err, &req) {
		fmt.Fprintf(w, "nil map: %v key=%s message=%q empty=%v\n", m == nil, req.Key, req.Message, req.Empty)
		fmt.Fprintln(w, err)
	}
	return nil
}

// allowed is the fake runner's allow-list: the only commands it answers.
var allowed = map[string]string{"git rev-parse --short HEAD": "a1b2c3d"}

// commands shows that $(...) is kept as written by default, and that a runner
// installed with WithCommandRunner turns substitution on. The command text is
// expanded first; a refused command is an error naming the command.
//
//ds:def id=commands-w5pk9d3r
func commands(w io.Writer) error {
	src := "GIT=git\nREV=$(${GIT} rev-parse --short HEAD)\nWHO=$(whoami)\nLIT='$(whoami)'\n"
	off := dotenv.Parse(src)
	rev, _, err := off.GetExpanded("REV")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "off: REV =", rev)

	runner := func(cmd string) (string, error) {
		out, ok := allowed[cmd]
		if !ok {
			return "", fmt.Errorf("command %q not allowed", cmd)
		}
		return out, nil
	}
	on := dotenv.Parse(src, dotenv.WithCommandRunner(runner))
	for _, key := range on.Keys() {
		v, _, err := on.GetExpanded(key)
		if err != nil {
			fmt.Fprintf(w, "%s error: %v\n", key, err)
			continue
		}
		fmt.Fprintf(w, "%s=%q\n", key, v)
	}
	// Options apply in order: a later switch-off wins over the runner.
	later := dotenv.Parse(src, dotenv.WithCommandRunner(runner), dotenv.WithCommandSubstitution(false))
	if rev, _, err = later.GetExpanded("REV"); err != nil {
		return err
	}
	fmt.Fprintln(w, "runner then off: REV =", rev)
	return nil
}
