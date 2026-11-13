// Command extends-fs merges JSONC "$extends" bases through the extends
// includer plugin: first from an fs.FS (here an in-memory testing/fstest.MapFS,
// the same code serves embed.FS or os.DirFS), then from disk with a confined
// root.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing/fstest"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/plugins/extends"
)

// Locations inside the FS. Slash-separated FS paths, never OS paths.
const (
	appPath    = "conf/app.jsonc"
	escapePath = "conf/escape.jsonc"
	loopPath   = "conf/loop.jsonc"
)

// layout is the in-memory tree: app extends a team file, which extends the
// shared base; each reference is relative to the file that writes it.
var layout = fstest.MapFS{
	"base.jsonc":        {Data: []byte(`{"port": 80, "db": {"host": "db.internal", "pool": 5}}`)},
	"conf/team.jsonc":   {Data: []byte(`{"$extends": "../base.jsonc", "db": {"pool": 10}}`)},
	"conf/app.jsonc":    {Data: []byte("{\n  // local overrides\n  \"$extends\": \"team.jsonc\",\n  \"port\": 8080\n}\n")},
	"conf/escape.jsonc": {Data: []byte(`{"$extends": "../../etc/passwd"}`)},
	"conf/loop.jsonc":   {Data: []byte(`{"$extends": "loop.jsonc"}`)},
}

// openFS parses one file of the FS with the FS includer installed.
// WithFileName gives the document its location, which is the folder its own
// relative references resolve from.
//
//ds:def id=extends-openfs-9ab42gm6
func openFS(name string) (*jsonc.Doc, error) {
	data, err := layout.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return jsonc.Parse(data, jsonc.WithFileName(name), jsonc.WithPlugin(extends.NewFS(layout)))
}

// mergeFromFS loads the chain app -> team -> base and decodes the merged
// result; the original document still renders with its "$extends" line.
//
//ds:def id=extends-mergefromfs-6whqb5u8
func mergeFromFS(w io.Writer) error {
	doc, err := openFS(appPath)
	if err != nil {
		return err
	}
	merged, err := doc.Extended()
	if err != nil {
		return err
	}
	var cfg struct {
		Port int `json:"port"`
		DB   struct {
			Host string `json:"host"`
			Pool int    `json:"pool"`
		} `json:"db"`
	}
	if err := merged.Decode(&cfg); err != nil {
		return err
	}
	refs, _ := doc.Extends()
	fmt.Fprintf(w, "extends:     %v\n", refs)
	fmt.Fprintf(w, "merged:      port=%d db.host=%s db.pool=%d\n", cfg.Port, cfg.DB.Host, cfg.DB.Pool)
	fmt.Fprintf(w, "app intact:  %t\n", string(doc.Render()) == string(layout[appPath].Data))
	return nil
}

// refusals shows the errors a caller can branch on: climbing above the FS
// root, a cycle, and $extends with no includer installed.
//
//ds:def id=extends-refusals-5a7q3uxx
func refusals(w io.Writer) error {
	escape, err := openFS(escapePath)
	if err != nil {
		return err
	}
	_, err = escape.Extended()
	fmt.Fprintf(w, "escape:      denied=%t\n", errors.Is(err, plugin.ErrDenied))

	loop, err := openFS(loopPath)
	if err != nil {
		return err
	}
	_, err = loop.Extended()
	fmt.Fprintf(w, "loop:        cycle=%t\n", errors.Is(err, jsonc.ErrExtendsCycle))

	bare, err := jsonc.Parse(layout[appPath].Data)
	if err != nil {
		return err
	}
	_, err = bare.Extended()
	fmt.Fprintf(w, "no includer: %t\n", errors.Is(err, jsonc.ErrNoIncluder))
	return nil
}

// readFromDisk uses the disk includer confined to one folder, through
// jsonc.Read: installing an Includer is what makes Read merge the bases.
//
//ds:def id=extends-readfromdisk-pgp6k8xu
func readFromDisk(w io.Writer, root string) error {
	for name, f := range layout {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.Data, 0o600); err != nil {
			return err
		}
	}
	inc := extends.New(extends.WithRoot(root), extends.AllowAbsolute(false))
	m, err := jsonc.Read(filepath.Join(root, filepath.FromSlash(appPath)), jsonc.WithPlugin(inc))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "disk read:   port=%v db=%v\n", m["port"], m["db"])
	_, err = jsonc.Read(filepath.Join(root, filepath.FromSlash(escapePath)), jsonc.WithPlugin(inc))
	fmt.Fprintf(w, "disk escape: denied=%t\n", errors.Is(err, plugin.ErrDenied))
	return nil
}

// run runs every step; the disk step works in root, a folder the caller owns
// (main makes a temp one, the test passes t.TempDir()).
func run(w io.Writer, root string) error {
	if err := mergeFromFS(w); err != nil {
		return err
	}
	if err := refusals(w); err != nil {
		return err
	}
	return readFromDisk(w, root)
}

func main() {
	dir, err := os.MkdirTemp("", "extends-fs")
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
