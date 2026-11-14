//go:build windows

package shell

import (
	"context"
	"errors"
	"testing"
	"time"
)

// These run the real PowerShell on a Windows machine. There is no Windows CI;
// a maintainer runs them by hand: go test ./shell/ on Windows.

func TestWindowsEcho(t *testing.T) {
	out, err := Run(context.Background(), "Write-Output hello")
	if err != nil || out != "hello" {
		t.Fatalf("Run = %q, %v; want hello", out, err)
	}
}

func TestWindowsExitCode(t *testing.T) {
	_, err := Run(context.Background(), "exit 3")
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 3 {
		t.Fatalf("err = %v, want *ExitError code 3", err)
	}
}

func TestWindowsTimeout(t *testing.T) {
	_, err := Runner{Timeout: 200 * time.Millisecond}.Run(context.Background(), "Start-Sleep -Seconds 30")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestWindowsCmdOverride(t *testing.T) {
	out, err := Runner{Shell: []string{"cmd", "/C"}}.Run(context.Background(), "echo hello")
	if err != nil || out != "hello" {
		t.Fatalf("Run = %q, %v; want hello", out, err)
	}
}
