//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

// cgeStanding is one line of a CGE group's table: places are counts by game size, nil for none.
func cgeStanding(t, s int, group string, row int, player string, user, games int, points float64, p2, p3, p4 []int) tournamentcsv.CGEStanding {
	return tournamentcsv.CGEStanding{
		CGETournament: t, CGESeason: s, Group: group, Row: row, Place: row, Player: player, CGEUser: user,
		Games: games, Points: points, Places2p: p2, Places3p: p3, Places4p: p4, Source: "CGE page",
	}
}

func TestReplaceCGEStandingsReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := ReplaceCGEStandings(ctx, pool, []tournamentcsv.CGEStanding{cgeStanding(1, 1, "old", 1, "x", 9, 1, 2, []int{1, 0}, nil, nil)}); err != nil {
		t.Fatalf("first ReplaceCGEStandings: %v", err)
	}
	unplaced := cgeStanding(4760, 2, "2024 group 1", 4, "sdzy", 65877, 2, -1.5, nil, nil, []int{0, 0, 0, 2})
	unplaced.Place, unplaced.Banned = 0, true
	got, err := ReplaceCGEStandings(ctx, pool, []tournamentcsv.CGEStanding{
		cgeStanding(4760, 2, "2024 group 1", 1, "Machote", 101239, 5, 21, []int{1, 1}, []int{1, 0, 0}, []int{2, 0, 0, 0}),
		unplaced,
	})
	if err != nil {
		t.Fatalf("ReplaceCGEStandings: %v", err)
	}
	if want := (FactsReplaced{Written: 2}); got != want {
		t.Errorf("ReplaceCGEStandings = %+v, want %+v", got, want)
	}
	const rows = `SELECT concat_ws('|', cge_tournament, cge_season, group_name, row_number, coalesce(place::text, '-'), player_name, cge_user,
	                     banned, games, points, coalesce(places_2p::text, '-'), coalesce(places_3p::text, '-'), places_4p, source)
	              FROM cge_standings ORDER BY row_number`
	want := []string{
		"4760|2|2024 group 1|1|1|Machote|101239|f|5|21|{1,1}|{1,0,0}|{2,0,0,0}|CGE page",
		"4760|2|2024 group 1|4|-|sdzy|65877|t|2|-1.5|-|-|{0,0,0,2}|CGE page",
	}
	if got := rowsOf(t, pool, rows); !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

// seat is one seat of a CGE game we hold: the player, their score and how it ended, and their CGE user.
type seat struct {
	name    string
	score   float64
	outcome string
	user    int
}

func done(name string, score float64, user int) seat { return seat{name, score, "finished", user} }
func gone(name string, user int) seat                { return seat{name, -5, "timed_out", user} }

// heldGame adds a played game of CGE tournament t's stage s with its seats, in order.
func heldGame(tb testing.TB, pool *pgxpool.Pool, t, s int, seats ...seat) {
	tb.Helper()
	ctx := context.Background()
	var batch int64
	if err := pool.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('admin') RETURNING batch_id").Scan(&batch); err != nil {
		tb.Fatalf("starting a batch: %v", err)
	}
	cols := []any{batch, len(seats), t, s}
	for i := range 4 {
		if i < len(seats) {
			cols = append(cols, seats[i].name, seats[i].score)
		} else {
			cols = append(cols, nil, nil)
		}
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO matches (batch_id, source_file, source_line, tournament, date, player_count, cge_tournament, cge_season,
		                     player1, score1, player2, score2, player3, score3, player4, score4)
		VALUES ($1, 'm.csv', 1, 'a game', '2025-01-01', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING match_id`,
		cols...).Scan(&id)
	if err != nil {
		tb.Fatalf("adding a game: %v", err)
	}
	for i, st := range seats {
		if _, err := pool.Exec(ctx, "INSERT INTO match_seats (match_id, seat, player_name, outcome, cge_user) VALUES ($1, $2, $3, $4, $5)",
			id, i+1, st.name, st.outcome, st.user); err != nil {
			tb.Fatalf("adding a seat: %v", err)
		}
	}
}

func samePlaces(a, b Places) bool {
	return slices.Equal(a.P2, b.P2) && slices.Equal(a.P3, b.P3) && slices.Equal(a.P4, b.P4)
}

func checked(t *testing.T, pool *pgxpool.Pool, rows ...tournamentcsv.CGEStanding) StandingsCheck {
	t.Helper()
	ctx := context.Background()
	if _, err := ReplaceCGEStandings(ctx, pool, rows); err != nil {
		t.Fatalf("ReplaceCGEStandings: %v", err)
	}
	got, err := NewAdminStore(asAdminLogin(t, pool)).CheckStandings(ctx)
	if err != nil {
		t.Fatalf("CheckStandings: %v", err)
	}
	return got
}

// differences returns the check's differences as "stage user kind" lines.
func differences(c StandingsCheck) []string {
	var got []string
	for _, st := range c.Stages {
		for _, d := range st.Differences {
			got = append(got, fmt.Sprintf("%d/%d %d %s", st.CGETournament, st.CGESeason, d.CGEUser, d.Kind))
		}
	}
	return got
}

func TestCheckStandingsFindsNothingWhenTheGamesAgree(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 14, done("ann", 200, 1), done("bob", 150, 2), done("cy", 100, 3))
	heldGame(t, pool, 19, 14, done("bob", 180, 2), done("ann", 170, 1))
	got := checked(t, pool,
		cgeStanding(19, 14, "Gold 1", 1, "ann", 1, 2, 5, []int{0, 1}, []int{1, 0, 0}, nil),
		cgeStanding(19, 14, "Gold 1", 2, "bob", 2, 2, 6, []int{1, 0}, []int{0, 1, 0}, nil),
		cgeStanding(19, 14, "Gold 1", 3, "cy", 3, 1, 0, nil, []int{0, 0, 1}, nil),
	)
	if len(got.Stages) != 1 || got.Stages[0].Players != 3 || got.Stages[0].Agree != 3 || len(differences(got)) != 0 {
		t.Errorf("check = %+v, want one stage of 3 players, all agreeing", got)
	}
}

func TestCheckStandingsAddsUpAPlayersGroupsInAStage(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 5638, 2, done("ann", 200, 1), done("bob", 150, 2))
	heldGame(t, pool, 5638, 2, done("ann", 300, 1), done("cy", 100, 3))
	got := checked(t, pool,
		cgeStanding(5638, 2, "Group 1 Game 3", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(5638, 2, "Group 1 Game 3", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
		cgeStanding(5638, 2, "Group 2 Game 4", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(5638, 2, "Group 2 Game 4", 2, "cy", 3, 1, 0, []int{0, 1}, nil, nil),
	)
	if d := differences(got); len(d) != 0 || got.Stages[0].Agree != 3 {
		t.Errorf("differences = %q, check %+v; want ann's two groups counted as one line that agrees", d, got)
	}
}

func TestCheckStandingsNamesEachKindOfDifference(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 21, 7, done("ann", 200, 1), done("bob", 150, 2))
	heldGame(t, pool, 21, 7, done("ann", 210, 1), done("bob", 190, 2))
	heldGame(t, pool, 21, 7, done("dee", 120, 4), done("eve", 110, 5))
	got := checked(t, pool,
		cgeStanding(21, 7, "Bronze 1", 1, "ann", 1, 3, 8, []int{3, 0}, nil, nil),
		cgeStanding(21, 7, "Bronze 1", 2, "bob", 2, 2, 4, []int{1, 1}, nil, nil),
		cgeStanding(21, 7, "Bronze 1", 3, "cy", 3, 2, 0, []int{0, 2}, nil, nil),
		cgeStanding(21, 7, "Bronze 2", 1, "dee", 4, 1, 4, []int{1, 0}, nil, nil),
	)
	want := []string{"21/7 1 games", "21/7 2 places", "21/7 3 not_held", "21/7 5 not_on_cge"}
	if d := differences(got); !slices.Equal(d, want) {
		t.Errorf("differences = %q, want %q", d, want)
	}
	if st := got.Stages[0]; st.Players != 5 || st.Agree != 1 {
		t.Errorf("stage = %d players, %d agree; want 5 and 1 (dee)", st.Players, st.Agree)
	}
}

func TestCheckStandingsSaysWhenAnOpponentTimedOut(t *testing.T) {
	pool := migrated(t)
	// CGE's AI played on for cy and finished second, so CGE put ann third.
	heldGame(t, pool, 19, 4, done("bob", 200, 2), done("ann", 150, 1), gone("cy", 3))
	got := checked(t, pool,
		cgeStanding(19, 4, "Wood 1", 1, "bob", 2, 1, 5, nil, []int{1, 0, 0}, nil),
		cgeStanding(19, 4, "Wood 1", 2, "ann", 1, 1, 0, nil, []int{0, 0, 1}, nil),
		cgeStanding(19, 4, "Wood 1", 3, "cy", 3, 1, 2, nil, []int{0, 1, 0}, nil),
	)
	want := []string{"19/4 1 places_after_a_timeout", "19/4 3 places_after_a_timeout"}
	if d := differences(got); !slices.Equal(d, want) {
		t.Errorf("differences = %q, want %q", d, want)
	}
}

func TestCheckStandingsShowsBothSidesOfADifference(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 3, done("ann", 200, 1), done("bob", 150, 2), done("cy", 150, 3))
	got := checked(t, pool,
		cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 2, 5, []int{1, 0}, []int{0, 1, 0}, nil),
		cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 1, nil, []int{0, 1, 0}, nil),
		cgeStanding(19, 3, "Gold 1", 3, "cy", 3, 1, 1, nil, []int{0, 1, 0}, nil),
	)
	d := got.Stages[0].Differences[0]
	if d.Player != "ann" || d.CGEGames != 2 || d.HeldGames != 1 || d.CGEPoints != 5 ||
		!samePlaces(d.CGEPlaces, Places{P2: []int{1, 0}, P3: []int{0, 1, 0}}) || !samePlaces(d.HeldPlaces, Places{P3: []int{1, 0, 0}}) {
		t.Errorf("difference = %+v, want ann: CGE 2 games, 5 points, a 2p first and a 3p second; held 1 game, a 3p first", d)
	}
}

func TestCheckStandingsSharesATiedPlace(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 3, done("ann", 200, 1), done("bob", 150, 2), done("cy", 150, 3))
	got := checked(t, pool,
		cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 1, 5, nil, []int{1, 0, 0}, nil),
		cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 1, nil, []int{0, 1, 0}, nil),
		cgeStanding(19, 3, "Gold 1", 3, "cy", 3, 1, 1, nil, []int{0, 1, 0}, nil),
	)
	if d := differences(got); len(d) != 0 {
		t.Errorf("differences = %q, want none: bob and cy tie for second", d)
	}
}

func TestCheckStandingsLeavesOutStagesOnlyOneSideHas(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 3, done("ann", 200, 1), done("bob", 150, 2))
	got := checked(t, pool,
		cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
		cgeStanding(5600, 1, "group 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
	)
	if len(got.Stages) != 1 || got.Stages[0].CGETournament != 19 || got.StagesOnlyOnCGE != 1 {
		t.Errorf("check = %+v, want stage 19/3 checked and one stage only on CGE", got)
	}
}

func TestCheckStandingsIgnoresAPlayerCGEListsWithNoGames(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 3, done("ann", 200, 1), done("bob", 150, 2))
	got := checked(t, pool,
		cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 3, "quit", 3, 0, 0, nil, nil, nil),
	)
	if d := differences(got); len(d) != 0 || got.Stages[0].Players != 2 {
		t.Errorf("differences = %q, players %d; want none, 2 players: one who played no game is no difference", d, got.Stages[0].Players)
	}
}

func TestCheckStandingsTakesAnEmptyLineAsNoGamesOfThatSize(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 17, done("ann", 200, 1), done("bob", 150, 2), done("cy", 100, 3))
	got := checked(t, pool,
		cgeStanding(19, 17, "Gold 1", 1, "ann", 1, 1, 5, []int{0, 0}, []int{1, 0, 0}, nil),
		cgeStanding(19, 17, "Gold 1", 2, "bob", 2, 1, 2, []int{0, 0}, []int{0, 1, 0}, []int{0, 0, 0, 0}),
		cgeStanding(19, 17, "Gold 1", 3, "cy", 3, 1, 0, nil, []int{0, 0, 1}, nil),
	)
	if d := differences(got); len(d) != 0 {
		t.Errorf("differences = %q, want none: CGE's zero lines are sizes not played", d)
	}
}

func TestCheckStandingsCountsAGameNoOnePlayedAsCGEDoes(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 19, 3, done("ann", 200, 1), done("bob", 150, 2))
	heldGame(t, pool, 19, 3, gone("ann", 1), gone("bob", 2))
	if _, err := pool.Exec(context.Background(), "UPDATE matches SET state = 'never_played' WHERE match_id = (SELECT max(match_id) FROM matches)"); err != nil {
		t.Fatal(err)
	}
	// CGE counts the game, and places its seats as its AI finished them.
	got := checked(t, pool,
		cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 2, 4, []int{1, 1}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 2, 4, []int{1, 1}, nil, nil),
	)
	want := []string{"19/3 1 places_after_a_timeout", "19/3 2 places_after_a_timeout"}
	if d := differences(got); !slices.Equal(d, want) {
		t.Errorf("differences = %q, want %q: the same games, the never-played one placed by CGE's AI", d, want)
	}
}

func TestCheckStandingsTakesACountWithoutTheGameNoOnePlayed(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 4369, 1, done("ann", 200, 1), done("bob", 150, 2))
	heldGame(t, pool, 4369, 1, gone("ann", 1), gone("bob", 2))
	if _, err := pool.Exec(context.Background(), "UPDATE matches SET state = 'never_played' WHERE match_id = (SELECT max(match_id) FROM matches)"); err != nil {
		t.Fatal(err)
	}
	got := checked(t, pool,
		cgeStanding(4369, 1, "group 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(4369, 1, "group 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
	)
	if d := differences(got); len(d) != 0 {
		t.Errorf("differences = %q, want none: some CGE tables leave out a game no one played", d)
	}
}
