// The standings check: CGE's own standings tables against the games we hold, player by player and stage by stage.

package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

// ReplaceCGEStandings replaces every CGE standings row, in one transaction.
// Players are kept as CGE names them, players here or not.
func ReplaceCGEStandings(ctx context.Context, pool *pgxpool.Pool, rows []tournamentcsv.CGEStanding) (FactsReplaced, error) {
	var groups, players, p2, p3, p4, sources []string
	var cgeT, cgeS, rowNumbers, places, users, games []int32
	var banned []bool
	var points []float64
	for _, r := range rows {
		cgeT, cgeS, groups = append(cgeT, int32(r.CGETournament)), append(cgeS, int32(r.CGESeason)), append(groups, r.Group)
		rowNumbers, places, players = append(rowNumbers, int32(r.Row)), append(places, int32(r.Place)), append(players, r.Player)
		users, banned, games = append(users, int32(r.CGEUser)), append(banned, r.Banned), append(games, int32(r.Games))
		points, sources = append(points, r.Points), append(sources, r.Source)
		p2, p3, p4 = append(p2, spaced(r.Places2p)), append(p3, spaced(r.Places3p)), append(p4, spaced(r.Places4p))
	}
	n, err := replaceAll(ctx, pool, "cge_standings", `
		INSERT INTO cge_standings (cge_tournament, cge_season, group_name, row_number, place, player_name, cge_user,
		                           banned, games, points, places_2p, places_3p, places_4p, source)
		SELECT t, s, g, r, nullif(p, 0), n, u, b, gm, pt,
		       string_to_array(nullif(p2, ''), ' ')::int[], string_to_array(nullif(p3, ''), ' ')::int[],
		       string_to_array(nullif(p4, ''), ' ')::int[], src
		FROM unnest($1::int[], $2::int[], $3::text[], $4::int[], $5::int[], $6::text[], $7::int[], $8::bool[],
		            $9::int[], $10::float8[], $11::text[], $12::text[], $13::text[], $14::text[])
		     AS x(t, s, g, r, p, n, u, b, gm, pt, p2, p3, p4, src)`,
		cgeT, cgeS, groups, rowNumbers, places, players, users, banned, games, points, p2, p3, p4, sources)
	return FactsReplaced{Written: n}, err
}

// spaced writes counts apart by spaces, "" for none.
func spaced(counts []int) string {
	s := ""
	for i, c := range counts {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprint(c)
	}
	return s
}

// Places counts a player's firsts, seconds and so on in games of 2, 3 and 4 players; nil for a size not played.
type Places struct {
	P2, P3, P4 []int
}

func (p Places) equal(q Places) bool {
	return slices.Equal(p.P2, q.P2) && slices.Equal(p.P3, q.P3) && slices.Equal(p.P4, q.P4)
}

// The kinds of difference between CGE's table and the games we hold.
const (
	DiffGames         = "games"                  // a different number of games, with or without those no one played
	DiffPlaces        = "places"                 // the same games, finished in other places
	DiffPlacesTimeout = "places_after_a_timeout" // as places, in a stage where one of their games had a timeout
	DiffNotHeld       = "not_held"               // CGE lists games of theirs; we hold none
	DiffNotOnCGE      = "not_on_cge"             // we hold games of theirs; CGE's table lacks them
)

// StandingDifference is one player whose line in a stage's CGE table (all their groups added up) and games here differ.
// HeldGames counts every game held, HeldNeverPlayed those no one played: some CGE tables count them, some do not.
type StandingDifference struct {
	CGEUser                              int
	Player                               string
	Kind                                 string
	CGEGames, HeldGames, HeldNeverPlayed int
	CGEPoints                            float64
	CGEPlaces, HeldPlaces                Places
}

// StageCheck is one CGE stage both sides hold: how many players either lists, how many agree, and the rest.
type StageCheck struct {
	CGETournament, CGESeason int
	Name                     string
	Players, Agree           int
	Differences              []StandingDifference
}

// StandingsCheck is every stage held on both sides, in CGE's order, and how many CGE lists that we hold no game of.
type StandingsCheck struct {
	Stages          []StageCheck
	StagesOnlyOnCGE int
}

// CheckStandings compares each player's games and places in every CGE stage
// we hold games of with CGE's standings table for it. A player CGE lists with
// no game is left out. Tied players share the better place.
func (a *AdminStore) CheckStandings(ctx context.Context) (StandingsCheck, error) {
	rows, err := a.pool.Query(ctx, standingsCheckQuery)
	if err != nil {
		return StandingsCheck{}, fmt.Errorf("checking the standings: %w", err)
	}
	var got StandingsCheck
	for rows.Next() {
		var t, s, user int
		var name, player string
		var onCGE, held, timeout bool
		var cgeGames, heldGames, heldPlayed int
		var cgePoints float64
		var cge, ours Places
		if err := rows.Scan(&t, &s, &name, &user, &player, &onCGE, &cgeGames, &cgePoints, &cge.P2, &cge.P3, &cge.P4,
			&held, &heldGames, &heldPlayed, &ours.P2, &ours.P3, &ours.P4, &timeout); err != nil {
			return StandingsCheck{}, fmt.Errorf("checking the standings: %w", err)
		}
		if n := len(got.Stages); n == 0 || got.Stages[n-1].CGETournament != t || got.Stages[n-1].CGESeason != s {
			got.Stages = append(got.Stages, StageCheck{CGETournament: t, CGESeason: s, Name: name})
		}
		st := &got.Stages[len(got.Stages)-1]
		st.Players++
		kind := differenceKind(onCGE, held, cgeGames != heldGames && cgeGames != heldPlayed, cge, ours, timeout)
		if kind == "" {
			st.Agree++
			continue
		}
		st.Differences = append(st.Differences, StandingDifference{
			CGEUser: user, Player: player, Kind: kind, CGEGames: cgeGames, HeldGames: heldGames, HeldNeverPlayed: heldGames - heldPlayed,
			CGEPoints: cgePoints, CGEPlaces: cge, HeldPlaces: ours,
		})
	}
	if err := rows.Err(); err != nil {
		return StandingsCheck{}, fmt.Errorf("checking the standings: %w", err)
	}
	if err := a.pool.QueryRow(ctx, `
		SELECT count(*) FROM (SELECT DISTINCT cge_tournament, cge_season FROM cge_standings) c
		WHERE NOT EXISTS (SELECT 1 FROM groups gr JOIN games m USING (group_id)
		                  WHERE (gr.cge_tournament, gr.cge_stage_number) = (c.cge_tournament, c.cge_season))`).Scan(&got.StagesOnlyOnCGE); err != nil {
		return StandingsCheck{}, fmt.Errorf("counting the stages only CGE holds: %w", err)
	}
	return got, nil
}

// differenceKind returns how a player's CGE line and games here differ, or "" when they agree.
func differenceKind(onCGE, held, otherGames bool, cge, ours Places, timeout bool) string {
	switch {
	case !held:
		return DiffNotHeld
	case !onCGE:
		return DiffNotOnCGE
	case otherGames:
		return DiffGames
	case !cge.equal(ours) && timeout:
		return DiffPlacesTimeout
	case !cge.equal(ours):
		return DiffPlaces
	}
	return ""
}

// standingsCheckQuery lines up each player's CGE line (their groups added up) with their games here, in the stages
// both hold, ordered by stage and CGE user. Places come from played games only: CGE's AI placed the rest. A size CGE
// shows only zeros for was not played.
const standingsCheckQuery = `
WITH ours AS (
	SELECT gr.cge_tournament AS t, gr.cge_stage_number AS s, m.game_id,
	       k.n, x.cge_user, x.player_name,
	       CASE WHEN m.state = 'played' THEN x.place END AS place,
	       m.state <> 'played' OR k.timed_out AS timeout
	FROM games m JOIN groups gr USING (group_id) JOIN seats x USING (game_id)
	-- Each game's seats counted once, not once per mention of n.
	JOIN (SELECT game_id, count(*) AS n, bool_or(outcome = 'timed_out') AS timed_out FROM seats GROUP BY game_id) k USING (game_id)
	WHERE gr.cge_tournament IS NOT NULL AND x.cge_user IS NOT NULL
),
held AS (
	SELECT t, s, cge_user, min(player_name) AS player_name, count(*) AS games, count(place) AS played, bool_or(timeout) AS timeout,
	       CASE WHEN bool_or(n = 2 AND place IS NOT NULL) THEN ARRAY[count(*) FILTER (WHERE n = 2 AND place = 1), count(*) FILTER (WHERE n = 2 AND place = 2)] END AS p2,
	       CASE WHEN bool_or(n = 3 AND place IS NOT NULL) THEN ARRAY[count(*) FILTER (WHERE n = 3 AND place = 1), count(*) FILTER (WHERE n = 3 AND place = 2),
	                                           count(*) FILTER (WHERE n = 3 AND place = 3)] END AS p3,
	       CASE WHEN bool_or(n = 4 AND place IS NOT NULL) THEN ARRAY[count(*) FILTER (WHERE n = 4 AND place = 1), count(*) FILTER (WHERE n = 4 AND place = 2),
	                                           count(*) FILTER (WHERE n = 4 AND place = 3), count(*) FILTER (WHERE n = 4 AND place = 4)] END AS p4
	FROM ours GROUP BY t, s, cge_user
),
cge AS (
	SELECT cge_tournament AS t, cge_season AS s, cge_user, min(player_name) AS player_name, sum(games)::int AS games, sum(points) AS points,
	       CASE WHEN sum(places_2p[1] + places_2p[2]) > 0 THEN ARRAY[sum(places_2p[1]), sum(places_2p[2])]::int[] END AS p2,
	       CASE WHEN sum(places_3p[1] + places_3p[2] + places_3p[3]) > 0 THEN ARRAY[sum(places_3p[1]), sum(places_3p[2]), sum(places_3p[3])]::int[] END AS p3,
	       CASE WHEN sum(places_4p[1] + places_4p[2] + places_4p[3] + places_4p[4]) > 0 THEN ARRAY[sum(places_4p[1]), sum(places_4p[2]), sum(places_4p[3]), sum(places_4p[4])]::int[] END AS p4
	FROM cge_standings GROUP BY cge_tournament, cge_season, cge_user HAVING sum(games) > 0
)
SELECT t, s, coalesce(ct.name, ''), cge_user, coalesce(c.player_name, h.player_name),
       c.cge_user IS NOT NULL, coalesce(c.games, 0), coalesce(c.points, 0), c.p2, c.p3, c.p4,
       h.cge_user IS NOT NULL, coalesce(h.games, 0), coalesce(h.played, 0), h.p2, h.p3, h.p4, coalesce(h.timeout, false)
FROM cge c FULL JOIN held h USING (t, s, cge_user)
LEFT JOIN cge_tournaments ct ON ct.cge_tournament = t
WHERE (t, s) IN (SELECT t, s FROM held) AND (t, s) IN (SELECT t, s FROM cge)
ORDER BY t, s, cge_user`
