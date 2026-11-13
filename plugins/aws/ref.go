package aws

import (
	"fmt"
	"strings"
)

// Service names the AWS store a reference reads.
type Service string

// The services a reference may name.
const (
	// SecretsManager is AWS Secrets Manager (GetSecretValue).
	SecretsManager Service = "sm"
	// ParameterStore is SSM Parameter Store (GetParameter, decrypted).
	ParameterStore Service = "ssm"
)

// Query parameters a Secrets Manager reference may carry.
const (
	queryStage   = "stage"
	queryVersion = "version"
)

// Ref is a parsed aws reference: the text after "aws:" in
// ${aws:sm/prod/db#password}.
//
// Grammar:
//
//	ref      = "sm/" secret-id [ "?" sm-query ] [ "#" key ]
//	         | "ssm/" param-name [ "#" key ]
//	sm-query = "stage=" label | "version=" version-id
//
// secret-id is a Secrets Manager secret name or full ARN
// (sm/arn:aws:secretsmanager:us-east-1:111122223333:secret:prod/db-AbCdEf).
// param-name is a Parameter Store name exactly as AWS spells it, so a
// hierarchical name keeps its leading slash: ssm//app/prod/db_url reads
// "/app/prod/db_url", ssm/plain reads "plain". A param-name may end in
// ":version" or ":label" (ssm//app/db:3), which AWS itself interprets.
//
// key reads one key of a secret whose value is a JSON object; it may contain
// any byte. Without it the whole value is returned. Neither the id nor the
// query may contain "#"; neither id may contain "?", whitespace or control
// bytes, which AWS names never do.
type Ref struct {
	Service Service
	// ID is the secret id or parameter name.
	ID string
	// VersionStage (stage=) or VersionID (version=) select a Secrets
	// Manager version; at most one is set.
	VersionStage string
	VersionID    string
	// Key is the JSON key to read; HasKey records that "#" was written.
	Key    string
	HasKey bool
}

// ParseRef parses a reference. The error wraps ErrInvalidRef and repeats
// only the reference, which is not a secret.
func ParseRef(s string) (Ref, error) {
	var r Ref
	rest := s
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		r.Key, r.HasKey = rest[i+1:], true
		rest = rest[:i]
		if r.Key == "" {
			return Ref{}, fmt.Errorf("%w %q: empty key after '#'", ErrInvalidRef, s)
		}
	}
	svc, id, ok := strings.Cut(rest, "/")
	if !ok {
		return Ref{}, fmt.Errorf("%w %q: want sm/<secret-id> or ssm/<parameter-name>", ErrInvalidRef, s)
	}
	r.Service = Service(svc)
	switch r.Service {
	case SecretsManager:
		if i := strings.IndexByte(id, '?'); i >= 0 {
			q := id[i+1:]
			id = id[:i]
			name, val, _ := strings.Cut(q, "=")
			if val == "" || !validName(val) {
				return Ref{}, fmt.Errorf("%w %q: want ?stage=<label> or ?version=<id>", ErrInvalidRef, s)
			}
			switch name {
			case queryStage:
				r.VersionStage = val
			case queryVersion:
				r.VersionID = val
			default:
				return Ref{}, fmt.Errorf("%w %q: want ?stage=<label> or ?version=<id>", ErrInvalidRef, s)
			}
		}
	case ParameterStore:
	default:
		return Ref{}, fmt.Errorf("%w %q: unknown service %q (want sm or ssm)", ErrInvalidRef, s, svc)
	}
	if id == "" || !validName(id) {
		return Ref{}, fmt.Errorf("%w %q: empty or malformed name", ErrInvalidRef, s)
	}
	r.ID = id
	return r, nil
}

// validName refuses what no AWS secret or parameter name contains, and the
// delimiters of this grammar.
func validName(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c == 0x7f || c == '?' || c == '#' {
			return false
		}
	}
	return true
}

// String renders the reference in canonical form; ParseRef(r.String())
// returns r for every r ParseRef produced.
func (r Ref) String() string {
	var b strings.Builder
	b.WriteString(string(r.Service))
	b.WriteByte('/')
	b.WriteString(r.ID)
	switch {
	case r.VersionStage != "":
		b.WriteString("?" + queryStage + "=" + r.VersionStage)
	case r.VersionID != "":
		b.WriteString("?" + queryVersion + "=" + r.VersionID)
	}
	if r.HasKey {
		b.WriteByte('#')
		b.WriteString(r.Key)
	}
	return b.String()
}

// cacheKey identifies one value read; the key is not part of it, so several
// keys of one secret cost one request.
func (r Ref) cacheKey() string {
	k := r
	k.Key, k.HasKey = "", false
	return k.String()
}
