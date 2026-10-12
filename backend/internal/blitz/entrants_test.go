package blitz_test

import (
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
)

func TestValidZoneAcceptsOnlyIANANames(t *testing.T) {
	tests := []struct {
		zone string
		want bool
	}{
		{"Europe/Berlin", true}, {"America/Argentina/Buenos_Aires", true}, {"UTC", true},
		{"EST", false}, {"CET", false}, {"Local", false}, {"", false}, {"Mars/Olympus", false}, {"GMT+2", false},
	}
	for _, tt := range tests {
		if got := blitz.ValidZone(tt.zone); got != tt.want {
			t.Errorf("ValidZone(%q) = %v, want %v", tt.zone, got, tt.want)
		}
	}
}

func TestOffsetFollowsDaylightSavingInBothHemispheres(t *testing.T) {
	jan, jul := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		zone string
		at   time.Time
		want string
	}{
		{"Europe/Berlin", jan, "+01:00"}, {"Europe/Berlin", jul, "+02:00"},
		{"Australia/Sydney", jan, "+11:00"}, {"Australia/Sydney", jul, "+10:00"},
		{"America/New_York", jan, "-05:00"}, {"America/New_York", jul, "-04:00"},
		{"Asia/Kolkata", jan, "+05:30"}, {"UTC", jul, "+00:00"},
	}
	for _, tt := range tests {
		got, err := blitz.Offset(tt.zone, tt.at)
		if err != nil || got != tt.want {
			t.Errorf("Offset(%q, %s) = %q, %v; want %q", tt.zone, tt.at.Format(time.DateOnly), got, err, tt.want)
		}
	}
	if _, err := blitz.Offset("Mars/Olympus", jan); err == nil {
		t.Error("Offset(Mars/Olympus) = nil error, want one")
	}
}
