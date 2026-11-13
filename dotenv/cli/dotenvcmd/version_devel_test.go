package dotenvcmd

import (
	"runtime/debug"
	"testing"
)

// TestVersionString_WorkspaceDevelStamp: a host built in a go.work workspace
// sees the CLI module stamped "(devel)"; that reads as an unstamped build.
func TestVersionString_WorkspaceDevelStamp(t *testing.T) {
	t.Parallel()
	info := &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/ubgo/textdata/cmd/textdatactl"},
		Deps: []*debug.Module{{Path: cliModulePath, Version: develStamp}},
	}
	if got := versionString(func() (*debug.BuildInfo, bool) { return info, true }); got != devVersion {
		t.Errorf("versionString = %q, want %q", got, devVersion)
	}
}
