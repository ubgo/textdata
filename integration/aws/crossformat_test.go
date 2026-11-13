package aws_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugins/aws"
)

// secretMark tags every fake secret value; none are real credentials.
const secretMark = "SECRETMARK"

// fakeAWS answers Secrets Manager GetSecretValue and SSM GetParameter from
// fixed maps and counts requests. Signing is proven in the plugin's own
// tests against the AWS SigV4 suite; here only the shape of the call matters.
func fakeAWS(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	sm := map[string]string{
		"prod/db":  `{"user":"app","password":"pw-` + secretMark + `"}`,
		"prod/app": `{"DB_PASSWORD":"db-` + secretMark + `","API_KEY":"k-` + secretMark + `"}`,
	}
	ssm := map[string]string{"/app/prod/db_url": "url-" + secretMark}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		var in struct{ SecretId, Name string }
		if err := json.Unmarshal(body, &in); err != nil {
			http.Error(w, `{"message":"bad body"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		var out any
		switch r.Header.Get("X-Amz-Target") {
		case "secretsmanager.GetSecretValue":
			if v, ok := sm[in.SecretId]; ok {
				out = map[string]string{"Name": in.SecretId, "SecretString": v}
			}
		case "AmazonSSM.GetParameter":
			if v, ok := ssm[in.Name]; ok {
				out = map[string]any{"Parameter": map[string]string{"Name": in.Name, "Value": v}}
			}
		}
		if out == nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"ResourceNotFoundException","message":"not found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// One plugin instance serves a dotenv file and a jsonc file: the plugin is
// written against the base interfaces only.
func TestCrossFormat(t *testing.T) {
	srv, hits := fakeAWS(t)
	p, err := aws.New(
		aws.WithRegion("eu-test-1"),
		aws.WithCredentials(aws.Credentials{AccessKeyID: "TESTID", SecretAccessKey: "test-secret"}),
		aws.WithEndpoint(srv.URL),
		aws.WithLookupPrefix("sm/prod/app#"),
	)
	if err != nil {
		t.Fatal(err)
	}

	envSrc := "PASS=${aws:sm/prod/db#password}\nURL=${aws:ssm//app/prod/db_url}\nKEY=${API_KEY}\n"
	env := dotenv.Parse(envSrc, dotenv.WithPlugin(p))
	m, err := env.ExpandedMap()
	if err != nil {
		t.Fatal(err)
	}
	if m["PASS"] != "pw-"+secretMark || m["URL"] != "url-"+secretMark || m["KEY"] != "k-"+secretMark {
		t.Errorf("dotenv: %v", m)
	}
	if env.Render() != envSrc {
		t.Errorf("dotenv render changed")
	}

	src := []byte(`{
  // database
  "db": {"user": "${aws:sm/prod/db#user}", "password": "${DB_PASSWORD}", "url": "${aws:ssm//app/prod/db_url}"},
}
`)
	doc, err := jsonc.Parse(src, jsonc.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"db.user": "app", "db.password": "db-" + secretMark, "db.url": "url-" + secretMark} {
		v, err := doc.GetExpanded(path)
		if err != nil || v != want {
			t.Errorf("jsonc %s = %v, %v; want %q", path, v, err, want)
		}
	}
	if string(doc.Render()) != string(src) {
		t.Errorf("jsonc render changed")
	}
	// prod/db, /app/prod/db_url and prod/app: three requests across both files.
	if n := hits.Load(); n != 3 {
		t.Errorf("shared cache: %d requests, want 3", n)
	}
}
