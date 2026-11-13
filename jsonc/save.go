package jsonc

import (
	"fmt"

	"github.com/ubgo/textdata/atomicfile"
	"github.com/ubgo/textdata/plugin"
)

// tempSuffix names Save's temp file next to the target.
const tempSuffix = ".jsonc-tmp"

// Save writes the document to its path atomically (temp file, fsync,
// rename), keeping the file's mode and writing through a symlink. Every
// SaveGuard runs first, on a clone; the first refusal aborts with the file
// untouched. A document with syntax errors is saved byte for byte too: Save
// never rewrites what it was given.
func (d *Doc) Save() error {
	if d.path == "" {
		return fmt.Errorf("jsonc: no path set; use Open, or SetPath before Save")
	}
	return d.write(d.path, atomicfile.Options{Mode: d.mode, KeepMode: d.existed})
}

// SaveAs writes the document to another path, leaving Path unchanged. An
// existing destination keeps its own mode; a new one gets this document's.
func (d *Doc) SaveAs(path string) error {
	if path == "" {
		return fmt.Errorf("jsonc: SaveAs needs a path")
	}
	return d.write(path, atomicfile.Options{Mode: d.mode, KeepMode: true})
}

func (d *Doc) write(path string, o atomicfile.Options) error {
	for i, g := range d.opts.guards {
		if err := g.GuardSave(d.Clone()); err != nil {
			return plugin.Errorf(errPrefix, d.opts.reg.NameFor(plugin.CapSaveGuard, i), "guard save "+path, err)
		}
	}
	o.TempSuffix = tempSuffix
	if err := atomicfile.Write(path, d.src, o); err != nil {
		return fmt.Errorf("jsonc: %w", err)
	}
	return nil
}
