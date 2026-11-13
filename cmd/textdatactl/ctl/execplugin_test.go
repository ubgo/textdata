package ctl

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDotenvExecPluginCallback: a dotenv exec plugin run as `textdatactl
// dotenv <plugin>` calls the host back through DOTENVCTL_BIN plus
// DOTENVCTL_BIN_ARGS and reaches the dotenv verbs, as it does under the
// standalone dotenvctl. Builds the real binary (skipped under -short).
func TestDotenvExecPluginCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the real binary")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the plugin fixture is a /bin/sh script")
	}
	bin := filepath.Join(t.TempDir(), Program)
	build := exec.Command("go", "build", "-o", bin, "..") // package dir is ctl/; main is the module root
	if outp, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, outp)
	}

	pluginDir := t.TempDir()
	script := "#!/bin/sh\n" + `"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get GREETING --expand` + "\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "dotenvctl-cb"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := envFile(t, "WHO=world\nGREETING=hello ${WHO}\n")

	child := exec.Command(bin, "dotenv", "-f", p, "cb")
	child.Env = append(os.Environ(), "PATH="+pluginDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	outp, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("dispatch: %v\n%s", err, outp)
	}
	if got := strings.TrimSpace(string(outp)); got != "hello world" {
		t.Errorf("callback = %q, want the expanded value through textdatactl dotenv", got)
	}
}
