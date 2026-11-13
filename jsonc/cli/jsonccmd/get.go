package jsonccmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/jsonc"
)

// getPayload is the --json data shape for `get`.
type getPayload struct {
	Path string `json:"path"`
	// Kind is the jsonc kind name: object, array, string, number, bool, null.
	Kind string `json:"kind"`
	// Value is the value as plain JSON, exactly as written (numbers keep
	// their literal; comments dropped).
	Value json.RawMessage `json:"value"`
}

// newGetCmd prints one value. A string prints bare (so `v=$(... get)` reads
// the string itself); anything else prints as indented plain JSON.
func newGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <file> <path>",
		Short: "Print the value at a path",
		Example: "  textdatactl jsonc get config.jsonc server.port\n" +
			"  textdatactl jsonc get config.jsonc /server/hosts/0      # JSON Pointer\n" +
			"  textdatactl jsonc get config.jsonc server --json         # {ok,data:{path,kind,value}}\n" +
			"  textdatactl jsonc get config.jsonc \"\"                    # the whole document",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			file, p := args[0], args[1]
			d, err := a.open(file)
			if err != nil {
				return err
			}
			kind, err := d.Kind(p)
			if err != nil {
				return a.docError(file, err)
			}
			raw, err := plainValue(d, p, kind)
			if err != nil {
				return a.failf(CodeSyntax, "%s: %v", file, err)
			}
			if kind == jsonc.KindString {
				s, _ := d.GetString(p) // kind checked above
				a.printer.Human(s)
			} else {
				var b bytes.Buffer
				if err := json.Indent(&b, raw, "", "  "); err != nil {
					return a.failf(CodeSyntax, "%s: %v", file, err)
				}
				a.printer.Human(b.String())
			}
			return a.printer.OK(getPayload{Path: p, Kind: kind.String(), Value: raw})
		},
	}
}

// plainValue returns the value at p as compact plain JSON, keeping number
// literals exactly (no float64 round trip).
func plainValue(d *jsonc.Doc, p string, kind jsonc.Kind) (json.RawMessage, error) {
	if kind == jsonc.KindString {
		s, err := d.GetString(p)
		if err != nil {
			return nil, err
		}
		return marshalNoEscape(s)
	}
	text, _ := d.ValueAt(p) // the source text of the value, comments included
	sub, err := jsonc.Parse([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("re-read %q: %w", p, err)
	}
	plain, err := sub.JSON()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := json.Compact(&b, plain); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// marshalNoEscape encodes v without HTML escaping, so "<" stays "<".
func marshalNoEscape(v any) (json.RawMessage, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
