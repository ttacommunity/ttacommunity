// Package httpapi holds the API's routes and handlers.
package httpapi

import "net/http"

// NewMux returns the API's router with every route registered.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

// healthz reports that the process is up. It checks no dependency, so a
// database outage does not get the pod restarted.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}
