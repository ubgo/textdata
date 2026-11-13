// Command exec-plugin installs a third-party dotenvctl plugin, a shell
// script named dotenvctl-hello, in a temp directory on PATH, lists it with
// `dotenvctl plugins`, and runs it as `dotenvctl hello`. The script prints the
// DOTENVCTL_* context it was given and calls the host back for a value.
//
// The program is also the host: dotenvctl hands plugins its own executable
// as DOTENVCTL_BIN, so when the script calls back, this binary runs again and
// serves that call through dotenvcmd.Execute (see serveAsHost).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ubgo/textdata/dotenv/cli/dotenvcmd"
)

// pluginName is the subcommand the script serves; the file on PATH is
// "dotenvctl-" + pluginName.
const pluginName = "hello"

// shadowedName collides with a built-in verb, so `plugins` reports it as
// shadowed and dotenvctl never runs it.
const shadowedName = "github"

// pluginPrefix is the name prefix dotenvctl looks for on PATH.
const pluginPrefix = "dotenvctl-"

// hostModeEnv marks the callback: when set, the binary acts as dotenvctl
// instead of running the demo. run sets it only for the plugin's lifetime.
const hostModeEnv = "EXEC_PLUGIN_EXAMPLE_HOST"

// hostModeOn is hostModeEnv's only meaningful value.
const hostModeOn = "1"

// pathVar is the search path dotenvctl scans for plugins.
const pathVar = "PATH"

// envFileName is the file the demo creates; sampleEnv is its content.
const (
	envFileName = ".env"
	sampleEnv   = "GREETING=hi from the file\n"
)

// execMode makes the plugin scripts executable: dotenvctl skips files
// without an execute bit, both when listing and when dispatching.
const execMode = 0o755

// helloScript is the plugin. It uses only shell built-ins, because the demo
// PATH holds nothing but the plugin directory. It reads the context
// variables, prints its arguments, and calls the host back for a value, so
// it inherits dotenvctl's parsing and expansion instead of parsing .env
// itself.
//
//ds:def id=helloscript-jcm2k9nu
const helloScript = `#!/bin/sh
echo "hello, args: [$*]"
echo "file: ${DOTENVCTL_FILE##*/}"
echo "json: $DOTENVCTL_JSON"
echo "placeholder: $DOTENVCTL_PLACEHOLDER"
echo "version set: ${DOTENVCTL_VERSION:+yes}"
printf 'callback: '
"$DOTENVCTL_BIN" ${DOTENVCTL_BIN_ARGS:-} -f "$DOTENVCTL_FILE" get GREETING
`

// shadowedScript would print if it ever ran; it never does.
const shadowedScript = "#!/bin/sh\necho never runs\n"

func main() {
	if code, ok := serveAsHost(); ok {
		os.Exit(code)
	}
	dir, err := os.MkdirTemp("", "exec-plugin-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// serveAsHost answers a plugin's callback: in host mode the binary is
// dotenvctl, exactly as a host that embeds dotenvcmd would be.
//
//ds:def id=serveashost-3adxb4gf
func serveAsHost() (int, bool) {
	if os.Getenv(hostModeEnv) != hostModeOn {
		return 0, false
	}
	return dotenvcmd.Execute(os.Args[1:], os.Stdout, os.Stderr), true
}

// run executes every step of the guide in dir.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	if runtime.GOOS == "windows" {
		return errors.New("exec-plugin: the plugin is a POSIX shell script")
	}
	envFile := filepath.Join(dir, envFileName)
	if err := os.WriteFile(envFile, []byte(sampleEnv), 0o600); err != nil {
		return err
	}
	if err := installPlugins(dir); err != nil {
		return err
	}
	restore, err := onlyOnPath(dir)
	if err != nil {
		return err
	}
	defer restore()

	if err := listPlugins(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "exit:", runPlugin(w, "-f", envFile, pluginName, "world"))
	fmt.Fprintln(w, "exit:", runPlugin(w, "-f", envFile, "--json", pluginName))
	return nil
}

// installPlugins writes the plugin and a shadowed one into dir. The file
// name is the whole registration: dotenvctl-<name> serves `dotenvctl <name>`.
//
//ds:def id=installplugins-hxu85kcd
func installPlugins(dir string) error {
	scripts := map[string]string{pluginName: helloScript, shadowedName: shadowedScript}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, pluginPrefix+name), []byte(body), execMode); err != nil {
			return err
		}
	}
	return nil
}

// onlyOnPath makes dir the whole PATH, so the listing shows only the demo's
// plugins and none installed on this machine. It returns the undo, which
// drops its error: putting back a variable just set under the same name
// cannot fail.
func onlyOnPath(dir string) (restore func(), err error) {
	old := os.Getenv(pathVar)
	if err := os.Setenv(pathVar, dir); err != nil {
		return nil, err
	}
	return func() { _ = os.Setenv(pathVar, old) }, nil
}

// pluginRow is one row of `dotenvctl plugins --json`.
type pluginRow struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Shadowed bool   `json:"shadowed"`
}

// pluginsEnvelope is the --json output of `dotenvctl plugins`.
type pluginsEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Plugins []pluginRow `json:"plugins"`
	} `json:"data"`
}

// listPlugins runs `dotenvctl plugins --json` and prints each name with its
// status. The JSON form is used because the human table prints full paths,
// which here are temp directories.
//
//ds:def id=listplugins-xgpent5s
func listPlugins(w io.Writer) error {
	var out bytes.Buffer
	if code := dotenvcmd.Execute([]string{"plugins", "--json"}, &out, w); code != dotenvcmd.ExitOK {
		return fmt.Errorf("plugins: exit %d", code)
	}
	var env pluginsEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		return err
	}
	for _, p := range env.Data.Plugins {
		status := "active"
		if p.Shadowed {
			status = "shadowed by built-in"
		}
		fmt.Fprintf(w, "plugin %s: %s (%s)\n", p.Name, status, filepath.Base(p.Path))
	}
	return nil
}

// runPlugin dispatches to the plugin like any verb. Host flags (-f, --json)
// go before the plugin name; everything after it reaches the script as is.
// Host mode is on while the plugin runs, so its callback lands in
// serveAsHost.
//
//ds:def id=runplugin-3ch8m563
func runPlugin(w io.Writer, args ...string) int {
	if err := os.Setenv(hostModeEnv, hostModeOn); err != nil {
		fmt.Fprintln(w, err)
		return dotenvcmd.ExitFailure
	}
	defer func() { _ = os.Unsetenv(hostModeEnv) }()
	return dotenvcmd.Execute(args, w, w)
}
