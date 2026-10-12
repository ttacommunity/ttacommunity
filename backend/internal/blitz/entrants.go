// Package blitz holds the rules of blitz tournaments that need no database: entrants' time zones, and later
// pairing rounds and ranking players.
package blitz

import (
	"strings"
	"time"
	_ "time/tzdata" // the api image carries no zone files; the zones are embedded, so LoadLocation works anywhere
)

// ValidZone reports whether name is an IANA zone (Region/City, or UTC) the embedded zone data knows. The old
// fixed-offset names (EST, CET) are refused: a player's zone follows their daylight saving.
func ValidZone(name string) bool {
	if name == "" || name == "Local" || (name != "UTC" && !strings.Contains(name, "/")) {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// Offset returns zone's offset from UTC at t, as "+02:00".
func Offset(zone string, t time.Time) (string, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return t.In(loc).Format("-07:00"), nil
}
