package store

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// catalogueGamesPage is how many games one page of a tournament's games holds.
const catalogueGamesPage = 50

// CatalogueGamesFilter narrows a tournament's games. Empty fields do not
// narrow; Player keeps games with that player (any case), Coded games with a
// replay code. Page counts from 1.
type CatalogueGamesFilter struct {
	Season, Stage, Group string
	Player               string
	Coded                bool
	Page                 int
}

// GameSeat is one player of a game. Outcome is CGE's (finished, resigned,
// timed_out), or "" where the game's seats are not known.
type GameSeat struct {
	Name    string
	Score   float64
	Outcome string
}

// CatalogueGame is one game of a tournament, where it sits and how it went.
// Game is 0 and Group "" when the game's name does not say.
type CatalogueGame struct {
	ID                   int64
	Date                 time.Time
	Season, Stage, Group string
	Game                 int
	State                string
	Code                 string
	Seats                []GameSeat
	// Rated is whether the game counts towards ratings: played, in a rated tournament, season and player count.
	Rated bool
}

// CatalogueGames is one page of a tournament's games. Total counts every game
// the filter keeps, and Coded those of them with a replay code, the Coded
// filter aside. Groups lists the groups of the chosen season (and stage), or
// is nil when no season is chosen.
type CatalogueGames struct {
	Total, Coded, Page, Pages int
	Groups                    []string
	Games                     []CatalogueGame
}

// shownGames is every placed game of a shown tournament (rated or made
// visible), never-played ones included, with its season and stage.
const shownGames = `
	SELECT m.game_id AS match_id, m.date, s.label AS season, st.label AS stage, m.group_name AS group_label,
	       coalesce(m.game_number, 0) AS game, m.state, coalesce(m.share_code, '') AS code,
	       m.state = 'played' AND s.rated AND m.player_count = ANY(t.rated_player_counts)
	         AND EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id) AS rated
	FROM game_lines m
	JOIN stages st USING (stage_id)
	JOIN tournaments s ON s.tournament_id = st.tournament_id
	JOIN series t ON t.series_id = s.series_id
	WHERE t.slug = $1
	  AND (t.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id))
	  AND ($2 = '' OR s.label = $2) AND ($3 = '' OR st.label = $3)
	  AND ($4 = '' OR lower($4) IN (SELECT lower(x) FROM unnest(m.players) x))`

// CatalogueGames returns one page of a shown tournament's games, newest
// first, for a published run. A page past the last gives the last. A
// tournament without placed games, or hidden, is ErrTournamentNotFound.
func (r *RatingsReader) CatalogueGames(ctx context.Context, run int64, slug string, f CatalogueGamesFilter) (CatalogueGames, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return CatalogueGames{}, err
	}
	fail := func(what string, err error) (CatalogueGames, error) {
		return CatalogueGames{}, fmt.Errorf("reading %s's %s: %w", slug, what, err)
	}
	var shown bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (`+shownGames+`)`, slug, "", "", "").Scan(&shown); err != nil {
		return fail("games", err)
	}
	if !shown {
		return CatalogueGames{}, ErrTournamentNotFound
	}

	got := CatalogueGames{Games: []CatalogueGame{}}
	if f.Season != "" {
		rows, err := r.pool.Query(ctx, `SELECT DISTINCT group_label FROM (`+shownGames+`) g WHERE group_label <> ''`, slug, f.Season, f.Stage, f.Player)
		if err != nil {
			return fail("groups", err)
		}
		if got.Groups, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return fail("groups", err)
		}
		slices.SortFunc(got.Groups, groupOrder)
	}

	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE NOT $6 OR code <> ''), count(*) FILTER (WHERE code <> '')
		FROM (`+shownGames+`) g WHERE ($5 = '' OR group_label = $5)`,
		slug, f.Season, f.Stage, f.Player, f.Group, f.Coded).Scan(&got.Total, &got.Coded); err != nil {
		return fail("game count", err)
	}
	got.Pages = max(1, (got.Total+catalogueGamesPage-1)/catalogueGamesPage)
	got.Page = min(max(f.Page, 1), got.Pages)

	rows, err := r.pool.Query(ctx, `
		SELECT g.match_id, g.date, g.season, g.stage, g.group_label, g.game, g.state, g.code, g.rated,
		       m.players, m.cultures, m.places, m.outcomes
		FROM (`+shownGames+`) g JOIN game_lines m ON m.game_id = g.match_id
		WHERE ($5 = '' OR g.group_label = $5) AND (NOT $6 OR g.code <> '')
		ORDER BY g.date DESC, g.season, g.stage, g.group_label, g.game DESC, g.match_id
		LIMIT $7 OFFSET $8`,
		slug, f.Season, f.Stage, f.Player, f.Group, f.Coded, catalogueGamesPage, (got.Page-1)*catalogueGamesPage)
	if err != nil {
		return fail("games", err)
	}
	got.Games, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueGame, error) {
		var g CatalogueGame
		var names []string
		var cultures []float64
		var places []int16
		var outcomes []*string
		err := row.Scan(&g.ID, &g.Date, &g.Season, &g.Stage, &g.Group, &g.Game, &g.State, &g.Code, &g.Rated,
			&names, &cultures, &places, &outcomes)
		g.Seats = byPlace(names, cultures, places, outcomes)
		return g, err
	})
	if err != nil {
		return fail("games", err)
	}
	return got, nil
}

// groupRanks orders divisions as CGE lists them: leagues' tiers, Royal
// League's ranks, then a bracket's upper half before its lower.
var groupRanks = []string{
	"Grandmaster", "Master", "Diamond", "Platinum", "Gold", "Silver", "Bronze", "Wood",
	"Emperor", "King", "Prince", "Duke", "Marquess", "Count", "Viscount", "Baron", "Knight",
	"Upper", "Lower",
}

var bestOf = regexp.MustCompile(`^Best (?:of )?`)

// groupOrder orders group labels by division rank, a rank's tiebreak group
// ("Best Duke") after its groups, then naturally; unranked labels come last.
func groupOrder(a, b string) int {
	key := func(s string) (rank, best int, rest string) {
		rest = bestOf.ReplaceAllString(s, "")
		best = len(s) - len(rest)
		first, _, _ := strings.Cut(rest, " ")
		rank = slices.Index(groupRanks, first)
		if rank < 0 {
			rank = len(groupRanks)
		}
		return rank, min(best, 1), rest
	}
	ra, ba, sa := key(a)
	rb, bb, sb := key(b)
	return cmp.Or(cmp.Compare(ra, rb), cmp.Compare(ba, bb), naturally(sa, sb))
}

var digits = regexp.MustCompile(`\d+|\D+`)

// naturally orders labels with their numbers as numbers: "Bronze 2" before "Bronze 10".
func naturally(a, b string) int {
	pa, pb := digits.FindAllString(a, -1), digits.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		if errA == nil && errB == nil {
			if c := cmp.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if c := cmp.Compare(pa[i], pb[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pa), len(pb))
}

// byPlace returns a game's seats from its seat-ordered arrays, best place first, a tie in seat order.
// An outcome not recorded is "".
func byPlace(names []string, cultures []float64, places []int16, outcomes []*string) []GameSeat {
	order := make([]int, len(names))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(places[a], places[b]) })
	seats := make([]GameSeat, 0, len(names))
	for _, i := range order {
		seat := GameSeat{Name: names[i], Score: cultures[i]}
		if i < len(outcomes) && outcomes[i] != nil {
			seat.Outcome = *outcomes[i]
		}
		seats = append(seats, seat)
	}
	return seats
}
