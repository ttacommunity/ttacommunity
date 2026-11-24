// Command shareproxy puts a local site build and its api behind one address, so one quick tunnel can show both with a
// dev sign-in: /v1/* goes to the api, everything else to the site, and admin routes are refused. See README, "Let
// someone sign in as a test player".
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	public := flag.String("public", "", "the tunnel's address, https://<words>.trycloudflare.com")
	listen := flag.String("listen", "127.0.0.1:4325", "where the proxy listens: point the tunnel here")
	api := flag.String("api", "http://127.0.0.1:8090", "the local api")
	site := flag.String("site", "http://127.0.0.1:4323", "the local site build (astro preview)")
	flag.Parse()
	h, err := newProxy(*public, *api, *site)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shareproxy:", err)
		os.Exit(2)
	}
	srv := &http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("shareproxy: %s on %s: /v1/* to %s, the rest to %s", *public, *listen, *api, *site)
	log.Fatal(srv.ListenAndServe())
}

// newProxy returns the proxy for the tunnel's https origin public, sending /v1/* to api and the rest to site, both
// http://127.0.0.1 origins. It refuses /v1/admin, tells the api requests from public come from the local site (its
// Origin check), and points redirects to either local address back at public. It returns an error for any other
// address.
func newProxy(public, api, site string) (http.Handler, error) {
	if !origin(public, "https://") {
		return nil, errors.New("-public must be the tunnel's address, as https://<words>.trycloudflare.com")
	}
	if !origin(api, "http://127.0.0.1:") || !origin(site, "http://127.0.0.1:") {
		return nil, errors.New("-api and -site must be on this machine, as http://127.0.0.1:<port>")
	}
	forward := func(to string) *httputil.ReverseProxy {
		u, _ := url.Parse(to)
		p := httputil.NewSingleHostReverseProxy(u)
		p.ModifyResponse = func(r *http.Response) error {
			if loc := r.Header.Get("Location"); loc != "" {
				r.Header.Set("Location", strings.NewReplacer(api, public, site, public).Replace(loc))
			}
			return nil
		}
		return p
	}
	toAPI, toSite := forward(api), forward(site)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/admin" || strings.HasPrefix(r.URL.Path, "/v1/admin/"):
			http.Error(w, "admin is not reached through this tunnel", http.StatusForbidden)
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			if r.Header.Get("Origin") == public {
				r.Header.Set("Origin", site)
			}
			toAPI.ServeHTTP(w, r)
		default:
			toSite.ServeHTTP(w, r)
		}
	}), nil
}

// origin reports whether s is a bare origin starting with prefix: no path, query or credentials.
func origin(s, prefix string) bool {
	rest, ok := strings.CutPrefix(s, prefix)
	return ok && rest != "" && !strings.ContainsAny(rest, "/?#@")
}
