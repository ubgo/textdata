package jsonc

import (
	"errors"
	"strings"
	"testing"

	"github.com/ubgo/textdata/expand"
)

// A failing $(cmd) must surface as an error carrying the value's position and
// unwrapping to *expand.CommandError, never as a silent "".
func TestFailingCommandIsPositionedError(t *testing.T) {
	refuse := func(string) (string, error) { return "", errors.New("not allowed") }
	d, err := Parse([]byte("{\n  \"v\": \"a-$(boom)\"\n}\n"), WithFileName("app.jsonc"), WithPlugin(runnerPlugin{fn: refuse}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.GetExpanded("v")
	if err == nil {
		t.Fatalf("GetExpanded(v) = %q, nil; want a command error", got)
	}
	var ce *expand.CommandError
	if !errors.As(err, &ce) || ce.Cmd != "boom" {
		t.Fatalf("error %v does not unwrap to *expand.CommandError for boom", err)
	}
	if !strings.HasPrefix(err.Error(), "app.jsonc:2:8:") {
		t.Fatalf("error %q lacks the value position app.jsonc:2:8", err)
	}
}
