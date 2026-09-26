package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
		wantBody string
	}{
		{name: "get is ok", method: http.MethodGet, path: "/healthz", wantCode: http.StatusOK, wantBody: `{"status":"ok"}` + "\n"},
		{name: "post is refused", method: http.MethodPost, path: "/healthz", wantCode: http.StatusMethodNotAllowed},
		{name: "unknown path is not found", method: http.MethodGet, path: "/nope", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewMux().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Errorf("%s %s: code = %d, want %d", tt.method, tt.path, rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("%s %s: body = %q, want %q", tt.method, tt.path, rec.Body.String(), tt.wantBody)
			}
			if tt.wantCode == http.StatusOK && rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf("%s %s: Content-Type = %q, want %q", tt.method, tt.path, rec.Header().Get("Content-Type"), "application/json")
			}
		})
	}
}
