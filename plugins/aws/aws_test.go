package aws

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ubgo/textdata/plugin"
)

// Fake values. Every secret carries secretMark so a test can assert no
// error message ever contains one. The credentials are obviously fake and
// do not match any vendor key pattern.
const (
	secretMark = "SECRETMARK"
	testRegion = "eu-test-1"
)

var testCreds = Credentials{AccessKeyID: "TESTKEYID", SecretAccessKey: "test-secret-key-" + secretMark}

// apiFailure is a forced error response.
type apiFailure struct {
	status int
	code   string // "__type"; "" sends a body without one
}

// fakeAWS serves GetSecretValue and GetParameter and verifies every
// request's SigV4 signature against testCreds.
type fakeAWS struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	sm       map[string]string // secret id ("|stage" / "|v:id" suffix) -> SecretString
	smBin    map[string][]byte
	ssm      map[string]string // name -> value
	failures map[string]apiFailure
	hits     atomic.Int64
	lastHdr  http.Header
	lastBody string
	delay    time.Duration
}

func newFakeAWS(t *testing.T) *fakeAWS {
	f := &fakeAWS{
		t: t,
		sm: map[string]string{
			"prod/db":             `{"user":"app","password":"pw-` + secretMark + `","port":5432,"tags":["a"]}`,
			"prod/db|AWSPREVIOUS": `{"password":"old-` + secretMark + `"}`,
			"prod/db|v:abc-123":   `{"password":"v-` + secretMark + `"}`,
			"prod/api-key":        "plain-" + secretMark,
			"prod/app":            `{"DB_PASSWORD":"db-` + secretMark + `","API_KEY":"k-` + secretMark + `"}`,
			"arn:aws:secretsmanager:eu-test-1:000000000000:secret:prod/x-AbCdEf": "arn-value",
		},
		smBin:    map[string][]byte{"prod/bin": []byte("bin-" + secretMark)},
		ssm:      map[string]string{"/app/prod/db_url": "url-" + secretMark, "plain": "p", "/app/prod/LOG_LEVEL": "debug", "/app/json": `{"k":"v"}`},
		failures: map[string]apiFailure{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAWS) fail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", jsonContent)
	w.WriteHeader(status)
	if code == "" {
		fmt.Fprintf(w, `{"message":%q}`, msg)
		return
	}
	fmt.Fprintf(w, `{"__type":"com.amazonaws.test#%s","message":%q}`, code, msg)
}

func (f *fakeAWS) serve(w http.ResponseWriter, r *http.Request) {
	f.hits.Add(1)
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.lastHdr = r.Header.Clone()
	f.lastBody = string(body)
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if r.Method != http.MethodPost || r.URL.Path != "/" || r.Header.Get("Content-Type") != jsonContent {
		f.fail(w, 400, "SerializationException", "bad request shape")
		return
	}
	target := r.Header.Get(headerTarget)
	service := map[string]string{smTarget: smSigningName, ssmTarget: ssmSigningName}[target]
	if service == "" {
		f.fail(w, 400, "UnknownOperationException", "operation "+target)
		return
	}
	if msg := verifySignature(r, body, service); msg != "" {
		f.fail(w, 403, "InvalidSignatureException", msg)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	switch target {
	case smTarget:
		var in smRequest
		if err := json.Unmarshal(body, &in); err != nil {
			f.fail(w, 400, "SerializationException", "bad json")
			return
		}
		if fl, ok := f.failures[in.SecretID]; ok {
			f.fail(w, fl.status, fl.code, "forced for "+in.SecretID)
			return
		}
		key := in.SecretID
		if in.VersionStage != "" {
			key += "|" + in.VersionStage
		}
		if in.VersionID != "" {
			key += "|v:" + in.VersionID
		}
		if v, ok := f.sm[key]; ok {
			json.NewEncoder(w).Encode(map[string]any{"Name": in.SecretID, "SecretString": v})
			return
		}
		if b, ok := f.smBin[key]; ok {
			json.NewEncoder(w).Encode(map[string]any{"Name": in.SecretID, "SecretBinary": base64.StdEncoding.EncodeToString(b)})
			return
		}
		f.fail(w, 400, "ResourceNotFoundException", "Secrets Manager can't find the specified secret.")
	case ssmTarget:
		var in ssmRequest
		if err := json.Unmarshal(body, &in); err != nil || !in.WithDecryption {
			f.fail(w, 400, "ValidationException", "decryption not requested")
			return
		}
		if fl, ok := f.failures[in.Name]; ok {
			f.fail(w, fl.status, fl.code, "forced for "+in.Name)
			return
		}
		if v, ok := f.ssm[in.Name]; ok {
			json.NewEncoder(w).Encode(map[string]any{"Parameter": map[string]any{"Name": in.Name, "Type": "SecureString", "Value": v}})
			return
		}
		f.fail(w, 400, "ParameterNotFound", "")
	}
}

// verifySignature re-signs the signed parts of r with testCreds and
// compares; "" means valid.
func verifySignature(r *http.Request, body []byte, service string) string {
	auth := r.Header.Get(headerAuth)
	prefix := sigAlgorithm + " Credential=" + testCreds.AccessKeyID + "/"
	if !strings.HasPrefix(auth, prefix) {
		return "unknown key or algorithm"
	}
	parts := strings.Split(strings.TrimPrefix(auth, prefix), ", ")
	if len(parts) != 3 {
		return "malformed Authorization"
	}
	scope := strings.Split(parts[0], "/")
	if len(scope) != 4 || scope[1] != testRegion || scope[2] != service || scope[3] != sigTerminator {
		return "wrong scope " + parts[0]
	}
	signed := strings.Split(strings.TrimPrefix(parts[1], "SignedHeaders="), ";")
	ts, err := time.Parse(sigTimeFormat, r.Header.Get(headerAmzDate))
	if err != nil {
		return "bad X-Amz-Date"
	}
	re := &http.Request{Method: r.Method, URL: &url.URL{Path: r.URL.Path, RawQuery: r.URL.RawQuery}, Host: r.Host, Header: http.Header{}}
	for _, h := range signed {
		if h != "host" && h != strings.ToLower(headerAmzDate) && h != strings.ToLower(headerAmzToken) {
			re.Header[http.CanonicalHeaderKey(h)] = r.Header.Values(h)
		}
	}
	cr := testCreds
	cr.SessionToken = r.Header.Get(headerAmzToken)
	res := signV4(re, body, cr, testRegion, service, ts, signOptions{normalize: true})
	if "Signature="+res.signature != parts[2] {
		return "signature mismatch"
	}
	return ""
}

func (f *fakeAWS) plugin(t *testing.T, opts ...Option) *Plugin {
	t.Helper()
	all := append([]Option{WithRegion(testRegion), WithCredentials(testCreds), WithEndpoint(f.srv.URL)}, opts...)
	p, err := New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestResolve(t *testing.T) {
	f := newFakeAWS(t)
	p := f.plugin(t)
	cases := []struct{ ref, want string }{
		{"sm/prod/db#password", "pw-" + secretMark},
		{"sm/prod/db#user", "app"},
		{"sm/prod/db#port", "5432"},
		{"sm/prod/db#tags", `["a"]`},
		{"sm/prod/db?stage=AWSPREVIOUS#password", "old-" + secretMark},
		{"sm/prod/db?version=abc-123#password", "v-" + secretMark},
		{"sm/prod/api-key", "plain-" + secretMark},
		{"sm/prod/bin", "bin-" + secretMark},
		{"sm/arn:aws:secretsmanager:eu-test-1:000000000000:secret:prod/x-AbCdEf", "arn-value"},
		{"ssm//app/prod/db_url", "url-" + secretMark},
		{"ssm/plain", "p"},
		{"ssm//app/json#k", "v"},
	}
	for _, c := range cases {
		got, err := p.Resolve("aws", c.ref)
		if err != nil {
			t.Errorf("Resolve(%q): %v", c.ref, err)
			continue
		}
		if got != c.want {
			t.Errorf("Resolve(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
	if _, err := p.Resolve("AWS", "ssm/plain"); err != nil {
		t.Errorf("scheme match must ignore case: %v", err)
	}
}

func TestSessionTokenSigned(t *testing.T) {
	f := newFakeAWS(t)
	cr := testCreds
	cr.SessionToken = "session-" + secretMark
	p := f.plugin(t, WithCredentials(cr))
	if _, err := p.Resolve("aws", "ssm/plain"); err != nil {
		t.Fatal(err)
	}
	if f.lastHdr.Get(headerAmzToken) != cr.SessionToken {
		t.Errorf("session token not sent")
	}
	if !strings.Contains(f.lastHdr.Get(headerAuth), "x-amz-security-token") {
		t.Errorf("session token not signed: %s", f.lastHdr.Get(headerAuth))
	}
}

func TestWrongKeyIsDenied(t *testing.T) {
	f := newFakeAWS(t)
	bad := testCreds
	bad.SecretAccessKey = "wrong"
	p := f.plugin(t, WithCredentials(bad))
	_, err := p.Resolve("aws", "ssm/plain")
	if !errors.Is(err, plugin.ErrDenied) {
		t.Fatalf("bad signature: %v, want ErrDenied", err)
	}
}

func TestResolveErrors(t *testing.T) {
	f := newFakeAWS(t)
	f.failures["denied"] = apiFailure{400, "AccessDeniedException"}
	f.failures["expired"] = apiFailure{400, "ExpiredTokenException"}
	f.failures["kms"] = apiFailure{400, "DecryptionFailure"}
	f.failures["internal"] = apiFailure{500, "InternalServiceError"}
	f.failures["throttled"] = apiFailure{400, "ThrottlingException"}
	f.failures["gw"] = apiFailure{502, ""}
	f.failures["forbidden"] = apiFailure{403, ""}
	f.failures["invalid"] = apiFailure{400, "InvalidRequestException"}
	f.failures["/p/denied"] = apiFailure{400, "AccessDeniedException"}
	f.failures["/p/kms"] = apiFailure{400, "InvalidKeyId"}
	f.failures["/p/busy"] = apiFailure{400, "ThrottlingException"}
	p := f.plugin(t)
	cases := []struct {
		ref  string
		want error
	}{
		{"sm/denied", plugin.ErrDenied},
		{"sm/expired", plugin.ErrDenied},
		{"sm/kms", plugin.ErrDenied},
		{"sm/forbidden", plugin.ErrDenied},
		{"sm/internal", plugin.ErrUnavailable},
		{"sm/throttled", plugin.ErrUnavailable},
		{"sm/gw", plugin.ErrUnavailable},
		{"sm/missing", plugin.ErrNotFound},
		{"sm/prod/db#nope", plugin.ErrNotFound},
		{"sm/prod/api-key#x", ErrInvalidRef}, // not JSON: the parser's text must not leak
		{"ssm//p/denied", plugin.ErrDenied},
		{"ssm//p/kms", plugin.ErrDenied},
		{"ssm//p/busy", plugin.ErrUnavailable},
		{"ssm//missing", plugin.ErrNotFound},
		{"ssm//app/prod/db_url#k", ErrInvalidRef},
		{"s3/bucket", ErrInvalidRef},
		{"sm/x?ver=1", ErrInvalidRef},
		{"sm/invalid", nil},
	}
	named := []error{plugin.ErrDenied, plugin.ErrUnavailable, plugin.ErrNotFound, ErrInvalidRef}
	for _, c := range cases {
		_, err := p.Resolve("aws", c.ref)
		if err == nil {
			t.Errorf("Resolve(%q): no error", c.ref)
			continue
		}
		if strings.Contains(err.Error(), secretMark) {
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
	if _, err := p.Resolve("vault", "x"); !errors.Is(err, plugin.ErrUnsupportedScheme) {
		t.Errorf("foreign scheme: %v", err)
	}
}

func TestAPIErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		hdr    string
		body   string
		want   error
	}{
		{400, "", `{"__type":"ResourceNotFoundException","message":"m"}`, plugin.ErrNotFound},
		{400, "", `{"__type":"ParameterVersionNotFound","message":"m"}`, plugin.ErrNotFound},
		{400, "", `{"__type":"com.amazon.coral.service#UnrecognizedClientException","message":"m"}`, plugin.ErrDenied},
		{403, "", `{"__type":"MissingAuthenticationToken","Message":"m"}`, plugin.ErrDenied},
		{400, "", `{"__type":"ThrottlingException","message":"m"}`, plugin.ErrUnavailable},
		{500, "", `{"__type":"InternalFailure","message":"m"}`, plugin.ErrUnavailable},
		{400, "AccessDeniedException:http://internal.amazon.com/", `{"message":"m"}`, plugin.ErrDenied},
		{403, "", `<html>m</html>`, plugin.ErrDenied},
		{401, "", ``, plugin.ErrDenied},
		{429, "", ``, plugin.ErrUnavailable},
		{503, "", `not json`, plugin.ErrUnavailable},
		{400, "", `{"__type":"ValidationException","message":"m"}`, nil},
		{400, "", ``, nil},
	}
	for i, c := range cases {
		err := apiError("read x", c.status, c.hdr, []byte(c.body))
		if c.want == nil {
			if errors.Is(err, plugin.ErrDenied) || errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, plugin.ErrNotFound) {
				t.Errorf("case %d: %v must not wrap a named error", i, err)
			}
			continue
		}
		if !errors.Is(err, c.want) {
			t.Errorf("case %d: %v, want %v", i, err, c.want)
		}
	}
	err := apiError("read x", 500, "", []byte(`{"message":"`+strings.Repeat("x", 1000)+`"}`))
	if len(err.Error()) > maxDetail+100 {
		t.Errorf("message not truncated: %d bytes", len(err.Error()))
	}
	if err := apiError("read x", 500, "", []byte("raw "+secretMark)); strings.Contains(err.Error(), secretMark) {
		t.Errorf("raw body repeated: %v", err)
	}
}

func TestUnreachableAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	p, err := New(WithRegion(testRegion), WithCredentials(testCreds), WithEndpoint(addr))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Resolve("aws", "ssm/plain"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Errorf("closed server: %v", err)
	}

	f := newFakeAWS(t)
	f.delay = 200 * time.Millisecond
	q := f.plugin(t, WithTimeout(20*time.Millisecond))
	if _, err := q.Resolve("aws", "ssm/plain"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Errorf("slow server: %v", err)
	}
}

func TestRequestShape(t *testing.T) {
	f := newFakeAWS(t)
	p := f.plugin(t)
	p.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	if _, err := p.Resolve("aws", "sm/prod/db?stage=AWSPREVIOUS#password"); err != nil {
		t.Fatal(err)
	}
	if f.lastBody != `{"SecretId":"prod/db","VersionStage":"AWSPREVIOUS"}` {
		t.Errorf("GetSecretValue body = %s", f.lastBody)
	}
	if f.lastHdr.Get(headerAmzDate) != "20260102T030405Z" {
		t.Errorf("X-Amz-Date = %s", f.lastHdr.Get(headerAmzDate))
	}
	if _, err := p.Resolve("aws", "ssm//app/prod/db_url"); err != nil {
		t.Fatal(err)
	}
	if f.lastBody != `{"Name":"/app/prod/db_url","WithDecryption":true}` {
		t.Errorf("GetParameter body = %s", f.lastBody)
	}
}

func TestEndpoints(t *testing.T) {
	p, err := New(WithRegion("eu-west-1"), WithCredentials(testCreds))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.endpointFor(smSigningName); got != "https://secretsmanager.eu-west-1.amazonaws.com" {
		t.Errorf("sm endpoint = %s", got)
	}
	if got := p.endpointFor(ssmSigningName); got != "https://ssm.eu-west-1.amazonaws.com" {
		t.Errorf("ssm endpoint = %s", got)
	}
	p.region = "cn-north-1"
	if got := p.endpointFor(ssmSigningName); got != "https://ssm.cn-north-1.amazonaws.com.cn" {
		t.Errorf("china endpoint = %s", got)
	}
}

func TestCache(t *testing.T) {
	f := newFakeAWS(t)
	p := f.plugin(t)
	for _, ref := range []string{"sm/prod/db#password", "sm/prod/db#user", "sm/prod/db"} {
		if _, err := p.Resolve("aws", ref); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.hits.Load(); n != 1 {
		t.Errorf("one secret: %d requests, want 1", n)
	}
	if _, err := p.Resolve("aws", "sm/prod/db?stage=AWSPREVIOUS#password"); err != nil {
		t.Fatal(err)
	}
	if n := f.hits.Load(); n != 2 {
		t.Errorf("another stage is another value: %d requests", n)
	}
	p.Purge()
	if _, err := p.Resolve("aws", "sm/prod/db#password"); err != nil {
		t.Fatal(err)
	}
	if n := f.hits.Load(); n != 3 {
		t.Errorf("after Purge: %d requests, want 3", n)
	}

	f.mu.Lock()
	f.failures["flaky"] = apiFailure{503, ""}
	f.mu.Unlock()
	if _, err := p.Resolve("aws", "sm/flaky"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatal(err)
	}
	f.mu.Lock()
	delete(f.failures, "flaky")
	f.sm["flaky"] = "ok"
	f.mu.Unlock()
	if v, err := p.Resolve("aws", "sm/flaky"); err != nil || v != "ok" {
		t.Errorf("failure was cached: %q, %v", v, err)
	}
}

func TestConcurrent(t *testing.T) {
	f := newFakeAWS(t)
	p := f.plugin(t, WithLookupPrefix("sm/prod/app#"))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Resolve("aws", "ssm//app/prod/db_url"); err != nil {
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
	f := newFakeAWS(t)
	f.failures["locked"] = apiFailure{400, "AccessDeniedException"}
	p := f.plugin(t,
		WithLookup("DB_URL", "ssm//app/prod/db_url"),
		WithLookup("GONE", "sm/missing"),
		WithLookup("LOCKED", "sm/locked"),
		WithLookupPrefix("sm/prod/app#"),
		WithLookupPrefix("ssm//app/prod/"),
	)
	cases := []struct {
		name   string
		want   string
		ok     bool
		denied bool
	}{
		{"DB_URL", "url-" + secretMark, true, false},
		{"DB_PASSWORD", "db-" + secretMark, true, false},
		{"LOG_LEVEL", "debug", true, false}, // second prefix
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
	q := f.plugin(t)
	before := f.hits.Load()
	if _, ok, err := q.Lookup("DB_PASSWORD"); ok || err != nil || f.hits.Load() != before {
		t.Errorf("unmapped lookup = %v, %v, or sent a request", ok, err)
	}
}

func TestNew(t *testing.T) {
	cases := []struct {
		opts []Option
		want error
	}{
		{[]Option{WithCredentials(testCreds)}, ErrInvalidRef},
		{[]Option{WithRegion("eu west"), WithCredentials(testCreds)}, ErrInvalidRef},
		{[]Option{WithRegion(testRegion)}, ErrNoCredentials},
		{[]Option{WithRegion(testRegion), WithCredentials(testCreds), WithEndpoint("localhost:4566")}, ErrInvalidRef},
		{[]Option{WithRegion(testRegion), WithCredentials(testCreds), WithEndpoint((&url.URL{Scheme: "http", User: url.UserPassword("u", "p"), Host: "localhost:4566"}).String())}, ErrInvalidRef},
		{[]Option{WithRegion(testRegion), WithCredentials(testCreds), WithLookup("X", "s3/x")}, ErrInvalidRef},
	}
	for i, c := range cases {
		if _, err := New(c.opts...); !errors.Is(err, c.want) {
			t.Errorf("case %d: %v, want %v", i, err, c.want)
		}
	}
	p, err := New(WithRegion(testRegion), WithCredentials(testCreds), WithName("aws-eu"), WithScheme("AWS-EU"), WithTimeout(0), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "aws-eu" || p.Schemes()[0] != "aws-eu" || p.timeout != DefaultTimeout {
		t.Errorf("options not applied")
	}

	// A credentials source is asked per request; its errors and empty
	// answers fail the read without a request being sent.
	f := newFakeAWS(t)
	boom := errors.New("sso expired")
	q := f.plugin(t, WithCredentialsSource(func() (Credentials, error) { return Credentials{}, boom }))
	if _, err := q.Resolve("aws", "ssm/plain"); !errors.Is(err, boom) {
		t.Errorf("source error: %v", err)
	}
	q = f.plugin(t, WithCredentialsSource(func() (Credentials, error) { return Credentials{}, nil }))
	if _, err := q.Resolve("aws", "ssm/plain"); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("empty source: %v", err)
	}
	if f.hits.Load() != 0 {
		t.Errorf("request sent without credentials")
	}
}

func TestCredentialsNeverPrinted(t *testing.T) {
	c := Credentials{AccessKeyID: "TESTKEYID", SecretAccessKey: "s-" + secretMark, SessionToken: "t-" + secretMark}
	for _, s := range []string{c.String(), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprint(&c)} {
		if strings.Contains(s, secretMark) {
			t.Errorf("credentials printed: %s", s)
		}
		if !strings.Contains(s, "TESTKEYID") {
			t.Errorf("access key id missing: %s", s)
		}
	}
}

// The environment and the shared credentials file are read only through
// FromEnv and FromSharedCredentials.
func TestFromEnv(t *testing.T) {
	f := newFakeAWS(t)
	t.Setenv(EnvAccessKeyID, testCreds.AccessKeyID)
	t.Setenv(EnvSecretAccessKey, testCreds.SecretAccessKey)
	t.Setenv(EnvSessionToken, "")
	t.Setenv(EnvRegion, testRegion)
	t.Setenv(EnvDefaultRegion, "ignored-1")

	if _, err := New(WithEndpoint(f.srv.URL)); err == nil {
		t.Fatal("New without FromEnv must not read AWS_REGION or keys")
	}
	p, err := New(FromEnv(), WithEndpoint(f.srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if p.region != testRegion {
		t.Errorf("region = %s", p.region)
	}
	if _, err := p.Resolve("aws", "ssm/plain"); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvRegion, "")
	p, err = New(FromEnv(), WithCredentials(testCreds))
	if err != nil {
		t.Fatal(err)
	}
	if p.region != "ignored-1" {
		t.Errorf("AWS_DEFAULT_REGION fallback: %s", p.region)
	}
	p, err = New(FromEnv(), WithRegion("explicit-1"))
	if err != nil || p.region != "explicit-1" {
		t.Errorf("later option lost: %v", err)
	}
}

func TestFromSharedCredentials(t *testing.T) {
	f := newFakeAWS(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "credentials")
	ini := "# comment\n[default]\naws_access_key_id = OTHERID\naws_secret_access_key = other\n\n" +
		"[ci]\n; comment\naws_access_key_id=" + testCreds.AccessKeyID + "\naws_secret_access_key = " + testCreds.SecretAccessKey + "\n" +
		"[broken]\naws_access_key_id = X\n"
	if err := os.WriteFile(file, []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := New(WithRegion(testRegion), FromSharedCredentials(file, "ci"), WithEndpoint(f.srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Resolve("aws", "ssm/plain"); err != nil {
		t.Fatal(err)
	}
	c, err := readSharedCredentials(file, "")
	if err != nil || c.AccessKeyID != "OTHERID" {
		t.Errorf("default profile: %v %v", c, err)
	}
	for _, prof := range []string{"nope", "broken"} {
		_, err := New(WithRegion(testRegion), FromSharedCredentials(file, prof))
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("profile %s: %v", prof, err)
		}
		if err != nil && strings.Contains(err.Error(), secretMark) {
			t.Errorf("profile %s: error leaks: %v", prof, err)
		}
	}
	if _, err := New(WithRegion(testRegion), FromSharedCredentials(filepath.Join(dir, "missing"), "")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".aws"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".aws", "credentials"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := readSharedCredentials("", "ci"); err != nil || c.AccessKeyID != testCreds.AccessKeyID {
		t.Errorf("default file: %v %v", c, err)
	}
}
