// Command secrets resolves ${scheme:ref} references in single values with
// value.Expand and the textdata value plugins: the password-store plugin
// (gopass) with a fake runner, the file plugin over a temp directory, and the
// vault and aws plugins against local httptest servers. Every store is a fake
// holding made-up values; nothing reaches a real secret store.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/plugins/aws"
	fileplugin "github.com/ubgo/textdata/plugins/file"
	gopassplugin "github.com/ubgo/textdata/plugins/gopass"
	"github.com/ubgo/textdata/plugins/vault"
	"github.com/ubgo/textdata/value"
)

// Fake settings. None of these is a real credential.
const (
	fakeVaultToken = "fake-vault-token"
	fakeRegion     = "eu-west-1"
	fakeKeyID      = "fake-key-id"
	fakeSecretKey  = "fake-secret-key"
)

// The fake stores' contents, keyed the way each backend addresses them.
var (
	gopassEntries = map[string]string{"work/api": "fake-gopass-token"}
	vaultBody     = `{"data":{"data":{"password":"fake-vault-password"},"metadata":{"version":1}}}`
	ssmParameters = map[string]string{"/app/prod/db_url": "postgres://db.internal/app"}
)

// Wire-level names the fakes answer to.
const (
	vaultKVPath        = "/v1/secret/data/app"
	targetGetParameter = "AmazonSSM.GetParameter"
	gopassNotFound     = 11 // gopass's exit status for a missing entry
	keyFileName        = "api.key"
	keyFileContent     = "fake-file-key\n"
)

func main() {
	dir, err := os.MkdirTemp("", "value-secrets")
	if err == nil {
		err = run(os.Stdout, dir)
		// Best-effort cleanup: the run's result matters more.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run builds the four plugins against their fakes and expands values through
// them, then shows how a plugin failure reads.
func run(w io.Writer, dir string) error {
	vaultSrv := httptest.NewServer(http.HandlerFunc(fakeVault))
	defer vaultSrv.Close()
	awsSrv := httptest.NewServer(http.HandlerFunc(fakeSSM))
	defer awsSrv.Close()
	if err := os.WriteFile(filepath.Join(dir, keyFileName), []byte(keyFileContent), 0o600); err != nil {
		return err
	}

	plugins, err := newPlugins(dir, vaultSrv.URL, awsSrv.URL)
	if err != nil {
		return err
	}
	if err := expandSecrets(w, plugins); err != nil {
		return err
	}
	return pluginErrors(w, plugins)
}

// newPlugins builds one instance of each value plugin. In production only
// the fakes change: drop gopass's WithRunner to run the real binary, point
// vault at VAULT_ADDR (vault.FromEnv()), and omit aws.WithEndpoint.
//
//ds:def id=value-newplugins-u7h3x9ma
func newPlugins(dir, vaultAddr, awsEndpoint string) ([]plugin.Plugin, error) {
	gp := gopassplugin.New(gopassplugin.WithRunner(fakeGopass))
	fp := fileplugin.New(dir) // confined to dir by default
	vp, err := vault.New(vault.WithAddress(vaultAddr), vault.WithToken(fakeVaultToken))
	if err != nil {
		return nil, err
	}
	ap, err := aws.New(
		aws.WithRegion(fakeRegion),
		aws.WithCredentialsSource(func() (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: fakeKeyID, SecretAccessKey: fakeSecretKey}, nil
		}),
		aws.WithEndpoint(awsEndpoint),
	)
	if err != nil {
		return nil, err
	}
	return []plugin.Plugin{gp, fp, vp, ap}, nil
}

// expandSecrets resolves one reference per store. Each value is expanded on
// its own, the way a CLI reads one secret-bearing field from its own config.
//
//ds:def id=value-expandsecrets-s2p6d4kc
func expandSecrets(w io.Writer, plugins []plugin.Plugin) error {
	for _, in := range []string{
		"${gopass:work/api}",
		"${file:api.key}",
		"${vault:secret/data/app#password}",
		"${aws:ssm//app/prod/db_url}",
		"Bearer ${gopass:work/api}",
		"[${nobody:thing}]", // no plugin owns the scheme: an ordinary, unset name
	} {
		out, err := value.Expand(context.Background(), in, value.WithPlugins(plugins...))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-34s => %s\n", in, out)
	}
	return nil
}

// pluginErrors: a missing entry is a *plugin.Error naming the plugin and
// unwrapping to plugin.ErrNotFound, never an empty value.
//
//ds:def id=value-pluginerrors-g8r5n1vt
func pluginErrors(w io.Writer, plugins []plugin.Plugin) error {
	_, err := value.Expand(context.Background(), "${gopass:work/missing}", value.WithPlugins(plugins...))
	var pe *plugin.Error
	if !errors.As(err, &pe) {
		return fmt.Errorf("want *plugin.Error, got %v", err)
	}
	fmt.Fprintf(w, "missing: plugin=%s not found=%t\n", pe.Plugin, errors.Is(err, plugin.ErrNotFound))
	return nil
}

// gopassExitError carries an exit status the way *exec.ExitError does, which
// is all the gopass plugin inspects.
type gopassExitError struct{ code int }

func (e gopassExitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e gopassExitError) ExitCode() int { return e.code }

// fakeGopass answers `gopass show -o <entry>` from gopassEntries.
func fakeGopass(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	if len(args) != 3 || args[0] != "show" || args[1] != "-o" {
		return nil, []byte("unexpected arguments"), exec.ErrNotFound
	}
	v, ok := gopassEntries[args[2]]
	if !ok {
		return nil, []byte("entry is not in the password store"), gopassExitError{code: gopassNotFound}
	}
	return []byte(v + "\n"), nil, nil
}

// fakeVault answers one KV v2 secret for the fake token.
func fakeVault(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Vault-Token") != fakeVaultToken {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.URL.Path != vaultKVPath {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[]}`)
		return
	}
	_, _ = io.WriteString(w, vaultBody)
}

// fakeSSM answers SSM GetParameter in the AWS JSON protocol. It does not
// check the signature; the aws plugin's own tests do.
func fakeSSM(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"Name"`
	}
	if r.Header.Get("X-Amz-Target") != targetGetParameter || json.NewDecoder(r.Body).Decode(&req) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	v, ok := ssmParameters[req.Name]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"__type":"ParameterNotFound"}`)
		return
	}
	// Marshalling a map of strings cannot fail.
	body, _ := json.Marshal(map[string]map[string]string{"Parameter": {"Name": req.Name, "Value": v}})
	_, _ = w.Write(body)
}
