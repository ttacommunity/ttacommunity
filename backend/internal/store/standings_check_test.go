//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	unplaced := cgeStanding(4760, 2, "2024 group 1", 4, "swift", 65877, 2, -1.5, nil, nil, []int{0, 0, 0, 2})
	unplaced.Place, unplaced.Banned = 0, true
	got, err := ReplaceCGEStandings(ctx, pool, []tournamentcsv.CGEStanding{
		cgeStanding(4760, 2, "2024 group 1", 1, "Magpie", 101239, 5, 21, []int{1, 1}, []int{1, 0, 0}, []int{2, 0, 0, 0}),
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
		"4760|2|2024 group 1|1|1|Magpie|101239|f|5|21|{1,1}|{1,0,0}|{2,0,0,0}|CGE page",
		"4760|2|2024 group 1|4|-|swift|65877|t|2|-1.5|-|-|{0,0,0,2}|CGE page",
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
	var batch, group, id int64
	if err := pool.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('admin') RETURNING batch_id").Scan(&batch); err != nil {
		tb.Fatalf("starting a batch: %v", err)
	}
	err := pool.QueryRow(ctx, `
		WITH se AS (INSERT INTO series (slug, name) VALUES ('held', 'Held') ON CONFLICT (slug) DO UPDATE SET name = excluded.name RETURNING series_id),
		t AS (INSERT INTO tournaments (series_id, label) SELECT series_id, $3 FROM se
		      ON CONFLICT (series_id, label) DO UPDATE SET label = excluded.label RETURNING tournament_id),
		st AS (INSERT INTO stages (tournament_id, label) SELECT tournament_id, $4 FROM t
		       ON CONFLICT (tournament_id, label) DO UPDATE SET label = excluded.label RETURNING stage_id)
		INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number) SELECT stage_id, '', $1, $2 FROM st
		ON CONFLICT (stage_id, name, cge_tournament, cge_stage_number, cge_group) DO UPDATE SET name = excluded.name RETURNING group_id`,
		t, s, strconv.Itoa(t), strconv.Itoa(s)).Scan(&group)
	if err != nil {
		tb.Fatalf("finding CGE stage %d/%d: %v", t, s, err)
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO games (batch_id, source_file, source_line, name, date, group_id)
		VALUES ($1, 'm.csv', 1, 'a game', '2025-01-01', $2) RETURNING game_id`, batch, group).Scan(&id)
	if err != nil {
		tb.Fatalf("adding a game: %v", err)
	}
	for i, st := range seats {
		for _, q := range []string{
			"INSERT INTO players (name) VALUES ($1) ON CONFLICT (name) DO NOTHING",
			"INSERT INTO player_accounts (cge_user, player_id, name) SELECT $2, player_id, $1 FROM players WHERE name = $1 ON CONFLICT DO NOTHING",
		} {
			args := []any{st.name}
			if strings.Contains(q, "$2") {
				args = append(args, st.user)
			}
			if _, err := pool.Exec(ctx, q, args...); err != nil {
				tb.Fatalf("adding %s: %v", st.name, err)
			}
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, recorded_score, place)
			SELECT $1, $2, player_id, $3, $5, $4, CASE WHEN $4 = 'finished' THEN greatest($6::float8, 0) ELSE 0 END, $6,
			       1 + (SELECT count(*) FROM unnest($7::float8[]) AS o(score) WHERE o.score > $6)
			FROM players WHERE name = $3`,
			id, i+1, st.name, st.outcome, st.user, st.score, scoresOf(seats))
		if err != nil {
			tb.Fatalf("adding a seat: %v", err)
		}
	}
}

func scoresOf(seats []seat) []float64 {
	var out []float64
	for _, s := range seats {
		out = append(out, s.score)
	}
	return out
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

// held is a game we hold, of CGE tournament t's stage s; never marks one no one played.
type held struct {
	t, s  int
	seats []seat
	never bool
}

func TestCheckStandingsNamesEachDifference(t *testing.T) {
	tests := []struct {
		name  string
		games []held
		lines []tournamentcsv.CGEStanding
		want  []string
	}{
		{
			name: "games that agree",
			games: []held{
				{t: 19, s: 14, seats: []seat{done("ann", 200, 1), done("bob", 150, 2), done("cy", 100, 3)}},
				{t: 19, s: 14, seats: []seat{done("bob", 180, 2), done("ann", 170, 1)}},
			},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(19, 14, "Gold 1", 1, "ann", 1, 2, 5, []int{0, 1}, []int{1, 0, 0}, nil),
				cgeStanding(19, 14, "Gold 1", 2, "bob", 2, 2, 6, []int{1, 0}, []int{0, 1, 0}, nil),
				cgeStanding(19, 14, "Gold 1", 3, "cy", 3, 1, 0, nil, []int{0, 0, 1}, nil),
			},
		},
		{
			name: "a player's groups in a stage add up to one line",
			games: []held{
				{t: 5638, s: 2, seats: []seat{done("ann", 200, 1), done("bob", 150, 2)}},
				{t: 5638, s: 2, seats: []seat{done("ann", 300, 1), done("cy", 100, 3)}},
			},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(5638, 2, "Group 1 Game 3", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
				cgeStanding(5638, 2, "Group 1 Game 3", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
				cgeStanding(5638, 2, "Group 2 Game 4", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
				cgeStanding(5638, 2, "Group 2 Game 4", 2, "cy", 3, 1, 0, []int{0, 1}, nil, nil),
			},
		},
		{
			name: "each kind of difference",
			games: []held{
				{t: 21, s: 7, seats: []seat{done("ann", 200, 1), done("bob", 150, 2)}},
				{t: 21, s: 7, seats: []seat{done("ann", 210, 1), done("bob", 190, 2)}},
				{t: 21, s: 7, seats: []seat{done("dee", 120, 4), done("eve", 110, 5)}},
			},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(21, 7, "Bronze 1", 1, "ann", 1, 3, 8, []int{3, 0}, nil, nil),
				cgeStanding(21, 7, "Bronze 1", 2, "bob", 2, 2, 4, []int{1, 1}, nil, nil),
				cgeStanding(21, 7, "Bronze 1", 3, "cy", 3, 2, 0, []int{0, 2}, nil, nil),
				cgeStanding(21, 7, "Bronze 2", 1, "dee", 4, 1, 4, []int{1, 0}, nil, nil),
			},
			want: []string{"21/7 1 games", "21/7 2 places", "21/7 3 not_held", "21/7 5 not_on_cge"},
		},
		{
			// CGE's AI played on for cy and finished second, so CGE put ann third.
			name:  "places after an opponent timed out",
			games: []held{{t: 19, s: 4, seats: []seat{done("bob", 200, 2), done("ann", 150, 1), gone("cy", 3)}}},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(19, 4, "Wood 1", 1, "bob", 2, 1, 5, nil, []int{1, 0, 0}, nil),
				cgeStanding(19, 4, "Wood 1", 2, "ann", 1, 1, 0, nil, []int{0, 0, 1}, nil),
				cgeStanding(19, 4, "Wood 1", 3, "cy", 3, 1, 2, nil, []int{0, 1, 0}, nil),
			},
			want: []string{"19/4 1 places_after_a_timeout", "19/4 3 places_after_a_timeout"},
		},
		{
			name:  "a tie shares second place",
			games: []held{{t: 19, s: 3, seats: []seat{done("ann", 200, 1), done("bob", 150, 2), done("cy", 150, 3)}}},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 1, 5, nil, []int{1, 0, 0}, nil),
				cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 1, nil, []int{0, 1, 0}, nil),
				cgeStanding(19, 3, "Gold 1", 3, "cy", 3, 1, 1, nil, []int{0, 1, 0}, nil),
			},
		},
		{
			name:  "a player CGE lists with no games is no difference",
			games: []held{{t: 19, s: 3, seats: []seat{done("ann", 200, 1), done("bob", 150, 2)}}},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
				cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
				cgeStanding(19, 3, "Gold 1", 3, "quit", 3, 0, 0, nil, nil, nil),
			},
		},
		{
			name:  "CGE's zero lines are sizes not played",
			games: []held{{t: 19, s: 17, seats: []seat{done("ann", 200, 1), done("bob", 150, 2), done("cy", 100, 3)}}},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(19, 17, "Gold 1", 1, "ann", 1, 1, 5, []int{0, 0}, []int{1, 0, 0}, nil),
				cgeStanding(19, 17, "Gold 1", 2, "bob", 2, 1, 2, []int{0, 0}, []int{0, 1, 0}, []int{0, 0, 0, 0}),
				cgeStanding(19, 17, "Gold 1", 3, "cy", 3, 1, 0, nil, []int{0, 0, 1}, nil),
			},
		},
		{
			// CGE counts the game, and places its seats as its AI finished them.
			name: "a game no one played, counted as CGE does",
			games: []held{
				{t: 19, s: 3, seats: []seat{done("ann", 200, 1), done("bob", 150, 2)}},
				{t: 19, s: 3, seats: []seat{gone("ann", 1), gone("bob", 2)}, never: true},
			},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 2, 4, []int{1, 1}, nil, nil),
				cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 2, 4, []int{1, 1}, nil, nil),
			},
			want: []string{"19/3 1 places_after_a_timeout", "19/3 2 places_after_a_timeout"},
		},
		{
			name: "a CGE count without the game no one played",
			games: []held{
				{t: 4369, s: 1, seats: []seat{done("ann", 200, 1), done("bob", 150, 2)}},
				{t: 4369, s: 1, seats: []seat{gone("ann", 1), gone("bob", 2)}, never: true},
			},
			lines: []tournamentcsv.CGEStanding{
				cgeStanding(4369, 1, "group 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
				cgeStanding(4369, 1, "group 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			for _, g := range tt.games {
				heldGame(t, pool, g.t, g.s, g.seats...)
				if g.never {
					if _, err := pool.Exec(context.Background(), "UPDATE games SET state = 'never_played' WHERE game_id = (SELECT max(game_id) FROM games)"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if d := differences(checked(t, pool, tt.lines...)); !slices.Equal(d, tt.want) {
				t.Errorf("differences = %q, want %q", d, tt.want)
			}
		})
	}
}

func TestCheckStandingsCountsPlayersAndAgreements(t *testing.T) {
	pool := migrated(t)
	heldGame(t, pool, 21, 7, done("ann", 200, 1), done("bob", 150, 2))
	heldGame(t, pool, 21, 7, done("dee", 120, 4), done("eve", 110, 5))
	heldGame(t, pool, 19, 3, done("ann", 200, 1), done("bob", 150, 2))
	got := checked(t, pool,
		cgeStanding(21, 7, "Bronze 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(21, 7, "Bronze 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
		cgeStanding(21, 7, "Bronze 1", 3, "cy", 3, 2, 0, []int{0, 2}, nil, nil),
		cgeStanding(21, 7, "Bronze 2", 1, "dee", 4, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 2, "bob", 2, 1, 0, []int{0, 1}, nil, nil),
		cgeStanding(19, 3, "Gold 1", 3, "quit", 3, 0, 0, nil, nil, nil),
		cgeStanding(5600, 1, "group 1", 1, "ann", 1, 1, 4, []int{1, 0}, nil, nil),
	)
	var stages []string
	for _, st := range got.Stages {
		stages = append(stages, fmt.Sprintf("%d/%d %d players %d agree", st.CGETournament, st.CGESeason, st.Players, st.Agree))
	}
	// 21/7: ann, bob, dee agree; cy is only on CGE, eve only here. 19/3: quit played nothing. 5600/1 is only on CGE.
	want := []string{"19/3 2 players 2 agree", "21/7 5 players 3 agree"}
	if !slices.Equal(stages, want) || got.StagesOnlyOnCGE != 1 {
		t.Errorf("stages = %q, only on CGE %d; want %q and 1", stages, got.StagesOnlyOnCGE, want)
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
