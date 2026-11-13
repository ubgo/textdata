// Command saving writes edited .env files back to disk: Save to the opened
// path, the error for a parsed file with no path, which mode each case gets,
// line endings and the trailing newline kept, SaveAs to another path, and a
// save through a symlink.
//
// Run it with `go run .`; it works in a fresh temp directory. The modes it
// prints are Unix permission bits.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/dotenv"
)

// Modes used to show which permissions a save keeps.
const (
	lockedMode fs.FileMode = 0o400 // a file someone locked down to read-only
	sharedMode fs.FileMode = 0o644 // an existing destination for SaveAs
)

func main() {
	dir, err := os.MkdirTemp("", "dotenv-saving")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = run(os.Stdout, dir)
	// Cleanup is best-effort: the run's result matters more than a leftover
	// temp directory.
	_ = os.RemoveAll(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run prints every step under a heading so the test can pin the whole output.
func run(w io.Writer, dir string) error {
	steps := []struct {
		title string
		show  func(io.Writer, string) error
	}{
		{"Save", saveOpened},
		{"no path", saveNoPath},
		{"modes", modes},
		{"line endings", lineEndings},
		{"SaveAs", saveAs},
		{"symlink", symlink},
	}
	for _, s := range steps {
		fmt.Fprintln(w, "==", s.title)
		if err := s.show(w, dir); err != nil {
			return err
		}
	}
	return nil
}

// write puts content at dir/name with exactly mode, past the umask.
func write(dir, name, content string, mode fs.FileMode) (string, error) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return "", err
	}
	return path, os.Chmod(path, mode)
}

// saveOpened edits an opened file and saves it in place. The write goes
// through a sibling temp file and a rename, so nothing is left beside it.
//
//ds:def id=saveopened-b6tw2q8m
func saveOpened(w io.Writer, dir string) error {
	path, err := write(dir, ".env", "# app\nPORT=8080\n", 0o600)
	if err != nil {
		return err
	}
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	f.Set("PORT", "9090")
	if err := f.Save(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%q, files in dir: %d\n", data, len(entries))
	return nil
}

// saveNoPath shows that a parsed File has no Path, so Save refuses instead of
// guessing; setting Path (or calling SaveAs) fixes it.
//
//ds:def id=savenopath-k3fx9c5r
func saveNoPath(w io.Writer, dir string) error {
	f := dotenv.Parse("A=1\n")
	fmt.Fprintln(w, f.Save())
	f.Path = filepath.Join(dir, ".env.parsed")
	if err := f.Save(); err != nil {
		return err
	}
	info, err := os.Stat(f.Path)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "saved with mode", info.Mode().Perm())
	return nil
}

// modes shows the rules: a file that existed keeps the mode Open saw, a new
// file gets 0600, and SaveAs onto an existing file keeps that file's mode.
//
//ds:def id=modes-q8vm4z2h
func modes(w io.Writer, dir string) error {
	locked, err := write(dir, ".env.locked", "A=1\n", lockedMode)
	if err != nil {
		return err
	}
	shared, err := write(dir, ".env.shared", "", sharedMode)
	if err != nil {
		return err
	}
	f, err := dotenv.Open(locked)
	if err != nil {
		return err
	}
	f.Set("A", "2")
	for _, target := range []string{locked, filepath.Join(dir, ".env.copy"), shared} {
		if err := f.SaveAs(target); err != nil {
			return err
		}
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-12s %v\n", filepath.Base(target), info.Mode().Perm())
	}
	return nil
}

// lineEndings shows that each line keeps its own terminator, new lines in a
// CRLF file end in \r\n, and a missing trailing newline stays missing.
//
//ds:def id=lineendings-n5dk7w3x
func lineEndings(w io.Writer, _ string) error {
	crlf := dotenv.Parse("A=1\r\nB=2\r\n")
	crlf.Set("B", "3")
	crlf.Set("C", "4")
	fmt.Fprintf(w, "CRLF:       %q\n", crlf.Render())

	mixed := dotenv.Parse("A=1\nB=2\r\n")
	mixed.Set("A", "x")
	fmt.Fprintf(w, "mixed:      %q\n", mixed.Render())

	noNewline := dotenv.Parse("A=1")
	noNewline.Set("B", "2")
	fmt.Fprintf(w, "no newline: %q\n", noNewline.Render())

	scratch := dotenv.Parse("")
	scratch.Set("A", "1")
	fmt.Fprintf(w, "scratch:    %q\n", scratch.Render())
	return nil
}

// saveAs writes variants of one loaded file. f.Path never changes, so the
// source file is never touched.
//
//ds:def id=saveas-c2hr6p9t
func saveAs(w io.Writer, dir string) error {
	f := dotenv.Parse("DOMAIN=staging.acme.io\n")
	f.Path = filepath.Join(dir, ".env.staging")
	for _, v := range []struct{ name, domain string }{{".env.prod", "acme.io"}, {".env.qa", "qa.acme.io"}} {
		f.Set("DOMAIN", v.domain)
		if err := f.SaveAs(filepath.Join(dir, v.name)); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, v.name))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %q\n", v.name, data)
	}
	fmt.Fprintln(w, "Path still:", filepath.Base(f.Path))
	fmt.Fprintln(w, "empty path:", f.SaveAs(""))
	return nil
}

// symlink saves through a link: the file it points to is replaced, and the
// link stays a link.
//
//ds:def id=symlink-x4bj8m2f
func symlink(w io.Writer, dir string) error {
	target, err := write(dir, "real.env", "A=1\n", 0o600)
	if err != nil {
		return err
	}
	link := filepath.Join(dir, "link.env")
	if err := os.Symlink(target, link); err != nil {
		return err
	}
	f, err := dotenv.Open(link)
	if err != nil {
		return err
	}
	f.Set("A", "2")
	if err := f.Save(); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "still a link: %v, target now %q\n", info.Mode()&fs.ModeSymlink != 0, data)
	return nil
}
