package vault_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugins/vault"
)

// secretMark tags every fake secret value so a failure message shows where a
// value came from; none of these are real credentials.
const secretMark = "SECRETMARK"

// fakeVault answers KV reads from a fixed map and counts requests, so the
// test can prove one plugin instance shares its cache across both formats.
func fakeVault(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	bodies := map[string]string{
		"/v1/secret/data/app": `{"data":{"data":{"password":"pw-` + secretMark + `","user":"app"},"metadata":{"version":3}}}`,
		"/v1/kv/app":          `{"data":{"DB_PASSWORD":"db-` + secretMark + `"}}`,
	}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, ok := bodies[r.URL.Path]
		if !ok || r.Method != http.MethodGet {
			http.Error(w, `{"errors":[]}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// One plugin instance serves a dotenv file and a jsonc file: the plugin is
// written against the base interfaces only.
func TestCrossFormat(t *testing.T) {
	srv, hits := fakeVault(t)
	p, err := vault.New(vault.WithAddress(srv.URL), vault.WithToken("test-token"), vault.WithLookupPrefix("kv/app#"))
	if err != nil {
		t.Fatal(err)
	}

	envSrc := "PASS=${vault:secret/data/app#password}\nKEY=${DB_PASSWORD}\nPLAIN=x\n"
	env := dotenv.Parse(envSrc, dotenv.WithPlugin(p))
	m, err := env.ExpandedMap()
	if err != nil {
		t.Fatal(err)
	}
	if m["PASS"] != "pw-"+secretMark || m["KEY"] != "db-"+secretMark || m["PLAIN"] != "x" {
		t.Errorf("dotenv: %v", m)
	}
	if got := env.Render(); got != envSrc {
		t.Errorf("dotenv render changed: %q", got)
	}

	src := []byte(`{
  // database
  "db": {"user": "${vault:secret/data/app#user}", "password": "${DB_PASSWORD}"},
}
`)
	doc, err := jsonc.Parse(src, jsonc.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"db.user": "app", "db.password": "db-" + secretMark} {
		v, err := doc.GetExpanded(path)
		if err != nil || v != want {
			t.Errorf("jsonc %s = %v, %v; want %q", path, v, err, want)
		}
	}
	if string(doc.Render()) != string(src) {
		t.Errorf("jsonc render changed")
	}
	// One request for secret/data/app and one for kv/app, across both files.
	if n := hits.Load(); n != 2 {
		t.Errorf("shared cache: %d requests, want 2", n)
	}
}
