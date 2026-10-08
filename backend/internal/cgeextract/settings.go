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

// Settings is one tournament's or stage's settings (StageID 0 for a tournament's). A nil setting is inherited;
// an empty one is none: no points for that player count, no tiebreakers.
type Settings struct {
	TournamentID, StageID int
	EachPlays             *int
	TimeoutPenalty        *float64
	IgnoreAI              *bool
	Points                map[int]*[]float64
	Tiebreak2p            *string
	Tiebreak3pPlus        *string
	Source                string
}

// SettingsFile is what a settings file gave, and the rows refused.
type SettingsFile struct {
	Settings []Settings
	Refused  []Refusal
}

var (
	specialHeader    = []string{"tournament_id", "tournament_name", "session_id", "share_code", "rules", "card_set"}
	settingNames     = []string{"each_plays", "timeout_penalty", "ignore_ai_players", "scoring_2p", "scoring_3p", "scoring_4p", "tiebreak_2p", "tiebreak_3p_plus"}
	tournamentHeader = append(append([]string{"tournament_id"}, settingNames...), "stages_read", "stages_in_extract")
	stageHeader      = append(append([]string{"tournament_id", "stage_id", "stage_number"}, settingNames...), "source", "fetched")
)

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

// ReadTournamentSettings reads tournament_settings.csv, each tournament's settings.
func ReadTournamentSettings(file string, r io.Reader) (SettingsFile, error) {
	return readSettings(file, r, tournamentHeader, false)
}

// ReadStageSettings reads stage_settings.csv, the stages whose settings differ from their tournament's.
func ReadStageSettings(file string, r io.Reader) (SettingsFile, error) {
	return readSettings(file, r, stageHeader, true)
}

func readSettings(file string, r io.Reader, header []string, stage bool) (SettingsFile, error) {
	var got SettingsFile
	err := readRows(file, r, header, func(line int, f func(string) string) {
		s, ok := parseSettings(f, stage)
		if !ok {
			got.Refused = append(got.Refused, Refusal{Line: line, Reason: BadRow})
			return
		}
		got.Settings = append(got.Settings, s)
	})
	return got, err
}

// parseSettings reads one row's settings: "" is inherited, "-" is none.
func parseSettings(f func(string) string, stage bool) (Settings, bool) {
	s := Settings{Points: map[int]*[]float64{}}
	var err error
	if s.TournamentID, err = strconv.Atoi(f("tournament_id")); err != nil || s.TournamentID < 1 {
		return Settings{}, false
	}
	if stage {
		if s.StageID, err = strconv.Atoi(f("stage_id")); err != nil || s.StageID < 1 {
			return Settings{}, false
		}
		s.Source = f("source")
	}
	if v := f("each_plays"); v != "" && v != "-" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Settings{}, false
		}
		s.EachPlays = &n
	}
	if v := f("timeout_penalty"); v != "" && v != "-" {
		p, err := strconv.ParseFloat(v, 64)
		if err != nil || p > 0 {
			return Settings{}, false
		}
		s.TimeoutPenalty = &p
	}
	switch v := f("ignore_ai_players"); v {
	case "yes", "no":
		ignore := v == "yes"
		s.IgnoreAI = &ignore
	case "":
	default:
		return Settings{}, false
	}
	for n := 2; n <= 4; n++ {
		v := f(fmt.Sprintf("scoring_%dp", n))
		if v == "" {
			continue
		}
		points := []float64{}
		if v != "-" {
			for part := range strings.SplitSeq(v, "-") {
				p, err := strconv.ParseFloat(part, 64)
				if err != nil || p < 0 {
					return Settings{}, false
				}
				points = append(points, p)
			}
			if len(points) != n {
				return Settings{}, false
			}
		}
		s.Points[n] = &points
	}
	s.Tiebreak2p, s.Tiebreak3pPlus = tiebreak(f("tiebreak_2p")), tiebreak(f("tiebreak_3p_plus"))
	return s, true
}

// tiebreak returns nil for "" (inherited), "" for "-" (none), else the tiebreakers as written.
func tiebreak(v string) *string {
	switch v {
	case "":
		return nil
	case "-":
		v = ""
	}
	return &v
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
