// Package vault reads secrets from HashiCorp Vault for every textdata format.
//
// One *Plugin is both a plugin.Resolver and a plugin.Lookuper over one HTTP
// client and one cache:
//
//	${vault:secret/data/app#password}   Resolver: KV v2 secret, one field
//	${vault:kv/app#password}            Resolver: KV v1 secret, one field
//	${DB_PASSWORD}                      Lookuper: a name mapped with WithLookup or WithLookupPrefix
//
// The plugin is read-only (it only issues GET requests), talks to Vault's
// HTTP API with net/http, and never reads the process environment unless the
// caller passes FromEnv. A Vault that is down is an error wrapping
// plugin.ErrUnavailable, a refusal wraps plugin.ErrDenied, and a missing path
// or field wraps plugin.ErrNotFound. No error ever contains a secret value.
package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/textdata/plugin"
)

// Defaults a Plugin starts from.
const (
	// DefaultName is the plugin name errors report.
	DefaultName = "vault"
	// DefaultScheme is the scheme the Resolver owns: ${vault:...}.
	DefaultScheme = "vault"
	// DefaultTimeout bounds one request to Vault.
	DefaultTimeout = 10 * time.Second
)

// Environment variables FromEnv reads, the same ones the vault CLI uses.
const (
	EnvAddr      = "VAULT_ADDR"
	EnvToken     = "VAULT_TOKEN"
	EnvNamespace = "VAULT_NAMESPACE"
)

// HTTP headers of the Vault API.
const (
	headerToken     = "X-Vault-Token"
	headerNamespace = "X-Vault-Namespace"
	headerRequest   = "X-Vault-Request"
)

// apiPrefix is where every Vault API path lives.
const apiPrefix = "/v1/"

// maxBody caps how much of a response is read: a misbehaving server cannot
// make a config read allocate without bound.
const maxBody = 4 << 20

// maxDetail caps how much of Vault's own error text an error repeats.
const maxDetail = 300

// ErrInvalidRef is wrapped by every error about a malformed reference or a
// malformed option, as opposed to a failed read.
var ErrInvalidRef = errors.New("invalid reference")

// Plugin resolves ${vault:ref} and looks up mapped ${NAME} references.
// Build it with New; it is safe for concurrent use.
type Plugin struct {
	name     string
	scheme   string
	addr     string
	token    string
	ns       string
	client   *http.Client
	timeout  time.Duration
	lookups  map[string]string
	prefixes []string

	// mu guards cache. The cache holds each secret's decoded key/value map
	// for the plugin's lifetime: a config read touches the same secret many
	// times and must see one consistent version of it. Only successful reads
	// are cached, so a transient failure is retried on the next reference.
	mu    sync.Mutex
	cache map[string]map[string]json.RawMessage
}

// config collects options before New validates them.
type config struct {
	name, scheme, addr, token, ns string
	client                        *http.Client
	timeout                       time.Duration
	lookups                       map[string]string
	prefixes                      []string
}

// Option configures a Plugin. Options apply in order; a later one wins.
type Option func(*config)

// WithAddress sets the Vault address, e.g. "https://vault.example.com:8200".
// Required (directly or through FromEnv): the plugin has no default server.
func WithAddress(addr string) Option { return func(c *config) { c.addr = addr } }

// WithToken sets the token sent as X-Vault-Token. An empty token sends none,
// for a Vault Agent or proxy that authenticates on the caller's behalf.
func WithToken(token string) Option { return func(c *config) { c.token = token } }

// WithNamespace sets the Vault Enterprise namespace (X-Vault-Namespace).
func WithNamespace(ns string) Option { return func(c *config) { c.ns = ns } }

// WithHTTPClient sets the HTTP client, for TLS settings, proxies or tests.
// nil keeps the default.
func WithHTTPClient(h *http.Client) Option { return func(c *config) { c.client = h } }

// WithTimeout bounds each request; zero or less keeps DefaultTimeout.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithName sets the plugin name errors report (default "vault"), so two
// instances pointed at two servers can be told apart.
func WithName(name string) Option { return func(c *config) { c.name = name } }

// WithScheme sets the scheme the Resolver owns (default "vault"), so two
// instances can coexist: ${vault:...} and ${vault-dr:...}.
func WithScheme(scheme string) Option { return func(c *config) { c.scheme = strings.ToLower(scheme) } }

// WithLookup maps ${name} to a reference (same grammar as ${vault:ref}).
// Mapped names are tried before any WithLookupPrefix.
func WithLookup(name, ref string) Option {
	return func(c *config) {
		if c.lookups == nil {
			c.lookups = map[string]string{}
		}
		c.lookups[name] = ref
	}
}

// WithLookupPrefix answers every ${NAME} by reading the reference
// prefix+NAME, e.g. "secret/data/app#" makes ${DB_PASSWORD} read field
// DB_PASSWORD of secret/data/app. Prefixes are tried in the order given; a
// reference that does not exist answers "not mine" (ok=false) and the next
// prefix, then the next Lookuper, is tried.
func WithLookupPrefix(prefix string) Option {
	return func(c *config) { c.prefixes = append(c.prefixes, prefix) }
}

// FromEnv fills the address, token and namespace from VAULT_ADDR,
// VAULT_TOKEN and VAULT_NAMESPACE, each only when set. This is the only way
// the plugin reads the environment, and it does so once, in New. Options
// after FromEnv override it.
func FromEnv() Option {
	return func(c *config) {
		if v, ok := os.LookupEnv(EnvAddr); ok {
			c.addr = v
		}
		if v, ok := os.LookupEnv(EnvToken); ok {
			c.token = v
		}
		if v, ok := os.LookupEnv(EnvNamespace); ok {
			c.ns = v
		}
	}
}

// New builds a Plugin. It fails when no address is set, the address is not
// an http(s) URL, or a WithLookup reference does not parse. It sends no
// request: an unreachable Vault shows up on the first read.
func New(opts ...Option) (*Plugin, error) {
	c := config{name: DefaultName, scheme: DefaultScheme, timeout: DefaultTimeout}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.addr == "" {
		return nil, fmt.Errorf("vault: %w: no address (use WithAddress or FromEnv)", ErrInvalidRef)
	}
	u, err := url.Parse(c.addr)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("vault: %w: address must be an http or https URL without credentials", ErrInvalidRef)
	}
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	if c.client == nil {
		c.client = &http.Client{}
	}
	if c.name == "" {
		c.name = DefaultName
	}
	if c.scheme == "" {
		c.scheme = DefaultScheme
	}
	for name, ref := range c.lookups {
		if _, err := ParseRef(ref); err != nil {
			return nil, fmt.Errorf("vault: lookup %s: %w", name, err)
		}
	}
	return &Plugin{
		name:     c.name,
		scheme:   c.scheme,
		addr:     strings.TrimRight(u.String(), "/"),
		token:    c.token,
		ns:       c.ns,
		client:   c.client,
		timeout:  c.timeout,
		lookups:  c.lookups,
		prefixes: c.prefixes,
		cache:    map[string]map[string]json.RawMessage{},
	}, nil
}

// Name implements plugin.Plugin.
func (p *Plugin) Name() string { return p.name }

// Schemes implements plugin.Resolver.
func (p *Plugin) Schemes() []string { return []string{p.scheme} }

// Resolve implements plugin.Resolver: the value of field ref.Field of the
// secret at ref.Path. A scheme other than the plugin's wraps
// plugin.ErrUnsupportedScheme.
func (p *Plugin) Resolve(scheme, ref string) (string, error) {
	if !strings.EqualFold(scheme, p.scheme) {
		return "", fmt.Errorf("%w %q", plugin.ErrUnsupportedScheme, scheme)
	}
	r, err := ParseRef(ref)
	if err != nil {
		return "", err
	}
	return p.Read(r)
}

// Lookup implements plugin.Lookuper. A name with neither a WithLookup
// mapping nor a matching WithLookupPrefix answers ok=false; a mapped
// reference that does not exist also answers ok=false (the base contract:
// a Lookuper says "not mine", only a Resolver says "not found"). Denied and
// unavailable are errors, never ok=false.
func (p *Plugin) Lookup(name string) (string, bool, error) {
	var refs []string
	if ref, ok := p.lookups[name]; ok {
		refs = append(refs, ref)
	}
	for _, pre := range p.prefixes {
		refs = append(refs, pre+name)
	}
	for _, ref := range refs {
		r, err := ParseRef(ref)
		if err != nil {
			continue // a prefix that cannot form a valid reference for this name
		}
		v, err := p.Read(r)
		if errors.Is(err, plugin.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", false, err
		}
		return v, true, nil
	}
	return "", false, nil
}

// Read returns one field of a secret, from the cache when this plugin has
// read the secret before. A field that is not a string is returned as its
// JSON text (42, true, {"a":1}).
func (p *Plugin) Read(r Ref) (string, error) {
	data, err := p.secret(r)
	if err != nil {
		return "", err
	}
	if !r.HasField {
		switch len(data) {
		case 1:
			for _, v := range data {
				return fieldText(v)
			}
		case 0:
			return "", fmt.Errorf("%s: %w: secret has no fields", r.Path, plugin.ErrNotFound)
		}
		return "", fmt.Errorf("%s: %w: secret has %d fields (%s); name one with #field", r.Path, ErrInvalidRef, len(data), fieldNames(data))
	}
	v, ok := data[r.Field]
	if !ok {
		return "", fmt.Errorf("%s: field %q: %w (fields: %s)", r.Path, r.Field, plugin.ErrNotFound, fieldNames(data))
	}
	return fieldText(v)
}

// Purge empties the cache, so the next reference reads Vault again.
func (p *Plugin) Purge() {
	p.mu.Lock()
	p.cache = map[string]map[string]json.RawMessage{}
	p.mu.Unlock()
}

// secret returns the key/value map of the secret r names, reading it once.
func (p *Plugin) secret(r Ref) (map[string]json.RawMessage, error) {
	key := r.cacheKey()
	p.mu.Lock()
	data, ok := p.cache[key]
	p.mu.Unlock()
	if ok {
		return data, nil
	}
	data, err := p.fetch(r)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[key] = data
	p.mu.Unlock()
	return data, nil
}

// fetch performs the GET and maps the outcome onto the plugin errors.
func (p *Plugin) fetch(r Ref) (map[string]json.RawMessage, error) {
	op := "read " + r.cacheKey()
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	u := p.addr + apiPrefix + escapePath(r.Path)
	if r.Version > 0 {
		u += "?version=" + strconv.Itoa(r.Version)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if p.token != "" {
		req.Header.Set(headerToken, p.token)
	}
	if p.ns != "" {
		req.Header.Set(headerNamespace, p.ns)
	}
	req.Header.Set(headerRequest, "true")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %v", op, plugin.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("%s: %w: reading response: %v", op, plugin.ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(op, resp.StatusCode, body)
	}
	return decodeSecret(op, body)
}

// statusError maps a non-200 status onto the plugin's named errors. Vault's
// own error text ({"errors": [...]}) is repeated, truncated; it describes the
// request, never a secret.
func statusError(op string, status int, body []byte) error {
	detail := errorDetail(body)
	var kind error
	switch {
	case status == http.StatusNotFound:
		kind = plugin.ErrNotFound
	case status == http.StatusForbidden || status == http.StatusUnauthorized:
		kind = plugin.ErrDenied
	case status == http.StatusTooManyRequests || status >= 500:
		kind = plugin.ErrUnavailable
	case status == 412 || status == 473:
		// 412: a performance standby has not caught up; 473: performance
		// standby redirect. Both mean "try again", not "no such secret".
		kind = plugin.ErrUnavailable
	}
	msg := fmt.Sprintf("status %d", status)
	if detail != "" {
		msg += ": " + detail
	}
	if kind == nil {
		return fmt.Errorf("%s: %s", op, msg)
	}
	return fmt.Errorf("%s: %w: %s", op, kind, msg)
}

// errorDetail extracts Vault's error messages, or "" when the body is not
// Vault's error shape. Never the raw body: a proxy might echo anything.
func errorDetail(body []byte) string {
	var e struct {
		Errors []string `json:"errors"`
	}
	if json.Unmarshal(body, &e) != nil || len(e.Errors) == 0 {
		return ""
	}
	s := strings.Join(e.Errors, "; ")
	if len(s) > maxDetail {
		s = s[:maxDetail] + "..."
	}
	return s
}

// decodeSecret turns a 200 response into the secret's key/value map,
// detecting KV v2 ({"data":{"data":{...},"metadata":{...}}}) versus KV v1
// ({"data":{...}}). A decode failure never repeats the parser's message: it
// can quote bytes of the secret.
func decodeSecret(op string, body []byte) (map[string]json.RawMessage, error) {
	var top struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("%s: response is not a Vault secret (invalid JSON)", op)
	}
	if top.Data == nil {
		return nil, fmt.Errorf("%s: %w: response has no data", op, plugin.ErrNotFound)
	}
	raw := top.Data
	inner, hasInner := raw["data"]
	_, hasMeta := raw["metadata"]
	if hasInner && hasMeta {
		if bytes.Equal(bytes.TrimSpace(inner), []byte("null")) {
			return nil, fmt.Errorf("%s: %w: version deleted or destroyed", op, plugin.ErrNotFound)
		}
		var v2 map[string]json.RawMessage
		if err := json.Unmarshal(inner, &v2); err != nil {
			return nil, fmt.Errorf("%s: response is not a Vault secret (data.data is not an object)", op)
		}
		raw = v2
	}
	return raw, nil
}

// fieldText renders a field: a JSON string as its contents, anything else
// as its compact JSON text.
func fieldText(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return "", errors.New("field is not valid JSON")
	}
	return b.String(), nil
}

// fieldNames lists a secret's keys, sorted. Names, never values.
func fieldNames(data map[string]json.RawMessage) string {
	names := make([]string, 0, len(data))
	for k := range data {
		names = append(names, strconv.Quote(k))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// escapePath escapes each segment, so a "?" or "%" in a secret name stays
// part of the path.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// Compile-time proof of the capabilities the plugin claims.
var (
	_ plugin.Resolver = (*Plugin)(nil)
	_ plugin.Lookuper = (*Plugin)(nil)
)
