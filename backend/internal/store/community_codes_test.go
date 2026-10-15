package store

import "testing"

func TestCodeMatches(t *testing.T) {
	tests := []struct {
		name, code, event string
		want              bool
	}{
		{"the code itself", "wrld", "wrld", true},
		{"a season on a code without one", "wrld", "wrld_2", false},
		{"a season", "ic_{}", "ic_30", true},
		{"no season", "ic_{}", "ic", false},
		{"an empty season", "ic_{}", "ic_", false},
		{"a season that is not a number", "ic_{}", "ic_x", false},
		{"another code's season", "ic_{}", "im_30", false},
		{"a two-digit season", "rl_{:02d}", "rl_05", true},
		{"a longer code", "sl", "slx", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeMatches(tt.code, tt.event); got != tt.want {
				t.Errorf("codeMatches(%q, %q) = %v, want %v", tt.code, tt.event, got, tt.want)
			}
		})
	}
}
