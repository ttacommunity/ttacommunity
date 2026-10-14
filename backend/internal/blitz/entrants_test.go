package blitz_test

import (
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
)

func TestValidZoneAcceptsIANANamesRegionsAndUnknown(t *testing.T) {
	tests := []struct {
		zone string
		want bool
	}{
		{"Europe/Berlin", true}, {"America/Argentina/Buenos_Aires", true}, {"UTC", true}, {"Unknown", true}, {"US East", true},
		{"EST", false}, {"CET", false}, {"Local", false}, {"", false}, {"Mars/Olympus", false}, {"GMT+2", false},
		{"us east", false}, {"Atlantis", false},
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
		{"US East", jan, "-05:00"}, {"US East", jul, "-04:00"}, {"Australia East", jan, "+11:00"}, {"Australia East", jul, "+10:00"},
	}
	minutes := map[string]int{"+01:00": 60, "+02:00": 120, "+11:00": 660, "+10:00": 600, "-05:00": -300, "-04:00": -240, "+05:30": 330, "+00:00": 0}
	for _, tt := range tests {
		got, err := blitz.Offset(tt.zone, tt.at)
		if err != nil || got != tt.want {
			t.Errorf("Offset(%q, %s) = %q, %v; want %q", tt.zone, tt.at.Format(time.DateOnly), got, err, tt.want)
		}
		if m, err := blitz.OffsetMinutes(tt.zone, tt.at); err != nil || m != minutes[tt.want] {
			t.Errorf("OffsetMinutes(%q, %s) = %d, %v; want %d", tt.zone, tt.at.Format(time.DateOnly), m, err, minutes[tt.want])
		}
	}
	if _, err := blitz.Offset("Mars/Olympus", jan); err == nil {
		t.Error("Offset(Mars/Olympus) = nil error, want one")
	}
	if _, err := blitz.OffsetMinutes("Mars/Olympus", jan); err == nil {
		t.Error("OffsetMinutes(Mars/Olympus) = nil error, want one")
	}
}

func TestAnUnknownZoneShowsNoOffsetAndPairsAsUTC(t *testing.T) {
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if got, err := blitz.Offset(blitz.Unknown, at); err != nil || got != "" {
		t.Errorf("Offset(Unknown) = %q, %v; want \"\"", got, err)
	}
	if got, err := blitz.OffsetMinutes(blitz.Unknown, at); err != nil || got != 0 {
		t.Errorf("OffsetMinutes(Unknown) = %d, %v; want 0", got, err)
	}
}

func TestEveryRegionKeepsTheTimeOfAnIANAZone(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range blitz.Regions {
		if seen[r.Name] || r.Name == blitz.Unknown || strings.Contains(r.Name, "/") {
			t.Errorf("region %q: named twice, Unknown, or like an IANA zone", r.Name)
		}
		seen[r.Name] = true
		if r.Zone == "UTC" || !strings.Contains(r.Zone, "/") || !blitz.ValidZone(r.Zone) {
			t.Errorf("region %q keeps the time of %q, want an IANA zone of a city", r.Name, r.Zone)
		}
	}
}

func TestFinishedAtReadsTheClockInTheZoneGiven(t *testing.T) {
	tests := []struct {
		name, day, clock, zone string
		want                   string
	}{
		{"an IANA zone in its summer", "2026-07-01", "21:30", "Europe/Berlin", "2026-07-01T19:30:00Z"},
		{"the day after Europe's clocks go forward", "2026-03-30", "21:30", "Europe Central", "2026-03-30T19:30:00Z"},
		{"the day before Europe's clocks go forward", "2026-03-28", "21:30", "Europe Central", "2026-03-28T20:30:00Z"},
		{"a southern summer, onto the day before in UTC", "2026-01-15", "08:00", "Australia East", "2026-01-14T21:00:00Z"},
		{"a southern winter", "2026-07-15", "08:00", "Australia East", "2026-07-14T22:00:00Z"},
		{"Unknown as UTC", "2026-07-01", "21:30", "Unknown", "2026-07-01T21:30:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := blitz.FinishedAt(tt.day, tt.clock, tt.zone)
			if err != nil || got.UTC().Format(time.RFC3339) != tt.want {
				t.Errorf("FinishedAt(%s, %s, %s) = %s, %v; want %s", tt.day, tt.clock, tt.zone, got.UTC().Format(time.RFC3339), err, tt.want)
			}
		})
	}
}

func TestFinishedAtRefusesWhatIsNotADayATimeAndAZone(t *testing.T) {
	for _, in := range [][3]string{
		{"2026-02-30", "21:30", "UTC"}, {"14/10/2026", "21:30", "UTC"}, {"2026-10-14", "24:00", "UTC"},
		{"2026-10-14", "", "UTC"}, {"2026-10-14", "21:30", "Atlantis"}, {"2026-10-14", "21:30", "EST"},
	} {
		if got, err := blitz.FinishedAt(in[0], in[1], in[2]); err == nil {
			t.Errorf("FinishedAt(%s, %s, %s) = %s, want an error", in[0], in[1], in[2], got)
		}
	}
}
