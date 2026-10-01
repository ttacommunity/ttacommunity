// Package gamename reads where a game sits (tournament, season, stage, group,
// game number) from the name a match file gives it, such as
// "Intermezzo S24 - Bronze 1 game 3". One rule per name shape.
package gamename

import (
	"regexp"
	"strconv"
	"strings"
)

// Placement is where a game sits. Stage is "" for a season's only stage,
// Group "" and Game 0 when the name does not say.
type Placement struct {
	Tournament string
	Season     string
	Stage      string
	Group      string
	Game       int
}

// Tournament is one tournament as the site names it.
type Tournament struct {
	Slug string
	Name string
}

// Tournaments lists every tournament a name rule can return.
var Tournaments = []Tournament{
	{"international", "International"},
	{"intermezzo", "Intermezzo"},
	{"royal_league", "Royal League"},
	{"worlds", "World Championship"},
	{"wimbledon", "Wimbledon"},
	{"french_open", "French Open"},
	{"australian_open", "Australian Open"},
	{"survivors_cup", "Survivors Cup"},
	{"leaderboard_trophy", "Leaderboard Trophy"},
	{"slow_burn", "Slow Burn"},
	{"eiffel_tower", "Eiffel Tower"},
	{"mercurial_ladder", "Mercurial Ladder"},
	{"sodium_ladder", "Sodium Ladder"},
	{"transcontinental_ladder", "Transcontinental Ladder"},
	{"premier_league", "Premier League"},
	{"duel_curve", "Duel Curve"},
	{"nations_league", "Nations League"},
}

// rule reads one name shape. Its pattern's first group is the season; place,
// when set, turns the groups into season and stage instead.
type rule struct {
	re         *regexp.Regexp
	tournament string
	stage      string
	place      func(m []string) (season, stage string, ok bool)
}

func r(pattern, tournament, stage string) rule {
	return rule{re: regexp.MustCompile(pattern), tournament: tournament, stage: stage}
}

func rp(pattern, tournament string, place func([]string) (string, string, bool)) rule {
	return rule{re: regexp.MustCompile(pattern), tournament: tournament, place: place}
}

// rules are tried in order; a more specific shape comes before a looser one.
var rules = []rule{
	r(`^International S(\d+)`, "international", ""),
	r(`^Intermezzo S(\d+)`, "intermezzo", ""),
	rp(`^RL_(\d)w_tb_s(\d+)`, "royal_league", func(m []string) (string, string, bool) {
		s, ok := number(m[2])
		return s, m[1] + "-way tiebreak", ok
	}),
	r(`^RL_tb_s?(\d+)`, "royal_league", "Tiebreak"),
	r(`^RL_s(\d+)`, "royal_league", ""),
	rp(`^Worlds (\d+)(?: -)? (?:[Ss]tage (\d+)|(finals))`, "worlds", func(m []string) (string, string, bool) {
		s, ok := worldsSeason(m[1])
		if m[3] != "" {
			return s, "Finals", ok
		}
		st, stOK := number(m[2])
		return s, "Stage " + st, ok && stOK
	}),
	rp(`^Worlds (\d+)`, "worlds", func(m []string) (string, string, bool) {
		s, ok := worldsSeason(m[1])
		return s, "", ok
	}),
	rp(`^Wimbledon (\d{4})(?: [Ss]tage (\d+))?`, "wimbledon", yearAndStage),
	rp(`^French Open (\d{4})(?: \((Odd|Even) Games\) Stage (\d+))?`, "french_open", func(m []string) (string, string, bool) {
		if m[2] == "" {
			return m[1], "", true
		}
		st, ok := number(m[3])
		return m[1], m[2] + " Games Stage " + st, ok
	}),
	rp(`^Australian Open (\d{4}) Stage (\d+)`, "australian_open", yearAndStage),
	rp(`^Survivors Cup (\d{4}) Stage (\d+)`, "survivors_cup", yearAndStage),
	rp(`^Leaderboard Trophy (\d{4}) Stage (\d+)`, "leaderboard_trophy", yearAndStage),
	r(`^Slow Burn S?(\d+)`, "slow_burn", ""),
	r(`^Eiffel Tower S(\d+)`, "eiffel_tower", ""),
	r(`^ML_[sr](\d+)`, "mercurial_ladder", ""),
	r(`^Mercurial Season (\d+)`, "mercurial_ladder", ""),
	r(`^Sodium (?:Ladder )?Season (\d+)`, "sodium_ladder", ""),
	r(`^TCL Round (\d+)`, "transcontinental_ladder", ""),
	r(`^Premier League (\d+)`, "premier_league", ""),
	rp(`^PL_s(\d+)`, "premier_league", func(m []string) (string, string, bool) {
		s, ok := number(m[1])
		return s, m[0], ok
	}),
	r(`^DC_s(\d+)`, "duel_curve", ""),
	r(`^NL_s(\d+)`, "nations_league", ""),
}

var gameAtEnd = regexp.MustCompile(`^(.*?)\s*(?:-\s*)?\bgame (\d+)$`)

// Parse returns where the named game sits, or false for a name no rule reads.
func Parse(name string) (Placement, bool) {
	for _, ru := range rules {
		m := ru.re.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		p := Placement{Tournament: ru.tournament, Stage: ru.stage}
		var ok bool
		if ru.place != nil {
			p.Season, p.Stage, ok = ru.place(m)
		} else {
			p.Season, ok = number(m[1])
		}
		if !ok {
			return Placement{}, false
		}
		p.Group, p.Game, ok = tail(name[len(m[0]):])
		if !ok {
			return Placement{}, false
		}
		return p, true
	}
	return Placement{}, false
}

// tail reads what follows a name's season and stage: a group and a game
// number, either of which may be missing.
func tail(s string) (group string, game int, ok bool) {
	if s == "" {
		return "", 0, true
	}
	if s[0] != ' ' {
		return "", 0, false
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSpace(strings.TrimPrefix(s, "-"))
	if m := gameAtEnd.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil || n < 1 {
			return "", 0, false
		}
		return strings.TrimRight(m[1], " -"), n, true
	}
	return strings.TrimRight(s, " -"), 0, true
}

// number returns a season or stage number without leading zeros, refusing 0.
func number(s string) (string, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return "", false
	}
	return strconv.Itoa(n), true
}

// worldsSeason names a World Championship: the first two by Age (A, then I),
// later ones by year.
func worldsSeason(s string) (string, bool) {
	switch {
	case s == "1":
		return "Age A", true
	case s == "2":
		return "Age I", true
	case len(s) == 4 && s >= "2024":
		return s, true
	}
	return "", false
}

func yearAndStage(m []string) (string, string, bool) {
	if m[2] == "" {
		return m[1], "", true
	}
	st, ok := number(m[2])
	return m[1], "Stage " + st, ok
}
