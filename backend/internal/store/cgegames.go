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

// SeasonLink counts the games of one CGE stage that matched games of one of our seasons.
type SeasonLink struct {
	CGETournament, CGESeason int
	Season                   string
	Games                    int
}

// Attached counts what AttachCGEGames did: games matched, never-played games
// set apart, the CGE games no game of ours matched, and where matched ones landed.
type Attached struct {
	Matched       int
	Renamed       int
	LacksTimedOut int
	NeverPlayed   int
	Cleared       int
	Unmatched     []cgegames.Game
	Seasons       []SeasonLink
}

// ours is one of our games as AttachCGEGames compares it.
type ours struct {
	id      int64
	season  string
	date    time.Time
	players []string
	scores  []float64
	used    bool
}

// scoresKey is a game's seat count and its positive (finished) scores, in any order.
func scoresKey(scores []float64) string {
	parts := []string{fmt.Sprint(len(scores))}
	for _, s := range scores {
		if s > 0 {
			parts = append(parts, fmt.Sprintf("%g", s))
		}
	}
	slices.Sort(parts[1:])
	return strings.Join(parts, "|")
}

// seatFits reports whether our score is CGE seat s's: the same for a finished
// seat, and any score not above 0 for a resigned or timed-out one (older match
// files score those -1 and 0 where CGE's convention is -5 and minus the place).
func seatFits(ours float64, s cgegames.Seat) bool {
	if s.Outcome == cgegames.Finished {
		return ours == s.Score
	}
	return ours <= 0
}

// cgeScores returns a CGE game's seat scores.
func cgeScores(g cgegames.Game) []float64 {
	scores := make([]float64, len(g.Seats))
	for i, s := range g.Seats {
		scores[i] = s.Score
	}
	return scores
}

// renamedSeat returns, when our game has CGE game g's scores seat for seat
// and all its players but one, the CGE seat whose name differs and our seat
// it is; ok is false otherwise.
func (o *ours) renamedSeat(g cgegames.Game) (cgeSeat, ourSeat int, ok bool) {
	if o.used || len(o.players) != len(g.Seats) {
		return 0, 0, false
	}
	cgeSeat, ourSeat = -1, -1
	for i, s := range g.Seats {
		j := slices.Index(o.players, s.Name)
		if j < 0 {
			if cgeSeat >= 0 {
				return 0, 0, false
			}
			cgeSeat = i
		} else if !seatFits(o.scores[j], s) {
			return 0, 0, false
		}
	}
	for j, p := range o.players {
		if !slices.ContainsFunc(g.Seats, func(s cgegames.Seat) bool { return s.Name == p }) {
			ourSeat = j
		}
	}
	if cgeSeat < 0 || ourSeat < 0 || !seatFits(o.scores[ourSeat], g.Seats[cgeSeat]) {
		return 0, 0, false
	}
	return cgeSeat, ourSeat, true
}

// playersKey is a game's players, in any order.
func playersKey(names []string) string {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x01")
}

// fits reports whether our game has CGE game g's players, each seat fitting (seatFits).
func (o *ours) fits(g cgegames.Game) bool {
	for _, s := range g.Seats {
		i := slices.Index(o.players, s.Name)
		if i < 0 || !seatFits(o.scores[i], s) {
			return false
		}
	}
	return true
}

// AttachCGEGames records CGE's facts on the games already loaded, in one
// transaction: each seat's CGE user and how it ended, and the game's CGE
// tournament, stage, finish date and replay code, and its group and number
// where ours lacks them. A CGE game matches a game of the same tournament with
// the same players whose seats fit (ours.fits), the nearest by day if several do,
// or failing that, its scores and all its players but one (one renamed since),
// or failing that, its players without the timed-out ones (our match file left them out). Scores and
// dates the ratings use are not changed. Never-played games are counted apart.
func AttachCGEGames(ctx context.Context, pool *pgxpool.Pool, games []cgegames.Game) (Attached, error) {
	var slugs []string
	for _, g := range games {
		if !slices.Contains(slugs, g.Tournament) {
			slugs = append(slugs, g.Tournament)
		}
	}
	rows, err := pool.Query(ctx, `
		SELECT m.match_id, t.slug, s.label, m.date,
		       array_remove(ARRAY[m.player1, m.player2, m.player3, m.player4], NULL),
		       array_remove(ARRAY[m.score1, m.score2, m.score3, m.score4], NULL)
		FROM matches m
		JOIN tournament_stages st USING (stage_id)
		JOIN tournament_seasons s USING (season_id)
		JOIN tournaments t USING (tournament_id)
		WHERE t.slug = ANY($1)
		ORDER BY m.match_id`, slugs)
	if err != nil {
		return Attached{}, fmt.Errorf("reading our games: %w", err)
	}
	byKey, byScores := map[string][]*ours{}, map[string][]*ours{}
	for rows.Next() {
		var o ours
		var slug string
		if err := rows.Scan(&o.id, &slug, &o.season, &o.date, &o.players, &o.scores); err != nil {
			return Attached{}, fmt.Errorf("reading our games: %w", err)
		}
		k := slug + "\x02" + playersKey(o.players)
		byKey[k] = append(byKey[k], &o)
		byScores[slug+"\x02"+scoresKey(o.scores)] = append(byScores[slug+"\x02"+scoresKey(o.scores)], &o)
	}
	if err := rows.Err(); err != nil {
		return Attached{}, fmt.Errorf("reading our games: %w", err)
	}

	var got Attached
	links := map[SeasonLink]int{}
	var ids []int64
	var cgeT, cgeS, nums []int32
	var finished []*time.Time
	var codes, groups []string
	var seatMatch []int64
	var seatNo, seatUser []int32
	var seatName, seatCGEName, seatOutcome []string
	var seatBanned []bool
	// Exact matches first, then renames and left-out timeouts among the games still unmatched,
	// so a coincidence cannot take a game another CGE game matches exactly.
	pending := games
	for pass := 0; pass < 2; pass++ {
		var later []cgegames.Game
		for _, g := range pending {
			if g.Unplayed {
				if pass == 0 {
					got.NeverPlayed++
				}
				continue
			}
			names := make([]string, len(g.Seats))
			for i, s := range g.Seats {
				names[i] = s.Name
			}
			o := nearest(byKey[g.Tournament+"\x02"+playersKey(names)], g)
			renamed, ourSeat := -1, -1
			if o == nil && pass == 0 {
				later = append(later, g)
				continue
			}
			if o == nil {
				// One player renamed (or the account deleted) since: the same game under all but one name, and every score.
				for _, c := range byScores[g.Tournament+"\x02"+scoresKey(cgeScores(g))] {
					// Only in the season the CGE game is placed in: a coincidence elsewhere is not a rename.
					if g.Season == "" || c.season != g.Season {
						continue
					}
					if cs, os, ok := c.renamedSeat(g); ok && (o == nil || g.Finished != nil && c.date.Sub(*g.Finished).Abs() < o.date.Sub(*g.Finished).Abs()) {
						o, renamed, ourSeat = c, cs, os
					}
				}
				if o == nil {
					// Our match file may have left out a player who timed out: the game without its timed-out seats.
					if short := slices.DeleteFunc(slices.Clone(g.Seats), func(s cgegames.Seat) bool { return s.Outcome == cgegames.TimedOut }); len(short) >= 2 && len(short) < len(g.Seats) {
						trimmed := g
						trimmed.Seats = short
						if o = nearest(byKey[g.Tournament+"\x02"+playersKey(seatNames(trimmed))], trimmed); o != nil {
							g = trimmed
							got.LacksTimedOut++
						}
					}
				} else {
					got.Renamed++
				}
				if o == nil {
					got.Unmatched = append(got.Unmatched, g)
					continue
				}
			}
			o.used = true
			got.Matched++
			links[SeasonLink{CGETournament: g.CGETournament, CGESeason: g.CGESeason, Season: o.season}]++
			ids, cgeT, cgeS, nums = append(ids, o.id), append(cgeT, int32(g.CGETournament)), append(cgeS, int32(g.CGESeason)), append(nums, int32(g.Number))
			finished, codes, groups = append(finished, g.Finished), append(codes, g.Code), append(groups, g.Group)
			for i, s := range g.Seats {
				j := slices.Index(o.players, s.Name)
				if i == renamed {
					j = ourSeat
				}
				seatMatch, seatNo = append(seatMatch, o.id), append(seatNo, int32(j+1))
				seatName, seatCGEName = append(seatName, o.players[j]), append(seatCGEName, s.Name)
				seatOutcome, seatBanned, seatUser = append(seatOutcome, string(s.Outcome)), append(seatBanned, s.Banned), append(seatUser, int32(s.CGEUser))
			}
		}
		pending = later
	}
	for l, n := range links {
		l.Games = n
		got.Seasons = append(got.Seasons, l)
	}
	slices.SortFunc(got.Seasons, func(a, b SeasonLink) int {
		return cmpInts(a.CGETournament, b.CGETournament, a.CGESeason, b.CGESeason)
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Attached{}, fmt.Errorf("starting to attach CGE's games: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM match_seats WHERE match_id = ANY($1)", ids); err != nil {
		return Attached{}, fmt.Errorf("clearing earlier seats: %w", err)
	}
	// A game of ours matched before but not now keeps no CGE facts, nor a replay code that came from CGE.
	var allCodes []string
	for _, g := range games {
		if g.Code != "" {
			allCodes = append(allCodes, g.Code)
		}
	}
	if err := tx.QueryRow(ctx, `
		WITH stale AS (
			UPDATE matches m SET cge_tournament = NULL, cge_season = NULL, cge_finished = NULL,
			       replay_code = CASE WHEN m.replay_code = ANY(coalesce($3::text[], '{}')) THEN NULL ELSE m.replay_code END
			FROM tournament_stages st
			JOIN tournament_seasons s USING (season_id)
			JOIN tournaments t USING (tournament_id)
			WHERE m.stage_id = st.stage_id AND t.slug = ANY($1) AND m.cge_tournament IS NOT NULL
			  AND m.source_file <> 'cge' AND NOT (m.match_id = ANY(coalesce($2::bigint[], '{}')))
			RETURNING m.match_id),
		gone AS (DELETE FROM match_seats WHERE match_id IN (SELECT match_id FROM stale))
		SELECT count(*) FROM stale`, slugs, ids, allCodes).Scan(&got.Cleared); err != nil {
		return Attached{}, fmt.Errorf("clearing matches that no longer hold: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO match_seats (match_id, seat, player_name, cge_name, outcome, cge_user, banned)
		SELECT x.m, x.n, x.name, x.cge, x.outcome, nullif(x.u, 0), x.banned
		FROM unnest($1::bigint[], $2::int[], $3::text[], $4::text[], $5::text[], $6::int[], $7::bool[]) AS x(m, n, name, cge, outcome, u, banned)`,
		seatMatch, seatNo, seatName, seatCGEName, seatOutcome, seatUser, seatBanned); err != nil {
		return Attached{}, fmt.Errorf("writing seats: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE matches m SET
			cge_tournament = x.t, cge_season = x.s, cge_finished = x.f,
			replay_code = coalesce(nullif(x.code, ''), m.replay_code),
			group_label = CASE WHEN m.group_label = '' THEN x.grp ELSE m.group_label END,
			game_number = coalesce(m.game_number, x.num)
		FROM unnest($1::bigint[], $2::int[], $3::int[], $4::date[], $5::text[], $6::text[], $7::int[])
		     AS x(id, t, s, f, code, grp, num)
		WHERE m.match_id = x.id`,
		ids, cgeT, cgeS, finished, codes, groups, nums); err != nil {
		return Attached{}, fmt.Errorf("recording CGE's facts on games: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Attached{}, fmt.Errorf("committing CGE's games: %w", err)
	}
	return got, nil
}

// nearest returns the unused game that fits g played closest to g's finish,
// the first if g has none, or nil.
func nearest(candidates []*ours, g cgegames.Game) *ours {
	var best *ours
	for _, o := range candidates {
		if o.used || !o.fits(g) || g.Season != "" && o.season != g.Season {
			continue
		}
		if best == nil || (g.Finished != nil && o.date.Sub(*g.Finished).Abs() < best.date.Sub(*g.Finished).Abs()) {
			best = o
		}
	}
	return best
}

// cmpInts orders by the first pair of values, then the second.
func cmpInts(a1, b1, a2, b2 int) int {
	if a1 != b1 {
		return a1 - b1
	}
	return a2 - b2
}
