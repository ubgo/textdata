package jsonc

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadMissingFile: Read is for consuming a file that should exist, so a
// missing one is an error wrapping fs.ErrNotExist and naming the path, while
// Open still yields an empty document Save can create.
func TestReadMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "none.jsonc")
	m, err := Read(p)
	if !errors.Is(err, fs.ErrNotExist) || m != nil {
		t.Fatalf("Read(missing) = %v, %v; want nil map and an fs.ErrNotExist error", m, err)
	}
	if !strings.Contains(err.Error(), p) {
		t.Errorf("Read(missing) error %q does not name the file", err)
	}
	d, err := Open(p)
	if err != nil || d.Existed() {
		t.Errorf("Open(missing) = %v, %v; want an empty document", d, err)
	}
}
