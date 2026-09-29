package round

import "testing"

func TestTenthMatchesPython(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{2.14, 2.1}, {2.15000001, 2.2}, {0.25, 0.2}, {0.35, 0.3}, {-0.04, 0}, {-1.26, -1.3},
	}
	for _, tt := range tests {
		if got := Tenth(tt.in); got != tt.want {
			t.Errorf("Tenth(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestTenthAwayMatchesSQLite(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{2.14, 2.1}, {0.25, 0.3}, {12.25, 12.3}, {-0.25, -0.3}, {66.66666, 66.7},
	}
	for _, tt := range tests {
		if got := TenthAway(tt.in); got != tt.want {
			t.Errorf("TenthAway(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
