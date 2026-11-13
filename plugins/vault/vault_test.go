package vault

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ubgo/textdata/plugin"
)

// Fake secret values. Every one carries secretMark so a test can assert no
// error message ever contains a secret.
const (
	secretMark = "SECRETMARK"
	testToken  = "test-token-value"
)

// fakeVault serves a tiny Vault HTTP API from a map of path -> response.
type fakeVault struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	secrets  map[string]string // "/v1/<path>[?version=N]" -> 200 body
	statuses map[string]int    // path -> forced status (body: Vault error shape)
	hits     atomic.Int64
	lastHdr  http.Header
	delay    time.Duration
}

func newFakeVault(t *testing.T) *fakeVault {
	f := &fakeVault{t: t, secrets: map[string]string{}, statuses: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	// KV v2
	f.secrets["/v1/secret/data/app"] = `{"data":{"data":{"password":"pw-` + secretMark + `","user":"app","port":5432,"tls":true,"opts":{"a": 1}},"metadata":{"version":3}}}`
	f.secrets["/v1/secret/data/app?version=1"] = `{"data":{"data":{"password":"old-` + secretMark + `"},"metadata":{"version":1}}}`
	f.secrets["/v1/secret/data/single"] = `{"data":{"data":{"token":"only-` + secretMark + `"},"metadata":{}}}`
	f.secrets["/v1/secret/data/deleted"] = `{"data":{"data":null,"metadata":{"deletion_time":"x"}}}`
	f.secrets["/v1/secret/data/empty"] = `{"data":{"data":{},"metadata":{}}}`
	// KV v1
	f.secrets["/v1/kv/app"] = `{"data":{"api_key":"k-` + secretMark + `","DB_PASSWORD":"db-` + secretMark + `"}}`
	// odd responses
	f.secrets["/v1/kv/garbage"] = `{"data":{"x":"` + secretMark + `"` // truncated JSON
	f.secrets["/v1/kv/nodata"] = `{"warnings":["x"]}`
	f.secrets["/v1/kv/sp%3Face"] = `{"data":{"v":"escaped"}}`
	return f
}

func (f *fakeVault) serve(w http.ResponseWriter, r *http.Request) {
	f.hits.Add(1)
	f.mu.Lock()
	f.lastHdr = r.Header.Clone()
	delay := f.delay
	key := r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		key += "?" + r.URL.RawQuery
	}
	status, forced := f.statuses[r.URL.EscapedPath()]
	body, ok := f.secrets[key]
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if r.Method != http.MethodGet {
		http.Error(w, "read-only plugin sent "+r.Method, http.StatusMethodNotAllowed)
		return
	}
	if forced {
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"errors":["forced %d"]}`, status)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"errors":[]}`)
		return
	}
	fmt.Fprint(w, body)
}

func (f *fakeVault) plugin(t *testing.T, opts ...Option) *Plugin {
	t.Helper()
	all := append([]Option{WithAddress(f.srv.URL), WithToken(testToken)}, opts...)
	p, err := New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestResolve(t *testing.T) {
	f := newFakeVault(t)
	p := f.plugin(t)
	cases := []struct{ ref, want string }{
		{"secret/data/app#password", "pw-" + secretMark},
		{"/secret/data/app#user", "app"},
		{"secret/data/app#port", "5432"},
		{"secret/data/app#tls", "true"},
		{"secret/data/app#opts", `{"a":1}`},
		{"secret/data/app?version=1#password", "old-" + secretMark},
		{"secret/data/single", "only-" + secretMark},
		{"kv/app#api_key", "k-" + secretMark},
	}
	for _, c := range cases {
		got, err := p.Resolve("vault", c.ref)
		if err != nil {
			t.Errorf("Resolve(%q): %v", c.ref, err)
			continue
		}
		if got != c.want {
			t.Errorf("Resolve(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
	if _, err := p.Resolve("VAULT", "kv/app#api_key"); err != nil {
		t.Errorf("scheme match must ignore case: %v", err)
	}
}

func TestResolveErrors(t *testing.T) {
	f := newFakeVault(t)
	f.statuses["/v1/secret/data/forbidden"] = http.StatusForbidden
	f.statuses["/v1/secret/data/unauth"] = http.StatusUnauthorized
	f.statuses["/v1/secret/data/sealed"] = http.StatusServiceUnavailable
	f.statuses["/v1/secret/data/boom"] = http.StatusInternalServerError
	f.statuses["/v1/secret/data/throttled"] = http.StatusTooManyRequests
	f.statuses["/v1/secret/data/standby"] = 412
	f.statuses["/v1/secret/data/bad"] = http.StatusBadRequest
	p := f.plugin(t)

	cases := []struct {
		ref  string
		want error // nil: an error that wraps none of the named errors
	}{
		{"secret/data/forbidden#x", plugin.ErrDenied},
		{"secret/data/unauth#x", plugin.ErrDenied},
		{"secret/data/sealed#x", plugin.ErrUnavailable},
		{"secret/data/boom#x", plugin.ErrUnavailable},
		{"secret/data/throttled#x", plugin.ErrUnavailable},
		{"secret/data/standby#x", plugin.ErrUnavailable},
		{"secret/data/missing#x", plugin.ErrNotFound},
		{"secret/data/app#nope", plugin.ErrNotFound},
		{"secret/data/deleted#x", plugin.ErrNotFound},
		{"secret/data/empty", plugin.ErrNotFound},
		{"kv/nodata#x", plugin.ErrNotFound},
		{"secret/data/app", ErrInvalidRef}, // several fields, none named
		{"secret/data/../sys", ErrInvalidRef},
		{"secret/data/app#", ErrInvalidRef},
		{"secret/data/app?v=2#x", ErrInvalidRef},
		{"secret/data/bad#x", nil},
		{"kv/garbage#x", nil},
	}
	named := []error{plugin.ErrDenied, plugin.ErrUnavailable, plugin.ErrNotFound, ErrInvalidRef}
	for _, c := range cases {
		_, err := p.Resolve("vault", c.ref)
		if err == nil {
			t.Errorf("Resolve(%q): no error", c.ref)
			continue
		}
		if strings.Contains(err.Error(), secretMark) || strings.Contains(err.Error(), testToken) {
			t.Errorf("Resolve(%q): error leaks a secret: %v", c.ref, err)
		}
		if c.want != nil {
			if !errors.Is(err, c.want) {
				t.Errorf("Resolve(%q) = %v, want wrapping %v", c.ref, err, c.want)
			}
			continue
		}
		for _, n := range named {
			if errors.Is(err, n) {
				t.Errorf("Resolve(%q) = %v, must not wrap %v", c.ref, err, n)
			}
		}
	}
	if _, err := p.Resolve("aws", "x"); !errors.Is(err, plugin.ErrUnsupportedScheme) {
		t.Errorf("foreign scheme: %v", err)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{401, plugin.ErrDenied},
		{403, plugin.ErrDenied},
		{404, plugin.ErrNotFound},
		{412, plugin.ErrUnavailable},
		{429, plugin.ErrUnavailable},
		{473, plugin.ErrUnavailable},
		{500, plugin.ErrUnavailable},
		{502, plugin.ErrUnavailable},
		{503, plugin.ErrUnavailable},
		{400, nil},
		{405, nil},
	}
	for _, c := range cases {
		err := statusError("read x", c.status, []byte(`{"errors":["why"]}`))
		if c.want == nil {
			if errors.Is(err, plugin.ErrDenied) || errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, plugin.ErrNotFound) {
				t.Errorf("status %d: %v must not wrap a named error", c.status, err)
			}
		} else if !errors.Is(err, c.want) {
			t.Errorf("status %d: %v, want %v", c.status, err, c.want)
		}
		if !strings.Contains(err.Error(), "why") {
			t.Errorf("status %d: Vault's message dropped: %v", c.status, err)
		}
	}
	// A body that is not Vault's error shape is never repeated.
	err := statusError("read x", 500, []byte("proxy says "+secretMark))
	if strings.Contains(err.Error(), secretMark) {
		t.Errorf("raw body repeated: %v", err)
	}
	long := `{"errors":["` + strings.Repeat("x", 1000) + `"]}`
	if err := statusError("read x", 500, []byte(long)); len(err.Error()) > maxDetail+100 {
		t.Errorf("detail not truncated: %d bytes", len(err.Error()))
	}
}

func TestUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	p, err := New(WithAddress(addr))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve("vault", "secret/data/app#password")
	if !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("closed server: %v, want ErrUnavailable", err)
	}
}

func TestTimeout(t *testing.T) {
	f := newFakeVault(t)
	f.delay = 200 * time.Millisecond
	p := f.plugin(t, WithTimeout(20*time.Millisecond))
	_, err := p.Resolve("vault", "secret/data/app#password")
	if !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("slow server: %v, want ErrUnavailable", err)
	}
}

func TestHeaders(t *testing.T) {
	f := newFakeVault(t)
	p := f.plugin(t, WithNamespace("team-a"))
	if _, err := p.Resolve("vault", "kv/app#api_key"); err != nil {
		t.Fatal(err)
	}
	if got := f.lastHdr.Get(headerToken); got != testToken {
		t.Errorf("token header = %q", got)
	}
	if got := f.lastHdr.Get(headerNamespace); got != "team-a" {
		t.Errorf("namespace header = %q", got)
	}

	p2, err := New(WithAddress(f.srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p2.Resolve("vault", "kv/app#DB_PASSWORD"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.lastHdr[headerToken]; ok {
		t.Errorf("empty token must send no header")
	}
	if _, ok := f.lastHdr[headerNamespace]; ok {
		t.Errorf("empty namespace must send no header")
	}
}

func TestEscapedPath(t *testing.T) {
	f := newFakeVault(t)
	p := f.plugin(t)
	// A '%' or '?' inside a segment cannot be written in a ref ('?' starts
	// the version), but escapePath keeps the request on the spelled path.
	if got := escapePath("kv/sp?ace"); got != "kv/sp%3Face" {
		t.Errorf("escapePath = %q", got)
	}
	v, err := p.Read(Ref{Path: "kv/sp?ace", Field: "v", HasField: true})
	if err != nil || v != "escaped" {
		t.Errorf("Read escaped = %q, %v", v, err)
	}
}

func TestCache(t *testing.T) {
	f := newFakeVault(t)
	p := f.plugin(t)
	for _, ref := range []string{"secret/data/app#password", "secret/data/app#user", "secret/data/app#port"} {
		if _, err := p.Resolve("vault", ref); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.hits.Load(); n != 1 {
		t.Errorf("three fields of one secret: %d requests, want 1", n)
	}
	if _, err := p.Resolve("vault", "secret/data/app?version=1#password"); err != nil {
		t.Fatal(err)
	}
	if n := f.hits.Load(); n != 2 {
		t.Errorf("another version is another secret: %d requests, want 2", n)
	}
	p.Purge()
	if _, err := p.Resolve("vault", "secret/data/app#password"); err != nil {
		t.Fatal(err)
	}
	if n := f.hits.Load(); n != 3 {
		t.Errorf("after Purge: %d requests, want 3", n)
	}

	// Failures are not cached: the next reference retries.
	f.mu.Lock()
	f.statuses["/v1/kv/flaky"] = http.StatusServiceUnavailable
	f.mu.Unlock()
	if _, err := p.Resolve("vault", "kv/flaky#a"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatal(err)
	}
	f.mu.Lock()
	delete(f.statuses, "/v1/kv/flaky")
	f.secrets["/v1/kv/flaky"] = `{"data":{"a":"recovered"}}`
	f.mu.Unlock()
	if v, err := p.Resolve("vault", "kv/flaky#a"); err != nil || v != "recovered" {
		t.Errorf("after recovery: %q, %v", v, err)
	}

	// Two plugins never share a cache.
	q := f.plugin(t)
	before := f.hits.Load()
	if _, err := q.Resolve("vault", "secret/data/app#password"); err != nil {
		t.Fatal(err)
	}
	if f.hits.Load() != before+1 {
		t.Errorf("a new plugin must not see another's cache")
	}
}

func TestConcurrent(t *testing.T) {
	f := newFakeVault(t)
	p := f.plugin(t, WithLookupPrefix("kv/app#"))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Resolve("vault", "secret/data/app#password"); err != nil {
				t.Error(err)
			}
			if _, ok, err := p.Lookup("DB_PASSWORD"); err != nil || !ok {
				t.Error(ok, err)
			}
			if i%4 == 0 {
				p.Purge()
			}
		}()
	}
	wg.Wait()
}

func TestLookup(t *testing.T) {
	f := newFakeVault(t)
	f.statuses["/v1/secret/data/forbidden"] = http.StatusForbidden
	p := f.plugin(t,
		WithLookup("DB_PASS", "secret/data/app#password"),
		WithLookup("GONE", "secret/data/missing#x"),
		WithLookup("LOCKED", "secret/data/forbidden#x"),
		WithLookupPrefix("secret/data/nothing-here#"),
		WithLookupPrefix("kv/app#"),
	)
	cases := []struct {
		name   string
		want   string
		ok     bool
		denied bool
	}{
		{"DB_PASS", "pw-" + secretMark, true, false},
		{"DB_PASSWORD", "db-" + secretMark, true, false}, // via second prefix
		{"api_key", "k-" + secretMark, true, false},
		{"GONE", "", false, false},
		{"UNKNOWN", "", false, false},
		{"LOCKED", "", false, true},
	}
	for _, c := range cases {
		v, ok, err := p.Lookup(c.name)
		if c.denied {
			if !errors.Is(err, plugin.ErrDenied) {
				t.Errorf("Lookup(%s) err = %v, want ErrDenied", c.name, err)
			}
			continue
		}
		if err != nil || ok != c.ok || v != c.want {
			t.Errorf("Lookup(%s) = %q, %v, %v; want %q, %v", c.name, v, ok, err, c.want, c.ok)
		}
	}

	// Without any mapping, Lookup never sends a request.
	q := f.plugin(t)
	before := f.hits.Load()
	if _, ok, err := q.Lookup("DB_PASSWORD"); ok || err != nil {
		t.Errorf("unmapped lookup = %v, %v", ok, err)
	}
	if f.hits.Load() != before {
		t.Errorf("unmapped lookup sent a request")
	}
}

func TestNew(t *testing.T) {
	bad := [][]Option{
		{},
		{WithAddress("vault.example.com:8200")},
		{WithAddress("ftp://vault.example.com")},
		{WithAddress("https://")},
		{WithAddress(withUserInfo("https", "vault.example.com"))},
		{WithAddress("https://vault.example.com"), WithLookup("X", "secret/../x")},
	}
	for i, opts := range bad {
		if _, err := New(opts...); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("case %d: New = %v, want ErrInvalidRef", i, err)
		}
	}
	p, err := New(WithAddress("https://vault.example.com:8200/"), WithName("vault-dr"), WithScheme("Vault-DR"), WithTimeout(-1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "vault-dr" || p.Schemes()[0] != "vault-dr" || p.timeout != DefaultTimeout || p.addr != "https://vault.example.com:8200" {
		t.Errorf("options not applied: %+v", p)
	}
	if _, err := p.Resolve("vault", "x#y"); !errors.Is(err, plugin.ErrUnsupportedScheme) {
		t.Errorf("renamed scheme still answered vault: %v", err)
	}
}

// The environment is read only through FromEnv, and only in New.
// withUserInfo builds a URL carrying user info at run time, so no source
// line spells one.
func withUserInfo(scheme, host string) string {
	return (&url.URL{Scheme: scheme, User: url.UserPassword("u", "p"), Host: host}).String()
}

func TestFromEnv(t *testing.T) {
	f := newFakeVault(t)
	t.Setenv(EnvAddr, f.srv.URL)
	t.Setenv(EnvToken, testToken)
	t.Setenv(EnvNamespace, "ns-env")

	if _, err := New(); err == nil {
		t.Fatal("New without FromEnv must not read VAULT_ADDR")
	}
	p, err := New(WithAddress("http://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	if p.token != "" || p.ns != "" {
		t.Fatalf("env read without FromEnv: %+v", p)
	}

	p, err = New(FromEnv())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Resolve("vault", "kv/app#api_key"); err != nil {
		t.Fatal(err)
	}
	if f.lastHdr.Get(headerToken) != testToken || f.lastHdr.Get(headerNamespace) != "ns-env" {
		t.Errorf("FromEnv headers: %v", f.lastHdr)
	}

	// A later option overrides the environment.
	p, err = New(FromEnv(), WithNamespace("explicit"))
	if err != nil {
		t.Fatal(err)
	}
	if p.ns != "explicit" {
		t.Errorf("later option lost: %q", p.ns)
	}
}
