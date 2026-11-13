// Command edit-config edits a commented JSONC config in place and saves it:
// every edit changes only the bytes it touches, so the comments, blank lines,
// key order and trailing commas of the rest of the file survive. It then shows
// what Save promises: atomic writes, kept file modes, symlinks left intact,
// and SaveGuard plugins that can veto a save.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/jsonc"
)

// fileName is the config file the example creates in its scratch folder.
const fileName = "app.jsonc"

// before is the file as a person wrote it.
//
//ds:def id=before-hqbcqwjn
const before = `{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "interval": 30,
  },

  /* feature switches */
  "features": {
    "beta": false,
  },
}
`

// existingMode is the permission the user gave the file; Save must keep it.
const existingMode fs.FileMode = 0o640

func main() {
	dir, err := os.MkdirTemp("", "edit-config-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := run(os.Stdout, dir); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order inside dir.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(before), existingMode); err != nil {
		return err
	}
	doc, err := jsonc.Open(path)
	if err != nil {
		return err
	}
	for _, step := range []func(*jsonc.Doc) error{setValues, placeKeys, deleteKey, disableKey} {
		if err := step(doc); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "--- after edits ---\n%s", doc.Render())
	if err := restoreKey(w, doc); err != nil {
		return err
	}
	if err := saveInPlace(w, doc); err != nil {
		return err
	}
	if err := createOnFirstRun(w, dir); err != nil {
		return err
	}
	if err := saveThroughSymlink(w, dir); err != nil {
		return err
	}
	if err := saveParsed(w, dir); err != nil {
		return err
	}
	return guardSave(w, dir)
}

// setValues replaces a value in place and creates a new one. Set creates any
// missing parent objects, at the end of their object, in its indentation and
// comma style.
//
//ds:def id=setvalues-fuvbnztb
func setValues(doc *jsonc.Doc) error {
	if err := doc.Set("sync.interval", 60); err != nil {
		return err
	}
	return doc.Set("log.level", "info")
}

// placeKeys puts a new key next to a chosen sibling instead of at the end.
// For a key that already exists, both update it where it is.
//
//ds:def id=placekeys-95kby248
func placeKeys(doc *jsonc.Doc) error {
	if err := doc.SetAfter("remote", "sync.branch", "main"); err != nil {
		return err
	}
	return doc.SetBefore("beta", "features.alpha", true)
}

// deleteKey removes a member with its attached comments and its comma.
//
//ds:def id=deletekey-tq94ac46
func deleteKey(doc *jsonc.Doc) error {
	return doc.Delete("features.beta")
}

// disableKey comments a member out so it no longer counts but stays in the
// file for a person to turn back on.
//
//ds:def id=disablekey-xhvfs3up
func disableKey(doc *jsonc.Doc) error {
	return doc.Disable("sync.interval")
}

// restoreKey brings the disabled member back byte for byte.
//
//ds:def id=restorekey-j3679hwq
func restoreKey(w io.Writer, doc *jsonc.Doc) error {
	if err := doc.Restore("sync.interval"); err != nil {
		return err
	}
	interval, err := doc.GetInt("sync.interval")
	if err != nil {
		return err
	}
	again := doc.Restore("sync.interval")
	fmt.Fprintln(w, "restored interval:", interval, "| second Restore is ErrExists:", errors.Is(again, jsonc.ErrExists))
	return nil
}

// saveInPlace writes the edits back atomically. The file keeps the mode the
// user gave it rather than a default.
//
//ds:def id=saveinplace-fta7uvyk
func saveInPlace(w io.Writer, doc *jsonc.Doc) error {
	if err := doc.Save(); err != nil {
		return err
	}
	info, err := os.Stat(doc.Path())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "saved %s, mode %v\n", filepath.Base(doc.Path()), info.Mode().Perm())
	return nil
}

// createOnFirstRun opens a file that does not exist yet: Open returns an
// empty document instead of an error, so Open, Set, Save also works the first
// time. A new file is created 0600.
//
//ds:def id=createonfirstrun-ncqexkt5
func createOnFirstRun(w io.Writer, dir string) error {
	doc, err := jsonc.Open(filepath.Join(dir, "new.jsonc"))
	if err != nil {
		return err
	}
	existed := doc.Existed()
	if err := doc.Set("port", 8080); err != nil {
		return err
	}
	if err := doc.Save(); err != nil {
		return err
	}
	info, err := os.Stat(doc.Path())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "first run: existed=%v, created %s mode %v\n%s", existed, filepath.Base(doc.Path()), info.Mode().Perm(), doc.Render())
	return nil
}

// saveThroughSymlink opens a config through a symlink. Save writes the target
// file and leaves the link a link; SaveAs writes a copy elsewhere and Path
// keeps pointing at the original.
//
//ds:def id=savethroughsymlink-dufagknt
func saveThroughSymlink(w io.Writer, dir string) error {
	link := filepath.Join(dir, "link.jsonc")
	if err := os.Symlink(fileName, link); err != nil {
		return err
	}
	doc, err := jsonc.Open(link)
	if err != nil {
		return err
	}
	if err := doc.Set("sync.remote", "upstream"); err != nil {
		return err
	}
	if err := doc.Save(); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	backup := filepath.Join(dir, "backup.jsonc")
	if err := doc.SaveAs(backup); err != nil {
		return err
	}
	fmt.Fprintln(w, "link still a symlink:", info.Mode()&fs.ModeSymlink != 0, "| Path after SaveAs:", filepath.Base(doc.Path()))
	return nil
}

// saveParsed saves a document parsed from bytes. It has no path, so Save
// refuses until SetPath names one.
//
//ds:def id=saveparsed-v9mu2nvu
func saveParsed(w io.Writer, dir string) error {
	doc, err := jsonc.Parse([]byte("{\"port\": 8080}\n"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "no path:", doc.Save())
	doc.SetPath(filepath.Join(dir, "from-bytes.jsonc"))
	if err := doc.Save(); err != nil {
		return err
	}
	fmt.Fprintln(w, "after SetPath, saved", filepath.Base(doc.Path()))
	return nil
}
