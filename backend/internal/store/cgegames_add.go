package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgegames"
)

// Added counts what AddCGEGames did: games added (and how many of them never
// played), games already held, games whose placing is not settled, and the
// games it would not add because they may be copies of ours under other scores.
type Added struct {
	Added       int
	NeverPlayed int
	AlreadyHeld int
	Unsettled   int
	Conflicts   []cgegames.Game
}

// seasonKey names one of our seasons.
type seasonKey struct{ tournament, season string }

// heldSeason is what we hold of a season: its last game's day, each game's
// players, and each game's players by its scores.
type heldSeason struct {
	last     time.Time
	players  map[string]bool
	byScores map[string][][]string
}

// mayBeOurs reports whether one of the season's games has g's scores and all
// its players but one, or g's players without the ones who timed out.
func (h heldSeason) mayBeOurs(g cgegames.Game) bool {
	var present []string
	for _, s := range g.Seats {
		if s.Outcome != cgegames.TimedOut {
			present = append(present, s.Name)
		}
	}
	if len(present) >= 2 && h.players[playersKey(present)] {
		return true
	}
	names := seatNames(g)
	for _, ours := range h.byScores[scoresKey(cgeScores(g))] {
		shared := 0
		for _, n := range names {
			if slices.Contains(ours, n) {
				shared++
			}
		}
		if shared >= len(names)-1 {
			return true
		}
	}
	return false
}

// AddCGEGames adds the CGE games we do not hold (by replay code or CGE
// identity), in one transaction, placed and named as our match files would,
// with CGE's scores and seats. A played game is added only when it cannot be
// a copy of ours (its season holds none of ours, or it finished after our last
// one there and none of ours has its players, or its scores and all its
// players but one), or its code is in confirmed; else it is a conflict. A
// never-played game is added dated by its group's last game; one without a
// settled placing (no Name) never is.
func AddCGEGames(ctx context.Context, pool *pgxpool.Pool, games []cgegames.Game, confirmed map[string]bool) (Added, error) {
	codes, idents, err := heldCGEGames(ctx, pool)
	if err != nil {
		return Added{}, err
	}
	seasons, err := heldSeasons(ctx, pool, games)
	if err != nil {
		return Added{}, err
	}
	groupLast := map[string]time.Time{}
	for _, g := range games {
		k := fmt.Sprint(g.CGETournament, "/", g.CGESeason, "/", g.Group)
		if g.Finished != nil && g.Finished.After(groupLast[k]) {
			groupLast[k] = *g.Finished
		}
	}

	var got Added
	var add []cgegames.Game
	var days []time.Time
	for _, g := range games {
		ident := fmt.Sprint(g.CGETournament, "/", g.CGESeason, "/", g.Group, "/", g.Number)
		switch {
		case (g.Code != "" && codes[g.Code]) || idents[ident]:
			got.AlreadyHeld++
			continue
		case g.Name == "":
			got.Unsettled++
			continue
		}
		day := g.Finished
		if g.Unplayed {
			last, ok := groupLast[fmt.Sprint(g.CGETournament, "/", g.CGESeason, "/", g.Group)]
			if !ok {
				got.Unsettled++
				continue
			}
			day = &last
		} else if h, ok := seasons[seasonKey{g.Tournament, g.Season}]; ok && !confirmed[g.Code] && (!day.After(h.last) || h.players[playersKey(seatNames(g))] || h.mayBeOurs(g)) {
			got.Conflicts = append(got.Conflicts, g)
			continue
		}
		add, days = append(add, g), append(days, *day)
	}
	if len(add) == 0 {
		return got, nil
	}
	var slugs []string
	for _, g := range add {
		slugs = append(slugs, g.Tournament)
	}
	var missing []string
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(array_agg(DISTINCT x.slug ORDER BY x.slug), '{}') FROM unnest($1::text[]) AS x(slug)
		WHERE NOT EXISTS (SELECT 1 FROM tournaments t WHERE t.slug = x.slug)`, slugs).Scan(&missing); err != nil {
		return got, fmt.Errorf("checking the tournaments: %w", err)
	}
	if len(missing) > 0 {
		return got, fmt.Errorf("no tournament %s in the catalogue: load it first (-classify or -facts cge_tournaments)", strings.Join(missing, ", "))
	}
	n, never, err := insertCGEGames(ctx, pool, add, days)
	got.Added, got.NeverPlayed = n, never
	return got, err
}

func seatNames(g cgegames.Game) []string {
	names := make([]string, len(g.Seats))
	for i, s := range g.Seats {
		names[i] = s.Name
	}
	return names
}

// heldCGEGames returns the replay codes and CGE identities (tournament/season/group/game) of the games we hold.
func heldCGEGames(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, map[string]bool, error) {
	rows, err := pool.Query(ctx, `
		SELECT coalesce(replay_code, ''), coalesce(cge_tournament || '/' || cge_season || '/' || group_label || '/' || game_number, '')
		FROM matches WHERE replay_code IS NOT NULL OR cge_tournament IS NOT NULL`)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the CGE games we hold: %w", err)
	}
	defer rows.Close()
	codes, idents := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var code, ident string
		if err := rows.Scan(&code, &ident); err != nil {
			return nil, nil, fmt.Errorf("reading the CGE games we hold: %w", err)
		}
		codes[code], idents[ident] = code != "", ident != ""
	}
	return codes, idents, rows.Err()
}

// heldSeasons returns what we hold of each season the games are placed in.
func heldSeasons(ctx context.Context, pool *pgxpool.Pool, games []cgegames.Game) (map[seasonKey]heldSeason, error) {
	var slugs, labels []string
	for _, g := range games {
		slugs, labels = append(slugs, g.Tournament), append(labels, g.Season)
	}
	rows, err := pool.Query(ctx, `
		SELECT t.slug, s.label, m.date, array_remove(ARRAY[m.player1, m.player2, m.player3, m.player4], NULL),
		       array_remove(ARRAY[m.score1, m.score2, m.score3, m.score4], NULL)
		FROM matches m
		JOIN tournament_stages st USING (stage_id)
		JOIN tournament_seasons s USING (season_id)
		JOIN tournaments t USING (tournament_id)
		WHERE (t.slug, s.label) IN (SELECT * FROM unnest($1::text[], $2::text[])) AND m.state = 'played'`, slugs, labels)
	if err != nil {
		return nil, fmt.Errorf("reading the seasons we hold: %w", err)
	}
	defer rows.Close()
	got := map[seasonKey]heldSeason{}
	for rows.Next() {
		var k seasonKey
		var day time.Time
		var players []string
		var scores []float64
		if err := rows.Scan(&k.tournament, &k.season, &day, &players, &scores); err != nil {
			return nil, fmt.Errorf("reading the seasons we hold: %w", err)
		}
		h, ok := got[k]
		if !ok {
			h = heldSeason{players: map[string]bool{}, byScores: map[string][][]string{}}
		}
		h.byScores[scoresKey(scores)] = append(h.byScores[scoresKey(scores)], players)
		if day.After(h.last) {
			h.last = day
		}
		h.players[playersKey(players)] = true
		got[k] = h
	}
	return got, rows.Err()
}

// insertCGEGames writes the games, each on its day, with their seats, players,
// seasons and stages, in one transaction; it returns how many it wrote, and
// how many of those never played. A copy of a game held under the same scores
// is skipped by the one-copy rule.
func insertCGEGames(ctx context.Context, pool *pgxpool.Pool, games []cgegames.Game, days []time.Time) (int, int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("starting to add CGE's games: %w", err)
	}
	defer tx.Rollback(ctx)
	var batch int64
	if err := tx.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id").Scan(&batch); err != nil {
		return 0, 0, fmt.Errorf("starting an import batch: %w", err)
	}

	var slugs, seasons, stages, names, groups, codes, states []string
	var lines, counts, cgeT, cgeS, nums []int32
	var finished []*time.Time
	var players [4][]*string
	var scores [4][]*float64
	for i, g := range games {
		slugs, seasons, stages, names = append(slugs, g.Tournament), append(seasons, g.Season), append(stages, g.Stage), append(names, g.Name)
		groups, codes = append(groups, g.Group), append(codes, g.Code)
		state := "played"
		if g.Unplayed {
			state = "never_played"
		}
		states, finished = append(states, state), append(finished, g.Finished)
		lines, counts = append(lines, int32(i+1)), append(counts, int32(len(g.Seats)))
		cgeT, cgeS, nums = append(cgeT, int32(g.CGETournament)), append(cgeS, int32(g.CGESeason)), append(nums, int32(g.Number))
		for s := range 4 {
			if s < len(g.Seats) {
				players[s], scores[s] = append(players[s], &g.Seats[s].Name), append(scores[s], &g.Seats[s].Score)
			} else {
				players[s], scores[s] = append(players[s], nil), append(scores[s], nil)
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tournament_seasons (tournament_id, label)
		SELECT DISTINCT t.tournament_id, x.season FROM unnest($1::text[], $2::text[]) AS x(slug, season)
		JOIN tournaments t USING (slug)
		ON CONFLICT (tournament_id, label) DO NOTHING`, slugs, seasons); err != nil {
		return 0, 0, fmt.Errorf("adding seasons: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tournament_stages (season_id, label)
		SELECT DISTINCT s.season_id, x.stage FROM unnest($1::text[], $2::text[], $3::text[]) AS x(slug, season, stage)
		JOIN tournaments t USING (slug)
		JOIN tournament_seasons s ON s.tournament_id = t.tournament_id AND s.label = x.season
		ON CONFLICT (season_id, label) DO NOTHING`, slugs, seasons, stages); err != nil {
		return 0, 0, fmt.Errorf("adding stages: %w", err)
	}
	rows, err := tx.Query(ctx, `
		INSERT INTO matches (batch_id, source_file, source_line, tournament, date, player_count,
		                     player1, score1, player2, score2, player3, score3, player4, score4,
		                     replay_code, cge_tournament, cge_season, cge_finished, group_label, game_number, state, stage_id)
		SELECT $1, 'cge', x.line, x.name, x.day, x.n, x.p1, x.s1, x.p2, x.s2, x.p3, x.s3, x.p4, x.s4,
		       nullif(x.code, ''), x.t, x.s, x.f, x.grp, x.num, x.state, st.stage_id
		FROM unnest($2::int[], $3::text[], $4::date[], $5::int[], $6::text[], $7::float8[], $8::text[], $9::float8[],
		            $10::text[], $11::float8[], $12::text[], $13::float8[], $14::text[], $15::int[], $16::int[], $17::date[],
		            $18::text[], $19::int[], $20::text[], $21::text[], $22::text[], $23::text[])
		     AS x(line, name, day, n, p1, s1, p2, s2, p3, s3, p4, s4, code, t, s, f, grp, num, state, slug, season, stage)
		JOIN tournaments tt ON tt.slug = x.slug
		JOIN tournament_seasons ss ON ss.tournament_id = tt.tournament_id AND ss.label = x.season
		JOIN tournament_stages st ON st.season_id = ss.season_id AND st.label = x.stage
		ON CONFLICT ON CONSTRAINT matches_one_copy_per_game DO NOTHING
		RETURNING source_line, match_id, state`,
		batch, lines, names, days, counts, players[0], scores[0], players[1], scores[1], players[2], scores[2], players[3], scores[3],
		codes, cgeT, cgeS, finished, groups, nums, states, slugs, seasons, stages)
	if err != nil {
		return 0, 0, fmt.Errorf("adding games: %w", err)
	}
	ids := map[int32]int64{}
	never := 0
	for rows.Next() {
		var line int32
		var id int64
		var state string
		if err := rows.Scan(&line, &id, &state); err != nil {
			return 0, 0, fmt.Errorf("adding games: %w", err)
		}
		ids[line] = id
		if state == "never_played" {
			never++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("adding games: %w", err)
	}

	var seatMatch []int64
	var seatNo, seatUser []int32
	var seatName, seatOutcome []string
	var seatBanned []bool
	for i, g := range games {
		id, ok := ids[int32(i+1)]
		if !ok {
			continue
		}
		for n, s := range g.Seats {
			seatMatch, seatNo, seatUser = append(seatMatch, id), append(seatNo, int32(n+1)), append(seatUser, int32(s.CGEUser))
			seatName, seatOutcome, seatBanned = append(seatName, s.Name), append(seatOutcome, string(s.Outcome)), append(seatBanned, s.Banned)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO match_seats (match_id, seat, player_name, cge_name, outcome, cge_user, banned)
		SELECT x.m, x.n, x.name, x.name, x.outcome, nullif(x.u, 0), x.banned
		FROM unnest($1::bigint[], $2::int[], $3::text[], $4::text[], $5::int[], $6::bool[]) AS x(m, n, name, outcome, u, banned)`,
		seatMatch, seatNo, seatName, seatOutcome, seatUser, seatBanned); err != nil {
		return 0, 0, fmt.Errorf("writing the added games' seats: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO players (name) SELECT DISTINCT unnest($1::text[]) ORDER BY 1 ON CONFLICT (name) DO NOTHING`,
		slices.Compact(slices.Sorted(slices.Values(seatName)))); err != nil {
		return 0, 0, fmt.Errorf("adding the added games' players: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("committing CGE's games: %w", err)
	}
	return len(ids), never, nil
}
