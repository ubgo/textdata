package gopassplugin_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	gopassplugin "github.com/ubgo/textdata/plugins/gopass"
)

// fakeGopass answers `gopass show -o <ref>` from a map and counts calls; no
// gopass binary, no store.
type fakeGopass struct {
	entries map[string]string
	calls   int
}

func (f *fakeGopass) run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.calls++
	if v, ok := f.entries[args[len(args)-1]]; ok {
		return []byte(v + "\n"), nil, nil
	}
	return nil, []byte("entry is not in the password store"), errors.New("exit status 11")
}

// TestSameInstanceBothFormats installs ONE plugin instance in a dotenv file
// and a jsonc file: both resolve through it, both keep the reference in what
// they render, and the shared cache means the second format costs no call.
func TestSameInstanceBothFormats(t *testing.T) {
	fg := &fakeGopass{entries: map[string]string{"personal/db": "pw-1", "app/TOKEN": "tok-2"}}
	p := gopassplugin.New(gopassplugin.WithRunner(fg.run), gopassplugin.WithPrefix("app/"))

	envSrc := "DB_PASSWORD=${gopass:personal/db}\nAUTH=Bearer ${TOKEN}\n"
	env := dotenv.Parse(envSrc, dotenv.WithPlugin(p))
	if v, _, err := env.GetExpanded("DB_PASSWORD"); err != nil || v != "pw-1" {
		t.Fatalf("dotenv resolve = %q, %v", v, err)
	}
	if v, _, err := env.GetExpanded("AUTH"); err != nil || v != "Bearer tok-2" {
		t.Fatalf("dotenv lookup = %q, %v", v, err)
	}
	if env.Render() != envSrc {
		t.Error("dotenv render changed")
	}

	jsSrc := []byte(`{
  // database
  "db": {"password": "${gopass:personal/db}"},
  "auth": "Bearer ${TOKEN}",
}`)
	doc, err := jsonc.Parse(jsSrc, jsonc.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := doc.GetExpanded("db.password"); err != nil || v != "pw-1" {
		t.Fatalf("jsonc resolve = %v, %v", v, err)
	}
	if v, err := doc.GetExpanded("auth"); err != nil || v != "Bearer tok-2" {
		t.Fatalf("jsonc lookup = %v, %v", v, err)
	}
	if string(doc.Render()) != string(jsSrc) {
		t.Error("jsonc render changed")
	}
	if fg.calls != 2 {
		t.Errorf("gopass calls = %d, want 2 (cache shared across formats)", fg.calls)
	}
}

// TestErrorsReachCaller: a missing entry and a store outage surface through
// both formats with the plugin's sentinel, never as an empty value.
func TestErrorsReachCaller(t *testing.T) {
	fg := &fakeGopass{}
	p := gopassplugin.New(gopassplugin.WithRunner(fg.run))
	env := dotenv.Parse("K=${gopass:nope}\n", dotenv.WithPlugin(p))
	if _, _, err := env.GetExpanded("K"); !errors.Is(err, plugin.ErrNotFound) || !strings.Contains(err.Error(), `plugin "gopass"`) {
		t.Errorf("dotenv err = %v", err)
	}
	doc, _ := jsonc.Parse([]byte(`{"k": "${gopass:nope}"}`), jsonc.WithPlugin(p))
	if _, err := doc.GetExpanded("k"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("jsonc err = %v", err)
	}

	down := gopassplugin.New(gopassplugin.WithRunner(func(context.Context, string, []string) ([]byte, []byte, error) {
		return nil, []byte("gpg: decryption failed"), errors.New("exit status 1")
	}))
	doc, _ = jsonc.Parse([]byte(`{"k": "${gopass:a}"}`), jsonc.WithPlugin(down))
	if _, err := doc.GetExpanded("k"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Errorf("jsonc outage = %v", err)
	}
}

// Example shows the plugin resolving a reference in a dotenv file. The runner
// is a stand-in; leave WithRunner out to run the real gopass binary.
func Example() {
	fake := func(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
		return []byte("value-of-" + args[2] + "\n"), nil, nil
	}
	p := gopassplugin.New(gopassplugin.WithRunner(fake))

	f := dotenv.Parse("DB_PASSWORD=${gopass:personal/db}\n", dotenv.WithPlugin(p))
	v, _, err := f.GetExpanded("DB_PASSWORD")
	fmt.Println(v, err)
	fmt.Print(f.Render())
	// Output:
	// value-of-personal/db <nil>
	// DB_PASSWORD=${gopass:personal/db}
}
