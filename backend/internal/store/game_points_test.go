//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestGameLinesGiveEachSeatItsGamePoints(t *testing.T) {
	tests := []struct {
		name     string
		settings []string
		outcomes string
		seats    []Seat
		want     string
	}{
		{"by place from the series' points", nil, "",
			[]Seat{{"ann", 200}, {"bob", 150}, {"cy", 90}}, "{5,2,0}"},
		{"a tie shares its places' points", nil, "",
			[]Seat{{"ann", 200}, {"bob", 200}, {"cy", 90}}, "{3.5,3.5,0}"},
		{"a timed-out seat takes the season's penalty instead", []string{"tournament: -1"}, "timed_out",
			[]Seat{{"ann", 200}, {"bob", 150}, {"cy", 0}}, "{5,2,-1}"},
		{"a timed-out seat without a penalty scores 0", nil, "timed_out",
			[]Seat{{"ann", 200}, {"bob", 150}, {"cy", 0}}, "{5,2,0}"},
		{"a stage's points override the series'", []string{"stage: 9,1,0"}, "",
			[]Seat{{"ann", 200}, {"bob", 150}, {"cy", 90}}, "{9,1,0}"},
		{"a game short of players takes the top of the smallest table", nil, "",
			[]Seat{{"ann", 200}, {"bob", 150}}, "{5,2}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
			setSeats(t, pool, "%game 1", tt.seats...)
			queries := map[string]string{
				"tournament: -1": `INSERT INTO settings (tournament_id, timeout_penalty) SELECT tournament_id, -1 FROM tournaments`,
				"stage: 9,1,0":   `INSERT INTO settings (stage_id, points_3p) SELECT stage_id, '{9,1,0}' FROM stages`,
			}
			for _, s := range tt.settings {
				if _, err := pool.Exec(ctx, queries[s]); err != nil {
					t.Fatal(err)
				}
			}
			if tt.outcomes != "" {
				if _, err := pool.Exec(ctx, "UPDATE seats SET outcome = $1 WHERE seat = 3", tt.outcomes); err != nil {
					t.Fatal(err)
				}
			}
			var got string
			if err := pool.QueryRow(ctx, "SELECT game_points::text FROM game_lines").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("game_points = %s, want %s", got, tt.want)
			}
		})
	}
}
