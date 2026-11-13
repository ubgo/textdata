module github.com/ubgo/textdata/dotenv/cli

go 1.26.4

retract [v0.1.1, v0.1.2] // superseded by the v0.1.4 fresh release

require (
	github.com/spf13/cobra v1.10.2
	github.com/ubgo/textdata/clikit v0.1.2
	github.com/ubgo/textdata/dotenv v0.1.2
	github.com/ubgo/textdata/dotenv/cli/providers/github v0.1.1
	github.com/ubgo/textdata/dotenv/cli/providers/vercel v0.1.1
	golang.org/x/term v0.45.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/ubgo/textdata v0.1.2 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
