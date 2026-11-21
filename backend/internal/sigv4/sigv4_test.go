package sigv4_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/sigv4"
)

// The cases, credentials and date are AWS's SigV4 test suite, as botocore keeps it (tests/unit/auth/aws4_testsuite).
func TestSignMatchesAWSTestSuite(t *testing.T) {
	creds := sigv4.Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"}
	at := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	tests := []struct {
		name, method, url, contentType, body, want string
	}{
		{"get-vanilla", "GET", "https://example.amazonaws.com/", "", "",
			"AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"},
		{"get-vanilla-query-order-key-case", "GET", "https://example.amazonaws.com/?Param2=value2&Param1=value1", "", "",
			"AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=b97d918cfa904a5beff61c982a1b6f458b799221646efd99d3219ec94cdf2500"},
		{"post-x-www-form-urlencoded", "POST", "https://example.amazonaws.com/", "application/x-www-form-urlencoded", "Param1=value1",
			"AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=ff11897932ad3f4e8b18135d722051e5ac45fc38421b1da7b9d196a0fe09473a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			sigv4.Sign(req, creds, "us-east-1", "service", sigv4.PayloadHash([]byte(tt.body)), at)
			if got := req.Header.Get("Authorization"); got != tt.want {
				t.Errorf("Authorization = %q, want %q", got, tt.want)
			}
			if got := req.Header.Get("X-Amz-Date"); got != "20150830T123600Z" {
				t.Errorf("X-Amz-Date = %q, want %q", got, "20150830T123600Z")
			}
		})
	}
}

func TestCredentialsStringHidesTheSecret(t *testing.T) {
	c := sigv4.Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "a-secret-value"}
	for _, s := range []string{c.String(), sigv4.Credentials.String(c)} {
		if strings.Contains(s, "a-secret-value") {
			t.Errorf("String() = %q, shows the secret", s)
		}
	}
}
