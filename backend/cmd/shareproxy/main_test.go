package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// backend is a local server that answers with its name and records what reached it.
type backend struct {
	name   string
	got    []string
	origin string
	srv    *httptest.Server
}

func newBackend(t *testing.T, name string, answer func(w http.ResponseWriter, r *http.Request)) *backend {
	t.Helper()
	b := &backend{name: name}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.got = append(b.got, r.URL.Path)
		b.origin = r.Header.Get("Origin")
		if answer != nil {
			answer(w, r)
			return
		}
		_, _ = w.Write([]byte(name))
	}))
	t.Cleanup(b.srv.Close)
	return b
}

const public = "https://words.trycloudflare.com"

func TestEachPathGoesWhereItBelongs(t *testing.T) {
	tests := []struct {
		name, path string
		want       int
		reached    string // "api", "site" or "" for neither
	}{
		{"a page", "/blitz/", http.StatusOK, "site"},
		{"the home page", "/", http.StatusOK, "site"},
		{"an api route", "/v1/blitz", http.StatusOK, "api"},
		{"a sign-in", "/v1/auth/discord", http.StatusOK, "api"},
		{"an admin route", "/v1/admin/me", http.StatusForbidden, ""},
		{"an admin route with a trailing path", "/v1/admin/blitz/us_open/2026", http.StatusForbidden, ""},
		{"a path only starting like the api", "/v1x", http.StatusOK, "site"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, site := newBackend(t, "api", nil), newBackend(t, "site", nil)
			h, err := newProxy(public, api.srv.URL, site.srv.URL)
			if err != nil {
				t.Fatalf("newProxy: %v", err)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.want)
			}
			reached := ""
			for _, b := range []*backend{api, site} {
				if len(b.got) > 0 {
					reached += b.name
				}
			}
			if reached != tt.reached {
				t.Errorf("GET %s reached %q, want %q", tt.path, reached, tt.reached)
			}
		})
	}
}

func TestTheAPISeesTheLocalSiteAsTheOrigin(t *testing.T) {
	tests := []struct {
		name, origin, want string
	}{
		{"a request from the tunnel's page", public, "local site"},
		{"a request with no Origin", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, site := newBackend(t, "api", nil), newBackend(t, "site", nil)
			h, err := newProxy(public, api.srv.URL, site.srv.URL)
			if err != nil {
				t.Fatalf("newProxy: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/me/blitz/us_open/2026/matches/1/best-of", strings.NewReader(`{}`))
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			want := tt.want
			if want == "local site" {
				want = site.srv.URL
			}
			if api.origin != want {
				t.Errorf("the api saw Origin %q, want %q", api.origin, want)
			}
		})
	}
}

func TestARedirectToALocalAddressComesBackToTheTunnel(t *testing.T) {
	var api, site *backend
	redirect := func(to func() string) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, to(), http.StatusFound) }
	}
	tests := []struct {
		name string
		to   func() string
		want string
	}{
		{"to the site", func() string { return site.srv.URL + "/account/?signed_in=1" }, public + "/account/?signed_in=1"},
		{"to the api", func() string { return api.srv.URL + "/v1/auth/discord/callback?code=dev" }, public + "/v1/auth/discord/callback?code=dev"},
		{"elsewhere", func() string { return "https://discord.com/oauth2/authorize" }, "https://discord.com/oauth2/authorize"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, site = newBackend(t, "api", redirect(func() string { return tt.to() })), newBackend(t, "site", nil)
			h, err := newProxy(public, api.srv.URL, site.srv.URL)
			if err != nil {
				t.Fatalf("newProxy: %v", err)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/discord", nil))
			if got := rec.Header().Get("Location"); got != tt.want {
				t.Errorf("Location = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheProxyRefusesToStartWithoutUsableAddresses(t *testing.T) {
	tests := []struct {
		name, public, api, site string
	}{
		{"no tunnel address", "", "http://127.0.0.1:8090", "http://127.0.0.1:4323"},
		{"a tunnel address that is not https", "http://words.trycloudflare.com", "http://127.0.0.1:8090", "http://127.0.0.1:4323"},
		{"a tunnel address with a path", public + "/blitz", "http://127.0.0.1:8090", "http://127.0.0.1:4323"},
		{"an api elsewhere", public, "https://api.example.com", "http://127.0.0.1:4323"},
		{"a site elsewhere", public, "http://127.0.0.1:8090", "http://192.168.1.5:4323"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := newProxy(tt.public, tt.api, tt.site); err == nil {
				t.Errorf("newProxy(%q, %q, %q) = nil error, want a refusal", tt.public, tt.api, tt.site)
			}
		})
	}
}
