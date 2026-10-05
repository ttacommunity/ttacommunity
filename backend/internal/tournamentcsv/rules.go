package tournamentcsv

import (
	"fmt"
	"io"
	"strconv"
)

// RulesHeader is a rules file's first line, exactly.
var RulesHeader = []string{"series", "season", "timeout_penalty", "scoring_3p", "scoring_4p", "tiebreaks", "source"}

// Rule is one season's rules as its tournament page states them. TimeoutPenalty
// is the points each timed-out game costs (0 or less); the rest is as written.
type Rule struct {
	Series         string
	Season         int
	TimeoutPenalty float64
	Scoring3p      string
	Scoring4p      string
	Tiebreaks      string
	Source         string
}

// ReadRules reads a rules CSV. A season given twice in a series keeps the first row.
func ReadRules(file string, r io.Reader) (Result[Rule], error) {
	return read(file, r, RulesHeader, parseRule, func(x Rule) string {
		return fmt.Sprint(x.Series, "\x00", x.Season)
	})
}

func parseRule(f func(int) string) (Rule, Reason) {
	s, okSeries := series(f(0))
	season, okSeason := positive(f(1))
	penalty, err := strconv.ParseFloat(f(2), 64)
	x := Rule{Series: s, Season: season, TimeoutPenalty: penalty, Scoring3p: f(3), Scoring4p: f(4), Tiebreaks: f(5), Source: f(6)}
	switch {
	case !okSeries:
		return x, BadSeries
	case !okSeason:
		return x, BadSeason
	case err != nil || penalty > 0:
		return x, BadPoints
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
