//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

func division(season int, name, player string) tournamentcsv.Division {
	return tournamentcsv.Division{Series: "international", Season: season, Division: name, Player: player, Source: "sheet"}
}

func rowsOf(t *testing.T, pool *pgxpool.Pool, query string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return got
}

const divisionRows = "SELECT concat_ws('|', series, season, division, player_name, source) FROM tournament_group_divisions ORDER BY season, player_name"

// load is one call of a Replace function.
type load func(context.Context, *pgxpool.Pool) (FactsReplaced, error)

func TestReplaceFactsReplacesEarlierRowsAndCountsUnknowns(t *testing.T) {
	pts, seven, points := 25.0, 7, "8,5"
	finished := time.Date(2025, 2, 24, 0, 0, 0, 0, time.UTC)
	placement := func(season, place int, player string, points *float64) tournamentcsv.Placement {
		return tournamentcsv.Placement{Series: "intermezzo", Season: season, Division: "Grandmaster", Player: player,
			Place: place, Points: points, Reason: "no games", Source: "sheet"}
	}
	correction := func(named int, season *int) tournamentcsv.Correction {
		return tournamentcsv.Correction{Series: "intermezzo", NameSeason: named, Season: season, Reason: "r", Source: "s"}
	}
	finish := func(series string, season int, d string) tournamentcsv.FinishDate {
		return tournamentcsv.FinishDate{Series: series, Season: season, Date: day(d), Source: "s"}
	}
	podium := func(season string, place int, player string) tournamentcsv.Podium {
		return tournamentcsv.Podium{Cup: "wimbledon", Season: season, Finished: finished, Place: place, Player: player, Source: "s"}
	}
	communityPoints := func(tournament string, edition int, group string, place, points int) tournamentcsv.CommunityPoints {
		return tournamentcsv.CommunityPoints{Tournament: tournament, FromEdition: edition, Group: group, Place: place, Points: points, Source: "sheet"}
	}
	quarter := func(tournament, season string, year, q int) tournamentcsv.CommunityQuarter {
		return tournamentcsv.CommunityQuarter{Tournament: tournament, Season: season, Year: year, Quarter: q, Source: "tab"}
	}
	tests := []struct {
		name        string
		setup       []string
		first, then load
		want        FactsReplaced
		rows        string
		wantRows    []string
	}{
		{
			name: "divisions of known players",
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceDivisions(ctx, p, []tournamentcsv.Division{division(19, "Gold", "bob")})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceDivisions(ctx, p, []tournamentcsv.Division{
					division(20, "Grandmaster", "ann"), division(20, "Grandmaster", "ghost"), division(21, "Master", "bob")})
			},
			want:     FactsReplaced{Written: 2, Unknown: 1},
			rows:     divisionRows,
			wantRows: []string{"international|20|Grandmaster|ann|sheet", "international|21|Master|bob|sheet"},
		},
		{
			name: "placements of known players",
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplacePlacements(ctx, p, []tournamentcsv.Placement{placement(9, 1, "bob", nil)})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplacePlacements(ctx, p, []tournamentcsv.Placement{placement(24, 1, "ann", &pts), placement(24, 2, "ghost", nil), placement(31, 2, "bob", nil)})
			},
			want: FactsReplaced{Written: 2, Unknown: 1},
			rows: "SELECT concat_ws('|', series, season, division, player_name, place, coalesce(points::text, '-'), reason, source) " +
				"FROM tournament_official_placements ORDER BY season, place, player_name",
			wantRows: []string{"intermezzo|24|Grandmaster|ann|1|25|no games|sheet", "intermezzo|31|Grandmaster|bob|2|-|no games|sheet"},
		},
		{
			name: "season corrections",
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCorrections(ctx, p, []tournamentcsv.Correction{correction(9, &seven)})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCorrections(ctx, p, []tournamentcsv.Correction{correction(8, &seven), correction(7, nil)})
			},
			want: FactsReplaced{Written: 2},
			rows: "SELECT concat_ws('|', series, name_season, coalesce(season::text, '-'), reason, source) " +
				"FROM tournament_season_corrections ORDER BY name_season",
			wantRows: []string{"intermezzo|7|-|r|s", "intermezzo|8|7|r|s"},
		},
		{
			name: "finish dates",
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceFinishDates(ctx, p, []tournamentcsv.FinishDate{finish("intermezzo", 1, "2019-03-24")})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceFinishDates(ctx, p, []tournamentcsv.FinishDate{finish("international", 34, "2026-08-14"), finish("worlds", 2025, "2025-12-15")})
			},
			want:     FactsReplaced{Written: 2},
			rows:     "SELECT concat_ws('|', series, season, finish_date, source) FROM tournament_finish_dates ORDER BY series, season",
			wantRows: []string{"international|34|2026-08-14|s", "worlds|2025|2025-12-15|s"},
		},
		{
			name: "career honours of known players",
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceHonours(ctx, p, []tournamentcsv.Honour{
					{Series: "sodium_ladder", Player: "ann", Gold: 5, Silver: 2, Points: &points, Source: "s"},
					{Series: "q_and_d", Player: "Marten Fisher", Gold: 3, Source: "q"},
					{Series: "q_and_d", Player: "bob", Gold: 2, Silver: 2, Bronze: 1, Source: "q"},
				})
			},
			want: FactsReplaced{Written: 2, Unknown: 1},
			rows: "SELECT concat_ws('|', series, player_name, gold, silver, bronze, coalesce(points, '-'), source) " +
				"FROM tournament_career_honours ORDER BY series, player_name",
			wantRows: []string{"q_and_d|bob|2|2|1|-|q", "sodium_ladder|ann|5|2|0|8,5|s"},
		},
		{
			name: "cup podiums of known players",
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplacePodiums(ctx, p, []tournamentcsv.Podium{podium("2023", 1, "ann")})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplacePodiums(ctx, p, []tournamentcsv.Podium{podium("2024", 1, "ann"), podium("2024", 2, "bob"), podium("2024", 3, "nobody")})
			},
			want:     FactsReplaced{Written: 2, Unknown: 1},
			rows:     "SELECT concat_ws('|', cup, season, final, finished, place, player_name, source) FROM tournament_cup_podiums ORDER BY cup, season, final, place",
			wantRows: []string{"wimbledon|2024||2025-02-24|1|ann|s", "wimbledon|2024||2025-02-24|2|bob|s"},
		},
		{
			name: "community standings keep names that are not players",
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityStandings(ctx, p, []tournamentcsv.Standing{{Edition: "2024", SheetRow: 9, Rank: 9, Player: "old", Points: 1, Events: 1, Source: "s"}})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityStandings(ctx, p, []tournamentcsv.Standing{
					{Edition: "2024", SheetRow: 1, Rank: 1, Player: "ann", Points: 7657, Events: 11, Source: "s"},
					{Edition: "2024", SheetRow: 2, Rank: 2, Player: "Marten Fisher", Points: 6687, Events: 13, Source: "s"},
				})
			},
			want:     FactsReplaced{Written: 2},
			rows:     "SELECT concat_ws('|', edition, sheet_row, rank, player_name, points, events, source) FROM community_standings ORDER BY rank",
			wantRows: []string{"2024|1|1|ann|7657|11|s", "2024|2|2|Marten Fisher|6687|13|s"},
		},
		{
			name: "community scores keep names that are not players",
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityScores(ctx, p, []tournamentcsv.Score{
					{Edition: "2024", Player: "ann", Event: "wrld", Score: 1600, Source: "s"},
					{Edition: "2024", Player: "Marten Fisher", Event: "qd", Score: 450, Source: "s"},
				})
			},
			want:     FactsReplaced{Written: 2},
			rows:     "SELECT concat_ws('|', edition, player_name, event, score, source) FROM community_scores ORDER BY score",
			wantRows: []string{"2024|Marten Fisher|qd|450|s", "2024|ann|wrld|1600|s"},
		},
		{
			name:  "community points of known tournaments",
			setup: []string{"INSERT INTO series (slug, name, kind) VALUES ('international', 'International Championship', 'league')"},
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityPoints(ctx, p, []tournamentcsv.CommunityPoints{communityPoints("international", 2025, "Wood", 9, 1)})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityPoints(ctx, p, []tournamentcsv.CommunityPoints{
					communityPoints("international", 2025, "Grandmaster", 1, 1000),
					communityPoints("international", 2024, "Wood", 2, 15),
					communityPoints("snail_cup", 2025, "Final", 1, 1000),
				})
			},
			want:     FactsReplaced{Written: 2, Unknown: 1},
			rows:     "SELECT concat_ws('|', tournament, from_edition, group_name, place, points, source) FROM community_points ORDER BY from_edition",
			wantRows: []string{"international|2024|Wood|2|15|sheet", "international|2025|Grandmaster|1|1000|sheet"},
		},
		{
			name:  "community quarters of known tournaments",
			setup: []string{"INSERT INTO series (slug, name, kind) VALUES ('wimbledon', 'Wimbledon', 'cup')"},
			first: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityQuarters(ctx, p, []tournamentcsv.CommunityQuarter{quarter("wimbledon", "2020", 2020, 3)})
			},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityQuarters(ctx, p, []tournamentcsv.CommunityQuarter{quarter("wimbledon", "2025", 2026, 1), quarter("snail_cup", "2024", 2024, 4)})
			},
			want:     FactsReplaced{Written: 1, Unknown: 1},
			rows:     "SELECT concat_ws('|', tournament, season, year, quarter, source) FROM community_quarters",
			wantRows: []string{"wimbledon|2025|2026|1|tab"},
		},
		{
			name:  "community ladders of known tournaments",
			setup: []string{"INSERT INTO series (slug, name, kind) VALUES ('sodium_ladder', 'Sodium Ladder', 'ladder')"},
			then: func(ctx context.Context, p *pgxpool.Pool) (FactsReplaced, error) {
				return ReplaceCommunityLadders(ctx, p, []tournamentcsv.CommunityLadder{
					{Tournament: "sodium_ladder", FromEdition: 2025, Ratio: 0.8, HalfStepsFrom: 4, Shares: []float64{1, 0.5, 0.166, 0}, Rounds: 8, Source: "tab"},
					{Tournament: "mercurial_ladder", FromEdition: 2024, Ratio: 0.8, Shares: []float64{1, 0.4, 0}, Rounds: 10, Source: "tab"},
				})
			},
			want:     FactsReplaced{Written: 1, Unknown: 1},
			rows:     "SELECT concat_ws('|', tournament, from_edition, ratio, coalesce(half_steps_from::text, '-'), shares, rounds, source) FROM community_ladders",
			wantRows: []string{"sodium_ladder|2025|0.8|4|{1,0.5,0.166,0}|8|tab"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			addPlayers(t, pool, "ann", "bob")
			for _, q := range tt.setup {
				if _, err := pool.Exec(ctx, q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			if tt.first != nil {
				if _, err := tt.first(ctx, pool); err != nil {
					t.Fatalf("first load: %v", err)
				}
			}
			got, err := tt.then(ctx, pool)
			if err != nil {
				t.Fatalf("second load: %v", err)
			}
			if got != tt.want {
				t.Errorf("replaced = %+v, want %+v", got, tt.want)
			}
			if rows := rowsOf(t, pool, tt.rows); !slices.Equal(rows, tt.wantRows) {
				t.Errorf("rows = %q, want %q", rows, tt.wantRows)
			}
		})
	}
}

func TestReplaceDivisionsIsAllOrNothing(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	if _, err := ReplaceDivisions(ctx, pool, []tournamentcsv.Division{division(20, "Grandmaster", "ann")}); err != nil {
		t.Fatalf("first ReplaceDivisions: %v", err)
	}

	bad := []tournamentcsv.Division{division(21, "Gold", "bob"), division(0, "Gold", "ann")}
	if _, err := ReplaceDivisions(ctx, pool, bad); err == nil {
		t.Fatal("ReplaceDivisions with season 0: no error, want one")
	}
	if rows, want := rowsOf(t, pool, divisionRows), []string{"international|20|Grandmaster|ann|sheet"}; !slices.Equal(rows, want) {
		t.Errorf("divisions after a refused load = %q, want the first load's %q", rows, want)
	}
}

const ruleRows = `SELECT concat_ws('|', se.slug, t.label, s.timeout_penalty, coalesce(s.tiebreak_2p, ''), s.source)
	FROM settings s JOIN tournaments t ON t.tournament_id = s.tournament_id JOIN series se ON se.series_id = t.series_id
	ORDER BY se.slug, t.label`

func TestReplaceRulesReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S6 - Bronze 1 game 1": day("2020-05-01"),
		"Intermezzo S7 - Bronze 1 game 1": day("2020-08-01"),
		"Intermezzo S9 - Bronze 1 game 1": day("2021-02-01"),
	})
	rule := func(season int, penalty float64) tournamentcsv.Rule {
		return tournamentcsv.Rule{Series: "intermezzo", Season: season, TimeoutPenalty: penalty, Scoring3p: "5-2-0", Source: "s"}
	}
	if _, err := ReplaceRules(ctx, pool, []tournamentcsv.Rule{rule(9, 0)}); err != nil {
		t.Fatalf("first ReplaceRules: %v", err)
	}

	got, err := ReplaceRules(ctx, pool, []tournamentcsv.Rule{rule(6, -1), rule(7, 0), rule(8, 0)})
	if err != nil {
		t.Fatalf("ReplaceRules: %v", err)
	}
	if want := (FactsReplaced{Written: 2, Unknown: 1}); got != want {
		t.Errorf("ReplaceRules = %+v, want %+v (season 8 has no games)", got, want)
	}
	if rows, want := rowsOf(t, pool, ruleRows), []string{"intermezzo|6|-1||s", "intermezzo|7|0||s"}; !slices.Equal(rows, want) {
		t.Errorf("rules = %q, want %q", rows, want)
	}
}

func TestReplaceRulesKeepsAPenaltySetInAdmin(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S6 - Bronze 1 game 1": day("2020-05-01"),
		"Intermezzo S7 - Bronze 1 game 1": day("2020-08-01"),
	})
	if err := NewAdminStore(asAdminLogin(t, pool)).SetSeasonPenalty(ctx, "intermezzo", "6", -2); err != nil {
		t.Fatalf("SetSeasonPenalty: %v", err)
	}
	rule := func(season int, penalty float64) tournamentcsv.Rule {
		return tournamentcsv.Rule{Series: "intermezzo", Season: season, TimeoutPenalty: penalty, Scoring3p: "5-2-0", Source: "s"}
	}

	got, err := ReplaceRules(ctx, pool, []tournamentcsv.Rule{rule(6, -1), rule(7, 0)})
	if err != nil {
		t.Fatalf("ReplaceRules: %v", err)
	}
	if want := (FactsReplaced{Written: 1, Kept: 1}); got != want {
		t.Errorf("ReplaceRules = %+v, want %+v", got, want)
	}
	if rows, want := rowsOf(t, pool, ruleRows), []string{"intermezzo|6|-2||admin", "intermezzo|7|0||s"}; !slices.Equal(rows, want) {
		t.Errorf("rules = %q, want %q", rows, want)
	}
}

func TestReplaceCGETournamentsLinksOursAndRatesNoNewOne(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	loadNamed(t, pool, "Survivors Cup 2026 Stage 1 - group 1 game 1")
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	finished := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	cge := func(id int, name, tournament, season string) tournamentcsv.CGETournament {
		return tournamentcsv.CGETournament{ID: id, Name: name, Status: "finished", Finished: &finished, Tournament: tournament, Season: season}
	}
	if _, err := ReplaceCGETournaments(ctx, pool, []tournamentcsv.CGETournament{cge(1, "Old", "", "")}); err != nil {
		t.Fatalf("first ReplaceCGETournaments: %v", err)
	}

	got, err := ReplaceCGETournaments(ctx, pool, []tournamentcsv.CGETournament{
		cge(5731, "Survivors Cup 2026", "survivors_cup", "2026"),
		cge(5373, "Nations Cup", "nations_cup", "2025"),
		cge(4335, "The banana tournament", "", ""),
	})
	if err != nil {
		t.Fatalf("ReplaceCGETournaments: %v", err)
	}
	if got != (FactsReplaced{Written: 3}) {
		t.Errorf("ReplaceCGETournaments = %+v, want 3 written", got)
	}
	rows := rowsOf(t, pool, `SELECT concat_ws('|', c.cge_tournament, c.name, coalesce(se.slug, '-'), coalesce(t.label, ''), c.finished)
		FROM cge_tournaments c LEFT JOIN series se ON se.series_id = c.series_id
		LEFT JOIN tournaments t ON t.tournament_id = c.tournament_id ORDER BY c.cge_tournament`)
	want := []string{"4335|The banana tournament|-||2026-09-03", "5373|Nations Cup|nations_cup|2025|2026-09-03", "5731|Survivors Cup 2026|survivors_cup|2026|2026-09-03"}
	if !slices.Equal(rows, want) {
		t.Errorf("cge_tournaments = %q, want %q", rows, want)
	}
	if name := rowsOf(t, pool, "SELECT name FROM series WHERE slug = 'nations_cup'"); !slices.Equal(name, []string{"Nations Cup"}) {
		t.Errorf("nations_cup is named %q, want its CGE name", name)
	}
	if rated := rowsOf(t, pool, communityRows); slices.Contains(rated, "nations_cup") || !slices.Contains(rated, "survivors_cup") {
		t.Errorf("community ranking = %q: want survivors_cup, and nations_cup left for admin", rated)
	}
}
