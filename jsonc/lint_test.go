package jsonc

import (
	"fmt"
	"strings"
	"testing"
)

func TestLintRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		opts []LintOption
		want []string // "line:col rule"
	}{
		{"clean", "// head\n{\n  // about a\n  \"a\": 1, // one\n  \"b\": [\n    1,\n    {\"c\": null}\n  ],\n  \"d\": {}\n}\n", []LintOption{ForbidTrailingCommas()}, nil},
		{"clean tabs", "{\n\t\"a\": {\n\t\t\"b\": 1,\n\t},\n}", nil, nil},
		{"empty", "", nil, nil},
		{"duplicate key", "{\n  \"a\": 1,\n  \"b\": {\"x\": 1, \"x\": 2},\n  \"a\": 3\n}", nil, []string{"3:17 duplicate-key", "4:3 duplicate-key"}},
		{"trailing comma allowed by default", "{\"a\": [1,],}", nil, nil},
		{"trailing comma forbidden", "{\"a\": [1,],}", []LintOption{ForbidTrailingCommas()}, []string{"1:9 trailing-comma", "1:11 trailing-comma"}},
		{"tab and space in one indent", "{\n  \"a\": 1,\n \t\"b\": 2\n}", nil, []string{"3:1 mixed-indent"}},
		{"tabs in a space file", "{\n  \"a\": {\n\t\"b\": 2\n  }\n}", nil, []string{"3:1 mixed-indent"}},
		{"inconsistent width", "{\n  \"a\": 1,\n   \"b\": {\n      \"c\": 1\n   }\n}", nil, []string{"3:1 mixed-indent", "4:1 mixed-indent"}},
		{"unit taken from the file", "{\"a\": 1,\n \"b\": 2}", nil, nil},
		{"comment between key and colon", "{\"a\" /* c */: 1}", nil, []string{"1:6 odd-comment"}},
		{"comment between colon and value", "{\n  \"a\": // c\n    1\n}", nil, []string{"2:8 odd-comment"}},
		{"comment before comma", "[1 /* c */, 2]", nil, []string{"1:4 odd-comment"}},
		{"only selected rules", "{\"a\" /* c */: 1, \"a\": 2,}", []LintOption{LintOnly(LintDuplicateKey), ForbidTrailingCommas()}, []string{"1:18 duplicate-key"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := mustParse(t, tt.src)
			var got []string
			for _, is := range Lint(d, tt.opts...) {
				got = append(got, fmt.Sprintf("%s %s", is.Pos, is.Rule))
			}
			if strings.Join(got, ";") != strings.Join(tt.want, ";") {
				t.Errorf("Lint(%q) = %v, want %v", tt.src, Lint(d, tt.opts...), tt.want)
			}
			if string(d.Render()) != tt.src {
				t.Error("Lint changed the document")
			}
		})
	}
}

func TestLintIssueString(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\"a\": 1, \"a\": 2}", WithFileName("conf.jsonc"))
	is := Lint(d)
	if len(is) != 1 {
		t.Fatalf("issues = %v", is)
	}
	want := `conf.jsonc:1:10: duplicate key "a" (first at 1:2); the last one wins (duplicate-key)`
	if is[0].String() != want {
		t.Fatalf("String() = %q, want %q", is[0].String(), want)
	}
	if len(AllLintRules()) != 4 {
		t.Fatal("AllLintRules")
	}
}

// TestLintInvalid: a document with syntax errors is linted as far as it
// parsed, without panicking.
func TestLintInvalid(t *testing.T) {
	t.Parallel()
	d, err := Parse([]byte("{\"a\": 1, \"a\": 2, \"b\": "))
	if err == nil {
		t.Fatal("want a syntax error")
	}
	if is := Lint(d); len(is) != 1 || is[0].Rule != LintDuplicateKey {
		t.Fatalf("issues = %v", is)
	}
}

// FuzzLint: Lint never panics, never changes the document, and every issue
// points inside it.
func FuzzLint(f *testing.F) {
	seed(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		d, _ := Parse(b)
		for _, is := range Lint(d, ForbidTrailingCommas()) {
			if !is.Pos.IsValid() || is.Pos.Offset > len(b) {
				t.Fatalf("issue outside the input: %v", is)
			}
		}
		if string(d.Render()) != string(b) {
			t.Fatal("Lint changed the document")
		}
	})
}
