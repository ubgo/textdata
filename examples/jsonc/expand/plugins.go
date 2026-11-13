package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
)

// vaultScheme is the ${scheme:ref} prefix the fake resolver owns.
const vaultScheme = "vault"

// fakeVault is a Resolver: it owns ${vault:...} references. A real one would
// call a secret store; this one answers from a map.
//
//ds:def id=fakevault-qkhb7zj3
type fakeVault struct{ secrets map[string]string }

//ds:def id=fakevault-name-mjgwk5tq
func (fakeVault) Name() string { return "fake-vault" }

//ds:def id=fakevault-schemes-pjpp743s
func (fakeVault) Schemes() []string { return []string{vaultScheme} }

//ds:def id=fakevault-resolve-ppavttx8
func (v fakeVault) Resolve(_, ref string) (string, error) {
	if s, ok := v.secrets[ref]; ok {
		return s, nil
	}
	return "", plugin.ErrNotFound
}

// trimSpace is a ValueTransformer: every expanded string passes through it
// with its canonical path, here to drop stray whitespace.
//
//ds:def id=trimspace-butyt4h4
type trimSpace struct{}

func (trimSpace) Name() string { return "trim-space" }

//ds:def id=trimspace-transformvalue-6yurxd5v
func (trimSpace) TransformValue(_, value string) (string, error) {
	return strings.TrimSpace(value), nil
}

// fakeGit is a CommandRunner: installing one turns $(command) on and runs it
// through this function instead of a shell.
//
//ds:def id=fakegit-xgrn2dcy
type fakeGit struct{}

func (fakeGit) Name() string { return "fake-git" }

//ds:def id=fakegit-runcommand-9hny8xsp
func (fakeGit) RunCommand(_ string) (string, error) {
	return "a1b2c3d", nil
}

// withPlugins installs a resolver, a transformer and a command runner, and
// lists what jsonc detected for each.
//
//ds:def id=withplugins-3au5u4sa
func withPlugins(w io.Writer) error {
	vault := fakeVault{secrets: map[string]string{"db/password": "  hush \n"}}
	doc, err := jsonc.Parse([]byte(src), jsonc.WithLookup(lookup), jsonc.WithPlugin(vault, trimSpace{}, fakeGit{}))
	if err != nil {
		return err
	}
	pw, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	build, err := doc.GetExpanded("build")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "password: %q | build: %v\n", pw, build)
	for _, info := range doc.Plugins() {
		fmt.Fprintln(w, "plugin:", info)
	}
	return nil
}

// commandSubstitution turns on $(command) through `sh -c`. Only do this for
// files you trust: expanding the file runs whatever it says.
//
//ds:def id=commandsubstitution-npg99swj
func commandSubstitution(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(`{"greeting": "$(echo hello)"}`), jsonc.WithCommandSubstitution(true))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("greeting")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "sh -c:", v)
	return nil
}

// readExpanded is the one-call path: WithExpand makes Read expand before it
// decodes. Doc methods ignore WithExpand; ExpandRequested reports it so a
// caller holding the Doc can honour it.
//
//ds:def id=readexpanded-n5grq4ta
func readExpanded(w io.Writer, dir string) error {
	path := filepath.Join(dir, configName)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		return err
	}
	m, err := jsonc.Read(path, jsonc.WithExpand(), jsonc.WithLookup(lookup))
	if err != nil {
		return err
	}
	doc, err := jsonc.Open(path, jsonc.WithExpand())
	if err != nil {
		return err
	}
	db, ok := m["db"].(map[string]any)
	if !ok {
		return fmt.Errorf("db is %T, want an object", m["db"])
	}
	fmt.Fprintln(w, "Read:", db["url"], "| ExpandRequested:", doc.ExpandRequested())
	return nil
}
