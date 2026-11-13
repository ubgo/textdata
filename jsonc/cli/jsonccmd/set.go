package jsonccmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/jsonc"
)

// newSetCmd replaces or creates the value at a path. The value is a string
// unless --json-value says it is JSON. Missing parent objects are created; a
// missing file is created (mode 0600).
func newSetCmd(a *app) *cobra.Command {
	var dryRun, asJSON bool
	c := &cobra.Command{
		Use:   "set <file> <path> <value>",
		Short: "Set a value, keeping every comment and byte you did not touch",
		Example: "  textdatactl jsonc set config.jsonc server.host api.local\n" +
			"  textdatactl jsonc set config.jsonc server.port 9090 --json-value\n" +
			"  textdatactl jsonc set config.jsonc features '{\"beta\": true}' --json-value\n" +
			"  textdatactl jsonc set config.jsonc debug true --json-value --dry-run   # preview the diff, write nothing",
		Args: cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			file, p, raw := args[0], args[1], args[2]
			var value any = raw
			if asJSON {
				// Validated here so a typo is a usage error (exit 2), not a
				// failed edit; written verbatim, so key order and number
				// literals stay exactly as typed.
				vd, err := jsonc.Parse([]byte(raw), a.options()...)
				if err != nil || len(vd.ValuePaths()) == 0 {
					return a.usagef("--%s: %q is not one JSON value", flagJSONValue, raw)
				}
				value = jsonc.Raw(raw)
			}
			var created bool
			res, err := a.edit(file, p, dryRun, true, func(d *jsonc.Doc) error {
				created = !d.Has(p)
				return d.Set(p, value)
			})
			if err != nil {
				return err
			}
			res.Created = created
			return a.printer.OK(res)
		},
	}
	c.Flags().BoolVar(&asJSON, flagJSONValue, false, "parse <value> as JSON (number, bool, null, object, array, quoted string)")
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
}
