package path

import (
	"errors"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Path
	}{
		{"", nil},
		{"a", Path{Key("a")}},
		{"sync.remote.url", Path{Key("sync"), Key("remote"), Key("url")}},
		{`a."b.c"[2]`, Path{Key("a"), Key("b.c"), Index(2)}},
		{"[0].name", Path{Index(0), Key("name")}},
		{"a[1][20]", Path{Key("a"), Index(1), Index(20)}},
		{`""`, Path{Key("")}},
		{`"q\"x\\y"`, Path{Key(`q"x\y`)}},
		{"ключ.名前", Path{Key("ключ"), Key("名前")}},
		{"a b.c-d", Path{Key("a b"), Key("c-d")}},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Parse(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		".a", "a.", "a..b", "a[", "a[x]", "a[-1]", "a[01]", "a[]", `a"b"`, `"unclosed`, `"bad\n"`,
		`"a"b`, "a.[0]", "a[0]b", "a[99999999999999999999]",
	} {
		_, err := Parse(in)
		if !errors.Is(err, ErrSyntax) {
			t.Errorf("Parse(%q) = %v, want ErrSyntax", in, err)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	t.Parallel()
	for _, p := range []Path{
		nil,
		{Key("a")},
		{Key("a.b"), Index(3), Key("")},
		{Index(0), Key(`x"y`), Key(`back\slash`), Key("br[ack]et")},
	} {
		s := p.String()
		got, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", s, err)
		}
		if !reflect.DeepEqual(got, p) && !(len(got) == 0 && len(p) == 0) {
			t.Errorf("Parse(%q) = %#v, want %#v", s, got, p)
		}
	}
}

func TestPointer(t *testing.T) {
	t.Parallel()
	got, err := ParsePointer("/a/b~1c/2/~0x/")
	if err != nil {
		t.Fatal(err)
	}
	want := Path{Key("a"), Key("b/c"), {Kind: KindEither, Key: "2", Index: 2}, Key("~x"), Key("")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParsePointer = %#v, want %#v", got, want)
	}
	if root, err := ParsePointer(""); err != nil || root != nil {
		t.Errorf("root pointer = %v, %v", root, err)
	}
	for _, bad := range []string{"a/b", "/a~2", "/a~"} {
		if _, err := ParsePointer(bad); !errors.Is(err, ErrSyntax) {
			t.Errorf("ParsePointer(%q) = %v, want ErrSyntax", bad, err)
		}
	}
	if s := (Path{Key("a"), Key("b/c"), Index(2), Key("~")}).Pointer(); s != "/a/b~1c/2/~0" {
		t.Errorf("Pointer = %q", s)
	}
	// A leading-zero token is a key, never an index.
	if p, _ := ParsePointer("/01"); p[0].Kind != KindKey {
		t.Errorf("/01 kind = %v, want KindKey", p[0].Kind)
	}
}

func TestParseAnyAndHelpers(t *testing.T) {
	t.Parallel()
	a, _ := ParseAny("/x/0")
	b, _ := ParseAny("x[0]")
	if a[1].Kind != KindEither || b[1].Kind != KindIndex {
		t.Errorf("ParseAny kinds = %v, %v", a[1].Kind, b[1].Kind)
	}
	if !a[1].MatchesIndex() || !a[1].MatchesKey() || Key("k").MatchesIndex() || Index(1).MatchesKey() {
		t.Error("Matches* wrong")
	}
	p := MustParse("a.b")
	parent, last, ok := p.Parent()
	if !ok || last != Key("b") || parent.String() != "a" {
		t.Errorf("Parent = %v %v %v", parent, last, ok)
	}
	q := parent.Append(Key("z"))
	if p.String() != "a.b" || q.String() != "a.z" {
		t.Errorf("Append aliased: p=%s q=%s", p, q)
	}
	if _, _, ok := Path(nil).Parent(); ok {
		t.Error("root has a parent")
	}
	defer func() {
		if recover() == nil {
			t.Error("MustParse did not panic")
		}
	}()
	MustParse("a..b")
}

// FuzzParseStringRoundTrip: whatever Parse accepts renders back to a string
// that parses to the same path; Pointer form round-trips too.
func FuzzParseStringRoundTrip(f *testing.F) {
	for _, s := range []string{"", "a", `a."b.c"[2]`, "[0].x", `"q\"\\"`, "a b.c", "/a/b~1c/0"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParseAny(s)
		if err != nil {
			return
		}
		back, err := Parse(p.String())
		if err != nil {
			t.Fatalf("Parse(%q.String()=%q): %v", s, p.String(), err)
		}
		if back.String() != p.String() {
			t.Fatalf("String not stable: %q -> %q", p.String(), back.String())
		}
		ptr, err := ParsePointer(p.Pointer())
		if err != nil || ptr.Pointer() != p.Pointer() {
			t.Fatalf("Pointer round trip of %q: %q, %v", s, p.Pointer(), err)
		}
	})
}

func TestStringRoundTripsThroughParseAny(t *testing.T) {
	for _, p := range []Path{
		{{Kind: KindKey, Key: "/a"}},
		{{Kind: KindKey, Key: "/"}, {Kind: KindKey, Key: "b"}},
		{{Kind: KindKey, Key: "x"}, {Kind: KindKey, Key: "/y"}},
	} {
		got, err := ParseAny(p.String())
		if err != nil {
			t.Fatalf("%q: %v", p.String(), err)
		}
		if got.String() != p.String() || len(got) != len(p) || got[0].Key != p[0].Key {
			t.Errorf("ParseAny(%q) = %#v, want %#v", p.String(), got, p)
		}
	}
}
