// Command verb-tour runs every textdatactl verb once, in process, on files in
// a temp directory: the jsonc verbs, the dotenv verbs and `version`. It goes
// through ctl.Execute, the same entry point as the textdatactl binary, so the
// arguments are exactly what you would type after `textdatactl`.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/cmd/textdatactl/ctl"
)

// fileMode is 0600 because env files hold credentials.
const fileMode = 0o600

// File names and contents of the demo. Two jsonc files to diff, and a .env
// family (with a contract) for envs, matrix and diff.
const (
	configFile = "config.jsonc"
	nextFile   = "next.jsonc"
	devFile    = ".env"
	prodFile   = ".env.prod"
	contract   = ".env.example"
)

var files = map[string]string{
	configFile: "{\n  // where the server listens\n  \"server\": {\n    \"host\": \"localhost\",\n    \"port\": 8080,\n    \"debug\": true\n  },\n  \"tags\": [\"a\", \"b\"],\n}\n",
	nextFile:   "{\"server\": {\"host\": \"0.0.0.0\", \"port\": 8080}, \"tags\": [\"a\", \"b\", \"c\"]}\n",
	devFile:    "HOST=localhost\nURL=http://${HOST}:8080\nDEBUG=true\n# OLD=1\n",
	prodFile:   "HOST=example.com\nURL=https://${HOST}\n",
	contract:   "HOST=\nURL=\nSENTRY_DSN=\n",
}

func main() {
	dir, err := os.MkdirTemp("", "verb-tour-")
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

// run writes the demo files into dir and runs both tours from inside it, so
// file names in the output are short and stable.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), fileMode); err != nil {
			return err
		}
	}
	back, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	defer os.Chdir(back)
	jsoncTour(w)
	dotenvTour(w)
	versionCheck(w)
	return nil
}

// textdatactl runs one command line and prints it, its output and its exit
// code, the way a shell session would show them.
func textdatactl(w io.Writer, args ...string) {
	fmt.Fprintf(w, "$ textdatactl %s\n", strings.Join(args, " "))
	code := ctl.Execute(args, w, w)
	fmt.Fprintf(w, "exit=%d\n", code)
}

// jsoncTour runs every jsonc verb: read, edit (with a dry run first),
// comment out and restore, delete, compare, convert, format check, validate.
//
//ds:def id=jsonctour-mjjstg7x
func jsoncTour(w io.Writer) {
	textdatactl(w, "jsonc", "get", configFile, "server.port")
	textdatactl(w, "jsonc", "set", configFile, "server.port", "9090", "--json-value")
	textdatactl(w, "jsonc", "unset", configFile, "server.debug", "--dry-run")
	textdatactl(w, "jsonc", "unset", configFile, "server.debug")
	textdatactl(w, "jsonc", "restore", configFile, "server.debug")
	textdatactl(w, "jsonc", "delete", configFile, "tags[1]", "--dry-run")
	textdatactl(w, "jsonc", "diff", configFile, nextFile)
	textdatactl(w, "jsonc", "to-json", configFile)
	textdatactl(w, "jsonc", "fmt", nextFile, "--check")
	textdatactl(w, "jsonc", "validate", configFile, "--forbid-trailing-commas")
}

// dotenvTour runs the dotenv verbs that read, edit and compare: list, keys,
// unset and restore, then the multi-environment trio envs, matrix and diff.
//
//ds:def id=dotenvtour-np6rj397
func dotenvTour(w io.Writer) {
	textdatactl(w, "dotenv", "list", "--expand")
	textdatactl(w, "dotenv", "keys")
	textdatactl(w, "dotenv", "unset", "DEBUG", "--dry-run")
	textdatactl(w, "dotenv", "unset", "DEBUG")
	textdatactl(w, "dotenv", "restore", "DEBUG")
	textdatactl(w, "dotenv", "envs")
	textdatactl(w, "dotenv", "matrix", "--contract", contract)
	textdatactl(w, "dotenv", "diff", devFile, prodFile, "--expand")
}

// versionCheck runs `version --json`. The version string depends on how the
// binary was built, so only the envelope's shape is printed.
func versionCheck(w io.Writer) {
	var out strings.Builder
	code := ctl.Execute([]string{"version", "--json"}, &out, &out)
	fmt.Fprintf(w, "$ textdatactl version --json\nok envelope: %t\nexit=%d\n", strings.HasPrefix(out.String(), `{"ok":true`), code)
}
