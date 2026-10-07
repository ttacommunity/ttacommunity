package cgeextract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Game is one game of the extract with its seats in file order.
type Game struct {
	SessionID, TournamentID, StageID, StageNumber, GroupID int
	GroupName, ShareCode, State, Speed, Timer              string
	StartedAt, FinishedAt                                  *time.Time
	PlayersInGame                                          int
	Seats                                                  []Seat
}

// Games groups seats into their games, in the order each game is first seen.
func Games(seats []Seat) []Game {
	var games []Game
	at := map[int]int{}
	for _, s := range seats {
		i, ok := at[s.SessionID]
		if !ok {
			i = len(games)
			at[s.SessionID] = i
			games = append(games, Game{
				SessionID: s.SessionID, TournamentID: s.TournamentID, StageID: s.StageID, StageNumber: s.StageNumber,
				GroupID: s.GroupID, GroupName: s.GroupName, ShareCode: s.ShareCode, State: s.State, Speed: s.Speed,
				Timer: s.Timer, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, PlayersInGame: s.PlayersInGame,
			})
		}
		games[i].Seats = append(games[i].Seats, s)
	}
	return games
}

// OurSeat is a seat of one of our games: its number, the name played under, its CGE account (0 if not
// known) and the score the source gave it.
type OurSeat struct {
	Seat    int
	Name    string
	Account int
	Score   float64
}

// OurGame is one of our games: its series (0 if not placed), share code ("" if none) and seats.
type OurGame struct {
	ID        int64
	Series    int64
	ShareCode string
	Seats     []OurSeat
}

// Matching is how the extract's games meet ours: our game id to CGE's session for each match, the sessions
// only CGE has (Added) or that may be copies of a game of ours (HeldBack), and our games CGE does not have.
type Matching struct {
	Matched  map[int64]int
	Added    []int
	HeldBack []int
	OursOnly []int64
}

// Match matches each CGE game to one of ours: by share code, else by its players and its finished seats'
// scores, when exactly one of our games without a code fits (a timed-out seat is left out, as our files score
// it -5 or -1 where CGE has none). names maps a lower-case name to the accounts that played under it; series
// maps a CGE tournament to our series (absent if not linked), and a match never crosses series. A game only
// CGE has is held back when an unmatched game of ours in its series has the same players.
func Match(cge []Game, ours []OurGame, names map[string][]int, series map[int]int64) Matching {
	got := Matching{Matched: map[int64]int{}}
	used := map[int64]bool{}
	byCode := map[string]int64{}
	byKey := map[string][]int{}
	for i, o := range ours {
		if o.ShareCode != "" {
			byCode[o.ShareCode] = o.ID
			continue
		}
		byKey[ourKey(o)] = append(byKey[ourKey(o)], i)
	}
	var unmatched []Game
	for _, g := range cge {
		if id, ok := byCode[g.ShareCode]; ok && !used[id] {
			got.Matched[id], used[id] = g.SessionID, true
			continue
		}
		var fits []int64
		if g.State == "finished" {
			for _, k := range slices.Compact([]string{cgeKey(g, false), cgeKey(g, true)}) {
				for _, i := range byKey[k] {
					if o := ours[i]; !used[o.ID] && sameSeries(series[g.TournamentID], o.Series) && playersFit(g, o, names) {
						fits = append(fits, o.ID)
					}
				}
			}
		}
		if len(fits) == 1 {
			got.Matched[fits[0]], used[fits[0]] = g.SessionID, true
			continue
		}
		unmatched = append(unmatched, g)
	}
	left := map[int64][]OurGame{}
	for _, o := range ours {
		if !used[o.ID] {
			got.OursOnly = append(got.OursOnly, o.ID)
			left[o.Series] = append(left[o.Series], o)
		}
	}
	for _, g := range unmatched {
		if g.State == "finished" && mayCopy(g, left, names, series[g.TournamentID]) {
			got.HeldBack = append(got.HeldBack, g.SessionID)
		} else {
			got.Added = append(got.Added, g.SessionID)
		}
	}
	slices.Sort(got.OursOnly)
	return got
}

// MatchSeats pairs our seats (by number) with the CGE game's seats (by index): by CGE account, else by a name
// the account played under. It returns the pairs and the CGE seats left without one, in seat order.
func MatchSeats(g Game, ours []OurSeat, names map[string][]int) (map[int]int, []int) {
	pairs := map[int]int{}
	taken := map[int]bool{}
	pair := func(o OurSeat, accounts []int) bool {
		for i, s := range g.Seats {
			if !taken[i] && slices.Contains(accounts, s.PlayerID) {
				pairs[o.Seat], taken[i] = i, true
				return true
			}
		}
		return false
	}
	var rest []OurSeat
	for _, o := range ours {
		if o.Account == 0 || !pair(o, []int{o.Account}) {
			rest = append(rest, o)
		}
	}
	for _, o := range rest {
		pair(o, names[strings.ToLower(o.Name)])
	}
	var missing []int
	for i := range g.Seats {
		if !taken[i] {
			missing = append(missing, i)
		}
	}
	return pairs, missing
}

// cgeKey is a game's seat count and its finished seats' scores, sorted; with resignedAsZero, its resigners
// count as 0, as the organisers' files score them.
func cgeKey(g Game, resignedAsZero bool) string {
	var scores []float64
	for _, s := range g.Seats {
		switch {
		case s.FinishStatus == Finished:
			scores = append(scores, *s.Score)
		case s.FinishStatus == Resigned && resignedAsZero:
			scores = append(scores, 0)
		}
	}
	return key(len(g.Seats), scores)
}

// ourKey is our game's seat count and its scores of 0 or more, sorted: the seats that finished.
func ourKey(o OurGame) string {
	var scores []float64
	for _, s := range o.Seats {
		if s.Score >= 0 {
			scores = append(scores, s.Score)
		}
	}
	return key(len(o.Seats), scores)
}

func key(seats int, scores []float64) string {
	slices.Sort(scores)
	return fmt.Sprint(seats, scores)
}

func sameSeries(a, b int64) bool { return a == 0 || b == 0 || a == b }

// playersFit reports whether each of our seats was played by an account of the CGE game.
func playersFit(g Game, o OurGame, names map[string][]int) bool {
	if len(g.Seats) != len(o.Seats) {
		return false
	}
	for _, s := range o.Seats {
		accounts := names[strings.ToLower(s.Name)]
		if s.Account != 0 {
			accounts = []int{s.Account}
		}
		if !slices.ContainsFunc(g.Seats, func(c Seat) bool { return slices.Contains(accounts, c.PlayerID) }) {
			return false
		}
	}
	return true
}

// mayCopy reports whether an unmatched game of ours in the same series (left, by series) has the CGE game's players.
func mayCopy(g Game, left map[int64][]OurGame, names map[string][]int, series int64) bool {
	return series != 0 && slices.ContainsFunc(left[series], func(o OurGame) bool { return playersFit(g, o, names) })
}
