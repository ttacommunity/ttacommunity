// Package sigv4 signs HTTP requests with AWS Signature Version 4, as S3-compatible stores such as R2 need.
// It follows https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-create-signed-request.html.
package sigv4

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Credentials is an access key pair.
type Credentials struct {
	AccessKeyID, SecretAccessKey string
}

// String keeps the secret out of logs.
func (c Credentials) String() string {
	return fmt.Sprintf("sigv4.Credentials{AccessKeyID: %s}", c.AccessKeyID)
}

// PayloadHash returns the hex SHA-256 of body, as Sign wants it.
func PayloadHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Sign sets X-Amz-Date to at and adds the Authorization header for c in region and service. It signs the host,
// Content-Type and every X-Amz-* header; payloadHash is PayloadHash of the body. Paths are encoded once, as S3 wants.
func Sign(req *http.Request, c Credentials, region, service, payloadHash string, at time.Time) {
	stamp := at.UTC().Format("20060102T150405Z")
	day := stamp[:8]
	req.Header.Set("X-Amz-Date", stamp)

	headers, signed := canonicalHeaders(req)
	canonical := strings.Join([]string{
		req.Method, canonicalPath(req.URL.EscapedPath()), canonicalQuery(req), headers, signed, payloadHash,
	}, "\n")
	scope := day + "/" + region + "/" + service + "/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + PayloadHash([]byte(canonical))

	key := mac([]byte("AWS4"+c.SecretAccessKey), day)
	for _, part := range []string{region, service, "aws4_request"} {
		key = mac(key, part)
	}
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.AccessKeyID, scope, signed, hex.EncodeToString(mac(key, toSign))))
}

func mac(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// canonicalHeaders returns the canonical header block and the signed header names.
func canonicalHeaders(req *http.Request) (string, string) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	values := map[string]string{"host": host}
	for name, vs := range req.Header {
		lower := strings.ToLower(name)
		if lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			trimmed := make([]string, len(vs))
			for i, v := range vs {
				trimmed[i] = strings.Join(strings.Fields(v), " ")
			}
			values[lower] = strings.Join(trimmed, ",")
		}
	}
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n + ":" + values[n] + "\n")
	}
	return b.String(), strings.Join(names, ";")
}

// canonicalPath re-encodes an escaped path with AWS's rules, segment by segment.
func canonicalPath(escaped string) string {
	if escaped == "" {
		return "/"
	}
	segments := strings.Split(escaped, "/")
	for i, s := range segments {
		segments[i] = encode(unescape(s))
	}
	return strings.Join(segments, "/")
}

// canonicalQuery returns the query sorted by name, then value, each encoded with AWS's rules.
func canonicalQuery(req *http.Request) string {
	q := req.URL.Query()
	pairs := make([]string, 0, len(q))
	for name, vs := range q {
		for _, v := range vs {
			pairs = append(pairs, encode(name)+"="+encode(v))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// encode percent-encodes all but AWS's unreserved characters (A-Z a-z 0-9 - . _ ~), in upper-case hex.
func encode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// unescape undoes percent-encoding, keeping a malformed segment as it is.
func unescape(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}
