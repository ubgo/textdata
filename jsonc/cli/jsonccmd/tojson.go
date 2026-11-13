package jsonccmd

import (
	"bytes"
	"encoding/json"

	"github.com/spf13/cobra"
)

// toJSONPayload is the --json data shape for `to-json`.
type toJSONPayload struct {
	// Value is the document as plain JSON (compacted inside the envelope).
	Value json.RawMessage `json:"value"`
}

// newToJSONCmd prints the document as plain JSON: comments, trailing commas
// and the BOM dropped, every other byte (layout included) as written. An
// empty file prints null.
func newToJSONCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "to-json <file>",
		Short: "Print the file as plain JSON (comments and trailing commas dropped)",
		Example: "  textdatactl jsonc to-json config.jsonc > config.json\n" +
			"  textdatactl jsonc to-json config.jsonc | jq .server",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file := args[0]
			d, err := a.open(file)
			if err != nil {
				return err
			}
			out, err := d.JSON()
			if err != nil {
				return a.docError(file, err)
			}
			if !a.printer.JSON {
				// Printed as is; a final newline is added only when missing.
				a.printer.Human(string(bytes.TrimSuffix(out, []byte("\n"))))
			}
			return a.printer.OK(toJSONPayload{Value: out})
		},
	}
}
