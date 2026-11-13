package dotenv

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/textdata/plugin"
)

// gopassStub is a Resolver for the "gopass" scheme backed by a map.
type gopassStub struct {
	secrets map[string]string
	down    bool
}

func (g *gopassStub) Name() string      { return "gopass" }
func (g *gopassStub) Schemes() []string { return []string{"gopass"} }
func (g *gopassStub) Resolve(scheme, ref string) (string, error) {
	if g.down {
		return "", plugin.ErrUnavailable
	}
	v, ok := g.secrets[ref]
	if !ok {
		return "", plugin.ErrNotFound
	}
	return v, nil
}

func TestResolverSchemes(t *testing.T) {
	t.Parallel()
	src := "DB_PASS=${gopass:personal/db}\nURL=app/${DB_PASS}@db\nOTHER=${vault:x}\nLIT='${gopass:personal/db}'\n"
	gp := &gopassStub{secrets: map[string]string{"personal/db": "pw"}}
	f := Parse(src, WithPlugin(gp))

	for key, want := range map[string]string{
		"DB_PASS": "pw",
		"URL":     "app/pw@db",
		"OTHER":   "", // no resolver owns vault: an ordinary unset name, as before
		"LIT":     "${gopass:personal/db}",
	} {
		got, _, err := f.GetExpanded(key)
		if err != nil || got != want {
			t.Errorf("GetExpanded(%s) = %q, %v; want %q", key, got, err, want)
		}
	}
	if f.Render() != src {
		t.Error("resolving changed the rendered file")
	}
	if got := f.Plugins()[0].String(); got != "gopass: Resolver" {
		t.Errorf("Plugins = %q", got)
	}

	// A resolver that is down is an error naming it, never a silent empty value.
	gp.down = true
	_, _, err := f.GetExpanded("URL")
	if !errors.Is(err, plugin.ErrUnavailable) || !strings.Contains(err.Error(), `plugin "gopass": resolve gopass:personal/db`) {
		t.Errorf("down resolver = %v", err)
	}

	// Without the plugin the same file reads as it always did.
	plain := Parse(src)
	if got, _, _ := plain.GetExpanded("DB_PASS"); got != "" {
		t.Errorf("no resolver: DB_PASS = %q", got)
	}
}

// docGuardPlugin is a cross-format guard: it sees only plugin.Document.
type docGuardPlugin struct {
	seen *[]string
}

func (docGuardPlugin) Name() string { return "no-secrets" }
func (g docGuardPlugin) GuardSave(doc plugin.Document) error {
	*g.seen = append(*g.seen, doc.FormatName()+"|"+doc.Location()+"|"+strings.Join(doc.ValuePaths(), ","))
	if v, _ := doc.ValueAt("TOKEN"); v != "" {
		return errors.New("TOKEN must stay empty in a committed file")
	}
	if !strings.HasSuffix(string(doc.Bytes()), "\n") {
		return errors.New("bytes")
	}
	return nil
}

func TestCrossFormatSaveGuard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("A=1\nTOKEN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var seen []string
	f, err := Open(p, WithPlugin(docGuardPlugin{seen: &seen}))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Save(); err != nil {
		t.Fatalf("clean save: %v", err)
	}
	if len(seen) != 1 || seen[0] != "dotenv|"+p+"|A,TOKEN" {
		t.Errorf("guard saw %q", seen)
	}
	f.Set("TOKEN", "abc")
	err = f.Save()
	if err == nil || !strings.Contains(err.Error(), `plugin "no-secrets"`) {
		t.Fatalf("guard did not veto: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "A=1\nTOKEN=\n" {
		t.Errorf("vetoed save changed the file: %q", b)
	}
	if got := f.Plugins()[0].String(); got != "no-secrets: SaveGuard" {
		t.Errorf("Plugins = %q", got)
	}
}

func TestSaveWritesThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "shared.env")
	if err := os.WriteFile(real, []byte("A=1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".env")
	if err := os.Symlink("shared.env", link); err != nil {
		t.Fatal(err)
	}
	f, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "2")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(real); string(b) != "A=2\n" {
		t.Errorf("real file = %q", b)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("Save replaced the symlink with a regular file")
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", st.Mode().Perm())
	}
}
