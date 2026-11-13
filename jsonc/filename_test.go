package jsonc

import "testing"

func TestFileNameAndPosAfterEdit(t *testing.T) {
	d := mustParse(t, "{\"a\": 1}\n", WithFileName("conf.jsonc"))
	if got := d.FileName(); got != "conf.jsonc" {
		t.Fatalf("FileName = %q", got)
	}
	if p, _ := d.Pos("a"); p.Line != 1 || p.Col != 7 {
		t.Fatalf("before edit: %d:%d", p.Line, p.Col)
	}
	// The cached index must not survive an edit that moves "a" down a line.
	if err := d.SetBefore("a", "z", 2); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("z", Raw("{\n  \"y\": 1\n}")); err != nil {
		t.Fatal(err)
	}
	p, err := d.Pos("a")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := mustParse(t, string(d.Render())).Pos("a")
	if p != want {
		t.Fatalf("after edit: cached %+v, fresh %+v\n%s", p, want, d.Render())
	}
}
