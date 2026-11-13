package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
)

// secretPath is the value the guard protects.
const secretPath = "db.password"

// errPlaintextSecret is the guard's refusal, matchable with errors.Is.
//
//ds:def id=errplaintextsecret-9xt5krkp
var errPlaintextSecret = errors.New("plaintext secret; write a ${scheme:ref} reference instead")

// noPlaintextSecret is a SaveGuard: it sees a read-only view of the document
// about to be written and refuses a literal password. A reference such as
// ${gopass:db} is fine, because the secret itself never lands on disk.
//
//ds:def id=noplaintextsecret-tngjgu3d
type noPlaintextSecret struct{}

//ds:def id=noplaintextsecret-name-xjvfd6gh
func (noPlaintextSecret) Name() string { return "no-plaintext-secret" }

//ds:def id=noplaintextsecret-guardsave-222q27ev
func (noPlaintextSecret) GuardSave(doc plugin.Document) error {
	v, ok := doc.ValueAt(secretPath)
	if ok && !strings.HasPrefix(v, "${") {
		return errPlaintextSecret
	}
	return nil
}

// guardSave installs the guard and tries to save a literal password. The
// save is refused and the file is not touched; a reference saves fine.
//
//ds:def id=guardsave-t9bk2qdw
func guardSave(w io.Writer, dir string) error {
	path := filepath.Join(dir, "guarded.jsonc")
	doc, err := jsonc.Open(path, jsonc.WithPlugin(noPlaintextSecret{}))
	if err != nil {
		return err
	}
	if err := doc.Set(secretPath, "hunter2"); err != nil {
		return err
	}
	refused := doc.Save()
	_, statErr := os.Stat(path)
	// The error names the file; print it without the scratch folder.
	fmt.Fprintln(w, "refused:", strings.ReplaceAll(fmt.Sprint(refused), dir+string(filepath.Separator), ""))
	fmt.Fprintln(w, "is errPlaintextSecret:", errors.Is(refused, errPlaintextSecret), "| file written:", !errors.Is(statErr, fs.ErrNotExist))
	if err := doc.Set(secretPath, "${gopass:db}"); err != nil {
		return err
	}
	return doc.Save()
}
