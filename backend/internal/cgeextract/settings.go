package cgeextract

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Special is a game's rules and card set, where they are not Digital and Random Mix.
type Special struct {
	Rules, CardSet string
}

// SpecialRules is what the special rules file gave: each game's rules by CGE session id, and the rows refused.
type SpecialRules struct {
	Games   map[int]Special
	Refused []Refusal
}

// StageSettings is one CGE stage's settings, as CGE scores its groups. Points and EachPlays are by player count
// (none: no points, 0 games); a tiebreaker list "" is none, and a nil WinnerScoreMinimum is none.
type StageSettings struct {
	TournamentID, StageID, StageNumber int
	// StageName is the stage's name as CGE shows it ("Season 2", "Stage 1 of 3"); "" for a tournament of one stage.
	StageName                  string
	TimeoutPenalty             float64
	IgnoreAI                   bool
	Points                     map[int][]float64
	EachPlays                  map[int]int
	Tiebreak2p, Tiebreak3pPlus string
	WinnerScoreMinimum         *float64
}

// Stages is what CGE's stages file gave, and the rows refused.
type Stages struct {
	Settings []StageSettings
	Refused  []Refusal
}

var specialHeader = []string{"tournament_id", "tournament_name", "session_id", "share_code", "rules", "card_set"}

// ReadSpecialRules reads the special rules file named file.
func ReadSpecialRules(file string, r io.Reader) (SpecialRules, error) {
	got := SpecialRules{Games: map[int]Special{}}
	err := readRows(file, r, specialHeader, func(line int, f func(string) string) {
		id, err := strconv.Atoi(f("session_id"))
		if err != nil || id < 1 {
			got.Refused = append(got.Refused, Refusal{Line: line, Reason: BadID})
			return
		}
		got.Games[id] = Special{Rules: f("rules"), CardSet: f("card_set")}
	})
	return got, err
}

// ConfirmedPairs is what a confirmed pairs file gave: our game id by CGE share code, checked by hand.
type ConfirmedPairs struct {
	Pairs   map[string]int64
	Refused []Refusal
}

// ReadConfirmedPairs reads a CSV of share_code, game_id and reason.
func ReadConfirmedPairs(file string, r io.Reader) (ConfirmedPairs, error) {
	got := ConfirmedPairs{Pairs: map[string]int64{}}
	err := readRows(file, r, []string{"share_code", "game_id", "reason"}, func(line int, f func(string) string) {
		id, err := strconv.ParseInt(f("game_id"), 10, 64)
		if err != nil || id < 1 || f("share_code") == "" {
			got.Refused = append(got.Refused, Refusal{Line: line, Reason: BadID})
			return
		}
		got.Pairs[f("share_code")] = id
	})
	return got, err
}

var stagesHeader = []string{"tournament_id", "tournament_name", "stage_id", "stage_number", "stage_name", "timeout_penalty",
	"each_player_plays_2p", "scoring_2p", "each_player_plays_3p", "scoring_3p", "each_player_plays_4p", "scoring_4p",
	"ignore_ai_in_scoring", "tiebreakers_2_players_tie", "tiebreakers_3plus_players_tie", "winner_score_minimum"}

// ReadStages reads CGE's stages file, tournaments_export_stages.csv: every stage's settings as CGE scores its
// groups. A row that does not fit is refused.
func ReadStages(file string, r io.Reader) (Stages, error) {
	var got Stages
	err := readRows(file, r, stagesHeader, func(line int, f func(string) string) {
		s, ok := parseStage(f)
		if !ok {
			got.Refused = append(got.Refused, Refusal{Line: line, Reason: BadRow})
			return
		}
		got.Settings = append(got.Settings, s)
	})
	return got, err
}

// parseStage reads one row of CGE's stages file.
func parseStage(f func(string) string) (StageSettings, bool) {
	s := StageSettings{Points: map[int][]float64{}, EachPlays: map[int]int{},
		Tiebreak2p: f("tiebreakers_2_players_tie"), Tiebreak3pPlus: f("tiebreakers_3plus_players_tie"), StageName: f("stage_name")}
	var err error
	if s.TournamentID, err = strconv.Atoi(f("tournament_id")); err != nil || s.TournamentID < 1 {
		return StageSettings{}, false
	}
	if s.StageID, err = strconv.Atoi(f("stage_id")); err != nil || s.StageID < 1 {
		return StageSettings{}, false
	}
	if s.StageNumber, err = strconv.Atoi(f("stage_number")); err != nil || s.StageNumber < 1 {
		return StageSettings{}, false
	}
	if s.TimeoutPenalty, err = strconv.ParseFloat(f("timeout_penalty"), 64); err != nil || s.TimeoutPenalty > 0 {
		return StageSettings{}, false
	}
	switch v := f("ignore_ai_in_scoring"); v {
	case "yes", "no":
		s.IgnoreAI = v == "yes"
	default:
		return StageSettings{}, false
	}
	for n := 2; n <= 4; n++ {
		games, err := strconv.Atoi(f(fmt.Sprintf("each_player_plays_%dp", n)))
		if err != nil || games < 0 {
			return StageSettings{}, false
		}
		points, ok := parsePoints(f(fmt.Sprintf("scoring_%dp", n)), n)
		if !ok {
			return StageSettings{}, false
		}
		s.EachPlays[n], s.Points[n] = games, points
	}
	if v := f("winner_score_minimum"); v != "" {
		least, err := strconv.ParseFloat(v, 64)
		if err != nil || least < 0 || least > 100 {
			return StageSettings{}, false
		}
		s.WinnerScoreMinimum = &least
	}
	return s, true
}

// parsePoints reads game points by place for n players, 1st first ("6-3-1-0"); "" is none.
func parsePoints(v string, n int) ([]float64, bool) {
	points := []float64{}
	if v == "" {
		return points, true
	}
	for part := range strings.SplitSeq(v, "-") {
		p, err := strconv.ParseFloat(part, 64)
		if err != nil || p < 0 {
			return nil, false
		}
		points = append(points, p)
	}
	return points, len(points) == n
}

// readRows reads a CSV whose first line must be header, calling row for each line with a field lookup.
func readRows(file string, r io.Reader, header []string, row func(line int, f func(string) string)) error {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = len(header)
	head, err := cr.Read()
	if err == nil && len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\ufeff")
	}
	if errors.Is(err, io.EOF) || (err == nil && !slices.Equal(head, header)) {
		return fmt.Errorf("%s: want the header %s", file, strings.Join(header, ","))
	}
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		row(line, func(name string) string { return strings.TrimSpace(rec[slices.Index(header, name)]) })
	}
}
