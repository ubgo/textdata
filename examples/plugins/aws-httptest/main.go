// Command aws-httptest installs the aws value plugin in a dotenv file and a
// JSONC document against an httptest server that answers like AWS Secrets
// Manager and SSM Parameter Store, so it runs without an AWS account. The
// server also checks that every request carries a Signature Version 4
// Authorization header for the right region and service.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/plugins/aws"
)

// Demo settings. The keys are fake; the server only checks their shape.
const (
	demoRegion = "eu-west-1"
	demoKeyID  = "AKIDEXAMPLE"
	demoSecret = "example-secret-key"
	demoToken  = "example-session-token"
)

// The X-Amz-Target values of the two operations the plugin calls.
const (
	targetGetSecret    = "secretsmanager.GetSecretValue"
	targetGetParameter = "AmazonSSM.GetParameter"
	stagePrevious      = "AWSPREVIOUS"
)

// sigV4Prefix starts every SigV4 Authorization header.
const sigV4Prefix = "AWS4-HMAC-SHA256 Credential=" + demoKeyID + "/"

// secrets and parameters are the fake stores. prod/db holds a JSON object,
// so a reference can pick one key with #key.
var (
	secrets = map[string]string{
		"prod/db":                  `{"username":"app","password":"pw-current","port":5432}`,
		"prod/db@" + stagePrevious: `{"username":"app","password":"pw-previous","port":5432}`,
		"prod/api-key":             "plain-api-key",
	}
	parameters = map[string]string{
		"/app/prod/db_url":       "postgres://db.internal/app",
		"/app/prod/FEATURE_FLAG": "on",
	}
)

// envSource uses both services, a JSON key, a version stage, a whole value
// and a plain name answered by WithLookupPrefix.
const envSource = `DB_PASSWORD=${aws:sm/prod/db#password}
DB_PORT=${aws:sm/prod/db#port}
PREVIOUS=${aws:sm/prod/db?stage=AWSPREVIOUS#password}
API_KEY=${aws:sm/prod/api-key}
DB_URL=${aws:ssm//app/prod/db_url}
FEATURE=${FEATURE_FLAG}
`

// jsoncSource reads a parameter from a JSONC document.
const jsoncSource = `{"db": {"url": "${aws:ssm//app/prod/db_url}"}}`

// fakeAWS records which signing scopes it saw (service names, from the
// Authorization header) and answers both operations.
type fakeAWS struct {
	mu     sync.Mutex
	scopes map[string]int
}

// request is the union of both operations' request bodies.
type request struct {
	SecretID       string `json:"SecretId"`
	VersionStage   string `json:"VersionStage"`
	Name           string `json:"Name"`
	WithDecryption bool   `json:"WithDecryption"`
}

// ServeHTTP checks the signature shape, then answers like the AWS JSON
// protocol: 200 with the value, or 400 with an error "__type".
func (f *fakeAWS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, sigV4Prefix) || r.Header.Get("X-Amz-Security-Token") != demoToken {
		writeError(w, http.StatusForbidden, "UnrecognizedClientException")
		return
	}
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "SerializationException")
		return
	}
	switch r.Header.Get("X-Amz-Target") {
	case targetGetSecret:
		if !f.signedFor(w, auth, "secretsmanager") {
			return
		}
		key := req.SecretID
		if req.VersionStage == stagePrevious {
			key += "@" + stagePrevious
		}
		v, ok := secrets[key]
		if !ok {
			writeError(w, http.StatusBadRequest, "ResourceNotFoundException")
			return
		}
		writeJSON(w, http.StatusOK, secretResponse{SecretString: v})
	case targetGetParameter:
		if !f.signedFor(w, auth, "ssm") {
			return
		}
		v, ok := parameters[req.Name]
		if !ok || !req.WithDecryption {
			writeError(w, http.StatusBadRequest, "ParameterNotFound")
			return
		}
		writeJSON(w, http.StatusOK, parameterResponse{Parameter: parameter{Name: req.Name, Value: v}})
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException")
	}
}

// signedFor checks the SigV4 credential scope names this region and service
// (".../eu-west-1/ssm/aws4_request") and counts the request; a request signed
// for anything else is refused the way AWS refuses it.
//
//ds:def id=aws-signedfor-jpc597t6
func (f *fakeAWS) signedFor(w http.ResponseWriter, auth, service string) bool {
	if !strings.Contains(auth, "/"+demoRegion+"/"+service+"/aws4_request") {
		writeError(w, http.StatusForbidden, "InvalidSignatureException")
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes[service]++
	return true
}

// writeError answers in the AWS JSON protocol's error shape.
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, errorResponse{Type: code, Message: code})
}

// secretResponse is the GetSecretValue reply body.
type secretResponse struct {
	SecretString string `json:"SecretString"`
}

// parameter and parameterResponse are the GetParameter reply body.
type parameter struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type parameterResponse struct {
	Parameter parameter `json:"Parameter"`
}

// errorResponse is the AWS JSON protocol error body; the plugin maps
// "__type" to its typed errors.
type errorResponse struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

// writeJSON encodes before writing the status, so an encoding failure becomes
// a 500 instead of a half-written 200. A failed write means the client has
// gone; a handler has nobody left to report that to.
func writeJSON[T secretResponse | parameterResponse | errorResponse](w http.ResponseWriter, status int, body T) {
	b, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// newPlugin points one plugin at the fake endpoint. The credentials come from
// a source asked before every request, the seam for credentials the plugin
// does not read itself (web identity, SSO, instance metadata).
//
//ds:def id=aws-newplugin-ax28y3p7
func newPlugin(endpoint string, asked *int) (*aws.Plugin, error) {
	source := func() (aws.Credentials, error) {
		*asked++
		return aws.Credentials{AccessKeyID: demoKeyID, SecretAccessKey: demoSecret, SessionToken: demoToken}, nil
	}
	return aws.New(
		aws.WithRegion(demoRegion),
		aws.WithCredentialsSource(source),
		aws.WithEndpoint(endpoint), // omit to call the regional AWS endpoints
		aws.WithLookupPrefix("ssm//app/prod/"),
	)
}

// readDotenv expands every key of the dotenv file through the plugin.
//
//ds:def id=aws-readdotenv-c8y6j8u7
func readDotenv(w io.Writer, p *aws.Plugin) error {
	f := dotenv.Parse(envSource, dotenv.WithPlugin(p))
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-12s %s\n", key+":", v)
	}
	return nil
}

// readJSONC reads a parameter through the same instance; it is cached.
func readJSONC(w io.Writer, p *aws.Plugin) error {
	doc, err := jsonc.Parse([]byte(jsoncSource), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("db.url")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-12s %v\n", "jsonc:", v)
	return nil
}

// credentialSources shows the other ways to supply credentials: a shared
// credentials file (one profile, read once in New) and the error New returns
// when nothing was configured.
//
//ds:def id=aws-credentialsources-dkp46znp
func credentialSources(w io.Writer, dir string) error {
	file := filepath.Join(dir, "credentials")
	ini := "[ci]\naws_access_key_id = " + demoKeyID + "\naws_secret_access_key = " + demoSecret + "\n"
	if err := os.WriteFile(file, []byte(ini), 0o600); err != nil {
		return err
	}
	_, err := aws.New(aws.WithRegion(demoRegion), aws.FromSharedCredentials(file, "ci"))
	fmt.Fprintf(w, "%-12s ok=%t\n", "shared file:", err == nil)
	_, err = aws.New(aws.WithRegion(demoRegion))
	fmt.Fprintf(w, "%-12s %t\n", "no creds:", errors.Is(err, aws.ErrNoCredentials))
	return nil
}

// run starts the fake server, reads both documents, and reports what the
// server saw and how often the credentials source was asked.
func run(w io.Writer, dir string) error {
	fake := &fakeAWS{scopes: map[string]int{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	asked := 0
	p, err := newPlugin(srv.URL, &asked)
	if err != nil {
		return err
	}
	if err := readDotenv(w, p); err != nil {
		return err
	}
	if err := readJSONC(w, p); err != nil {
		return err
	}
	_, err = p.Resolve("aws", "sm/prod/missing")
	fmt.Fprintf(w, "%-12s not found=%t\n", "missing:", errors.Is(err, plugin.ErrNotFound))

	services := make([]string, 0, len(fake.scopes))
	for s, n := range fake.scopes {
		services = append(services, fmt.Sprintf("%s=%d", s, n))
	}
	sort.Strings(services)
	fmt.Fprintf(w, "%-12s %s (credentials asked %d times)\n", "requests:", strings.Join(services, " "), asked)

	return credentialSources(w, dir)
}

func main() {
	dir, err := os.MkdirTemp("", "aws-httptest")
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
