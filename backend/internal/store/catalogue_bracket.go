package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// SeasonBracket is a season of a tournament shown as a bracket: its stages as rounds, first to last, or a blitz
// edition's knockout, which is Live: it changes as results are recorded, not with the run.
type SeasonBracket struct {
	Season string
	Live   bool
	Rounds []BracketRound
}

// BracketRound is one stage of a bracket and its groups, by number. Qualifiers marks a blitz knockout's first
// column: its players, each alone in the group of their place in the draw.
type BracketRound struct {
	Stage      string
	Qualifiers bool
	Groups     []BracketGroup
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
// number of the group the player is in the round after, 0 for none; Seed is a blitz knockout player's, else 0.
type BracketSeat struct {
	Entrant int
	Name    string
	Seed    int
	Place   int
	Points  *float64
	Outcome string
	Next    int
}

// shownAsBracket is true for a series t whose seasons are shown as a bracket.
const shownAsBracket = `t.bracket`

// bracketStates orders game states from the least advanced: an unconfirmed game, its result in doubt, holds its
// group back.
var bracketStates = []string{"unconfirmed", "never_played", "prepared", "running", "played"}

var numberedGroup = regexp.MustCompile(`(?i)^group ([0-9]+)$`)

// SeasonBracket returns the bracket of a shown tournament's season, for a published run. A tournament not marked as a
// bracket, hidden, or without that season's games is ErrTournamentNotFound. Each group's seats come from its games:
// points are summed and the best place kept, in order of points (then place), or of seat while unplayed. A blitz
// tournament's comes from its edition's knockout (blitzBracket).
func (r *RatingsReader) SeasonBracket(ctx context.Context, run int64, slug, season string) (SeasonBracket, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return SeasonBracket{}, err
	}
	var blitz bool
	switch err := r.pool.QueryRow(ctx, "SELECT blitz FROM series WHERE slug = $1 AND bracket", slug).Scan(&blitz); {
	case errors.Is(err, pgx.ErrNoRows):
		return SeasonBracket{}, ErrTournamentNotFound
	case err != nil:
		return SeasonBracket{}, fmt.Errorf("reading %s's bracket: %w", slug, err)
	case blitz:
		return r.blitzBracket(ctx, slug, season)
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
	}
	linkRounds(got.Rounds)
	return got, nil
}

// linkRounds sets each seat's Next to the number of the group its player is in the round after.
func linkRounds(rounds []BracketRound) {
	for i := 1; i < len(rounds); i++ {
		next := map[int]int{}
		for _, g := range rounds[i].Groups {
			for _, s := range g.Seats {
				next[s.Entrant] = g.Number
			}
		}
		for _, g := range rounds[i-1].Groups {
			for k := range g.Seats {
				g.Seats[k].Next = next[g.Seats[k].Entrant]
			}
		}
	}
}

// blitzBracket returns a shown blitz edition's knockout as a bracket, from its published rounds: first its
// Qualifiers, each player alone in the group of their place in the draw (2k+1 and 2k+2 for match slot k), then each
// knockout round's matches, as group slot + 1, the winner first once decided (place 1, the loser 2). Before the
// knockout is drawn it has no rounds; an edition not shown is ErrTournamentNotFound.
func (r *RatingsReader) blitzBracket(ctx context.Context, slug, edition string) (SeasonBracket, error) {
	ed, err := r.BlitzEdition(ctx, slug, edition)
	if errors.Is(err, ErrNoBlitzEdition) {
		return SeasonBracket{}, ErrTournamentNotFound
	}
	if err != nil {
		return SeasonBracket{}, err
	}
	names := map[int64]string{}
	for _, e := range ed.Entrants {
		names[e.ID] = e.Name
	}
	entrants := map[int64]int{}
	seat := func(id int64, seed int) BracketSeat {
		if _, ok := entrants[id]; !ok {
			entrants[id] = len(entrants) + 1
		}
		return BracketSeat{Entrant: entrants[id], Name: names[id], Seed: seed}
	}
	got := SeasonBracket{Season: edition, Live: true, Rounds: []BracketRound{}}
	for _, rd := range ed.Rounds {
		if rd.Kind != "knockout" {
			continue
		}
		matches := slices.SortedFunc(slices.Values(rd.Matches), func(a, b BlitzMatch) int { return cmp.Compare(*a.Slot, *b.Slot) })
		if len(got.Rounds) == 0 {
			q := BracketRound{Stage: "Qualifiers", Qualifiers: true, Groups: []BracketGroup{}}
			for _, m := range matches {
				for k, side := range [][2]int64{{m.A, int64(*m.SeedA)}, {*m.B, int64(*m.SeedB)}} {
					s := seat(side[0], int(side[1]))
					s.Place = 1
					q.Groups = append(q.Groups, BracketGroup{Name: fmt.Sprintf("seed %d", side[1]), Number: 2**m.Slot + k + 1, State: "played", Seats: []BracketSeat{s}})
				}
			}
			got.Rounds = append(got.Rounds, q)
		}
		out := BracketRound{Stage: fmt.Sprintf("Round %d", rd.Number), Groups: make([]BracketGroup, len(matches))}
		for i, m := range matches {
			a, b := seat(m.A, *m.SeedA), seat(*m.B, *m.SeedB)
			if m.Game != nil {
				a.Outcome, b.Outcome = m.Game.A.Outcome, m.Game.B.Outcome
			}
			g := BracketGroup{Name: fmt.Sprintf("match %d", *m.Slot+1), Number: *m.Slot + 1, State: "prepared", Seats: []BracketSeat{a, b}}
			switch m.Result {
			case "a":
				g.State, g.Seats[0].Place, g.Seats[1].Place = "played", 1, 2
			case "b":
				g.State, g.Seats = "played", []BracketSeat{b, a}
				g.Seats[0].Place, g.Seats[1].Place = 1, 2
			}
			out.Groups[i] = g
		}
		got.Rounds = append(got.Rounds, out)
	}
	linkRounds(got.Rounds)
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
