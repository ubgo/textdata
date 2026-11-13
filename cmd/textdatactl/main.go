// Command textdatactl is the umbrella CLI for every textdata format:
//
//	textdatactl dotenv get API_URL
//	textdatactl dotenv set DEBUG=true --dry-run
//	textdatactl jsonc get config.jsonc server.port
//	textdatactl jsonc diff base.jsonc prod.jsonc
//	textdatactl version
//
// Each format ships its verbs as a command package (dotenv/cli/dotenvcmd,
// jsonc/cli/jsonccmd);
// textdatactl mounts them under the format's name. main stays a one-liner so
// the whole CLI runs in-process in tests (see package ctl).
package main

import (
	"os"

	"github.com/ubgo/textdata/cmd/textdatactl/ctl"
)

func main() {
	os.Exit(ctl.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
