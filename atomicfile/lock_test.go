package atomicfile

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Child-process roles, selected by lockChildEnv. The test binary re-executes
// itself (TestLockChild) so the contention is between real processes, the
// case Lock exists for; goroutines alone would not prove it.
const (
	lockChildEnv  = "ATOMICFILE_LOCK_CHILD"
	lockChildPath = "ATOMICFILE_LOCK_PATH"
	lockChildN    = "ATOMICFILE_LOCK_N"

	roleIncrement = "increment" // read-modify-write a counter N times, each under the lock
	roleHold      = "hold"      // take the lock, print lockedLine, then wait to be killed

	lockedLine = "locked"
)

// TestLockChild is the child side of the multi-process tests; it does nothing
// in a normal run.
func TestLockChild(t *testing.T) {
	role := os.Getenv(lockChildEnv)
	if role == "" {
		t.Skip("child process role only")
	}
	path := os.Getenv(lockChildPath)
	ctx := context.Background()
	switch role {
	case roleIncrement:
		n, _ := strconv.Atoi(os.Getenv(lockChildN))
		for i := 0; i < n; i++ {
			if err := incrementLocked(ctx, path); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	case roleHold:
		lk, err := Lock(ctx, path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(lockedLine)
		time.Sleep(time.Minute) // killed by the parent long before this
		_ = lk.Close()
	}
	os.Exit(0)
}

// incrementLocked is the read-modify-write Lock protects: read a number,
// yield (to widen the race window), write number+1 with Write.
func incrementLocked(ctx context.Context, path string) error {
	lk, err := Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lk.Close()
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	time.Sleep(100 * time.Microsecond)
	return Write(path, []byte(strconv.Itoa(n+1)), Options{})
}

// child starts the test binary in role against path.
func child(t *testing.T, role, path string, extra ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockChild$")
	cmd.Env = append(os.Environ(), lockChildEnv+"="+role, lockChildPath+"="+path)
	cmd.Env = append(cmd.Env, extra...)
	cmd.Stderr = os.Stderr
	return cmd
}

// TestLockProcessesDoNotLoseEdits: four processes each increment a counter
// 50 times under the lock; every increment survives.
// ds:def id=testlockprocessesdonotloseedits-vtgj7tkj owner=@maintainers stability=stable
func TestLockProcessesDoNotLoseEdits(t *testing.T) {
	t.Parallel()
	const procs, each = 4, 50
	path := filepath.Join(t.TempDir(), "counter")
	cmds := make([]*exec.Cmd, procs)
	for i := range cmds {
		cmds[i] = child(t, roleIncrement, path, lockChildN+"="+strconv.Itoa(each))
		if err := cmds[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("child: %v", err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != strconv.Itoa(procs*each) {
		t.Fatalf("counter = %s, want %d: edits were lost", got, procs*each)
	}
}

// holdInChild starts a child that holds the lock on path and returns once it
// reports holding it.
func holdInChild(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	cmd := child(t, roleHold, path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != lockedLine {
		t.Fatalf("child did not take the lock: %q, %v", line, err)
	}
	return cmd
}

// TestLockTimeout: while another process holds the lock, Lock with a deadline
// fails with ErrLockTimeout and context.DeadlineExceeded, naming the lock file.
// ds:def id=testlocktimeout-ahj9bk9q owner=@maintainers stability=stable
func TestLockTimeout(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.jsonc")
	holdInChild(t, path)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	lk, err := Lock(ctx, path)
	if err == nil {
		_ = lk.Close()
		t.Fatal("Lock succeeded while another process held it")
	}
	if !errors.Is(err, ErrLockTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrLockTimeout and DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), path+DefaultLockSuffix) {
		t.Errorf("err = %v, want it to name the lock file", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Lock waited %v past a 100ms deadline", d)
	}
}

// TestLockCancel: a cancelled context is context.Canceled, not a timeout.
func TestLockCancel(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "f")
	lk, err := Lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err = Lock(ctx, path)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err = %v, want context.Canceled only", err)
	}
}

// TestLockReleasedWhenHolderDies: killing the holding process (SIGKILL, no
// cleanup) frees the lock; nothing stale is left.
// ds:def id=testlockreleasedwhenholderdies-w6jdfhs9 owner=@maintainers stability=stable
func TestLockReleasedWhenHolderDies(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "f")
	cmd := holdInChild(t, path)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lk, err := Lock(ctx, path)
	if err != nil {
		t.Fatalf("Lock after the holder died: %v", err)
	}
	if err := lk.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestLockGoroutinesExclude: two Locks in one process exclude each other, and
// Close is idempotent and nil-safe.
func TestLockGoroutinesExclude(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "f")
	first, err := Lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		lk, err := Lock(context.Background(), path)
		if err == nil {
			err = lk.Close()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("second Lock returned (%v) while the first was held", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	if err := <-got; err != nil {
		t.Fatal(err)
	}
	var none *FileLock
	if err := none.Close(); err != nil {
		t.Errorf("nil Close = %v", err)
	}
}

// TestLockFollowsSymlinkAndKeepsSidecar: a link and its target share one lock
// (the sidecar sits next to the real file), and the sidecar is left in place
// after Close.
func TestLockFollowsSymlinkAndKeepsSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "real.jsonc")
	link := filepath.Join(dir, "link.jsonc")
	if err := os.WriteFile(real, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	lk, err := Lock(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	if lk.Path() != real+DefaultLockSuffix {
		t.Errorf("Path = %s, want %s", lk.Path(), real+DefaultLockSuffix)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := Lock(ctx, real); !errors.Is(err, ErrLockTimeout) {
		t.Errorf("lock of the target while the link is locked: %v, want ErrLockTimeout", err)
	}
	if err := lk.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(real + DefaultLockSuffix); err != nil {
		t.Errorf("sidecar removed: %v", err)
	}
}

// TestLockMissingDir: the directory must exist; the error says so.
func TestLockMissingDir(t *testing.T) {
	t.Parallel()
	_, err := Lock(context.Background(), filepath.Join(t.TempDir(), "nope", "f"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}
