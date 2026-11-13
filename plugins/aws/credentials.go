package aws

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credentials sign requests. String and GoString never print the secret
// key or the session token, so a credentials value that ends up in a log or
// a %v is harmless.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	// SessionToken is set for temporary credentials (STS, SSO, IAM roles).
	SessionToken string
}

// String shows the access key id only.
func (c Credentials) String() string {
	tok := "none"
	if c.SessionToken != "" {
		tok = "<hidden>"
	}
	return fmt.Sprintf("Credentials{AccessKeyID: %q, SecretAccessKey: <hidden>, SessionToken: %s}", c.AccessKeyID, tok)
}

// GoString is String, so %#v hides the secret too.
func (c Credentials) GoString() string { return c.String() }

// valid reports whether c can sign a request.
func (c Credentials) valid() bool { return c.AccessKeyID != "" && c.SecretAccessKey != "" }

// CredentialsSource returns the credentials to sign the next request with.
// It is called once per request to AWS (not per reference: the cache absorbs
// repeats), so a source may rotate temporary credentials; it should cache
// them itself when fetching them is expensive. This is the seam for any
// credential kind the plugin does not read itself (web identity, SSO, IMDS).
type CredentialsSource func() (Credentials, error)

// ErrNoCredentials is returned by New when no credentials were configured,
// and wrapped when a CredentialsSource returns incomplete ones.
var ErrNoCredentials = errors.New("no AWS credentials")

// Environment variables FromEnv reads, the ones every AWS SDK reads.
const (
	EnvAccessKeyID     = "AWS_ACCESS_KEY_ID"
	EnvSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	EnvSessionToken    = "AWS_SESSION_TOKEN"
	EnvRegion          = "AWS_REGION"
	EnvDefaultRegion   = "AWS_DEFAULT_REGION"
)

// Shared credentials file keys and defaults.
const (
	defaultProfile     = "default"
	iniAccessKeyID     = "aws_access_key_id"
	iniSecretAccessKey = "aws_secret_access_key"
	iniSessionToken    = "aws_session_token"
)

// readSharedCredentials reads one profile of an AWS shared credentials file
// (~/.aws/credentials format: INI sections, "key = value", "#" and ";"
// comments). Errors name the file and profile, never a value.
func readSharedCredentials(file, profile string) (Credentials, error) {
	if profile == "" {
		profile = defaultProfile
	}
	if file == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Credentials{}, fmt.Errorf("shared credentials: %w", err)
		}
		file = filepath.Join(home, ".aws", "credentials")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return Credentials{}, fmt.Errorf("shared credentials: %w", err)
	}
	var c Credentials
	found := false
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == profile {
				found = true
			}
			continue
		}
		if section != profile {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case iniAccessKeyID:
			c.AccessKeyID = v
		case iniSecretAccessKey:
			c.SecretAccessKey = v
		case iniSessionToken:
			c.SessionToken = v
		}
	}
	if err := sc.Err(); err != nil {
		return Credentials{}, fmt.Errorf("shared credentials %s: %w", file, err)
	}
	if !found {
		return Credentials{}, fmt.Errorf("shared credentials %s: %w: profile %q not found", file, ErrNoCredentials, profile)
	}
	if !c.valid() {
		return Credentials{}, fmt.Errorf("shared credentials %s: %w: profile %q lacks %s or %s", file, ErrNoCredentials, profile, iniAccessKeyID, iniSecretAccessKey)
	}
	return c, nil
}
