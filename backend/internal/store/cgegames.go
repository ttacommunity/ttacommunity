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
	Matched     int
	NeverPlayed int
	Unmatched   []cgegames.Game
	Seasons     []SeasonLink
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

// playersKey is a game's players, in any order.
func playersKey(names []string) string {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x01")
}

// fits reports whether our game has CGE game g's seats: a finished seat's
// score exactly, and a resigned or timed-out one at any score not above 0,
// since older match files score those -1 and 0 where CGE's convention is -5
// and minus the place.
func (o *ours) fits(g cgegames.Game) bool {
	for _, s := range g.Seats {
		i := slices.Index(o.players, s.Name)
		if i < 0 || (s.Outcome == cgegames.Finished && o.scores[i] != s.Score) || (s.Outcome != cgegames.Finished && o.scores[i] > 0) {
			return false
		}
	}
	return true
}

// AttachCGEGames records CGE's facts on the games already loaded, in one
// transaction: each seat's CGE user and how it ended, and the game's CGE
// tournament, stage, finish date and replay code, and its group and number
// where ours lacks them. A CGE game matches a game of the same tournament with
// the same players whose seats fit (ours.fits), the nearest by day if several do. Scores and
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
	byKey := map[string][]*ours{}
	for rows.Next() {
		var o ours
		var slug string
		if err := rows.Scan(&o.id, &slug, &o.season, &o.date, &o.players, &o.scores); err != nil {
			return Attached{}, fmt.Errorf("reading our games: %w", err)
		}
		k := slug + "\x02" + playersKey(o.players)
		byKey[k] = append(byKey[k], &o)
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
	var seatName, seatOutcome []string
	var seatBanned []bool
	for _, g := range games {
		if g.Unplayed {
			got.NeverPlayed++
			continue
		}
		names := make([]string, len(g.Seats))
		for i, s := range g.Seats {
			names[i] = s.Name
		}
		o := nearest(byKey[g.Tournament+"\x02"+playersKey(names)], g)
		if o == nil {
			got.Unmatched = append(got.Unmatched, g)
			continue
		}
		o.used = true
		got.Matched++
		links[SeasonLink{CGETournament: g.CGETournament, CGESeason: g.CGESeason, Season: o.season}]++
		ids, cgeT, cgeS, nums = append(ids, o.id), append(cgeT, int32(g.CGETournament)), append(cgeS, int32(g.CGESeason)), append(nums, int32(g.Number))
		finished, codes, groups = append(finished, g.Finished), append(codes, g.Code), append(groups, g.Group)
		for _, s := range g.Seats {
			seatMatch, seatNo = append(seatMatch, o.id), append(seatNo, int32(slices.Index(o.players, s.Name)+1))
			seatName, seatOutcome, seatBanned = append(seatName, s.Name), append(seatOutcome, string(s.Outcome)), append(seatBanned, s.Banned)
			seatUser = append(seatUser, int32(s.CGEUser))
		}
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
	if _, err := tx.Exec(ctx, `
		INSERT INTO match_seats (match_id, seat, player_name, outcome, cge_user, banned)
		SELECT x.m, x.n, x.name, x.outcome, nullif(x.u, 0), x.banned
		FROM unnest($1::bigint[], $2::int[], $3::text[], $4::text[], $5::int[], $6::bool[]) AS x(m, n, name, outcome, u, banned)`,
		seatMatch, seatNo, seatName, seatOutcome, seatUser, seatBanned); err != nil {
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
		if o.used || !o.fits(g) {
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
