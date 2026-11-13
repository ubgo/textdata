// Command lint checks a JSONC file that parses but has mistakes: a duplicate
// key, a mixed indent, a comment where tools lose it, trailing commas for a
// consumer that rejects them. It then runs a Validator plugin through
// Validate, the hook for rules of your own.
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
)

// fileName is the name every Issue reports.
const fileName = "app.jsonc"

// src parses fine, which is why these mistakes are easy to miss: "port" is
// written twice (the second one wins), "debug" is indented with a tab, and a
// comment sits between a key and its value.
//
//ds:def id=src-xm6dgddf
const src = "{\n" +
	"  \"port\": 8080,\n" +
	"  \"port\": 9090,\n" +
	"\t\"debug\": true,\n" +
	"  \"name\": /* service */ \"billing\",\n" +
	"}\n"

// requiredKey is the key the example Validator insists on.
const requiredKey = "owner"

func main() {
	if err := run(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order.
func run(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(src), jsonc.WithFileName(fileName))
	if err != nil {
		return err
	}
	lintAll(w, doc)
	lintSome(w, doc)
	return validate(w)
}

// lintAll runs every rule. Trailing commas are legal JSONC, so the
// trailing-comma rule reports only with ForbidTrailingCommas.
//
//ds:def id=lintall-nxrpgbfp
func lintAll(w io.Writer, doc *jsonc.Doc) {
	fmt.Fprintln(w, "--- all rules, trailing commas forbidden ---")
	for _, issue := range jsonc.Lint(doc, jsonc.ForbidTrailingCommas()) {
		fmt.Fprintln(w, issue)
	}
}

// lintSome picks rules with LintOnly, for a CI gate that cares about one
// kind of mistake. Issue fields are there for your own reporting.
//
//ds:def id=lintsome-ey8ud5cw
func lintSome(w io.Writer, doc *jsonc.Doc) {
	fmt.Fprintln(w, "--- duplicate keys only ---")
	for _, issue := range jsonc.Lint(doc, jsonc.LintOnly(jsonc.LintDuplicateKey)) {
		fmt.Fprintf(w, "rule=%s line=%d col=%d msg=%q\n", issue.Rule, issue.Pos.Line, issue.Pos.Col, issue.Msg)
	}
	fmt.Fprintln(w, "rules available:", jsonc.AllLintRules())
}

// errMissingOwner is the Validator's finding, matchable with errors.Is.
//
//ds:def id=errmissingowner-8q6x3kye
var errMissingOwner = errors.New("missing required key " + requiredKey)

// requireOwner is a Validator plugin: a rule of your own over the read-only
// plugin.Document view. It runs only when you call Validate, never on read.
//
//ds:def id=requireowner-rd48xv57
type requireOwner struct{}

//ds:def id=requireowner-name-regush65
func (requireOwner) Name() string { return "require-owner" }

//ds:def id=requireowner-validate-84ecf37m
func (requireOwner) Validate(doc plugin.Document) error {
	if _, ok := doc.ValueAt(requiredKey); !ok {
		return errMissingOwner
	}
	return nil
}

// validate installs the Validator and runs it on a document without, then
// with, the required key.
//
//ds:def id=validate-tnfzj9rp
func validate(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(`{"name": "billing"}`), jsonc.WithPlugin(requireOwner{}))
	if err != nil {
		return err
	}
	verr := doc.Validate()
	fmt.Fprintln(w, "validate:", verr)
	fmt.Fprintln(w, "is errMissingOwner:", errors.Is(verr, errMissingOwner))
	if err := doc.Set(requiredKey, "payments-team"); err != nil {
		return err
	}
	fmt.Fprintln(w, "after Set, validate:", doc.Validate())
	return nil
}
