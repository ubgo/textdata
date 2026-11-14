//go:build unix

package value

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/shell"
)

// These run real commands through sh -c, so they are Unix-only.

// TestCommandsOn: WithCommands runs $(…) through the shell, the command text
// expanded first, the trailing newline trimmed.
func TestCommandsOn(t *testing.T) {
	out, err := Expand(context.Background(), "hi $(echo $WHO)!", vars(map[string]string{"WHO": "ada"}), WithCommands())
	if err != nil || out != "hi ada!" {
		t.Fatalf("Expand = %q, %v", out, err)
	}
	_, err = Expand(context.Background(), "$(exit 7)", WithCommands())
	var ce *expand.CommandError
	var ee *shell.ExitError
	if !errors.As(err, &ce) || !errors.As(err, &ee) || ee.Code != 7 {
		t.Fatalf("err = %v, want *expand.CommandError wrapping *shell.ExitError", err)
	}
}

// TestCommandsContext: Expand's context bounds the default shell, and a
// shell.Runner given to WithRunner gets the context too.
func TestCommandsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := Expand(ctx, "$(sleep 30)", WithCommands()); !errors.Is(err, shell.ErrTimeout) {
		t.Errorf("WithCommands: err = %v, want shell.ErrTimeout", err)
	}
	if _, err := Expand(ctx, "$(sleep 30)", WithRunner(shell.Runner{})); !errors.Is(err, shell.ErrTimeout) {
		t.Errorf("WithRunner(shell.Runner{}): err = %v, want shell.ErrTimeout", err)
	}
	r := shell.Runner{Timeout: 50 * time.Millisecond}
	if _, err := Expand(context.Background(), "$(sleep 30)", WithRunner(r)); !errors.Is(err, shell.ErrTimeout) {
		t.Errorf("Runner.Timeout: err = %v, want shell.ErrTimeout", err)
	}
}
