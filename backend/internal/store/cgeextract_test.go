//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/cgeextract"
)

func extractSeat(session, tournament, player int, name, code string, culture float64) cgeextract.Seat {
	return cgeextract.Seat{SessionID: session, TournamentID: tournament, PlayerID: player, PlayerName: name,
		ShareCode: code, State: "finished", FinishStatus: cgeextract.Finished, Score: &culture}
}

func TestPlanCGEExtractMatchesOurGamesByCodeAndByPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
	setSeats(t, pool, "%game 2", Seat{"ann", 120}, Seat{"cy", 90})
	if _, err := pool.Exec(ctx, "UPDATE games SET share_code = 'CODEAAAA' WHERE name LIKE '%game 1'"); err != nil {
		t.Fatal(err)
	}
	x := cgeextract.Extract{Seats: []cgeextract.Seat{
		extractSeat(100, 19, 1, "ann", "CODEAAAA", 200), extractSeat(100, 19, 2, "bob", "CODEAAAA", 150),
		extractSeat(101, 19, 1, "ann", "CODEBBBB", 120), extractSeat(101, 19, 3, "cy", "CODEBBBB", 90),
		extractSeat(102, 19, 2, "bob", "CODECCCC", 70), extractSeat(102, 19, 3, "cy", "CODECCCC", 60),
	}}

	plan, err := PlanCGEExtract(ctx, pool, x)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	var sessions []int
	for _, s := range plan.Matching.Matched {
		sessions = append(sessions, s)
	}
	slices.Sort(sessions)
	if !slices.Equal(sessions, []int{100, 101}) || !slices.Equal(plan.Matching.Added, []int{102}) || len(plan.Matching.OursOnly) != 0 {
		t.Errorf("matched %v, added %v, ours only %v; want 100 and 101 matched, 102 added", sessions, plan.Matching.Added, plan.Matching.OursOnly)
	}
}
