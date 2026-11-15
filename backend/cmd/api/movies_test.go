package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestTheMovieHubIsOffOrHasALongSecret(t *testing.T) {
	long := strings.Repeat("s", 32)
	tests := []struct {
		name, secret string
		wantOn       bool
		wantErr      bool
	}{
		{"off", "", false, false},
		{"a secret too short to trust", long[:31], false, true},
		{"a long secret", long, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub, err := openMovieHub(func(k string) string { return map[string]string{"MOVIES_HUB_SECRET": tt.secret}[k] }, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("openMovieHub error = %v, want an error: %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "MOVIES_HUB_SECRET") {
				t.Errorf("openMovieHub error = %v, want it to name MOVIES_HUB_SECRET", err)
			}
			if (hub != nil) != tt.wantOn {
				t.Fatalf("openMovieHub = %v, want the hub on: %v", hub, tt.wantOn)
			}
			if tt.wantOn && string(hub.Secret()) != long {
				t.Error("openMovieHub's secret is not MOVIES_HUB_SECRET")
			}
		})
	}
}

func TestTheMovieHubNeverPrintsItsSecret(t *testing.T) {
	secret := strings.Repeat("s", 32)
	hub, err := openMovieHub(func(string) string { return secret }, nil, nil)
	if err != nil {
		t.Fatalf("openMovieHub: %v", err)
	}
	for _, verb := range []string{"%v", "%+v", "%s"} {
		if got := fmt.Sprintf(verb, hub); strings.Contains(got, secret) {
			t.Errorf("Sprintf(%q) = %q, shows the secret", verb, got)
		}
	}
}
