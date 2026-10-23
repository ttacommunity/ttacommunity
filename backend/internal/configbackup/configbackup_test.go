package configbackup_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ttacommunity/backend/internal/configbackup"
)

func TestClusterFromRefusesAMissingSetting(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		env  map[string]string
		dir  string
	}{
		{"no API server address", map[string]string{"KUBERNETES_SERVICE_PORT": "443"}, dir},
		{"no API server port", map[string]string{"KUBERNETES_SERVICE_HOST": "10.43.0.1"}, dir},
		{"no service account files", map[string]string{"KUBERNETES_SERVICE_HOST": "10.43.0.1", "KUBERNETES_SERVICE_PORT": "443"}, dir + "/missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := configbackup.ClusterFrom(func(k string) string { return tt.env[k] }, tt.dir); err == nil {
				t.Error("ClusterFrom: no error, want a refusal")
			}
		})
	}
}

func TestSecretsExportsAnApplicableList(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		fmt.Fprint(w, `{"kind":"SecretList","apiVersion":"v1","metadata":{"resourceVersion":"1"},"items":[
			{"metadata":{"name":"zeta","namespace":"ttacommunity","uid":"u1","resourceVersion":"5","managedFields":[{}]},"type":"Opaque","data":{"password":"cGFzcw=="}},
			{"metadata":{"name":"alpha","namespace":"ttacommunity","uid":"u2"},"type":"kubernetes.io/dockerconfigjson"}]}`)
	}))
	defer srv.Close()
	got, err := configbackup.Secrets(context.Background(), configbackup.Cluster{API: srv.URL, Namespace: "ttacommunity", Token: "tok", Client: srv.Client()})
	if err != nil {
		t.Fatalf("Secrets: %v", err)
	}
	if gotPath != "/api/v1/namespaces/ttacommunity/secrets" || gotAuth != "Bearer tok" {
		t.Errorf("request = %s with %q, want the namespace's secrets with the bearer token", gotPath, gotAuth)
	}
	want := `{"apiVersion":"v1","items":[
		{"apiVersion":"v1","data":{},"kind":"Secret","metadata":{"name":"alpha","namespace":"ttacommunity"},"type":"kubernetes.io/dockerconfigjson"},
		{"apiVersion":"v1","data":{"password":"cGFzcw=="},"kind":"Secret","metadata":{"name":"zeta","namespace":"ttacommunity"},"type":"Opaque"}],
		"kind":"List"}`
	if !sameJSON(t, got, want) {
		t.Errorf("Secrets =\n%s\nwant\n%s", got, want)
	}
}

func TestSecretsRefusesABadAnswer(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"forbidden", http.StatusForbidden, `{"kind":"Status","code":403}`},
		{"not JSON", http.StatusOK, `<html>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()
			if _, err := configbackup.Secrets(context.Background(), configbackup.Cluster{API: srv.URL, Namespace: "ttacommunity", Token: "tok", Client: srv.Client()}); err == nil {
				t.Error("Secrets: no error, want a refusal")
			}
		})
	}
}

// cloudflare is a fake Cloudflare API: an account with one zone, a two-page DNS list, a two-page (cursor) ruleset
// list, two Workers, and one R2 bucket. It records which paths were asked for.
func cloudflare(t *testing.T, override func(w http.ResponseWriter, r *http.Request) bool) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	ok := func(w http.ResponseWriter, result string, info string) {
		if info == "" {
			fmt.Fprintf(w, `{"success":true,"errors":[],"messages":[],"result":%s}`, result)
			return
		}
		fmt.Fprintf(w, `{"success":true,"errors":[],"messages":[],"result":%s,"result_info":%s}`, result, info)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer cf-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			ok(w, "null", "")
			return
		}
		if override != nil && override(w, r) {
			return
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/zones":
			if q.Get("name") != "example.org" {
				ok(w, "[]", `{"page":1,"per_page":50,"total_pages":1,"count":0,"total_count":0}`)
				return
			}
			ok(w, `[{"id":"zone1","name":"example.org","account":{"id":"acct1","name":"Example"}}]`, `{"page":1,"per_page":50,"total_pages":1,"count":1,"total_count":1}`)
		case "/zones/zone1/dns_records":
			if q.Get("page") == "2" {
				ok(w, `[{"id":"dns3"}]`, `{"page":2,"per_page":2,"total_pages":2,"count":1,"total_count":3}`)
				return
			}
			ok(w, `[{"id":"dns1"},{"id":"dns2"}]`, `{"page":1,"per_page":2,"total_pages":2,"count":2,"total_count":3}`)
		case "/zones/zone1/rulesets":
			if q.Get("cursor") == "more" {
				ok(w, `[{"id":"rs2","kind":"zone"}]`, `{"cursor":""}`)
				return
			}
			ok(w, `[{"id":"rs1","kind":"zone"},{"id":"managed1","kind":"managed"}]`, `{"cursor":"more"}`)
		case "/zones/zone1/rulesets/rs1", "/zones/zone1/rulesets/rs2":
			ok(w, fmt.Sprintf(`{"id":%q,"rules":[{"expression":"true"}]}`, strings.TrimPrefix(r.URL.Path, "/zones/zone1/rulesets/")), "")
		case "/accounts/acct1/workers/scripts":
			ok(w, `[{"id":"site","tag":"tag-site"},{"id":"byhand","tag":"tag-byhand"}]`, "")
		case "/accounts/acct1/r2/buckets":
			if r.Header.Get("cf-r2-jurisdiction") != "eu" {
				ok(w, `{"buckets":[]}`, "")
				return
			}
			ok(w, `{"buckets":[{"name":"bucket-a","jurisdiction":"eu"}]}`, `{"cursor":"","per_page":50}`)
		case "/accounts/acct1/r2/buckets/bucket-a/lifecycle":
			ok(w, `{"rules":[{"id":"delete-after-90-days"}]}`, "")
		case "/accounts/acct1/r2/buckets/bucket-a/lock":
			ok(w, `{"rules":[{"id":"keep-29-days"}]}`, "")
		case "/zones/zone1/email/routing", "/zones/zone1/email/routing/rules/catch_all", "/accounts/acct1/access/organizations":
			ok(w, `{"enabled":true}`, "")
		default:
			ok(w, "[]", "")
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func TestCloudflareExportsEverySettingWhole(t *testing.T) {
	srv, asked := cloudflare(t, nil)
	got, err := configbackup.Cloudflare(context.Background(), configbackup.Account{API: srv.URL, Token: "cf-tok", Zone: "example.org", Client: srv.Client()})
	if err != nil {
		t.Fatalf("Cloudflare: %v", err)
	}
	var export map[string]json.RawMessage
	if err := json.Unmarshal(got, &export); err != nil {
		t.Fatalf("Cloudflare: not JSON: %v", err)
	}
	for _, key := range []string{"zone", "organization", "access_applications", "access_policies", "access_groups", "access_service_tokens", "identity_providers", "dns_records", "turnstile_widgets", "email_routing", "rulesets", "workers", "r2_buckets"} {
		if _, ok := export[key]; !ok {
			t.Errorf("export has no %q", key)
		}
	}
	if _, ok := export["builds"]; ok {
		t.Error("export has builds, which the Builds API refuses an account token for")
	}
	checks := []struct {
		name, path, want string
	}{
		{"both DNS pages", "dns_records", `[{"id":"dns1"},{"id":"dns2"},{"id":"dns3"}]`},
		{"both ruleset pages, zone rulesets only, each fetched whole", "rulesets", `{"rs1":{"id":"rs1","rules":[{"expression":"true"}]},"rs2":{"id":"rs2","rules":[{"expression":"true"}]}}`},
		{"the Workers", "workers", `[{"id":"site","tag":"tag-site"},{"id":"byhand","tag":"tag-byhand"}]`},
		{"each EU bucket's lifecycle and locks", "r2_buckets", `{"bucket-a":{"lifecycle":{"rules":[{"id":"delete-after-90-days"}]},"locks":{"rules":[{"id":"keep-29-days"}]}}}`},
		{"the email routing parts", "email_routing", `{"addresses":[],"catch_all":{"enabled":true},"rules":[],"settings":{"enabled":true}}`},
		{"the zone's name", "zone", `"example.org"`},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !sameJSON(t, export[c.path], c.want) {
				t.Errorf("export.%s =\n%s\nwant\n%s", c.path, export[c.path], c.want)
			}
		})
	}
	if n := strings.Count(strings.Join(*asked, "\n"), "/zones/zone1/dns_records"); n != 2 {
		t.Errorf("dns_records asked %d times, want 2 (one per page)", n)
	}
}

func TestCloudflareRefusesABadAnswer(t *testing.T) {
	tests := []struct {
		name     string
		zone     string
		override func(w http.ResponseWriter, r *http.Request) bool
	}{
		{"an unknown zone", "other.org", nil},
		{"a refused token", "example.org", func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/zones/zone1/dns_records" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"messages":[],"result":null}`)
				return true
			}
			return false
		}},
		{"success false with a 200", "example.org", func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/accounts/acct1/challenges/widgets" {
				fmt.Fprint(w, `{"success":false,"errors":[{"code":7003,"message":"Could not route to /widgets"}],"messages":[],"result":null}`)
				return true
			}
			return false
		}},
		{"an answer that is not JSON", "example.org", func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/accounts/acct1/access/policies" {
				fmt.Fprint(w, `<html>Envelope Error</html>`)
				return true
			}
			return false
		}},
		{"a missing lifecycle", "example.org", func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/accounts/acct1/r2/buckets/bucket-a/lifecycle" {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"success":false,"errors":[{"code":10006,"message":"bucket not found"}],"messages":[],"result":null}`)
				return true
			}
			return false
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := cloudflare(t, tt.override)
			if _, err := configbackup.Cloudflare(context.Background(), configbackup.Account{API: srv.URL, Token: "cf-tok", Zone: tt.zone, Client: srv.Client()}); err == nil {
				t.Error("Cloudflare: no error, want a refusal")
			}
		})
	}
}

// sameJSON reports whether two JSON documents hold the same value.
func sameJSON(t *testing.T, got []byte, want string) bool {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	return reflect.DeepEqual(g, w)
}
