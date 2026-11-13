package jsonccmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/jsonc"
)

// newUnsetCmd comments an object member out (`// "port": 8080,`) so it no
// longer counts but stays in the file; restore brings it back byte for byte.
// Removal is the separate delete verb.
func newUnsetCmd(a *app) *cobra.Command {
	var dryRun bool
	c := &cobra.Command{
		Use:   "unset <file> <path>",
		Short: "Comment a member out (reversible with restore)",
		Example: "  textdatactl jsonc unset config.jsonc server.debug            # '// \"debug\": true,'\n" +
			"  textdatactl jsonc unset config.jsonc server.debug --dry-run  # preview the diff",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			file, p := args[0], args[1]
			res, err := a.edit(file, p, dryRun, false, func(d *jsonc.Doc) error { return d.Disable(p) })
			if err != nil {
				return err
			}
			return a.printer.OK(res)
		},
	}
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
}

// newRestoreCmd re-enables a member unset commented out.
func newRestoreCmd(a *app) *cobra.Command {
	var dryRun bool
	c := &cobra.Command{
		Use:   "restore <file> <path>",
		Short: "Re-enable a member unset commented out",
		Example: "  textdatactl jsonc restore config.jsonc server.debug\n" +
			"  textdatactl jsonc restore config.jsonc server.debug --dry-run",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			file, p := args[0], args[1]
			res, err := a.edit(file, p, dryRun, false, func(d *jsonc.Doc) error { return d.Restore(p) })
			if err != nil {
				return err
			}
			return a.printer.OK(res)
		},
	}
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
}

// newDeleteCmd removes a member (with its attached comments and comma) or an
// array element.
func newDeleteCmd(a *app) *cobra.Command {
	var dryRun bool
	c := &cobra.Command{
		Use:   "delete <file> <path>",
		Short: "Remove a member or array element with its comments",
		Example: "  textdatactl jsonc delete config.jsonc server.legacy\n" +
			"  textdatactl jsonc delete config.jsonc hosts[2] --dry-run",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			file, p := args[0], args[1]
			res, err := a.edit(file, p, dryRun, false, func(d *jsonc.Doc) error { return d.Delete(p) })
			if err != nil {
				return err
			}
			return a.printer.OK(res)
		},
	}
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
}
