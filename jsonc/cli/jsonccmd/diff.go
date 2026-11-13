package jsonccmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/jsonc/diff"
	"github.com/ubgo/textdata/pos"
)

// diffPayload is the --json data shape for `diff`.
type diffPayload struct {
	// Equal is true when the files mean the same (exit 0).
	Equal   bool         `json:"equal"`
	Changes []wireChange `json:"changes"`
}

// wireChange is one diff.Change on the wire. Old/New are omitted when the
// side has no value (added/removed); a JSON null value is written as null.
type wireChange struct {
	Kind    diff.Kind       `json:"kind"`
	Path    string          `json:"path"`
	Pointer string          `json:"pointer"`
	Old     json.RawMessage `json:"old,omitempty"`
	New     json.RawMessage `json:"new,omitempty"`
	OldPos  *wirePos        `json:"old_pos,omitempty"`
	NewPos  *wirePos        `json:"new_pos,omitempty"`
}

// wirePos is a value's position in one of the files.
type wirePos struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// rootLabel is how the human form shows the empty (root) path.
const rootLabel = "(root)"

// newDiffCmd compares the meaning of two files: comments, layout and key
// order are invisible; changes are listed by path.
//
// Exit contract mirrors diff(1): 0 same, 1 differences, 2 trouble.
func newDiffCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <a> <b>",
		Short: "List the changes by path between two files, ignoring comments and layout",
		Example: "  textdatactl jsonc diff base.jsonc prod.jsonc\n" +
			"  textdatactl jsonc diff base.jsonc prod.jsonc --json | jq '.data.changes[].path'",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			fa, fb := args[0], args[1]
			da, err := a.open(fa)
			if err != nil {
				return err
			}
			db, err := a.open(fb)
			if err != nil {
				return err
			}
			changes, err := diff.Diff(da, db)
			if err != nil {
				return a.docError(fa+" vs "+fb, err)
			}
			payload := diffPayload{Equal: len(changes) == 0, Changes: make([]wireChange, 0, len(changes))}
			for _, c := range changes {
				w := wireChange{Kind: c.Kind, Path: c.Path, Pointer: c.Pointer}
				if c.Kind != diff.Added {
					w.Old, w.OldPos = mustJSON(c.Old), wireAt(fa, c.OldPos)
				}
				if c.Kind != diff.Removed {
					w.New, w.NewPos = mustJSON(c.New), wireAt(fb, c.NewPos)
				}
				payload.Changes = append(payload.Changes, w)
				label := c.Path
				if label == "" {
					label = rootLabel
				}
				switch c.Kind {
				case diff.Added:
					a.printer.Humanf("+ %s: %s", label, w.New)
				case diff.Removed:
					a.printer.Humanf("- %s: %s", label, w.Old)
				default:
					a.printer.Humanf("~ %s: %s -> %s", label, w.Old, w.New)
				}
			}
			if err := a.printer.OK(payload); err != nil {
				return err
			}
			if !payload.Equal {
				// Differences are a normal outcome, reported above; the exit
				// code is the diff(1) shell contract.
				return &ExitError{Code: ExitFailure, Quiet: true}
			}
			return nil
		},
	}
}

// mustJSON encodes a decoded diff value. Decoded values (maps, slices,
// strings, json.Number, bools, nil) always encode, so an error cannot occur;
// it would show as null rather than abort the report.
func mustJSON(v any) json.RawMessage {
	b, err := marshalNoEscape(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

func wireAt(file string, p pos.Pos) *wirePos {
	if !p.IsValid() {
		return nil
	}
	return &wirePos{File: file, Line: p.Line, Col: p.Col}
}
