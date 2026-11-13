// Package aws reads secrets from AWS Secrets Manager and SSM Parameter Store
// for every textdata format.
//
// One *Plugin is both a plugin.Resolver and a plugin.Lookuper over one HTTP
// client and one cache:
//
//	${aws:sm/prod/db#password}      Secrets Manager secret, one key of its JSON value
//	${aws:sm/prod/api-key}          Secrets Manager secret, whole value
//	${aws:ssm//app/prod/db_url}     Parameter Store parameter "/app/prod/db_url", decrypted
//	${DB_PASSWORD}                  Lookuper: a name mapped with WithLookup or WithLookupPrefix
//
// See Ref for the full grammar. The plugin is read-only (GetSecretValue and
// GetParameter only), speaks the AWS JSON protocol over net/http, and signs
// with its own Signature Version 4 implementation, checked against AWS's
// published test suite: no AWS SDK. It never reads the environment or a
// file unless the caller passes FromEnv or FromSharedCredentials.
//
// AWS being down or throttling is an error wrapping plugin.ErrUnavailable, a
// refusal (permissions, bad or expired credentials, KMS) wraps
// plugin.ErrDenied, and a missing secret, parameter or key wraps
// plugin.ErrNotFound. No error ever contains a secret value.
package aws

import (
	"bytes"
	"context"
	"encoding/base64"
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
	DefaultName = "aws"
	// DefaultScheme is the scheme the Resolver owns: ${aws:...}.
	DefaultScheme = "aws"
	// DefaultTimeout bounds one request to AWS.
	DefaultTimeout = 10 * time.Second
)

// AWS service identifiers: the SigV4 signing name, the endpoint prefix, and
// the X-Amz-Target of the one operation each service is called with.
const (
	smSigningName  = "secretsmanager"
	ssmSigningName = "ssm"
	smTarget       = "secretsmanager.GetSecretValue"
	ssmTarget      = "AmazonSSM.GetParameter"
	jsonContent    = "application/x-amz-json-1.1"
	headerTarget   = "X-Amz-Target"
	headerErrType  = "X-Amzn-Errortype"
)

// maxBody caps how much of a response is read.
const maxBody = 1 << 20

// maxDetail caps how much of AWS's error message an error repeats.
const maxDetail = 300

// ErrInvalidRef is wrapped by every error about a malformed reference or
// option, as opposed to a failed read.
var ErrInvalidRef = errors.New("invalid reference")

// Plugin resolves ${aws:ref} and looks up mapped ${NAME} references. Build
// it with New; it is safe for concurrent use.
type Plugin struct {
	name     string
	scheme   string
	region   string
	creds    CredentialsSource
	endpoint string // "" means the regional AWS endpoints
	client   *http.Client
	timeout  time.Duration
	lookups  map[string]string
	prefixes []string
	// now is the signing clock; tests pin it.
	now func() time.Time

	// mu guards cache: raw values by Ref.cacheKey, kept for the plugin's
	// lifetime so one config read sees one version of each secret. Only
	// successes are cached.
	mu    sync.Mutex
	cache map[string]string
}

// config collects options before New validates them.
type config struct {
	name, scheme, region, endpoint string
	creds                          CredentialsSource
	client                         *http.Client
	timeout                        time.Duration
	lookups                        map[string]string
	prefixes                       []string
	err                            error
}

// Option configures a Plugin. Options apply in order; a later one wins.
type Option func(*config)

// WithRegion sets the AWS region, e.g. "eu-west-1". Required (directly or
// through FromEnv).
func WithRegion(region string) Option { return func(c *config) { c.region = region } }

// WithCredentials signs with fixed credentials.
func WithCredentials(cr Credentials) Option {
	return func(c *config) { c.creds = func() (Credentials, error) { return cr, nil } }
}

// WithCredentialsSource signs with whatever fn returns, asked before each
// request: the hook for temporary or externally managed credentials.
func WithCredentialsSource(fn CredentialsSource) Option {
	return func(c *config) { c.creds = fn }
}

// FromEnv reads AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN
// and the region (AWS_REGION, else AWS_DEFAULT_REGION), once, in New. Each
// part is used only when set, and options after FromEnv override it. This
// and FromSharedCredentials are the only ways the plugin reads the
// environment.
func FromEnv() Option {
	return func(c *config) {
		if r, ok := os.LookupEnv(EnvRegion); ok && r != "" {
			c.region = r
		} else if r, ok := os.LookupEnv(EnvDefaultRegion); ok && r != "" {
			c.region = r
		}
		id, okID := os.LookupEnv(EnvAccessKeyID)
		secret, okSecret := os.LookupEnv(EnvSecretAccessKey)
		if okID || okSecret {
			cr := Credentials{AccessKeyID: id, SecretAccessKey: secret, SessionToken: os.Getenv(EnvSessionToken)}
			c.creds = func() (Credentials, error) { return cr, nil }
		}
	}
}

// FromSharedCredentials reads one profile of a shared credentials file,
// once, in New. file "" means ~/.aws/credentials; profile "" means
// "default". A missing file or profile fails New.
func FromSharedCredentials(file, profile string) Option {
	return func(c *config) {
		cr, err := readSharedCredentials(file, profile)
		if err != nil {
			c.err = err
			return
		}
		c.creds = func() (Credentials, error) { return cr, nil }
	}
}

// WithEndpoint sends every request to url instead of the regional AWS
// endpoints (LocalStack, a VPC endpoint, a test server). Requests are still
// signed for the configured region.
func WithEndpoint(u string) Option { return func(c *config) { c.endpoint = u } }

// WithHTTPClient sets the HTTP client; nil keeps the default.
func WithHTTPClient(h *http.Client) Option { return func(c *config) { c.client = h } }

// WithTimeout bounds each request; zero or less keeps DefaultTimeout.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithName sets the plugin name errors report (default "aws").
func WithName(name string) Option { return func(c *config) { c.name = name } }

// WithScheme sets the scheme the Resolver owns (default "aws"), so
// instances for two regions or accounts can coexist: ${aws:...},
// ${aws-eu:...}.
func WithScheme(scheme string) Option { return func(c *config) { c.scheme = strings.ToLower(scheme) } }

// WithLookup maps ${name} to a reference (same grammar as ${aws:ref}).
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
// prefix+NAME: "sm/prod/app#" reads key NAME of one JSON secret,
// "ssm//app/prod/" reads parameter /app/prod/NAME. Prefixes are tried in
// order; a reference that does not exist answers ok=false.
func WithLookupPrefix(prefix string) Option {
	return func(c *config) { c.prefixes = append(c.prefixes, prefix) }
}

// New builds a Plugin. It fails without a region or credentials, on a bad
// endpoint, a failed FromSharedCredentials, or a WithLookup reference that
// does not parse. It sends no request.
func New(opts ...Option) (*Plugin, error) {
	c := config{name: DefaultName, scheme: DefaultScheme, timeout: DefaultTimeout}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.err != nil {
		return nil, fmt.Errorf("aws: %w", c.err)
	}
	if c.region == "" || !validName(c.region) || strings.Contains(c.region, "/") {
		return nil, fmt.Errorf("aws: %w: no or malformed region (use WithRegion or FromEnv)", ErrInvalidRef)
	}
	if c.creds == nil {
		return nil, fmt.Errorf("aws: %w (use WithCredentials, WithCredentialsSource, FromEnv or FromSharedCredentials)", ErrNoCredentials)
	}
	if c.endpoint != "" {
		u, err := url.Parse(c.endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return nil, fmt.Errorf("aws: %w: endpoint must be an http or https URL without credentials", ErrInvalidRef)
		}
		c.endpoint = strings.TrimRight(u.String(), "/")
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
			return nil, fmt.Errorf("aws: lookup %s: %w", name, err)
		}
	}
	return &Plugin{
		name: c.name, scheme: c.scheme, region: c.region, creds: c.creds,
		endpoint: c.endpoint, client: c.client, timeout: c.timeout,
		lookups: c.lookups, prefixes: c.prefixes,
		now: time.Now, cache: map[string]string{},
	}, nil
}

// Name implements plugin.Plugin.
func (p *Plugin) Name() string { return p.name }

// Schemes implements plugin.Resolver.
func (p *Plugin) Schemes() []string { return []string{p.scheme} }

// Resolve implements plugin.Resolver. A scheme other than the plugin's
// wraps plugin.ErrUnsupportedScheme.
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

// Lookup implements plugin.Lookuper. An unmapped name, or a mapped
// reference that does not exist, answers ok=false; denied and unavailable
// are errors.
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
			continue
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

// Read returns the value r names, from the cache when this plugin has read
// it before. With a key, the value must be a JSON object; a key whose value
// is not a string is returned as its JSON text.
func (p *Plugin) Read(r Ref) (string, error) {
	raw, err := p.value(r)
	if err != nil {
		return "", err
	}
	if !r.HasKey {
		return raw, nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &obj) != nil || obj == nil {
		// Never repeat the parser's message: it can quote the secret.
		return "", fmt.Errorf("%s: %w: value is not a JSON object, so #%s cannot be read", r.cacheKey(), ErrInvalidRef, r.Key)
	}
	v, ok := obj[r.Key]
	if !ok {
		return "", fmt.Errorf("%s: key %q: %w (keys: %s)", r.cacheKey(), r.Key, plugin.ErrNotFound, keyNames(obj))
	}
	return fieldText(v)
}

// Purge empties the cache, so the next reference reads AWS again.
func (p *Plugin) Purge() {
	p.mu.Lock()
	p.cache = map[string]string{}
	p.mu.Unlock()
}

// value returns the raw value r names, reading it once.
func (p *Plugin) value(r Ref) (string, error) {
	key := r.cacheKey()
	p.mu.Lock()
	v, ok := p.cache[key]
	p.mu.Unlock()
	if ok {
		return v, nil
	}
	v, err := p.fetch(r)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	p.cache[key] = v
	p.mu.Unlock()
	return v, nil
}

// Request and response shapes of the two operations.
type (
	smRequest struct {
		SecretID     string `json:"SecretId"`
		VersionStage string `json:"VersionStage,omitempty"`
		VersionID    string `json:"VersionId,omitempty"`
	}
	smResponse struct {
		SecretString *string `json:"SecretString"`
		SecretBinary *string `json:"SecretBinary"`
	}
	ssmRequest struct {
		Name           string `json:"Name"`
		WithDecryption bool   `json:"WithDecryption"`
	}
	ssmResponse struct {
		Parameter *struct {
			Value *string `json:"Value"`
		} `json:"Parameter"`
	}
)

// fetch performs one signed call and maps the outcome onto plugin errors.
func (p *Plugin) fetch(r Ref) (string, error) {
	op := "read " + r.cacheKey()
	var (
		service, target string
		payload         any
	)
	switch r.Service {
	case SecretsManager:
		service, target = smSigningName, smTarget
		payload = smRequest{SecretID: r.ID, VersionStage: r.VersionStage, VersionID: r.VersionID}
	case ParameterStore:
		service, target = ssmSigningName, ssmTarget
		payload = ssmRequest{Name: r.ID, WithDecryption: true}
	default:
		return "", fmt.Errorf("%s: %w: unknown service", op, ErrInvalidRef)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	cr, err := p.creds()
	if err != nil {
		return "", fmt.Errorf("%s: credentials: %w", op, err)
	}
	if !cr.valid() {
		return "", fmt.Errorf("%s: %w: credentials source returned an empty key", op, ErrNoCredentials)
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpointFor(service)+"/", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	req.Header.Set("Content-Type", jsonContent)
	req.Header.Set(headerTarget, target)
	signV4(req, body, cr, p.region, service, p.now(), signOptions{normalize: true})

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: %w: %v", op, plugin.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("%s: %w: reading response: %v", op, plugin.ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", apiError(op, resp.StatusCode, resp.Header.Get(headerErrType), data)
	}
	return decodeValue(op, r.Service, data)
}

// endpointFor is the base URL for a service: the override, or the
// regional endpoint (the .com.cn domain for China regions).
func (p *Plugin) endpointFor(service string) string {
	if p.endpoint != "" {
		return p.endpoint
	}
	domain := "amazonaws.com"
	if strings.HasPrefix(p.region, "cn-") {
		domain = "amazonaws.com.cn"
	}
	return "https://" + service + "." + p.region + "." + domain
}

// decodeValue extracts the value from a 200 response. Decode failures never
// repeat the parser's message.
func decodeValue(op string, svc Service, data []byte) (string, error) {
	switch svc {
	case SecretsManager:
		var out smResponse
		if json.Unmarshal(data, &out) != nil {
			return "", fmt.Errorf("%s: response is not a GetSecretValue result (invalid JSON)", op)
		}
		if out.SecretString != nil {
			return *out.SecretString, nil
		}
		if out.SecretBinary != nil {
			b, err := base64.StdEncoding.DecodeString(*out.SecretBinary)
			if err != nil {
				return "", fmt.Errorf("%s: SecretBinary is not valid base64", op)
			}
			return string(b), nil
		}
		return "", fmt.Errorf("%s: %w: secret has no value", op, plugin.ErrNotFound)
	default:
		var out ssmResponse
		if json.Unmarshal(data, &out) != nil {
			return "", fmt.Errorf("%s: response is not a GetParameter result (invalid JSON)", op)
		}
		if out.Parameter == nil || out.Parameter.Value == nil {
			return "", fmt.Errorf("%s: %w: parameter has no value", op, plugin.ErrNotFound)
		}
		return *out.Parameter.Value, nil
	}
}

// Error codes by meaning. The code is the "__type" of the error body (after
// any "namespace#") or the X-Amzn-ErrorType header (before any ":").
var (
	notFoundCodes = map[string]bool{
		"ResourceNotFoundException": true, // Secrets Manager
		"ParameterNotFound":         true, // SSM
		"ParameterVersionNotFound":  true,
	}
	deniedCodes = map[string]bool{
		"AccessDeniedException":         true,
		"AccessDenied":                  true,
		"UnrecognizedClientException":   true,
		"InvalidClientTokenId":          true,
		"InvalidSignatureException":     true,
		"SignatureDoesNotMatch":         true,
		"IncompleteSignature":           true,
		"MissingAuthenticationToken":    true,
		"ExpiredTokenException":         true,
		"ExpiredToken":                  true,
		"DecryptionFailure":             true, // Secrets Manager: KMS refused
		"InvalidKeyId":                  true, // SSM: KMS key disabled or unusable
		"KMSAccessDeniedException":      true,
		"NotAuthorized":                 true,
		"RequestExpired":                true,
		"InvalidClientTokenIdException": true,
	}
	unavailableCodes = map[string]bool{
		"InternalServiceError":        true, // Secrets Manager
		"InternalServerError":         true, // SSM
		"InternalFailure":             true,
		"ServiceUnavailable":          true,
		"ServiceUnavailableException": true,
		"ThrottlingException":         true,
		"Throttling":                  true,
		"TooManyRequestsException":    true,
		"RequestLimitExceeded":        true,
		"RequestTimeout":              true,
		"RequestTimeoutException":     true,
	}
)

// apiError maps a non-200 response onto the plugin's named errors: by code
// first, by status when the code is unknown. AWS's message describes the
// request (and may name an ARN), never a secret value; it is repeated,
// truncated.
func apiError(op string, status int, headerType string, body []byte) error {
	var e struct {
		Type     string `json:"__type"`
		Message  string `json:"message"`
		Message2 string `json:"Message"`
	}
	_ = json.Unmarshal(body, &e)
	code := e.Type
	if i := strings.LastIndexByte(code, '#'); i >= 0 {
		code = code[i+1:]
	}
	if code == "" && headerType != "" {
		code, _, _ = strings.Cut(headerType, ":")
	}
	msg := e.Message
	if msg == "" {
		msg = e.Message2
	}
	if len(msg) > maxDetail {
		msg = msg[:maxDetail] + "..."
	}

	var kind error
	switch {
	case notFoundCodes[code]:
		kind = plugin.ErrNotFound
	case deniedCodes[code]:
		kind = plugin.ErrDenied
	case unavailableCodes[code]:
		kind = plugin.ErrUnavailable
	case code != "" && status < 500 && status != http.StatusTooManyRequests && status != http.StatusForbidden && status != http.StatusUnauthorized:
		// A known-shape client error (InvalidRequestException, ...) that is
		// none of the above: a plain error.
	case status == http.StatusForbidden || status == http.StatusUnauthorized:
		kind = plugin.ErrDenied
	case status == http.StatusTooManyRequests || status >= 500:
		kind = plugin.ErrUnavailable
	}

	detail := "status " + strconv.Itoa(status)
	if code != "" {
		detail += " " + code
	}
	if msg != "" {
		detail += ": " + msg
	}
	if kind == nil {
		return fmt.Errorf("%s: %s", op, detail)
	}
	return fmt.Errorf("%s: %w: %s", op, kind, detail)
}

// fieldText renders a JSON value: a string as its contents, anything else as
// its compact JSON text.
func fieldText(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return "", errors.New("key holds invalid JSON")
	}
	return b.String(), nil
}

// keyNames lists an object's keys, sorted. Names, never values.
func keyNames(obj map[string]json.RawMessage) string {
	names := make([]string, 0, len(obj))
	for k := range obj {
		names = append(names, strconv.Quote(k))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Compile-time proof of the capabilities the plugin claims.
var (
	_ plugin.Resolver = (*Plugin)(nil)
	_ plugin.Lookuper = (*Plugin)(nil)
)
