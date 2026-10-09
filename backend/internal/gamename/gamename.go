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
	// Kind and TopDivisions are what a tournament starts with when first added; admin changes them after.
	Kind         string
	TopDivisions []string
}

// Rules are how the engine treats a tournament when first added: how its places score in the community
// leaderboard, its column code in the organisers' sheet ("ic_{}" adds the season), and which of the engine's rule
// sets derives its results ("" for none).
var Rules = map[string]struct{ CommunityScoring, CommunityCode, Results string }{
	"international":    {"league", "ic_{}", "international"},
	"intermezzo":       {"league", "im_{}", "intermezzo"},
	"royal_league":     {"", "rl_{:02d}", "royal_league"},
	"worlds":           {"worlds", "wrld", "worlds"},
	"australian_open":  {"rounds", "aus", ""},
	"wimbledon":        {"knockout", "wmbl", ""},
	"eiffel_tower":     {"floors", "eif", ""},
	"mercurial_ladder": {"", "ml", ""},
	"sodium_ladder":    {"", "sl", ""},
}

// Tournaments lists every tournament a name rule can return.
var Tournaments = []Tournament{
	{"international", "International", "league", []string{"Grandmaster", "Diamond"}},
	{"intermezzo", "Intermezzo", "league", []string{"Grandmaster", "Diamond"}},
	{"royal_league", "Royal League", "league", []string{"Emperor"}},
	{"worlds", "World Championship", "cup", []string{"Final"}},
	{"wimbledon", "Wimbledon", "cup", nil},
	{"french_open", "French Open", "cup", nil},
	{"australian_open", "Australian Open", "cup", nil},
	{"survivors_cup", "Survivors Cup", "cup", nil},
	{"leaderboard_trophy", "Leaderboard Trophy", "cup", nil},
	{"slow_burn", "Slow Burn", "cup", nil},
	{"eiffel_tower", "Eiffel Tower", "league", nil},
	{"mercurial_ladder", "Mercurial Ladder", "ladder", nil},
	{"sodium_ladder", "Sodium Ladder", "ladder", nil},
	{"transcontinental_ladder", "Transcontinental Ladder", "ladder", nil},
	{"premier_league", "Premier League", "league", nil},
	{"duel_curve", "Duel Curve", "league", nil},
	{"nations_league", "Nations League", "league", []string{"Apple"}},
}

// Scoring is the points per place, by player count, a tournament starts with when first added; admin changes it after.
var Scoring = map[string]map[int][]float64{
	"international": {3: {5, 2, 0}, 4: {6, 3, 1, 0}},
	"intermezzo":    {3: {5, 2, 0}},
	"royal_league":  {2: {2, 0}},
	"worlds":        {2: {4, 0}, 3: {5, 2, 0}, 4: {6, 3, 1, 0}},
}

// LevelNames are what a tournament calls its seasons and stages when first added, where not the defaults.
var LevelNames = map[string][2]string{
	"slow_burn": {"Edition", "Season"},
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
	rp(`^Worlds (\d{4}) Play Off Stage (\d+)`, "worlds", func(m []string) (string, string, bool) {
		st, ok := number(m[2])
		return m[1] + " Play Off", "Stage " + st, ok
	}),
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
	// Slow Burn's editions are its seasons; its stages are what its rules call seasons, so they are labelled so.
	rp(`^Slow Burn Season (\d+) Stage (\d+)`, "slow_burn", func(m []string) (string, string, bool) {
		year, ok := slowBurnYears[m[1]]
		st, stOK := number(m[2])
		return m[1] + " (" + year + ")", "Season " + st, ok && stOK
	}),
	// Slow Burn's first edition names only its stages, as "S" numbers.
	rp(`^Slow Burn S?(\d+)`, "slow_burn", func(m []string) (string, string, bool) {
		st, ok := number(m[1])
		return "1 (2024)", "Season " + st, ok
	}),
	r(`^Eiffel Tower S(\d+)`, "eiffel_tower", ""),
	r(`^ML_[sr](\d+)`, "mercurial_ladder", ""),
	r(`^Mercurial Season (\d+)`, "mercurial_ladder", ""),
	r(`^Sodium (?:Ladder )?Season (\d+)`, "sodium_ladder", ""),
	r(`^TCL Round (\d+)`, "transcontinental_ladder", ""),
	rp(`^Premier League First Edition`, "premier_league", func([]string) (string, string, bool) {
		return "First Edition (2023)", "", true
	}),
	r(`^Premier League (\d+)`, "premier_league", ""),
	rp(`^PL_s(\d+)`, "premier_league", func(m []string) (string, string, bool) {
		s, ok := number(m[1])
		return s, m[0], ok
	}),
	rp(`^DC Grand Prix (\d{4})`, "duel_curve", func(m []string) (string, string, bool) {
		return "Grand Prix " + m[1], "", true
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

// slowBurnYears gives the year each Slow Burn edition started, as CGE shows it.
var slowBurnYears = map[string]string{"1": "2024", "2": "2026"}

// worldsSeason names a World Championship by its year; the first also by its Age.
func worldsSeason(s string) (string, bool) {
	switch {
	case s == "1":
		return "2022 (Age A)", true
	case s == "2":
		return "2023", true
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
