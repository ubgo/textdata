package jsonc

import (
	"errors"
	"testing"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/pos"
)

// TestStrictExpand: WithStrictExpand refuses unresolved references with a
// *pos.Error at the string's line:col wrapping the typed expand error.
// ds:def id=teststrictexpand-8h7qa9nr owner=@maintainers stability=stable
func TestStrictExpand(t *testing.T) {
	t.Parallel()
	src := `{
  // agx-like config
  "default": "claude",
  "token": "${TOKEN}",
  "fallback": "${NOPE:-none}",
  "bad": "x${NOPE}",
  "vault": "${vault:db}",
  "cmd": "$(gopass show x)"
}`
	lookup := WithLookup(func(n string) (string, bool) { return "t0k", n == "TOKEN" })
	d, err := Parse([]byte(src), WithFileName("config.jsonc"), WithStrictExpand(), lookup)
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{"token": "t0k", "fallback": "none", "default": "claude"} {
		got, err := d.GetExpanded(p)
		if err != nil || got != want {
			t.Errorf("GetExpanded(%s) = %v, %v; want %q", p, got, err, want)
		}
	}
	tests := []struct {
		path string
		at   string
	}{
		{"bad", "6:10"},
		{"vault", "7:12"},
		{"cmd", "8:10"},
	}
	for _, tt := range tests {
		_, err := d.GetExpanded(tt.path)
		var pe *pos.Error
		if !errors.As(err, &pe) || !errors.Is(err, expand.ErrStrict) {
			t.Fatalf("GetExpanded(%s) err = %v; want a positioned strict error", tt.path, err)
		}
		if pe.File != "config.jsonc" || pe.Pos.String() != tt.at {
			t.Errorf("GetExpanded(%s) at %s:%s, want config.jsonc:%s", tt.path, pe.File, pe.Pos, tt.at)
		}
	}
	if _, err := d.Expanded(); !errors.Is(err, expand.ErrStrict) {
		t.Errorf("Expanded err = %v", err)
	}
	var ue *expand.UnsetError
	if _, err := d.GetExpanded("bad"); !errors.As(err, &ue) || ue.Name != "NOPE" || ue.Offset != 1 {
		t.Errorf("UnsetError = %+v", ue)
	}
}

// TestStrictExpandOffByDefault: without the option an unresolved reference
// still reads as "" and a command stays verbatim.
func TestStrictExpandOffByDefault(t *testing.T) {
	t.Parallel()
	d, err := Parse([]byte(`{"a": "[${NOPE}][$(id)]"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetExpanded("a"); err != nil || got != "[][$(id)]" {
		t.Fatalf("GetExpanded = %v, %v", got, err)
	}
}
