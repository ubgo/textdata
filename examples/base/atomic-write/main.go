// Command atomic-write replaces files with package atomicfile: a new file at
// 0600, an overwrite that keeps a file's mode, a write through a symlink, and a
// failed rename that leaves the old contents in place. Every format's Save uses
// this package; call it directly when your own tool writes a config file.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/atomicfile"
)

// Names of the files the demo creates inside its temp directory.
const (
	newFile    = "new.env"
	sharedFile = "shared.env"
	realFile   = "real.env"
	linkFile   = "link.env"
	safeFile   = "safe.env"
)

// sharedMode is the deliberately wider mode an existing file was given by its
// owner; KeepMode must preserve it rather than reset it to DefaultMode.
const sharedMode fs.FileMode = 0o644

func main() {
	dir, err := os.MkdirTemp("", "atomic-write-example-")
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

// run executes every step of the guide in dir.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	for _, step := range []func(io.Writer, string) error{writeNew, keepMode, throughSymlink, failedRename} {
		if err := step(w, dir); err != nil {
			return err
		}
	}
	return nil
}

// writeNew creates a file. With zero Options it gets DefaultMode (0600),
// because the files textdata writes often hold secrets.
//
//ds:def id=writenew-wxy7xh4t
func writeNew(w io.Writer, dir string) error {
	p := filepath.Join(dir, newFile)
	if err := atomicfile.Write(p, []byte("TOKEN=abc\n"), atomicfile.Options{}); err != nil {
		return err
	}
	return report(w, "new file", p)
}

// keepMode overwrites a file its owner made 0644. KeepMode keeps that mode;
// without it the overwrite would reset the file to Options.Mode or 0600.
//
//ds:def id=keepmode-6ehrb3xd
func keepMode(w io.Writer, dir string) error {
	p := filepath.Join(dir, sharedFile)
	if err := atomicfile.Write(p, []byte("A=1\n"), atomicfile.Options{Mode: sharedMode}); err != nil {
		return err
	}
	if err := atomicfile.Write(p, []byte("A=2\n"), atomicfile.Options{KeepMode: true}); err != nil {
		return err
	}
	return report(w, "kept mode", p)
}

// throughSymlink writes to a symlink. Write follows it and replaces the file
// it names, so the link survives; NoFollow would replace the link itself.
//
//ds:def id=throughsymlink-ynna9fr8
func throughSymlink(w io.Writer, dir string) error {
	target, link := filepath.Join(dir, realFile), filepath.Join(dir, linkFile)
	if err := os.Symlink(realFile, link); err != nil { // dangling until the write
		return err
	}
	resolved, err := atomicfile.Resolve(link)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(link, []byte("VIA=link\n"), atomicfile.Options{}); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "resolve: %s, link still a link: %v\n", filepath.Base(resolved), info.Mode()&fs.ModeSymlink != 0)
	return report(w, "via link", target)
}

// failedRename injects a rename failure, standing in for a crash or a full
// disk. The target keeps its old bytes and the temp file is cleaned up.
//
//ds:def id=failedrename-2zj7reuu
func failedRename(w io.Writer, dir string) error {
	p := filepath.Join(dir, safeFile)
	if err := atomicfile.Write(p, []byte("OLD=1\n"), atomicfile.Options{}); err != nil {
		return err
	}
	errDisk := errors.New("disk full")
	err := atomicfile.Write(p, []byte("NEW=1\n"), atomicfile.Options{
		Rename: func(string, string) error { return errDisk },
	})
	fmt.Fprintf(w, "write error: %v (is disk full: %v)\n", err, errors.Is(err, errDisk))
	_, statErr := os.Stat(p + atomicfile.DefaultTempSuffix)
	fmt.Fprintf(w, "temp file left behind: %v\n", !errors.Is(statErr, fs.ErrNotExist))
	return report(w, "after failure", p)
}

// report prints a file's mode and contents.
func report(w io.Writer, label, p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-13s %s %v %q\n", label+":", filepath.Base(p), info.Mode().Perm(), data)
	return nil
}
