//go:build unix

package jsonc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/shell"
)

// TestCommandContextTimeout: WithCommandContext bounds a hanging $(command);
// expansion fails quickly with an error unwrapping to *expand.CommandError and
// shell.ErrTimeout instead of blocking.
func TestCommandContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	d, err := Parse([]byte(`{"token": "$(sleep 30)", "user": "$(printf ada)"}`), WithCommandSubstitution(true), WithCommandContext(ctx))
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = d.GetExpanded("token")
	var ce *expand.CommandError
	if !errors.As(err, &ce) || !errors.Is(err, shell.ErrTimeout) {
		t.Fatalf("err = %v, want *expand.CommandError wrapping shell.ErrTimeout", err)
	}
	if dur := time.Since(start); dur > 10*time.Second {
		t.Errorf("took %s", dur)
	}
	if _, err := d.Expanded(); !errors.Is(err, shell.ErrTimeout) {
		t.Errorf("Expanded: err = %v, want shell.ErrTimeout", err)
	}

	// A clone keeps the bound.
	if _, err := d.Clone().GetExpanded("token"); !errors.Is(err, shell.ErrTimeout) {
		t.Errorf("Clone: err = %v, want shell.ErrTimeout", err)
	}

	// Without substitution on, the context changes nothing: no command runs.
	off, err := Parse([]byte(`{"token": "$(sleep 30)"}`), WithCommandContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := off.GetExpanded("token"); err != nil || v != "$(sleep 30)" {
		t.Errorf("substitution off = %v, %v", v, err)
	}
}
