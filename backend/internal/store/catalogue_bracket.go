package store

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// SeasonBracket is a season of a tournament shown as a bracket: its stages as rounds, first to last.
type SeasonBracket struct {
	Season string
	Rounds []BracketRound
}

// BracketRound is one stage of a bracket and its groups, by number.
type BracketRound struct {
	Stage  string
	Groups []BracketGroup
}

// BracketGroup is one group of a round. Number is n for a group named "group n", else its place in the round; State
// is its game's (played, running, prepared, never_played or unconfirmed), the least advanced where it has several, never-played
// games aside unless all are.
type BracketGroup struct {
	Name   string
	Number int
	State  string
	Seats  []BracketSeat
}

// BracketSeat is one player of a group. Entrant numbers the season's players from 1, in their first round's order;
// Place is 0 and Points nil until the group is played (Points stay nil without points per place); Next is the
// number of the group the player is in the round after, 0 for none.
type BracketSeat struct {
	Entrant int
	Name    string
	Place   int
	Points  *float64
	Outcome string
	Next    int
}

// shownAsBracket is true for a series t whose seasons are shown as a bracket: marked so, and not a blitz series, whose
// knockout is kept in the blitz tables rather than stages and groups.
const shownAsBracket = `(t.bracket AND NOT t.blitz)`

// bracketStates orders game states from the least advanced: an unconfirmed game, its result in doubt, holds its
// group back.
var bracketStates = []string{"unconfirmed", "never_played", "prepared", "running", "played"}

var numberedGroup = regexp.MustCompile(`(?i)^group ([0-9]+)$`)

// SeasonBracket returns the bracket of a shown tournament's season, for a published run. A tournament not marked as a
// bracket, hidden, or without that season's games is ErrTournamentNotFound. Each group's seats come from its games:
// points are summed and the best place kept, in order of points (then place), or of seat while unplayed.
func (r *RatingsReader) SeasonBracket(ctx context.Context, run int64, slug, season string) (SeasonBracket, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return SeasonBracket{}, err
	}
	fail := func(what string, err error) (SeasonBracket, error) {
		return SeasonBracket{}, fmt.Errorf("reading %s %s's %s: %w", slug, season, what, err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT st.label, m.group_name, m.state, m.player_ids, m.players, m.places, m.game_points, m.outcomes,
		       array(SELECT `+hiddenName("n")+` FROM unnest(m.players) n)
		FROM game_lines m
		JOIN stages st USING (stage_id)
		JOIN tournaments s ON s.tournament_id = st.tournament_id
		JOIN series t ON t.series_id = s.series_id
		WHERE t.slug = $1 AND s.label = $2 AND `+shownAsBracket+`
		  AND (t.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id))
		ORDER BY min(m.date) OVER (PARTITION BY st.stage_id), st.label, m.game_number NULLS LAST, m.game_id`, slug, season)
	if err != nil {
		return fail("bracket", err)
	}
	type seat struct {
		id      int64
		name    string
		place   int
		points  *float64
		outcome string
	}
	type group struct {
		name, state string
		seats       []*seat
	}
	type round struct {
		stage  string
		groups []*group
	}
	var rounds []*round
	var stage, groupName, state string
	var ids []int64
	var names []string
	var places []int16
	var points []*float64
	var outcomes []*string
	var hidden []bool
	_, err = pgx.ForEachRow(rows, []any{&stage, &groupName, &state, &ids, &names, &places, &points, &outcomes, &hidden}, func() error {
		if len(rounds) == 0 || rounds[len(rounds)-1].stage != stage {
			rounds = append(rounds, &round{stage: stage})
		}
		rd := rounds[len(rounds)-1]
		i := slices.IndexFunc(rd.groups, func(g *group) bool { return g.name == groupName })
		if i < 0 {
			rd.groups = append(rd.groups, &group{name: groupName, state: state})
			i = len(rd.groups) - 1
		}
		g := rd.groups[i]
		if g.state == "never_played" || state != "never_played" && slices.Index(bracketStates, state) < slices.Index(bracketStates, g.state) {
			g.state = state
		}
		for k, id := range ids {
			j := slices.IndexFunc(g.seats, func(s *seat) bool { return s.id == id })
			if j < 0 {
				name := names[k]
				if hidden[k] {
					name = DeletedPlayer
				}
				g.seats = append(g.seats, &seat{id: id, name: name})
				j = len(g.seats) - 1
			}
			s := g.seats[j]
			if state != "played" {
				continue
			}
			if p := int(places[k]); s.place == 0 || p < s.place {
				s.place = p
			}
			if points[k] != nil {
				s.points = ptrTo(*points[k] + valueOr(s.points))
			}
			if outcomes[k] != nil {
				s.outcome = *outcomes[k]
			}
		}
		return nil
	})
	if err != nil {
		return fail("bracket", err)
	}
	if len(rounds) == 0 {
		return SeasonBracket{}, ErrTournamentNotFound
	}

	got := SeasonBracket{Season: season, Rounds: make([]BracketRound, len(rounds))}
	entrants := map[int64]int{}
	for i, rd := range rounds {
		slices.SortStableFunc(rd.groups, func(a, b *group) int { return groupOrder(a.name, b.name) })
		out := BracketRound{Stage: rd.stage, Groups: make([]BracketGroup, len(rd.groups))}
		for j, g := range rd.groups {
			if g.state == "played" {
				slices.SortStableFunc(g.seats, func(a, b *seat) int {
					return cmp.Or(cmp.Compare(valueOr(b.points), valueOr(a.points)), cmp.Compare(a.place, b.place))
				})
			}
			number := j + 1
			if m := numberedGroup.FindStringSubmatch(g.name); m != nil {
				number, _ = strconv.Atoi(m[1])
			}
			dg := BracketGroup{Name: g.name, Number: number, State: g.state, Seats: make([]BracketSeat, len(g.seats))}
			for k, s := range g.seats {
				if _, ok := entrants[s.id]; !ok {
					entrants[s.id] = len(entrants) + 1
				}
				dg.Seats[k] = BracketSeat{Entrant: entrants[s.id], Name: s.name, Place: s.place, Points: s.points, Outcome: s.outcome}
			}
			out.Groups[j] = dg
		}
		got.Rounds[i] = out
		if i == 0 {
			continue
		}
		next := map[int]int{}
		for _, g := range out.Groups {
			for _, s := range g.Seats {
				next[s.Entrant] = g.Number
			}
		}
		for _, g := range got.Rounds[i-1].Groups {
			for k := range g.Seats {
				g.Seats[k].Next = next[g.Seats[k].Entrant]
			}
		}
	}
	return got, nil
}

// valueOr returns what p points to, or 0 for nil.
func valueOr(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ptrTo returns a pointer to a copy of v.
func ptrTo(v float64) *float64 { return &v }
