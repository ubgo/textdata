package ctl

import (
	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/versioncmd"

	"github.com/ubgo/textdata/clikit/outfmt"
)

// flagJSON is the version command's envelope flag, named once.
const flagJSON = "json"

// newVersionCmd is `textdatactl version`: build provenance (version and where
// it came from, commit, build time, Go version, platform).
//
// The facts and the human layout come from voltkit's versioncmd (Collect,
// WriteText), shared with every ubgo CLI; the --json form is the clikit/outfmt
// {ok,data} envelope rather than versioncmd's own, so every textdatactl verb
// speaks one envelope. data is versioncmd.Info, whose field names are that
// package's stable contract.
func newVersionCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print version and build provenance",
		Long:  versioncmd.DefaultLong(),
		Example: "  textdatactl version\n" +
			"  textdatactl version --json | jq -r .data.version",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			info := versioncmd.Collect(Program)
			if jsonOut {
				p := &outfmt.Printer{Out: c.OutOrStdout(), Err: c.ErrOrStderr(), Program: Program, JSON: true}
				return p.OK(info)
			}
			return versioncmd.WriteText(c.OutOrStdout(), info)
		},
	}
	c.Flags().BoolVar(&jsonOut, flagJSON, false, "emit a machine-readable {ok,data} envelope")
	return c
}
