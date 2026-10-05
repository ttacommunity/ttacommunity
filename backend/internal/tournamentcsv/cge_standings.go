package tournamentcsv

import (
	"io"
	"strconv"
	"strings"
)

// CGEStandingsHeader is a CGE standings file's first line, exactly.
var CGEStandingsHeader = []string{
	"cge_tournament", "cge_season", "group", "row", "place", "player", "cge_user", "banned",
	"games", "points", "places_2p", "places_3p", "places_4p", "source",
}

// CGEStanding is one player's line in a CGE group's standings table, as CGE
// shows it. Row is its order in the table; Place is 0 when CGE shows none.
// PlacesNp counts the player's firsts, seconds and so on in games of N
// players, and is nil when they played none.
type CGEStanding struct {
	CGETournament, CGESeason int
	Group                    string
	Row, Place               int
	Player                   string
	CGEUser                  int
	Banned                   bool
	Games                    int
	Points                   float64
	Places2p                 []int
	Places3p                 []int
	Places4p                 []int
	Source                   string
}

// ReadCGEStandings reads a CGE standings CSV. A group's row given twice keeps the first.
func ReadCGEStandings(file string, r io.Reader) (Result[CGEStanding], error) {
	return read(file, r, CGEStandingsHeader, parseCGEStanding, func(x CGEStanding) string {
		return strconv.Itoa(x.CGETournament) + "/" + strconv.Itoa(x.CGESeason) + "/" + x.Group + "/" + strconv.Itoa(x.Row)
	})
}

func parseCGEStanding(f func(int) string) (CGEStanding, Reason) {
	x := CGEStanding{Group: f(2), Player: f(5), Banned: f(7) == "true", Source: f(13)}
	t, tOK := positive(f(0))
	s, sOK := positive(f(1))
	row, rowOK := positive(f(3))
	user, userOK := positive(f(6))
	games, gamesErr := strconv.Atoi(f(8))
	points, pointsErr := strconv.ParseFloat(f(9), 64)
	x.CGETournament, x.CGESeason, x.Row, x.CGEUser, x.Games, x.Points = t, s, row, user, games, points
	placeOK := true
	if f(4) != "" {
		x.Place, placeOK = positive(f(4))
	}
	p2, bad2 := places(f(10), 2)
	p3, bad3 := places(f(11), 3)
	p4, bad4 := places(f(12), 4)
	x.Places2p, x.Places3p, x.Places4p = p2, p3, p4
	switch {
	case !tOK, !sOK:
		return x, BadSeason
	case !trimmed(x.Group), !rowOK:
		return x, BadRow
	case !placeOK:
		return x, BadPlace
	case !trimmed(x.Player), !userOK:
		return x, BadPlayer
	case f(7) != "" && f(7) != "true", gamesErr != nil, games < 0:
		return x, BadRow
	case pointsErr != nil:
		return x, BadPoints
	case bad2, bad3, bad4, sum(p2)+sum(p3)+sum(p4) > games:
		return x, BadPlace
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}

// places reads a game size's placing counts, n numbers apart by spaces; ""
// is none. It reports whether the field is malformed.
func places(s string, n int) ([]int, bool) {
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, " ")
	if len(parts) != n {
		return nil, true
	}
	got := make([]int, n)
	for i, p := range parts {
		c, err := strconv.Atoi(p)
		if err != nil || c < 0 {
			return nil, true
		}
		got[i] = c
	}
	return got, false
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}
