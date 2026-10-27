package httpapi

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

func TestBlitzGameReadsTheRecordOrRefusesIt(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	record := func(day, clock, zone string) *BlitzGameRecord {
		d, _ := time.Parse(time.DateOnly, day)
		return &BlitzGameRecord{ShareCode: "ABCD1234", FinishedOn: openapi_types.Date{Time: d}, FinishedTime: clock, Zone: zone,
			A: BlitzGameSide{Culture: 250, Outcome: "finished"}, B: BlitzGameSide{Culture: 200, Outcome: "resigned"}}
	}
	tests := []struct {
		name    string
		body    *BlitzGameRecord
		wantMsg string // a part of the refusal; "" for a game
	}{
		{"a game", record("2026-10-10", "21:30", "Europe Central"), ""},
		{"no body", nil, "share_code, a, b"},
		{"a zone that is not one", record("2026-10-10", "21:30", "Mars"), "not a time zone"},
		{"a finish after now", record("2026-10-12", "21:30", "Europe Central"), "cannot finish after now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, msg := blitzGame(tt.body, now)
			if tt.wantMsg == "" {
				if msg != "" || g.ShareCode != "ABCD1234" || g.A.Culture != 250 || g.B.Outcome != "resigned" || !g.Finished.Before(now) {
					t.Errorf("blitzGame = %+v, %q; want the game recorded", g, msg)
				}
				return
			}
			if !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("blitzGame message = %q, want one containing %q", msg, tt.wantMsg)
			}
		})
	}
}
