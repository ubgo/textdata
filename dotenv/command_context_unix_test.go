//go:build unix

package dotenv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/shell"
)

// TestCommandContextTimeout: WithCommandContext bounds a hanging $(command);
// the read fails quickly with an *expand.CommandError wrapping
// shell.ErrTimeout instead of blocking.
func TestCommandContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	f := Parse("TOKEN=$(sleep 30)\n", WithCommandSubstitution(true), WithCommandContext(ctx))

	start := time.Now()
	_, _, err := f.GetExpanded("TOKEN")
	var ce *expand.CommandError
	if !errors.As(err, &ce) || !errors.Is(err, shell.ErrTimeout) {
		t.Fatalf("err = %v, want *expand.CommandError wrapping shell.ErrTimeout", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("took %s", d)
	}

	// Without substitution on, the context changes nothing: no command runs.
	off := Parse("TOKEN=$(sleep 30)\n", WithCommandContext(ctx))
	if v, _, err := off.GetExpanded("TOKEN"); err != nil || v != "$(sleep 30)" {
		t.Errorf("substitution off = %q, %v", v, err)
	}

	// A live context runs commands as before.
	live := Parse("USER=$(printf 'ada\\n')\n", WithCommandSubstitution(true), WithCommandContext(context.Background()))
	if v, _, err := live.GetExpanded("USER"); err != nil || v != "ada" {
		t.Errorf("live context = %q, %v", v, err)
	}
}
