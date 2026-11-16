package movies

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const inventedHubSecret = "invented-hub-secret-000000000000000"

func TestHubClientAsksTheHubToSubscribe(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("request = %s %s, want a form POST", r.Method, r.Header.Get("Content-Type"))
		}
		_ = r.ParseForm()
		got = r.PostForm
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	h := NewHubClient(nil, srv.URL, "https://api.example/v1/movies/hub", []byte(inventedHubSecret))
	if err := h.Subscribe(context.Background(), "UC0000000000000000000001"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	want := map[string]string{
		"hub.mode":     "subscribe",
		"hub.topic":    Topic("UC0000000000000000000001"),
		"hub.callback": "https://api.example/v1/movies/hub",
		"hub.verify":   "async",
		"hub.secret":   inventedHubSecret,
	}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, got.Get(k), v)
		}
	}
}

func TestHubClientReportsARefusalWithoutItsSecret(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"accepted", http.StatusAccepted, false},
		{"done at once", http.StatusNoContent, false},
		{"refused", http.StatusBadRequest, true},
		{"failing", http.StatusServiceUnavailable, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, "hub says no to "+inventedHubSecret)
			}))
			defer srv.Close()
			h := NewHubClient(nil, srv.URL, "https://api.example/v1/movies/hub", []byte(inventedHubSecret))
			err := h.Subscribe(context.Background(), "UC0000000000000000000001")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Subscribe = %v, want an error: %v", err, tt.wantErr)
			}
			if err != nil && (!strings.Contains(err.Error(), fmt.Sprint(tt.status)) || strings.Contains(err.Error(), inventedHubSecret)) {
				t.Errorf("err = %q, want the status and never the secret", err)
			}
			for _, s := range []string{fmt.Sprintf("%v", h), fmt.Sprintf("%+v", h), fmt.Sprintf("%#v", h)} {
				if strings.Contains(s, inventedHubSecret) {
					t.Errorf("%q shows the secret", s)
				}
			}
		})
	}
}

func TestHubClientRefusesABadChannel(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	h := NewHubClient(nil, srv.URL, "https://api.example/v1/movies/hub", []byte(inventedHubSecret))
	if err := h.Subscribe(context.Background(), "not-a-channel"); err == nil || requests != 0 {
		t.Errorf("Subscribe(a bad channel id) = %v after %d requests, want an error and none", err, requests)
	}
}
