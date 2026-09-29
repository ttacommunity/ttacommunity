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

func TestToMatchesPython(t *testing.T) {
	tests := []struct {
		in     float64
		places int
		want   float64
	}{
		{0.98765, 4, 0.9877}, {0.12345, 4, 0.1235}, {1.00005, 4, 1.0001}, {-0.33333, 4, -0.3333},
	}
	for _, tt := range tests {
		if got := To(tt.in, tt.places); got != tt.want {
			t.Errorf("To(%v, %d) = %v, want %v", tt.in, tt.places, got, tt.want)
		}
	}
}
