module github.com/ubgo/textdata/cmd/textdatactl

go 1.26.4

retract [v0.1.1, v0.1.2] // superseded by the v0.1.4 fresh release

require (
	github.com/spf13/cobra v1.10.2
	github.com/ubgo/textdata/clikit v0.1.4
	github.com/ubgo/textdata/dotenv/cli v0.1.4
	github.com/ubgo/textdata/jsonc/cli v0.1.5
	github.com/ubgo/voltkit/versioncmd v0.2.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/ubgo/buildinfo v0.1.2 // indirect
	github.com/ubgo/textdata v0.1.4 // indirect
	github.com/ubgo/textdata/dotenv v0.1.4 // indirect
	github.com/ubgo/textdata/dotenv/cli/providers/github v0.1.4 // indirect
	github.com/ubgo/textdata/dotenv/cli/providers/vercel v0.1.4 // indirect
	github.com/ubgo/textdata/jsonc v0.1.5 // indirect
	github.com/ubgo/voltkit/output v0.2.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
)
