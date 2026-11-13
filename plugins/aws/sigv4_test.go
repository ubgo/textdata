package aws

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// suiteContext is a vector's context.json.
type suiteContext struct {
	Credentials struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		Token           string `json:"token"`
	} `json:"credentials"`
	Normalize bool   `json:"normalize"`
	Region    string `json:"region"`
	Service   string `json:"service"`
	SignBody  bool   `json:"sign_body"`
	OmitToken bool   `json:"omit_session_token"`
	Timestamp string `json:"timestamp"`
}

// TestSigV4Suite signs every request of AWS's SigV4 test suite and compares
// each stage byte for byte.
func TestSigV4Suite(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "sigv4", "*", "request.txt"))
	if err != nil || len(dirs) < 30 {
		t.Fatalf("test suite missing: %d vectors, %v", len(dirs), err)
	}
	for _, reqFile := range dirs {
		dir := filepath.Dir(reqFile)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			var ctx suiteContext
			if err := json.Unmarshal(read(t, dir, "context.json"), &ctx); err != nil {
				t.Fatal(err)
			}
			ts, err := time.Parse(time.RFC3339, ctx.Timestamp)
			if err != nil {
				t.Fatal(err)
			}
			req, body := parseSuiteRequest(t, string(read(t, dir, "request.txt")))
			cr := Credentials{AccessKeyID: ctx.Credentials.AccessKeyID, SecretAccessKey: ctx.Credentials.SecretAccessKey, SessionToken: ctx.Credentials.Token}
			res := signV4(req, body, cr, ctx.Region, ctx.Service, ts, signOptions{normalize: ctx.Normalize, signBody: ctx.SignBody, omitSessionToken: ctx.OmitToken})

			check := func(stage, got, file string) {
				t.Helper()
				if want := string(read(t, dir, file)); got != want {
					t.Errorf("%s:\n got: %q\nwant: %q", stage, got, want)
				}
			}
			check("canonical request", res.canonicalRequest, "header-canonical-request.txt")
			check("string to sign", res.stringToSign, "header-string-to-sign.txt")
			check("signature", res.signature, "header-signature.txt")
			check("authorization", "Authorization:"+req.Header.Get(headerAuth), "")
		})
	}
}

// read returns a vector file; for file "" it returns the Authorization line
// of header-signed-request.txt.
func read(t *testing.T, dir, file string) []byte {
	t.Helper()
	if file == "" {
		for _, line := range strings.Split(string(read(t, dir, "header-signed-request.txt")), "\n") {
			if strings.HasPrefix(line, "Authorization:") {
				return []byte(line)
			}
		}
		t.Fatal("no Authorization line")
	}
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// parseSuiteRequest parses the suite's raw request text: a request line
// whose path may hold spaces, headers with folded continuation lines and
// repeated names, a blank line, a body.
func parseSuiteRequest(t *testing.T, raw string) (*http.Request, []byte) {
	t.Helper()
	head, body, _ := strings.Cut(raw, "\n\n")
	sc := bufio.NewScanner(strings.NewReader(head))
	sc.Scan()
	line := sc.Text()
	method, rest, _ := strings.Cut(line, " ")
	target := strings.TrimSuffix(rest, " HTTP/1.1")
	p, q, _ := strings.Cut(target, "?")
	req := &http.Request{Method: method, URL: &url.URL{Path: p, RawQuery: q}, Header: http.Header{}}
	var last string
	for sc.Scan() {
		l := sc.Text()
		if l == "" {
			break
		}
		if l[0] == ' ' || l[0] == '\t' {
			vs := req.Header[last]
			vs[len(vs)-1] += " " + strings.TrimSpace(l)
			continue
		}
		name, val, _ := strings.Cut(l, ":")
		if strings.EqualFold(name, "host") {
			req.Host = val
			continue
		}
		last = http.CanonicalHeaderKey(name)
		req.Header.Add(last, val)
	}
	return req, []byte(body)
}
