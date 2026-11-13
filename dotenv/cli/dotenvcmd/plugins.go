package dotenvcmd

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/providerkit"
	"github.com/ubgo/textdata/dotenv/cli/envkit"
)

// pluginDeps assembles the Deps every plugin receives — the ONLY channel
// between plugins and the host (PLUGINS_SPEC §2). Built per invocation, so
// plugin state can never leak across runs.
func (a *app) pluginDeps() providerkit.Deps {
	return providerkit.Deps{
		Printer:     a.printer,
		File:        a.file,
		Source:      a.pluginSource,
		Runner:      a.runner,
		Interactive: a.interactiveFn,
		Ask:         a.askFn,
	}
}

// pluginSource resolves the selection for plugins by delegating to envkit —
// the CLI adds nothing here but its file path and error mapping, which is the
// point: the selection semantics a plugin sees are byte-identical to the ones
// a Go caller of envkit.SelectFile sees.
func (a *app) pluginSource(opts providerkit.SelectOpts) (providerkit.Source, error) {
	sel, err := envkit.SelectFile(a.file, opts)
	if err != nil {
		return nil, a.selectionError(err)
	}
	return sel.Flat(), nil
}

// selectionError maps an envkit selection failure onto the CLI's error codes
// by the error's identity, never its text (a path or a ${VAR:?message} can
// hold any words): a named-but-absent key is "not found", a failed expansion
// is "required", everything else is IO.
func (a *app) selectionError(err error) error {
	switch {
	case errors.Is(err, envkit.ErrKeyNotFound):
		return a.failf(outfmt.CodeNotFound, "%v", err)
	case errors.Is(err, envkit.ErrExpand):
		return a.failf(outfmt.CodeRequired, "%v", err)
	case errors.Is(err, envkit.ErrNameCollision), errors.Is(err, envkit.ErrPlaceholderPattern):
		// The flags themselves conflict or are malformed: a usage error.
		return a.failf(outfmt.CodeUsage, "%v", err)
	default:
		return a.failf(outfmt.CodeIO, "%v", err)
	}
}

// stdinOrDefault resolves the answer source, defaulting to the real stdin so
// a direct (non-Execute) construction still behaves.
func (a *app) stdinOrDefault() io.Reader {
	if a.stdin == nil {
		return os.Stdin
	}
	return a.stdin
}

// stdinIsTerminal reports whether a human can answer a prompt.
//
// A real isatty check (x/term), NOT a char-device check: /dev/null IS a
// character device, so the naive Stat test classifies CI's `< /dev/null`
// stdin as interactive — the confirm gate would then prompt into the void
// and die with "aborted" instead of the actionable "--yes required" message.
// Caught by exactly that misbehavior in testing.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// askOnTerminal prompts on stderr (stdout may be piped or an envelope) and
// accepts y/yes, case-insensitive. Anything else — including read failure —
// is a No, because the safe default for "write to a remote store?" is no.
func (a *app) askOnTerminal(prompt string) bool {
	if _, err := a.errOut.Write([]byte(prompt)); err != nil {
		return false
	}
	line, err := bufio.NewReader(a.stdinOrDefault()).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
