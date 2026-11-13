// Command extends layers JSONC files with "$extends": a local file names one
// or more base files, which are deep-merged first with the local file on top.
// It reads the files from an in-memory fs.FS through the extends plugin, so
// nothing on disk is touched, then shows the one-call Read on a real folder.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"testing/fstest"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugins/extends"
)

// appPath is the file the example opens; its references are relative to its
// own folder, so "../defaults/base.jsonc" is defaults/base.jsonc in the FS.
const appPath = "config/app.jsonc"

// files is the in-memory tree. base.jsonc itself extends logging.jsonc, so the
// chain is logging, then base, then app.
//
//ds:def id=files-6kdabnqd
var files = fstest.MapFS{
	"defaults/logging.jsonc": {Data: []byte(`{"log": {"level": "warn", "format": "json"}}`)},
	"defaults/base.jsonc": {Data: []byte(`{
  "$extends": "logging.jsonc",
  "server": {"host": "0.0.0.0", "port": 8080},
}`)},
	appPath: {Data: []byte(`{
  // Shared defaults first, then this file on top
  "$extends": ["../defaults/base.jsonc"],
  "server": {"port": 9090},
  "log": {"level": "debug"},
}
`)},
	"loop/a.jsonc": {Data: []byte(`{"$extends": "b.jsonc"}`)},
	"loop/b.jsonc": {Data: []byte(`{"$extends": "a.jsonc"}`)},
}

func main() {
	dir, err := os.MkdirTemp("", "extends-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := run(os.Stdout, dir); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order; dir is a scratch folder for the
// one-call Read on disk.
func run(w io.Writer, dir string) error {
	doc, err := openWithIncluder(files, appPath)
	if err != nil {
		return err
	}
	if err := extend(w, doc); err != nil {
		return err
	}
	if err := withoutIncluder(w); err != nil {
		return err
	}
	if err := detectCycle(w); err != nil {
		return err
	}
	return readOnDisk(w, dir)
}

// openWithIncluder parses a file from fsys with the extends plugin
// installed. Without an Includer, "$extends" is an ordinary key and reading a
// file never opens another one.
//
//ds:def id=openwithincluder-67anw3rh
func openWithIncluder(fsys fs.FS, name string) (*jsonc.Doc, error) {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return jsonc.Parse(src, jsonc.WithFileName(name), jsonc.WithPlugin(extends.NewFS(fsys)))
}

// extend lists the references, then builds the merged view. The receiver
// is not modified: Render, and so Save, still write "$extends" as written.
//
//ds:def id=extend-93jurhr8
func extend(w io.Writer, doc *jsonc.Doc) error {
	refs, err := doc.Extends()
	if err != nil {
		return err
	}
	cfg, err := doc.Extended()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "references:", refs)
	fmt.Fprintf(w, "merged view:\n%s\n", cfg.Render())
	fmt.Fprintln(w, "merged view has $extends:", cfg.Has(jsonc.ExtendsKey), "| original still has it:", doc.Has(jsonc.ExtendsKey))
	return nil
}

// withoutIncluder shows that Extended refuses when nothing can load bases,
// rather than quietly ignoring them.
//
//ds:def id=withoutincluder-scxbxr8t
func withoutIncluder(w io.Writer) error {
	doc, err := jsonc.Parse(files[appPath].Data, jsonc.WithFileName(appPath))
	if err != nil {
		return err
	}
	_, err = doc.Extended()
	fmt.Fprintln(w, "no includer is ErrNoIncluder:", errors.Is(err, jsonc.ErrNoIncluder))
	return nil
}

// detectCycle shows a loop between two files reported with the chain.
//
//ds:def id=detectcycle-spct5f3s
func detectCycle(w io.Writer) error {
	doc, err := openWithIncluder(files, "loop/a.jsonc")
	if err != nil {
		return err
	}
	_, err = doc.Extended()
	fmt.Fprintln(w, "cycle:", err)
	fmt.Fprintln(w, "is ErrExtendsCycle:", errors.Is(err, jsonc.ErrExtendsCycle))
	return nil
}

// readOnDisk writes the same layout to a real folder and uses the one-call
// Read: with an Includer installed it applies $extends before decoding.
// WithRoot keeps every base inside dir, symlinks included.
//
//ds:def id=readondisk-5bukm7sy
func readOnDisk(w io.Writer, dir string) error {
	for name, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.Data, 0o600); err != nil {
			return err
		}
	}
	m, err := jsonc.Read(filepath.Join(dir, filepath.FromSlash(appPath)), jsonc.WithPlugin(extends.New(extends.WithRoot(dir))))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Read:", m["server"], m["log"])
	return nil
}
