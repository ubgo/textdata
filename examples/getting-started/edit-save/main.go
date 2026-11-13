// Command edit-save opens a .env and a JSONC file, changes a few values, saves
// them, and prints the files before and after: every comment, blank line and
// layout choice the edits did not touch is written back as it was. It works on
// copies in a temp directory, so running it changes nothing in the repository.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
)

// The files the example edits, written fresh into a temp directory each run.
const (
	envName    = ".env"
	configName = "config.jsonc"

	envSource = `# Database settings for local development
DB_HOST=localhost
DB_PORT=5432

export LOG_LEVEL="debug" # quoted, with an inline comment
`
	configSource = `{
  // Where the API listens.
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
  },
  /* Features switched on in this environment. */
  "features": ["search", "billing"],
}
`
)

func main() {
	dir, err := os.MkdirTemp("", "edit-save-example-")
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

// run writes the two sample files, edits and saves each, and prints them.
func run(w io.Writer, dir string) error {
	envPath, configPath := filepath.Join(dir, envName), filepath.Join(dir, configName)
	if err := os.WriteFile(envPath, []byte(envSource), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(configPath, []byte(configSource), 0o600); err != nil {
		return err
	}
	if err := editEnv(envPath); err != nil {
		return err
	}
	if err := editConfig(configPath); err != nil {
		return err
	}
	return show(w, envPath, configPath)
}

// editEnv changes a value, adds a key next to its neighbour, comments one out
// (reversible with Restore) and saves. Save writes atomically and keeps the
// file's mode.
//
//ds:def id=editenv-8qgna9mt
func editEnv(path string) error {
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	f.Set("DB_PORT", "6543")   // only the value's bytes change
	f.Set("LOG_LEVEL", "info") // the export prefix and comment stay
	if _, err := f.SetAfter("DB_PORT", "DB_NAME", "app"); err != nil {
		return err
	}
	f.Unset("DB_HOST", false) // commented out, not deleted
	return f.Save()
}

// editConfig changes a number, adds a member to an existing object in that
// object's layout, appends to an array and saves.
//
//ds:def id=editconfig-3sybqwqh
func editConfig(path string) error {
	doc, err := jsonc.Open(path)
	if err != nil {
		return err
	}
	if err := doc.Set("server.port", 9090); err != nil {
		return err
	}
	if err := doc.Set("server.tls", true); err != nil {
		return err
	}
	if err := doc.Append("features", "audit"); err != nil {
		return err
	}
	return doc.Save()
}

// show prints both files as they are now on disk.
func show(w io.Writer, paths ...string) error {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "--- %s\n%s", filepath.Base(p), data)
	}
	return nil
}
