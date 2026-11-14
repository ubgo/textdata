package schema_test

// These tests pin FromStruct + Validate against a config shaped like the agx
// v2 config (~/.agx/config.jsonc read into a typed struct): maps of structs,
// a string-kind named TextMarshaler used as map key and as value, pointers,
// omitempty, a tag enum, and typo'd keys inside map value structs. Positions
// are asserted so a regression in where an issue points is caught too.

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc/generate"
	"github.com/ubgo/textdata/jsonc/schema"
)

// agxAgentID mirrors agentio.AgentID: a string-kind named type that owns its
// text form. encoding/json writes it as a JSON string, both as a value and as
// a map key, so its schema must be "string", never "object".
type agxAgentID string

// MarshalText writes the id verbatim.
func (a agxAgentID) MarshalText() ([]byte, error) { return []byte(a), nil }

// UnmarshalText rejects an empty id, as a real id type would.
func (a *agxAgentID) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return errors.New("agent id is empty")
	}
	*a = agxAgentID(b)
	return nil
}

// agxBilling is how a profile pays; the closed set lives in the enum tag.
type agxBilling string

// agxProfile is a named agent profile. It is reached twice (map value and
// *agxProfile), so FromStruct puts it in $defs.
type agxProfile struct {
	Name    string            `json:"name"`
	Billing agxBilling        `json:"billing" enum:"plan,api" default:"plan"`
	Args    []string          `json:"args,omitempty"`
	Timeout *int              `json:"timeout,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// agxBinding ties an agent to a profile.
type agxBinding struct {
	Agent   agxAgentID `json:"agent"`
	Profile string     `json:"profile"`
	Model   *string    `json:"model,omitempty"`
}

// agxConfig is the root of the file.
type agxConfig struct {
	Default  agxAgentID                `json:"default" enum:"claude,codex"`
	Profiles map[string]agxProfile     `json:"profiles"`
	Bindings map[agxAgentID]agxBinding `json:"bindings,omitempty"`
	Tags     []string                  `json:"tags,omitempty"`
	Extra    *agxProfile               `json:"extra,omitempty"`
}

func agxSchema(t *testing.T) *schema.Schema {
	t.Helper()
	s, err := schema.FromStruct(agxConfig{})
	if err != nil {
		t.Fatalf("FromStruct: %v", err)
	}
	return s
}

// TestAgxShapeSchema pins the schema FromStruct emits for each field shape.
func TestAgxShapeSchema(t *testing.T) {
	t.Parallel()
	s := agxSchema(t)
	profile := s.Defs["agxProfile"]
	if profile == nil {
		t.Fatalf("agxProfile not in $defs: %v", reflect.ValueOf(s.Defs).MapKeys())
	}
	profileRef := "#/$defs/agxProfile"
	bindings := s.Properties["bindings"]
	binding := bindings.AdditionalProperties
	extra := s.Properties["extra"]

	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"nothing required (omitempty and untagged)", s.Required, []string(nil)},
		{"root rejects unknown keys", *s.AdditionalProperties.Bool, false},
		{"TextMarshaler value is a string", s.Properties["default"].Types, []string{schema.TypeString}},
		{"TextMarshaler value keeps its enum", rawStrings(s.Properties["default"].Enum), []string{`"claude"`, `"codex"`}},
		{"map[string]struct is an object", s.Properties["profiles"].Types, []string{schema.TypeObject}},
		{"map[string]struct values ref the def", s.Properties["profiles"].AdditionalProperties.Ref, profileRef},
		{"map[TextMarshaler]struct is an object", bindings.Types, []string{schema.TypeObject}},
		{"map value struct is inline once", binding.Types, []string{schema.TypeObject}},
		{"map value struct rejects unknown keys", *binding.AdditionalProperties.Bool, false},
		{"TextMarshaler field in map value is a string", binding.Properties["agent"].Types, []string{schema.TypeString}},
		{"*string is string or null", binding.Properties["model"].Types, []string{schema.TypeString, schema.TypeNull}},
		{"[]string is array or null", s.Properties["tags"].Types, []string{schema.TypeArray, schema.TypeNull}},
		{"[]string items are strings", s.Properties["tags"].Items.Types, []string{schema.TypeString}},
		{"*struct is anyOf ref, null", []string{extra.AnyOf[0].Ref, extra.AnyOf[1].Types[0]}, []string{profileRef, schema.TypeNull}},
		{"def rejects unknown keys", *profile.AdditionalProperties.Bool, false},
		{"enum tag on named string", rawStrings(profile.Properties["billing"].Enum), []string{`"plan"`, `"api"`}},
		{"enum field default", string(profile.Properties["billing"].Default), `"plan"`},
		{"enum field is a string", profile.Properties["billing"].Types, []string{schema.TypeString}},
		{"*int is integer or null", profile.Properties["timeout"].Types, []string{schema.TypeInteger, schema.TypeNull}},
		{"map[string]string values", profile.Properties["env"].AdditionalProperties.Types, []string{schema.TypeString}},
	} {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func rawStrings(l []json.RawMessage) []string {
	var out []string
	for _, r := range l {
		out = append(out, string(r))
	}
	return out
}

// agxValid is a full, valid file; each case below changes one line of it.
const agxValid = `// agx v2 config
{
  "$schema": "./config.schema.json",
  "default": "claude",
  /* profiles keyed by name */
  "profiles": {
    "work": {
      "name": "Work",
      "billing": "plan", // plan | api
      "args": ["--fast"],
      "timeout": 30,
      "env": {"A": "1"}
    },
    "home": {"name": "Home", "billing": "api", "timeout": null}
  },
  "bindings": {
    "claude": {"agent": "claude", "profile": "work", "model": null},
    "codex": {"agent": "codex", "profile": "home", "model": "o4"}
  },
  "tags": ["a", "b"],
  "extra": {"name": "X", "billing": "api"}
}
`

// TestAgxShapeValidate runs Validate on edits of agxValid and asserts every
// issue's position, path and keyword.
func TestAgxShapeValidate(t *testing.T) {
	t.Parallel()
	s := agxSchema(t)
	for _, tc := range []struct {
		name, from, to string
		errs, warns    []string
		// msg, when set, must appear in the first error's rendered text.
		msg string
	}{
		{name: "valid"},
		{name: "omitted optional keys", from: `"tags": ["a", "b"],`, to: `"tags": null,`},
		{
			name: "enum rejects free in map value",
			from: `"billing": "plan", // plan | api`, to: `"billing": "free", // plan | api`,
			errs: []string{"9:18 profiles.work.billing enum"},
			msg:  `cfg.jsonc:9:18: profiles.work.billing: must be one of "plan", "api"; got "free"`,
		},
		{
			name: "enum rejects free behind pointer",
			from: `"extra": {"name": "X", "billing": "api"}`, to: `"extra": {"name": "X", "billing": "free"}`,
			errs: []string{"21:37 extra.billing enum"},
		},
		{
			name: "enum on TextMarshaler root value",
			from: `"default": "claude",`, to: `"default": "gemini",`,
			errs: []string{"4:14 default enum"},
		},
		{
			name: "typo in map value struct key",
			from: `"billing": "plan", // plan | api`, to: `"biling": "plan", // plan | api`,
			warns: []string{"9:7 profiles.work.biling additionalProperties"},
		},
		{
			name: "typo in root key",
			from: `"profiles": {`, to: `"profles": {`,
			warns: []string{"6:3 profles additionalProperties"},
		},
		{
			name: "typo in map[TextMarshaler] value struct",
			from: `"model": "o4"`, to: `"modle": "o4"`,
			warns: []string{"18:52 bindings.codex.modle additionalProperties"},
		},
		{
			name: "typo behind pointer to struct",
			from: `"extra": {"name": "X",`, to: `"extra": {"nmae": "X",`,
			warns: []string{"21:13 extra.nmae additionalProperties"},
		},
		{
			name: "TextMarshaler value must be a string",
			from: `"agent": "claude"`, to: `"agent": {"id": "claude"}`,
			errs: []string{"17:25 bindings.claude.agent type"},
		},
		{
			name: "pointer to int rejects a string",
			from: `"timeout": 30,`, to: `"timeout": "30",`,
			errs: []string{"11:18 profiles.work.timeout type"},
		},
		{
			name: "slice item type",
			from: `"tags": ["a", "b"],`, to: `"tags": ["a", 2],`,
			errs: []string{"20:17 tags[1] type"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := agxValid
			if tc.from != "" {
				if !strings.Contains(src, tc.from) {
					t.Fatalf("fixture has no %q", tc.from)
				}
				src = strings.Replace(src, tc.from, tc.to, 1)
			}
			res := schema.Validate(mustDoc(t, src), s)
			if got := summary(res.Errors); !reflect.DeepEqual(got, orEmpty(tc.errs)) {
				t.Errorf("errors: got %q, want %q", got, tc.errs)
			}
			if got := summary(res.Warnings); !reflect.DeepEqual(got, orEmpty(tc.warns)) {
				t.Errorf("warnings: got %q, want %q", got, tc.warns)
			}
			if tc.msg != "" && (len(res.Errors) == 0 || res.Errors[0].Error() != tc.msg) {
				t.Errorf("message: got %v, want %q", res.Errors, tc.msg)
			}
		})
	}
}

// TestAgxShapeStrictKeys: with StrictKeys a typo'd key is an error at the
// key, so Result.OK and Err (and a Plugin's save guard) refuse the file; the
// same key is no longer also a warning.
// ds:def id=testagxshapestrictkeys-46m8nuv5 owner=@maintainers stability=stable
func TestAgxShapeStrictKeys(t *testing.T) {
	t.Parallel()
	s := agxSchema(t)
	for _, tc := range []struct {
		name, from, to string
		errs           []string
		msg            string
	}{
		{name: "valid stays valid"},
		{
			name: "typo in map value struct",
			from: `"billing": "plan", // plan | api`, to: `"biling": "plan", // plan | api`,
			errs: []string{"9:7 profiles.work.biling additionalProperties"},
			msg:  `cfg.jsonc:9:7: profiles.work.biling: unknown key "biling"`,
		},
		{
			name: "typo in root",
			from: `"profiles": {`, to: `"profles": {`,
			errs: []string{"6:3 profles additionalProperties"},
		},
		{
			name: "typo behind pointer to struct (anyOf branch)",
			from: `"extra": {"name": "X",`, to: `"extra": {"nmae": "X",`,
			errs: []string{"21:13 extra.nmae additionalProperties"},
		},
		{
			name: "typo and bad enum together",
			from: `"extra": {"name": "X", "billing": "api"}`, to: `"extra": {"nmae": "X", "billing": "free"}`,
			errs: []string{"21:13 extra.nmae additionalProperties", "21:37 extra.billing enum"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := strings.Replace(agxValid, tc.from, tc.to, 1)
			res := schema.Validate(mustDoc(t, src), s, schema.StrictKeys())
			if got := summary(res.Errors); !reflect.DeepEqual(got, orEmpty(tc.errs)) {
				t.Errorf("errors: got %q, want %q", got, tc.errs)
			}
			if len(res.Warnings) > 0 {
				t.Errorf("warnings: got %q, want none", summary(res.Warnings))
			}
			if (res.Err() == nil) != (len(tc.errs) == 0) {
				t.Errorf("Err() = %v with errors %q", res.Err(), tc.errs)
			}
			if tc.msg != "" && (len(res.Errors) == 0 || res.Errors[0].Error() != tc.msg) {
				t.Errorf("message: got %v, want %q", res.Errors, tc.msg)
			}
		})
	}
}

// TestAgxShapeGenerateAgrees checks jsonc/generate reads the same tags: the
// file it writes for the struct validates clean against FromStruct's schema,
// and the enum note lists the same values.
func TestAgxShapeGenerateAgrees(t *testing.T) {
	t.Parallel()
	f, err := generate.FromStruct(agxConfig{Default: "claude", Profiles: map[string]agxProfile{"work": {Name: "W", Billing: "plan"}}})
	if err != nil {
		t.Fatalf("generate.FromStruct: %v", err)
	}
	b, err := f.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "One of: plan, api.") || !strings.Contains(out, "One of: claude, codex.") {
		t.Errorf("enum notes missing:\n%s", out)
	}
	res := schema.Validate(mustDoc(t, out), agxSchema(t))
	if !res.OK() || len(res.Warnings) > 0 {
		t.Errorf("generated file does not validate: %v %v\n%s", res.Errors, res.Warnings, out)
	}
}

// TestAgxShapeStrictKeysPlugin: StrictKeys passed to Plugin makes the save
// guard refuse a typo'd key, which a plain Plugin lets through as a warning.
func TestAgxShapeStrictKeysPlugin(t *testing.T) {
	t.Parallel()
	src := strings.Replace(agxValid, `"profiles": {`, `"profles": {`, 1)
	for _, tc := range []struct {
		name    string
		opts    []schema.Option
		wantErr string
	}{
		{name: "default lets it save"},
		{name: "strict refuses", opts: []schema.Option{schema.StrictKeys()}, wantErr: `cfg.jsonc:6:3: profles: unknown key "profles"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := schema.Plugin(agxSchema(t), tc.opts...).GuardSave(mustDoc(t, src))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("GuardSave = %v, want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("GuardSave = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
