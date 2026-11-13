package gopassplugin

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ubgo/textdata/plugin"
)

// exitErr carries an exit status the way *exec.ExitError does.
type exitErr int

func (e exitErr) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitErr) ExitCode() int { return int(e) }

// fakeStore is a gopass stand-in: a map of entries, a record of every argv.
type fakeStore struct {
	mu      sync.Mutex
	entries map[string]string
	calls   [][]string
	fail    error  // returned for every call when set
	stderr  string // returned with fail
}

func (f *fakeStore) run(_ context.Context, binary string, args []string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{binary}, args...))
	if f.fail != nil {
		return nil, []byte(f.stderr), f.fail
	}
	ref := args[len(args)-1]
	v, ok := f.entries[ref]
	if !ok {
		return nil, []byte(fmt.Sprintf("failed to retrieve secret %q: entry is not in the password store\n", ref)), exitErr(11)
	}
	return []byte(v + "\n"), nil, nil
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newFake(entries map[string]string) *fakeStore { return &fakeStore{entries: entries} }

func TestResolve(t *testing.T) {
	t.Parallel()
	fs := newFake(map[string]string{"personal/db": "s3cr3t-value", "multi": "line1\n"})
	p := New(WithRunner(fs.run))
	got, err := p.Resolve("gopass", "personal/db")
	if err != nil || got != "s3cr3t-value" {
		t.Fatalf("Resolve = %q, %v", got, err)
	}
	want := [][]string{{"gopass", "show", "-o", "personal/db"}}
	if !reflect.DeepEqual(fs.calls, want) {
		t.Errorf("argv = %q, want %q", fs.calls, want)
	}
	// Scheme matching follows the formats: case-insensitive.
	if _, err := p.Resolve("GOPASS", "personal/db"); err != nil {
		t.Errorf("upper-case scheme: %v", err)
	}
	if _, err := p.Resolve("vault", "x"); !errors.Is(err, plugin.ErrUnsupportedScheme) {
		t.Errorf("other scheme err = %v", err)
	}
	if got, _ := p.Resolve("gopass", "multi"); got != "line1\n" {
		t.Errorf("only one newline is trimmed: %q", got)
	}
}

func TestTrimNewline(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"a\n": "a", "a\r\n": "a", "a\n\n": "a\n", "a": "a", "": "", " a \n": " a "} {
		if got := trimNewline(in); got != want {
			t.Errorf("trimNewline(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	p := New(WithRunner(newFake(nil).run))
	_, err := p.Resolve("gopass", "missing/entry")
	if !errors.Is(err, plugin.ErrNotFound) || errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "missing/entry") {
		t.Errorf("error does not name the ref: %v", err)
	}
	// The message alone is enough when a wrapper loses the exit code.
	fs := &fakeStore{fail: errors.New("exit status 1"), stderr: "Error: entry is not in the password store"}
	if _, err := New(WithRunner(fs.run)).Show("x"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("text-only not-found = %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	t.Parallel()
	cases := map[string]*fakeStore{
		"missing binary": {fail: &exec.Error{Name: "gopass", Err: exec.ErrNotFound}},
		"gpg failure":    {fail: exitErr(1), stderr: "gpg: decryption failed: No secret key\nmore"},
		"store locked":   {fail: errors.New("signal: killed")},
	}
	for name, fs := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := New(WithRunner(fs.run)).Resolve("gopass", "a/b")
			if !errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, plugin.ErrNotFound) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	_, err := New(WithRunner(cases["gpg failure"].run)).Show("a/b")
	if !strings.Contains(err.Error(), "decryption failed") || strings.Contains(err.Error(), "more") {
		t.Errorf("stderr detail = %v", err)
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	slow := func(ctx context.Context, _ string, _ []string) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, errors.New("signal: killed")
	}
	start := time.Now()
	_, err := New(WithRunner(slow), WithTimeout(20*time.Millisecond)).Show("a")
	if !errors.Is(err, plugin.ErrUnavailable) || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("timeout not applied")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(WithRunner(slow), WithContext(ctx), WithTimeout(0)).Show("a"); !errors.Is(err, plugin.ErrUnavailable) || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("canceled parent: %v", err)
	}
}

func TestSecretNeverInError(t *testing.T) {
	t.Parallel()
	const secret = "hunter2-very-secret"
	// A misbehaving wrapper echoes the secret on stderr and fails.
	leaky := func(context.Context, string, []string) ([]byte, []byte, error) {
		return []byte(secret + "\n"), []byte("warning: value " + secret + " looks weak"), exitErr(2)
	}
	_, err := New(WithRunner(leaky)).Show("a/b")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret in error: %v", err)
	}
	long := func(context.Context, string, []string) ([]byte, []byte, error) {
		return nil, []byte(strings.Repeat("x", 5000)), exitErr(1)
	}
	_, err = New(WithRunner(long)).Show("a")
	if len(err.Error()) > maxDetail+100 {
		t.Errorf("stderr not capped: %d bytes", len(err.Error()))
	}
}

func TestBadRefRunsNothing(t *testing.T) {
	t.Parallel()
	fs := newFake(nil)
	p := New(WithRunner(fs.run))
	for _, ref := range []string{"", "-c", "--help", "a\nb", "a\x00b", "a\tb"} {
		if _, err := p.Resolve("gopass", ref); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
	if fs.count() != 0 {
		t.Errorf("gopass ran for a bad ref: %q", fs.calls)
	}
}

func TestCache(t *testing.T) {
	t.Parallel()
	fs := newFake(map[string]string{"a": "1"})
	p := New(WithRunner(fs.run))
	for i := 0; i < 3; i++ {
		if v, err := p.Show("a"); v != "1" || err != nil {
			t.Fatal(v, err)
		}
	}
	if fs.count() != 1 {
		t.Errorf("cached: %d calls, want 1", fs.count())
	}
	// Failures are not cached.
	p.Show("missing")
	p.Show("missing")
	if fs.count() != 3 {
		t.Errorf("not-found cached: %d calls", fs.count())
	}
	p.ClearCache()
	p.Show("a")
	if fs.count() != 4 {
		t.Errorf("ClearCache: %d calls", fs.count())
	}

	off := newFake(map[string]string{"a": "1"})
	q := New(WithRunner(off.run), WithCache(false))
	q.Show("a")
	q.Show("a")
	if off.count() != 2 {
		t.Errorf("cache off: %d calls, want 2", off.count())
	}
}

func TestCacheConcurrent(t *testing.T) {
	t.Parallel()
	fs := newFake(map[string]string{"a": "1", "b": "2"})
	p := New(WithRunner(fs.run))
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ref := []string{"a", "b"}[i%2]
			if _, err := p.Show(ref); err != nil {
				t.Error(err)
			}
			if i%7 == 0 {
				p.ClearCache()
			}
		}(i)
	}
	wg.Wait()
}

func TestLookup(t *testing.T) {
	t.Parallel()
	fs := newFake(map[string]string{"app/DB_PASSWORD": "pw", "elsewhere/key": "k"})
	p := New(WithRunner(fs.run), WithPrefix("app/"), WithMapping(map[string]string{"API_KEY": "elsewhere/key"}))
	if v, ok, err := p.Lookup("DB_PASSWORD"); v != "pw" || !ok || err != nil {
		t.Errorf("prefix lookup = %q %v %v", v, ok, err)
	}
	if v, ok, err := p.Lookup("API_KEY"); v != "k" || !ok || err != nil {
		t.Errorf("mapped lookup = %q %v %v", v, ok, err)
	}
	if _, ok, err := p.Lookup("UNSET"); ok || err != nil {
		t.Errorf("missing entry must be ok=false, nil: %v %v", ok, err)
	}
	before := fs.count()
	for _, name := range []string{"", "-x", "../up", "a/b", `a\b`, "a\nb"} {
		if _, ok, err := p.Lookup(name); ok || err != nil {
			t.Errorf("Lookup(%q) = %v %v", name, ok, err)
		}
	}
	if fs.count() != before {
		t.Errorf("unsafe names ran gopass: %q", fs.calls[before:])
	}

	down := New(WithRunner((&fakeStore{fail: exitErr(1)}).run), WithPrefix("p/"))
	if _, ok, err := down.Lookup("X"); ok || !errors.Is(err, plugin.ErrUnavailable) {
		t.Errorf("store down must be an error, not unset: %v %v", ok, err)
	}
}

func TestLookupOffByDefault(t *testing.T) {
	t.Parallel()
	fs := newFake(map[string]string{"X": "1"})
	if _, ok, err := New(WithRunner(fs.run)).Lookup("X"); ok || err != nil || fs.count() != 0 {
		t.Errorf("Lookup without prefix/mapping ran: %v %v %d", ok, err, fs.count())
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()
	fs := newFake(map[string]string{"a": "1"})
	p := New(WithRunner(nil), WithRunner(fs.run), WithBinary(""), WithBinary("/opt/gopass"), WithContext(nil))
	p.Show("a")
	if fs.calls[0][0] != "/opt/gopass" {
		t.Errorf("binary = %q", fs.calls[0][0])
	}
	if p.Name() != "gopass" || !reflect.DeepEqual(p.Schemes(), []string{"gopass"}) {
		t.Error("Name/Schemes")
	}
	if p.ctx == nil || p.timeout != DefaultTimeout {
		t.Error("defaults")
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	t.Parallel()
	p := New(WithBinary("textdata-no-such-gopass-binary"))
	_, err := p.Show("a")
	if !errors.Is(err, plugin.ErrUnavailable) || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

// FuzzRef: no ref panics, a ref that runs always reaches gopass as exactly one
// trailing argument after "show -o", never as a flag.
func FuzzRef(f *testing.F) {
	for _, s := range []string{"personal/db", "", "-o", "--", "a b", "a\x00", "ü/ß", "../x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, ref string) {
		var argv []string
		run := func(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
			argv = args
			return []byte("v\n"), nil, nil
		}
		p := New(WithRunner(run), WithPrefix("p/"))
		v, err := p.Resolve("gopass", ref)
		if err != nil {
			if argv != nil {
				t.Fatalf("ran gopass for rejected ref %q", ref)
			}
		} else {
			if v != "v" || len(argv) != 3 || argv[0] != "show" || argv[1] != "-o" || argv[2] != ref || strings.HasPrefix(ref, "-") {
				t.Fatalf("ref %q: argv %q value %q", ref, argv, v)
			}
		}
		argv = nil
		p.Lookup(ref)
		if argv != nil && (len(argv) != 3 || strings.HasPrefix(argv[2], "-") || strings.Contains(argv[2], "..")) {
			t.Fatalf("Lookup(%q) argv %q", ref, argv)
		}
	})
}
