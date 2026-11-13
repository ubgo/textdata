package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `--- all rules, trailing commas forbidden ---
app.jsonc:3:3: duplicate key "port" (first at 2:3); the last one wins (duplicate-key)
app.jsonc:4:1: indented 1 tab, expected 2 spaces (the file indents by 2 spaces) (mixed-indent)
app.jsonc:5:11: comment between key "name" and its value; formatters and other tools may move or drop it (odd-comment)
app.jsonc:5:34: trailing comma after the last member (trailing-comma)
--- duplicate keys only ---
rule=duplicate-key line=3 col=3 msg="duplicate key \"port\" (first at 2:3); the last one wins"
rules available: [duplicate-key trailing-comma mixed-indent odd-comment]
validate: jsonc: plugin "require-owner": validate: missing required key owner
is errMissingOwner: true
after Set, validate: <nil>
`

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != want {
		t.Errorf("output changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
