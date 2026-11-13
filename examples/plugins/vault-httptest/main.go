// Command vault-httptest installs the vault value plugin in a dotenv file and
// a JSONC document against an httptest server that answers like Vault's KV v1
// and KV v2 engines, so it runs without a Vault server or a real token.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/plugins/vault"
)

// demoToken is the only token the fake server accepts.
const demoToken = "demo-token"

// Vault API paths the fake serves (below the address, including /v1/).
const (
	pathKV2       = "/v1/secret/data/app"
	pathKV1       = "/v1/kv/legacy"
	pathForbidden = "/v1/secret/data/locked"
)

// queryVersion is the KV v2 read parameter that pins a version; pinnedVersion
// is the one version the fake keeps besides the latest.
const (
	queryVersion  = "version"
	pinnedVersion = "1"
)

// Response bodies, shaped exactly like Vault's. KV v2 nests the fields under
// data.data next to metadata; KV v1 puts them directly under data.
const (
	bodyKV2Latest = `{"data":{"data":{"password":"pw-v2","DB_USER":"app"},"metadata":{"version":2}}}`
	bodyKV2V1     = `{"data":{"data":{"password":"pw-v1","DB_USER":"app"},"metadata":{"version":1}}}`
	bodyKV1       = `{"data":{"token":"kv1-token"}}`
	bodyDenied    = `{"errors":["1 error occurred:\n\t* permission denied\n\n"]}`
	bodyNotFound  = `{"errors":[]}`
)

// envSource uses every reference shape: a field, an older version, a
// one-field secret without #field, and two plain names the plugin looks up.
const envSource = `DB_PASSWORD=${vault:secret/data/app#password}
OLD_PASSWORD=${vault:secret/data/app?version=1#password}
LEGACY_TOKEN=${vault:kv/legacy}
API_TOKEN=${API_TOKEN}
USER=${DB_USER}
`

// jsoncSource reads the same secret from a JSONC document.
const jsoncSource = `{"db": {"password": "${vault:secret/data/app#password}"}}`

// newFakeVault answers GET requests like Vault and counts them, so the
// plugin's cache can be observed. A wrong token is refused with 403.
//
//ds:def id=vault-newfakevault-wnk7tqk8
func newFakeVault(requests *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Vault-Token") != demoToken || r.URL.Path == pathForbidden {
			reply(w, http.StatusForbidden, bodyDenied)
			return
		}
		switch {
		case r.URL.Path == pathKV2 && r.URL.Query().Get(queryVersion) == pinnedVersion:
			reply(w, http.StatusOK, bodyKV2V1)
		case r.URL.Path == pathKV2:
			reply(w, http.StatusOK, bodyKV2Latest)
		case r.URL.Path == pathKV1:
			reply(w, http.StatusOK, bodyKV1)
		default:
			reply(w, http.StatusNotFound, bodyNotFound)
		}
	}))
}

// reply writes one canned body. A failed write means the client has gone; a
// handler has nobody left to report that to.
func reply(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// newPlugin points one plugin at the server. WithLookup maps one name to a
// reference; WithLookupPrefix answers any other name from fields of one secret.
//
//ds:def id=vault-newplugin-sx7bn45q
func newPlugin(addr string) (*vault.Plugin, error) {
	return vault.New(
		vault.WithAddress(addr),
		vault.WithToken(demoToken),
		vault.WithLookup("API_TOKEN", "kv/legacy#token"),
		vault.WithLookupPrefix("secret/data/app#"),
	)
}

// connectFromEnv is the production shape: address, token and namespace come
// from VAULT_ADDR, VAULT_TOKEN and VAULT_NAMESPACE, read once here and only
// because FromEnv was passed. Options after FromEnv override it.
//
//ds:def id=vault-connectfromenv-9bnckh59
func connectFromEnv() (*vault.Plugin, error) {
	return vault.New(vault.FromEnv(), vault.WithLookupPrefix("secret/data/app#"))
}

// readDotenv expands every key of the dotenv file through the plugin.
//
//ds:def id=vault-readdotenv-yt53dn7r
func readDotenv(w io.Writer, p *vault.Plugin) error {
	f := dotenv.Parse(envSource, dotenv.WithPlugin(p))
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-13s %s\n", key+":", v)
	}
	return nil
}

// readJSONC reads the same secret from JSONC with the same plugin instance.
func readJSONC(w io.Writer, p *vault.Plugin) error {
	doc, err := jsonc.Parse([]byte(jsoncSource), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-13s %v\n", "jsonc:", v)
	return nil
}

// showErrors branches on the named errors: a refusal and a reference that
// leaves out #field on a secret with more than one field.
//
//ds:def id=vault-showerrors-znzb8ew9
func showErrors(w io.Writer, p *vault.Plugin) {
	_, err := p.Resolve("vault", "secret/data/locked#password")
	fmt.Fprintf(w, "%-13s denied=%t\n", "locked:", errors.Is(err, plugin.ErrDenied))
	_, err = p.Resolve("vault", "secret/data/app")
	fmt.Fprintf(w, "%-13s invalid=%t\n", "no #field:", errors.Is(err, vault.ErrInvalidRef))
}

// run starts the fake server, reads both documents and reports how many
// requests reached it.
func run(w io.Writer) error {
	var requests atomic.Int32
	srv := newFakeVault(&requests)
	defer srv.Close()
	p, err := newPlugin(srv.URL)
	if err != nil {
		return err
	}
	if err := readDotenv(w, p); err != nil {
		return err
	}
	if err := readJSONC(w, p); err != nil {
		return err
	}
	showErrors(w, p)
	fmt.Fprintf(w, "%-13s %d\n", "requests:", requests.Load())
	return nil
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
