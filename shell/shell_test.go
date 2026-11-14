package shell

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/plugin"
)

// A Runner is usable wherever a plugin.CommandRunner is accepted.
var _ plugin.CommandRunner = Runner{}

// fakeLook stands in for exec.LookPath: programs in found resolve to a fake
// absolute path, anything else is exec.ErrNotFound. It lets the Windows
// branch run on any OS.
func fakeLook(found ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, f := range found {
			if f == name {
				return `C:\bin\` + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
}

// TestDefaultShellPerOS pins the shell choice for each OS through the
// injected lookup: sh -c off Windows; pwsh, else powershell.exe, on Windows.
func TestDefaultShellPerOS(t *testing.T) {
	ps := []string{"-NoProfile", "-NonInteractive", "-Command"}
	tests := []struct {
		name  string
		goos  string
		found []string
		want  []string
	}{
		{"linux", "linux", []string{"sh"}, []string{`C:\bin\sh`, "-c"}},
		{"darwin", "darwin", []string{"sh"}, []string{`C:\bin\sh`, "-c"}},
		{"windows with pwsh", "windows", []string{"pwsh", "powershell.exe"}, append([]string{`C:\bin\pwsh`}, ps...)},
		{"windows without pwsh", "windows", []string{"powershell.exe"}, append([]string{`C:\bin\powershell.exe`}, ps...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Runner{goos: tt.goos, lookPath: fakeLook(tt.found...)}.argv()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("argv = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWindowsNoShell: a Windows machine with neither PowerShell is
// ErrNoShell, from argv and from Run, and nothing runs.
func TestWindowsNoShell(t *testing.T) {
	r := Runner{goos: "windows", lookPath: fakeLook()}
	_, err := r.Run(context.Background(), "Write-Output hi")
	if !errors.Is(err, ErrNoShell) {
		t.Fatalf("err = %v, want ErrNoShell", err)
	}
	if !strings.Contains(err.Error(), "powershell.exe") || !strings.Contains(err.Error(), "Write-Output hi") {
		t.Errorf("error should name the shell and the command: %v", err)
	}
}

// TestShellOverrideIgnoresOS: Runner.Shell wins over the OS default, on
// Windows too (cmd /C), and is not modified by Run.
func TestShellOverrideIgnoresOS(t *testing.T) {
	sh := []string{"cmd", "/C"}
	got, err := Runner{Shell: sh, goos: "windows", lookPath: fakeLook("cmd", "pwsh")}.argv()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{`C:\bin\cmd`, "/C"}; !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if sh[0] != "cmd" {
		t.Errorf("Runner.Shell was modified: %q", sh)
	}
}

// TestDefaultShellReturnsCopy: DefaultShell's result can be changed by the
// caller without changing what Run uses.
func TestDefaultShellReturnsCopy(t *testing.T) {
	a := DefaultShell()
	a[0] = "changed"
	if b := DefaultShell(); b[0] == "changed" {
		t.Fatal("DefaultShell shares its slice")
	}
}

// TestHeadWriter: stderr is kept up to MaxStderr bytes, marked when cut, and
// a multi-byte character split by the cut is dropped, not left broken.
func TestHeadWriter(t *testing.T) {
	w := &headWriter{max: 4}
	w.Write([]byte("ab"))
	w.Write([]byte("cé")) // 'é' is two bytes; the cut falls inside it
	w.Write([]byte("more"))
	if got := w.String(); got != "abc"+truncMark {
		t.Errorf("String() = %q", got)
	}
	short := &headWriter{max: MaxStderr}
	short.Write([]byte("boom\n"))
	if got := short.String(); got != "boom" {
		t.Errorf("String() = %q, want %q", got, "boom")
	}
}
