package aws

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// AWS Signature Version 4, header form, written against the published AWS
// test suite (testdata/sigv4). Only what a JSON-protocol POST needs is used
// in production, but the signer handles the whole suite so it stays honest.

// SigV4 constants.
const (
	sigAlgorithm     = "AWS4-HMAC-SHA256"
	sigTerminator    = "aws4_request"
	sigTimeFormat    = "20060102T150405Z"
	sigDateFormat    = "20060102"
	headerAmzDate    = "X-Amz-Date"
	headerAmzToken   = "X-Amz-Security-Token"
	headerAmzContent = "X-Amz-Content-Sha256"
	headerAuth       = "Authorization"
)

// signOptions are the suite's context switches.
type signOptions struct {
	// normalize removes "." and ".." segments and collapses repeated
	// slashes in the canonical path: every service except S3 wants it.
	normalize bool
	// signBody adds the X-Amz-Content-Sha256 header (S3 style).
	signBody bool
	// omitSessionToken adds X-Amz-Security-Token after signing, unsigned
	// (the suite's post-sts-header-after; some services want it so).
	omitSessionToken bool
}

// signResult exposes the intermediate strings so the test suite can compare
// each stage, not only the final signature.
type signResult struct {
	canonicalRequest string
	stringToSign     string
	signature        string
}

// signV4 signs req in place: it sets X-Amz-Date, X-Amz-Security-Token (when
// the credentials carry one), optionally X-Amz-Content-Sha256, and
// Authorization. body is the exact request body. Every header present on
// req when it is called is signed, plus host; set headers first, sign last.
func signV4(req *http.Request, body []byte, c Credentials, region, service string, t time.Time, o signOptions) signResult {
	t = t.UTC()
	amzDate := t.Format(sigTimeFormat)
	date := t.Format(sigDateFormat)

	req.Header.Del(headerAuth)
	req.Header.Set(headerAmzDate, amzDate)
	if c.SessionToken != "" && !o.omitSessionToken {
		req.Header.Set(headerAmzToken, c.SessionToken)
	}
	payloadHash := hexSHA256(body)
	if o.signBody {
		req.Header.Set(headerAmzContent, payloadHash)
	}

	canonHeaders, signedHeaders := canonicalHeaders(req)
	canonical := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL, o.normalize),
		canonicalQuery(req.URL.RawQuery),
		canonHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := date + "/" + region + "/" + service + "/" + sigTerminator
	toSign := sigAlgorithm + "\n" + amzDate + "\n" + scope + "\n" + hexSHA256([]byte(canonical))

	key := hmacSHA256([]byte("AWS4"+c.SecretAccessKey), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, sigTerminator)
	sig := hex.EncodeToString(hmacSHA256(key, toSign))

	req.Header.Set(headerAuth, sigAlgorithm+" Credential="+c.AccessKeyID+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+sig)
	if c.SessionToken != "" && o.omitSessionToken {
		req.Header.Set(headerAmzToken, c.SessionToken)
	}
	return signResult{canonicalRequest: canonical, stringToSign: toSign, signature: sig}
}

// canonicalURI encodes the decoded path once per segment (the suite's
// convention for every service but S3), after normalizing when asked.
func canonicalURI(u *url.URL, normalize bool) string {
	p := u.Path
	if p == "" {
		p = "/"
	}
	if normalize {
		trailing := strings.HasSuffix(p, "/")
		p = path.Clean("/" + p)
		if trailing && p != "/" {
			p += "/"
		}
	}
	return uriEncode(p, false)
}

// canonicalQuery sorts the decoded parameters by name then value and
// re-encodes them with SigV4's encoding.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	type kv struct{ k, v string }
	var params []kv
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		params = append(params, kv{uriEncode(unescapeQuery(k), true), uriEncode(unescapeQuery(v), true)})
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].k != params[j].k {
			return params[i].k < params[j].k
		}
		return params[i].v < params[j].v
	})
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

// unescapeQuery decodes %XX; text that is not valid percent-encoding is used
// as written.
func unescapeQuery(s string) string {
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	return s
}

// canonicalHeaders renders "name:value\n" lines (lower-case names, sorted;
// values trimmed with inner whitespace runs collapsed; repeated headers
// joined with ",") and the ";"-joined signed header list. host is always
// signed.
func canonicalHeaders(req *http.Request) (canon, signed string) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	vals := map[string][]string{"host": {host}}
	for name, vs := range req.Header {
		ln := strings.ToLower(name)
		if ln == "authorization" {
			continue
		}
		vals[ln] = append(vals[ln], vs...)
	}
	names := make([]string, 0, len(vals))
	for n := range vals {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		vs := make([]string, len(vals[n]))
		for i, v := range vals[n] {
			vs[i] = strings.Join(strings.Fields(v), " ")
		}
		b.WriteString(n)
		b.WriteByte(':')
		b.WriteString(strings.Join(vs, ","))
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

// uriEncode is SigV4's encoding: unreserved characters (A-Z a-z 0-9 - . _ ~)
// as is, everything else %XX in upper case; "/" kept unless encodeSlash.
func uriEncode(s string, encodeSlash bool) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
		}
	}
	return b.String()
}

func hexSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}
