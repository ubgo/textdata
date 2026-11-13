package jsonc

import "testing"

// TestRestoreBeforeLiveMember: a member disabled without a comma, with a live
// member after it, comes back with the comma it needs.
func TestRestoreBeforeLiveMember(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  \"a\": 1,\n  // \"b\": 2\n  \"c\": 3\n}")
	if err := d.Restore("b"); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Render()), "{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": 3\n}"; got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// A comment that only looks like the start of a member is skipped.
	d = mustParse(t, "{\n  \"a\": 1,\n  // \"b\" is documented here\n  // \"b\": 2\n}")
	if err := d.Restore("b"); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Render()), "{\n  \"a\": 1,\n  // \"b\" is documented here\n  \"b\": 2\n}"; got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestDisableArrayElement(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "[\n  1\n]")
	if err := d.Disable("[0]"); !errorsIs(err, ErrType) {
		t.Errorf("Disable(element) = %v", err)
	}
}

func TestDisableCommaOnOwnLine(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  \"a\": 1\n  ,\n  \"b\": 2\n}")
	if err := d.Disable("a"); !errorsIs(err, ErrLayout) {
		t.Errorf("Disable(comma on its own line) = %v", err)
	}
}
