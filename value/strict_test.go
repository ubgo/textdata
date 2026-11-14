package value

import (
	"context"
	"errors"
	"testing"

	"github.com/ubgo/textdata/expand"
)

// TestStrict: with WithStrict every unresolved reference is a typed error naming
// it and its offset; the forms that resolve read as without it.
// ds:def id=teststrict-uemkh2yq owner=@maintainers stability=stable
func TestStrict(t *testing.T) {
	t.Parallel()
	src := vars(map[string]string{"TOKEN": "t0k", "EMPTY": ""})
	ok := []struct{ in, want string }{
		{"Bearer ${TOKEN}", "Bearer t0k"},
		{"${EMPTY}", ""},
		{"${NOPE:-d}", "d"},
		{"${NOPE-d}", "d"},
		{"${store:api}", "secret(api)"},
	}
	for _, tt := range ok {
		got, err := Expand(context.Background(), tt.in, WithStrict(), src, WithPlugins(fakeStore{}))
		if err != nil || got != tt.want {
			t.Errorf("Expand(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}

	refused := []struct {
		in   string
		want error
	}{
		{"Bearer ${NOPE}", &expand.UnsetError{Name: "NOPE", Offset: 7}},
		{"${vault:db}", &expand.UnknownSchemeError{Scheme: "vault", Ref: "db"}},
		{"${gopas:work/api}", &expand.UnknownSchemeError{Scheme: "gopas", Ref: "work/api"}},
		{"$(gopass show -o work/api)", &expand.CommandOffError{Cmd: "gopass show -o work/api"}},
	}
	for _, tt := range refused {
		_, err := Expand(context.Background(), tt.in, WithStrict(), src, WithPlugins(fakeStore{}))
		if !errors.Is(err, expand.ErrStrict) || err.Error() != tt.want.Error() {
			t.Errorf("Expand(%q) err = %v; want %v", tt.in, err, tt.want)
		}
	}

	var re *expand.RequiredError
	if _, err := Expand(context.Background(), "${NOPE:?needed}", WithStrict()); !errors.As(err, &re) {
		t.Errorf("${NOPE:?} err = %v, want *expand.RequiredError", err)
	}
}

// TestStrictWithCommands: with commands on, strict mode runs $(command) as
// usual; only the commands-off case is refused.
func TestStrictWithCommands(t *testing.T) {
	t.Parallel()
	run := WithRunner(runnerFunc(func(cmd string) (string, error) { return "ran:" + cmd, nil }))
	if got := mustExpand(t, "$(x)", WithStrict(), run); got != "ran:x" {
		t.Errorf("got %q", got)
	}
}

// TestNotStrictByDefault: without WithStrict the refused forms read as before.
func TestNotStrictByDefault(t *testing.T) {
	t.Parallel()
	if got := mustExpand(t, "[$NOPE][${vault:db}][$(id)]"); got != "[][][$(id)]" {
		t.Errorf("got %q", got)
	}
}
