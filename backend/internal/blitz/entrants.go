// Package blitz holds the rules of blitz tournaments that need no database: entrants' time zones (an IANA zone,
// a region, or Unknown), pairing rounds and ranking players.
package blitz

import (
	"fmt"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // the api image carries no zone files; the zones are embedded, so LoadLocation works anywhere
)

// Unknown is the zone of a member whose zone is not known yet: shown with no offset, and paired as UTC.
const Unknown = "Unknown"

// Region is a part of the world a member may be placed in while their zone is not known; it keeps the time of
// Zone, one of its cities.
type Region struct{ Name, Zone string }

// Regions are the regions admin offers, west to east.
var Regions = []Region{
	{"US West", "America/Los_Angeles"}, {"US Mountain", "America/Denver"}, {"US Central", "America/Chicago"},
	{"US East", "America/New_York"}, {"South America", "America/Sao_Paulo"}, {"Europe West", "Europe/London"},
	{"Europe Central", "Europe/Berlin"}, {"Africa West", "Africa/Lagos"}, {"Africa South", "Africa/Johannesburg"},
	{"Europe East", "Europe/Helsinki"}, {"Africa East", "Africa/Nairobi"}, {"Russia West", "Europe/Moscow"},
	{"Middle East", "Asia/Riyadh"}, {"South Asia", "Asia/Kolkata"}, {"Southeast Asia", "Asia/Bangkok"},
	{"East Asia", "Asia/Shanghai"}, {"Australia West", "Australia/Perth"}, {"Japan & Korea", "Asia/Tokyo"},
	{"Australia East", "Australia/Sydney"}, {"New Zealand", "Pacific/Auckland"},
}

// location finds a zone's time: a region's city's, UTC for Unknown, else the IANA zone's.
func location(zone string) (*time.Location, error) {
	if zone == Unknown {
		return time.UTC, nil
	}
	if i := slices.IndexFunc(Regions, func(r Region) bool { return r.Name == zone }); i >= 0 {
		zone = Regions[i].Zone
	}
	return time.LoadLocation(zone)
}

// ValidZone reports whether name is Unknown, one of Regions, or an IANA zone (Region/City, or UTC) the embedded
// zone data knows. The old fixed-offset names (EST, CET) are refused: a player's zone follows their daylight saving.
func ValidZone(name string) bool {
	if name == Unknown || slices.ContainsFunc(Regions, func(r Region) bool { return r.Name == name }) {
		return true
	}
	if name == "" || name == "Local" || (name != "UTC" && !strings.Contains(name, "/")) {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// Offset returns zone's offset from UTC at t, as "+02:00", or "" for Unknown.
func Offset(zone string, t time.Time) (string, error) {
	if zone == Unknown {
		return "", nil
	}
	loc, err := location(zone)
	if err != nil {
		return "", err
	}
	return t.In(loc).Format("-07:00"), nil
}

// OffsetMinutes returns zone's offset from UTC at t in minutes, east positive; Unknown is UTC's.
func OffsetMinutes(zone string, t time.Time) (int, error) {
	loc, err := location(zone)
	if err != nil {
		return 0, err
	}
	_, seconds := t.In(loc).Zone()
	return seconds / 60, nil
}

// FinishedAt is the instant a game finished: day (2026-10-14) at clock (21:30) on the clocks of zone, an IANA
// zone, a region or Unknown (UTC). It refuses a day, clock or zone it cannot read.
func FinishedAt(day, clock, zone string) (time.Time, error) {
	if !ValidZone(zone) {
		return time.Time{}, fmt.Errorf("not a time zone: %q", zone)
	}
	loc, err := location(zone)
	if err != nil {
		return time.Time{}, err
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", day+" "+clock, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("not a day and a time: %q at %q", day, clock)
	}
	return at, nil
}
