// Command file-confined reads ${file:...} references with the file value
// plugin and shows the guard rails it ships with: confinement to the
// document's folder (on by default), a size limit, and the two ways to widen
// the root.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	fileplugin "github.com/ubgo/textdata/plugins/file"
)

// File and folder names of the demo layout:
//
//	<top>/shared.txt        outside the config folder
//	<top>/cfg/key.pem       next to the documents
//	<top>/cfg/.env
//	<top>/cfg/app.jsonc
const (
	cfgDir     = "cfg"
	keyFile    = "key.pem"
	sharedFile = "shared.txt"
	envFile    = ".env"
	jsoncFile  = "app.jsonc"
)

// envSource reads a file next to it and tries to climb out of its folder.
const envSource = "TLS_KEY=${file:./key.pem}\nSHARED=${file:../shared.txt}\n"

// jsoncSource reads the same key from a JSONC document.
const jsoncSource = "{\n  \"tls\": {\"key\": \"${file:./key.pem}\"}\n}\n"

// smallLimit is below the key's size, to trigger the size check.
const smallLimit = 4

// writeLayout creates the demo files under top and returns the config folder.
func writeLayout(top string) (string, error) {
	cfg := filepath.Join(top, cfgDir)
	files := map[string]string{
		filepath.Join(cfg, keyFile):    "pem-body\n",
		filepath.Join(top, sharedFile): "shared-value\n",
		filepath.Join(cfg, envFile):    envSource,
		filepath.Join(cfg, jsoncFile):  jsoncSource,
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return "", err
		}
	}
	return cfg, nil
}

// readConfined installs one confined plugin in both formats. Relative refs
// resolve against cfg, and a ref leaving cfg is refused before any byte is read.
//
//ds:def id=file-readconfined-mx9js8hm
func readConfined(w io.Writer, cfg string) error {
	p := fileplugin.New(cfg) // the root defaults to cfg itself
	env, err := dotenv.Open(filepath.Join(cfg, envFile), dotenv.WithPlugin(p))
	if err != nil {
		return err
	}
	key, _, err := env.GetExpanded("TLS_KEY")
	if err != nil {
		return err
	}
	_, _, escape := env.GetExpanded("SHARED")
	doc, err := jsonc.Open(filepath.Join(cfg, jsoncFile), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	jkey, err := doc.GetExpanded("tls.key")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv TLS_KEY: %s\n", key)
	fmt.Fprintf(w, "jsonc tls.key:  %v\n", jkey)
	fmt.Fprintf(w, "../shared.txt:  denied=%t\n", errors.Is(escape, plugin.ErrDenied))
	return nil
}

// readWithLimits shows the size limit and the trailing-newline switch, using
// the plugin directly through Read (no document needed).
//
//ds:def id=file-readwithlimits-es378e6n
func readWithLimits(w io.Writer, cfg string) error {
	small := fileplugin.New(cfg, fileplugin.WithMaxSize(smallLimit))
	_, err := small.Read(keyFile)
	fmt.Fprintf(w, "over %d bytes:   too large=%t\n", smallLimit, errors.Is(err, fileplugin.ErrTooLarge))

	exact := fileplugin.New(cfg, fileplugin.WithTrimNewline(false))
	v, err := exact.Read(keyFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "untrimmed:      %s\n", strconv.Quote(v))
	return nil
}

// widenRoot reaches the parent folder two ways: WithRoot("..") keeps a
// boundary one level up; Unconfined drops the boundary entirely.
//
//ds:def id=file-widenroot-45ec6u4b
func widenRoot(w io.Writer, cfg string) error {
	parent := fileplugin.New(cfg, fileplugin.WithRoot(".."))
	v, err := parent.Read("../" + sharedFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "WithRoot(..):   %s\n", v)

	abs := filepath.Join(filepath.Dir(cfg), sharedFile)
	_, err = fileplugin.New(cfg).Read(abs)
	fmt.Fprintf(w, "confined abs:   denied=%t\n", errors.Is(err, plugin.ErrDenied))

	free := fileplugin.New(cfg, fileplugin.Unconfined())
	v, err = free.Read(abs)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Unconfined abs: %s\n", v)
	return nil
}

// run builds the layout in top, a folder the caller owns (main makes a temp
// one, the test passes t.TempDir()), and runs each step.
func run(w io.Writer, top string) error {
	cfg, err := writeLayout(top)
	if err != nil {
		return err
	}
	for _, step := range []func(io.Writer, string) error{readConfined, readWithLimits, widenRoot} {
		if err := step(w, cfg); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	dir, err := os.MkdirTemp("", "file-confined")
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
