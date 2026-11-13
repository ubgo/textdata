package jsonc

import "testing"

// TestMixedLineEndings pins the line ending of inserted text in files that
// mix LF and CRLF: it is the break where the text goes, or the document's
// first line break outside a comment, never a break inside a block comment.
// Where the edit adds a key, deleting it again must give back the input byte
// for byte.
func TestMixedLineEndings(t *testing.T) {
	tests := []struct {
		name, src, want string
		edit            func(d *Doc) error
		undo            func(d *Doc) error // nil: no byte-exact inverse to check
	}{
		{
			name: "empty object, CRLF only inside a block comment (fuzz seed 4db72ac8856a4209)",
			src:  "{/*\r\n*/\n}",
			want: "{/*\r\n*/\n  \"k\": 1\n}",
			edit: func(d *Doc) error { return d.Set("k", 1) },
			undo: func(d *Doc) error { return d.Delete("k") },
		},
		{
			name: "empty object, LF only inside a block comment",
			src:  "{/*\n*/\r\n}",
			want: "{/*\n*/\r\n  \"k\": 1\r\n}",
			edit: func(d *Doc) error { return d.Set("k", 1) },
			undo: func(d *Doc) error { return d.Delete("k") },
		},
		{
			name: "after a last member whose comment spans a CRLF",
			src:  "{\n  \"a\": 1 /* x\r\n  */\n}",
			want: "{\n  \"a\": 1, /* x\r\n  */\n  \"b\": 2\n}",
			edit: func(d *Doc) error { return d.Set("b", 2) },
			undo: func(d *Doc) error { return d.Delete("b") },
		},
		{
			name: "after a last member with a comma whose comment spans a CRLF",
			src:  "{\n  \"a\": 1, /* x\r\n  */\n}",
			want: "{\n  \"a\": 1, /* x\r\n  */\n  \"b\": 2,\n}",
			edit: func(d *Doc) error { return d.Set("b", 2) },
			undo: func(d *Doc) error { return d.Delete("b") },
		},
		{
			name: "before a member that follows a CRLF block comment",
			src:  "{\n  \"a\": 1, /* x\r\n  */\n  \"c\": 3 /* y\r\n */\n}",
			want: "{\n  \"a\": 1, /* x\r\n  */\n  \"b\": 2,\n  \"c\": 3 /* y\r\n */\n}",
			edit: func(d *Doc) error { return d.SetBefore("c", "b", 2) },
			undo: func(d *Doc) error { return d.Delete("b") },
		},
		{
			name: "array append after an element whose comment spans a CRLF",
			src:  "[\n  1 /* x\r\n  */\n]",
			want: "[\n  1, /* x\r\n  */\n  2\n]",
			edit: func(d *Doc) error { return d.Append("", 2) },
			undo: func(d *Doc) error { return d.Remove("", 1) },
		},
		{
			name: "multi-line value in an LF file whose first break is in a comment",
			src:  "/*\r\n*/\n{\n  \"a\": 1\n}\n",
			want: "/*\r\n*/\n{\n  \"a\": {\n    \"x\": 1\n  }\n}\n",
			edit: func(d *Doc) error { return d.Set("a", map[string]any{"x": 1}) },
		},
		{
			name: "comment above a member that follows a CRLF block comment",
			src:  "{\n  \"a\": 1, /* x\r\n  */\n  \"c\": 3\n}",
			want: "{\n  \"a\": 1, /* x\r\n  */\n  // note\n  \"c\": 3\n}",
			edit: func(d *Doc) error { return d.SetComment("c", "note") },
			undo: func(d *Doc) error { return d.RemoveComment("c") },
		},
		{
			name: "plain CRLF file",
			src:  "{\r\n  \"a\": 1\r\n}\r\n",
			want: "{\r\n  \"a\": 1,\r\n  \"b\": 2\r\n}\r\n",
			edit: func(d *Doc) error { return d.Set("b", 2) },
			undo: func(d *Doc) error { return d.Delete("b") },
		},
		{
			name: "plain LF file",
			src:  "{\n  \"a\": 1\n}\n",
			want: "{\n  \"a\": 1,\n  \"b\": 2\n}\n",
			edit: func(d *Doc) error { return d.Set("b", 2) },
			undo: func(d *Doc) error { return d.Delete("b") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.edit(d); err != nil {
				t.Fatal(err)
			}
			if got := string(d.Render()); got != tt.want {
				t.Fatalf("edit:\n got %q\nwant %q", got, tt.want)
			}
			if tt.undo == nil {
				return
			}
			if err := tt.undo(d); err != nil {
				t.Fatal(err)
			}
			if got := string(d.Render()); got != tt.src {
				t.Fatalf("undo did not restore the input:\n got %q\nwant %q", got, tt.src)
			}
		})
	}
}

// TestDocEOLSkipsComments pins the document line ending: the first line
// break outside a comment, else the first one, else LF.
func TestDocEOLSkipsComments(t *testing.T) {
	for src, want := range map[string]string{
		"/*\r\n*/\n{}":  "\n",
		"// a\r\n{}\n":  "\r\n",
		"/*\n*/\r\n{}":  "\r\n",
		"/*\r\n*/{}":    "\r\n",
		"{}":            "\n",
		"{\r\n}\n":      "\r\n",
		"{\"a\":1}\r\n": "\r\n",
	} {
		d, _ := Parse([]byte(src))
		if got := d.docEOL(); got != want {
			t.Errorf("docEOL(%q) = %q, want %q", src, got, want)
		}
	}
}
