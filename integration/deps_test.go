package integration

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Short names for the module paths in the table below.
const (
	base     = "github.com/ubgo/textdata"
	dotenv   = base + "/dotenv"
	jsonc    = base + "/jsonc"
	clikit   = base + "/clikit"
	dotenvCL = base + "/dotenv/cli"
	jsoncCL  = base + "/jsonc/cli"
	ghProv   = base + "/dotenv/cli/providers/github"
	vcProv   = base + "/dotenv/cli/providers/vercel"
	cobra    = "github.com/spf13/cobra"
	pflag    = "github.com/spf13/pflag"
)

// wantRequires is the owner rule "importing one textdata package never pulls
// a dependency it does not use", written down per module: the exact direct
// (non-indirect) requires of every module in go.work. Adding a require means
// changing this table in the same commit, which is the moment to ask whether
// the module really uses it outside its tests. The library modules (base,
// formats, value plugins) require the base and nothing else; cobra appears only
// in CLI modules; cross-format tests live here, in a module nobody imports.
var wantRequires = map[string][]string{
	".":                           nil,
	"dotenv":                      {base},
	"jsonc":                       {base},
	"plugins/gopass":              {base},
	"plugins/file":                {base},
	"plugins/extends":             {base},
	"plugins/vault":               {base},
	"plugins/aws":                 {base},
	"clikit":                      {cobra, pflag},
	"dotenv/cli/providers/github": {cobra, pflag, clikit},
	"dotenv/cli/providers/vercel": {cobra, clikit},
	"dotenv/cli":                  {cobra, clikit, dotenv, ghProv, vcProv, "golang.org/x/term"},
	"jsonc/cli":                   {cobra, base, clikit, jsonc},
	"cmd/textdatactl":             {cobra, clikit, dotenvCL, jsoncCL, "github.com/ubgo/voltkit/versioncmd"},
	"integration": {base, dotenv, jsonc,
		base + "/plugins/aws", base + "/plugins/extends", base + "/plugins/file",
		base + "/plugins/gopass", base + "/plugins/vault"},
	// examples holds the docs' runnable programs: never imported or tagged, so
	// it may require every module its programs show.
	"examples": {cobra, base, clikit, base + "/cmd/textdatactl", dotenv, dotenvCL, ghProv, vcProv, jsonc, jsoncCL,
		base + "/plugins/aws", base + "/plugins/extends", base + "/plugins/file",
		base + "/plugins/gopass", base + "/plugins/vault"},
}

// cliModules may require cobra; no other module may.
var cliModules = []string{"clikit", "dotenv/cli/providers/github", "dotenv/cli/providers/vercel", "dotenv/cli", "jsonc/cli", "cmd/textdatactl", "examples"}

func TestModuleRequires(t *testing.T) {
	root := filepath.Join("..")
	mods := workspaceModules(t, root)
	if got, want := sortedKeys(mods), sortedKeys(wantRequires); !reflect.DeepEqual(got, want) {
		t.Fatalf("go.work modules = %v\nthe table lists %v", got, want)
	}
	for dir := range mods {
		got := directRequires(t, filepath.Join(root, dir, "go.mod"))
		want := slices.Clone(wantRequires[dir])
		slices.Sort(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s/go.mod direct requires = %v, want %v", dir, got, want)
		}
		if slices.Contains(got, cobra) && !slices.Contains(cliModules, dir) {
			t.Errorf("%s requires cobra but is not a CLI module", dir)
		}
		if dir != "integration" && slices.Contains(got, base+"/integration") {
			t.Errorf("%s requires the test-only integration module", dir)
		}
	}
}

// workspaceModules reads the use directives of go.work as clean relative
// directories ("." for the base).
func workspaceModules(t *testing.T, root string) map[string]bool {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	mods := map[string]bool{}
	inUse := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "use (":
			inUse = true
		case inUse && line == ")":
			inUse = false
		case inUse && line != "":
			mods[filepath.ToSlash(filepath.Clean(line))] = true
		case strings.HasPrefix(line, "use "):
			mods[filepath.ToSlash(filepath.Clean(strings.TrimPrefix(line, "use ")))] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return mods
}

// directRequires lists the module paths a go.mod requires without an
// "// indirect" marker, sorted. It handles the single-line and block forms,
// which is all go mod edit and go mod tidy write.
func directRequires(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	inReq := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "require (":
			inReq = true
			continue
		case inReq && line == ")":
			inReq = false
			continue
		case strings.HasPrefix(line, "require "):
			line = strings.TrimPrefix(line, "require ")
		case !inReq:
			continue
		}
		if line == "" || strings.HasPrefix(line, "//") || strings.Contains(line, "// indirect") {
			continue
		}
		if f := strings.Fields(line); len(f) >= 2 && !slices.Contains(out, f[0]) {
			out = append(out, f[0])
		}
	}
	slices.Sort(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
