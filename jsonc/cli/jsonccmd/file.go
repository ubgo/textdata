package jsonccmd

import (
	"errors"
	"fmt"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/textdiff"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
)

// failf reports an operation failure through the Printer (both modes) and
// returns the quiet ExitFailure error, so a verb fails in one line.
func (a *app) failf(code outfmt.ErrCode, format string, args ...any) error {
	return a.fail(ExitFailure, code, fmt.Sprintf(format, args...))
}

// usagef is failf for a malformed operand: exit 2, code "usage".
func (a *app) usagef(format string, args ...any) error {
	return a.fail(ExitUsage, outfmt.CodeUsage, fmt.Sprintf(format, args...))
}

func (a *app) fail(exit int, code outfmt.ErrCode, msg string) error {
	if err := a.printer.Fail(code, msg); err != nil {
		return &ExitError{Code: exit, Err: err}
	}
	return &ExitError{Code: exit, Quiet: true, Err: errors.New(msg)}
}

// open reads an existing file. A missing file is a failure here (jsonc.Open
// would hand back an empty document, which every reading verb would report
// as a confusing "path not found").
func (a *app) open(file string) (*jsonc.Doc, error) {
	d, err := a.openOrNew(file)
	if err != nil {
		return nil, err
	}
	if !d.Existed() {
		return nil, a.failf(outfmt.CodeIO, "%s: no such file", file)
	}
	return d, nil
}

// openOrNew reads file, or starts an empty document when it does not exist
// (set bootstraps a file). Syntax errors are failures with file:line:col.
func (a *app) openOrNew(file string) (*jsonc.Doc, error) {
	d, err := jsonc.Open(file, a.options()...)
	if err == nil {
		return d, nil
	}
	if d != nil && d.Err() != nil {
		return nil, a.failf(CodeSyntax, "%v", err)
	}
	return nil, a.failf(outfmt.CodeIO, "%v", err)
}

// docError maps a jsonc library error onto the CLI's codes. The sentinels
// are the contract; message text is never inspected.
func (a *app) docError(file string, err error) error {
	msg := err.Error()
	if pos.First(err) == nil {
		// Positioned errors already name the file.
		msg = file + ": " + msg
	}
	switch {
	case errors.Is(err, path.ErrSyntax):
		return a.fail(ExitUsage, outfmt.CodeUsage, msg)
	case errors.Is(err, jsonc.ErrNotFound):
		return a.failf(outfmt.CodeNotFound, "%s", msg)
	case errors.Is(err, jsonc.ErrInvalid):
		return a.failf(CodeSyntax, "%s", msg)
	case errors.Is(err, jsonc.ErrType):
		return a.failf(CodeType, "%s", msg)
	case errors.Is(err, jsonc.ErrExists):
		return a.failf(CodeExists, "%s", msg)
	case errors.Is(err, jsonc.ErrLayout):
		return a.failf(CodeLayout, "%s", msg)
	case errors.Is(err, jsonc.ErrStrict):
		return a.failf(CodeStrict, "%s", msg)
	default:
		return a.failf(outfmt.CodeIO, "%s", msg)
	}
}

// editPayload is the --json data shape of every mutating verb.
type editPayload struct {
	File string `json:"file"`
	Path string `json:"path"`
	// Created is true when set added a value that did not exist (set only).
	Created bool `json:"created,omitempty"`
	// Changed is false when the edit left the bytes identical; nothing was
	// written then and the file's mtime is untouched.
	Changed bool `json:"changed"`
	// DryRun mirrors the flag so a script can tell a preview from a write.
	DryRun bool `json:"dry_run"`
	// Diff is the would-be line diff of a dry run ("-old"/"+new" lines).
	Diff []string `json:"diff,omitempty"`
}

// edit runs one mutation on file: open, apply fn, then Save (atomic) unless
// the bytes are unchanged or this is a dry run, which prints the diff
// instead. create lets set start a file that does not exist.
func (a *app) edit(file, p string, dryRun, create bool, fn func(*jsonc.Doc) error) (*editPayload, error) {
	var d *jsonc.Doc
	var err error
	if create {
		d, err = a.openOrNew(file)
	} else {
		d, err = a.open(file)
	}
	if err != nil {
		return nil, err
	}
	before := d.Render()
	if err := fn(d); err != nil {
		return nil, a.docError(file, err)
	}
	after := d.Render()
	res := &editPayload{File: file, Path: p, DryRun: dryRun, Changed: string(before) != string(after)}
	if dryRun {
		res.Diff = textdiff.Lines(string(before), string(after))
		for _, line := range res.Diff {
			a.printer.Human(line)
		}
	}
	if !res.Changed {
		a.printer.Human("no changes")
	}
	if res.Changed && !dryRun {
		if err := d.Save(); err != nil {
			return nil, a.failf(outfmt.CodeIO, "%v", err)
		}
	}
	return res, nil
}
