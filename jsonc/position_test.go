package jsonc

import "testing"

func TestPos(t *testing.T) {
	d := mustParse(t, "{\n  // c\n  \"a\": {\"b\": [1, 22]}\n}\n")
	cases := []struct {
		path      string
		key       bool
		line, col int
	}{
		{"a", false, 3, 8},
		{"a", true, 3, 3},
		{"a.b[1]", false, 3, 18},
		{"a.b[1]", true, 3, 18},
		{"", false, 1, 1},
	}
	for _, c := range cases {
		get := d.Pos
		if c.key {
			get = d.KeyPos
		}
		p, err := get(c.path)
		if err != nil {
			t.Fatalf("%q: %v", c.path, err)
		}
		if p.Line != c.line || p.Col != c.col {
			t.Errorf("%q key=%v: got %d:%d, want %d:%d", c.path, c.key, p.Line, p.Col, c.line, c.col)
		}
	}
	if _, err := d.Pos("nope"); !errorsIs(err, ErrNotFound) {
		t.Errorf("missing path: %v", err)
	}
}
