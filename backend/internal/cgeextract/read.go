package cgeextract

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Header is the extract's first line, as CGE writes it (after a byte order mark).
var Header = []string{
	"tournament_id", "tournament_name", "organiser_id", "organiser_name", "moderators", "tournament_created_at",
	"tournament_started_at", "tournament_finished_at", "stage_id", "stage_number", "stage_started_at", "stage_finished_at",
	"group_id", "group_name", "session_id", "share_code", "session_state", "started_at", "finished_at", "speed", "timer",
	"player_id", "player_name", "starting_position", "players_in_game", "standing", "score", "finish_status",
}

// Reason says why a row was refused.
type Reason string

// The reasons a row is refused.
const (
	BadID       Reason = "bad_id"
	BadTime     Reason = "bad_time"
	BadState    Reason = "bad_state"
	BadStatus   Reason = "bad_status"
	BadScore    Reason = "bad_score"
	BadStanding Reason = "bad_standing"
	BadRow      Reason = "bad_row"
	NoPlayer    Reason = "no_player"
	Duplicate   Reason = "duplicate"
)

// Refusal is a row that was not used. It names the line, never the player.
type Refusal struct {
	Line   int
	Reason Reason
}

// Tournament is what the extract says of one CGE tournament.
type Tournament struct {
	ID                         int
	Name                       string
	OrganiserID                int
	OrganiserName              string
	Moderators                 []string
	Created, Started, Finished *time.Time
}

// Stage is what the extract says of one CGE stage.
type Stage struct {
	ID, TournamentID, Number int
	Started, Finished        *time.Time
}

// Extract is what the file gave: its seats in file order, its tournaments and stages by id, and the rows refused.
type Extract struct {
	Rows        int
	Seats       []Seat
	Tournaments map[int]Tournament
	Stages      map[int]Stage
	Refused     []Refusal
}

var states = []string{"finished", "running", "prepared", "cancelled"}

// statuses maps CGE's finish status to ours.
var statuses = map[string]string{"finished": Finished, "resigned": Resigned, "timed out": TimedOut, "": ""}

// Read reads the extract named file. A row that does not fit is refused with its reason; a seat seen on an
// earlier line (the same game and player) is refused as a duplicate. It returns an error only for a file it
// cannot read or whose header is not Header.
func Read(file string, r io.Reader) (Extract, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err == nil && len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\ufeff")
	}
	if errors.Is(err, io.EOF) || (err == nil && !slices.Equal(head, Header)) {
		return Extract{}, fmt.Errorf("%s: want the header %s", file, strings.Join(Header, ","))
	}
	if err != nil {
		return Extract{}, fmt.Errorf("%s: %w", file, err)
	}
	got := Extract{Tournaments: map[int]Tournament{}, Stages: map[int]Stage{}}
	seen := map[[2]int]bool{}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return Extract{}, fmt.Errorf("%s: %w", file, err)
		}
		got.Rows++
		seat, t, st, why := parse(rec)
		if why == "" && seen[[2]int{seat.SessionID, seat.PlayerID}] {
			why = Duplicate
		}
		if why != "" {
			got.Refused = append(got.Refused, Refusal{Line: line, Reason: why})
			continue
		}
		seen[[2]int{seat.SessionID, seat.PlayerID}] = true
		got.Seats = append(got.Seats, seat)
		got.Tournaments[t.ID] = t
		got.Stages[st.ID] = st
	}
}

// parse reads one row, or returns why it is refused.
func parse(rec []string) (Seat, Tournament, Stage, Reason) {
	if len(rec) != len(Header) {
		return Seat{}, Tournament{}, Stage{}, BadRow
	}
	f := func(name string) string { return strings.TrimSpace(rec[slices.Index(Header, name)]) }
	ids := map[string]int{}
	for _, name := range []string{"tournament_id", "organiser_id", "stage_id", "stage_number", "group_id", "session_id",
		"player_id", "starting_position", "players_in_game"} {
		n, err := strconv.Atoi(f(name))
		if name == "player_id" && err == nil && n == 0 && f("player_name") == "" {
			return Seat{}, Tournament{}, Stage{}, NoPlayer // a seat CGE holds no account for
		}
		if err != nil || n < 0 || (n == 0 && name != "starting_position") {
			return Seat{}, Tournament{}, Stage{}, BadID
		}
		ids[name] = n
	}
	times := map[string]*time.Time{}
	for _, name := range []string{"tournament_created_at", "tournament_started_at", "tournament_finished_at",
		"stage_started_at", "stage_finished_at", "started_at", "finished_at"} {
		if f(name) == "" {
			continue
		}
		at, err := time.Parse(time.DateTime, f(name))
		if err != nil {
			return Seat{}, Tournament{}, Stage{}, BadTime
		}
		times[name] = &at
	}
	if !slices.Contains(states, f("session_state")) {
		return Seat{}, Tournament{}, Stage{}, BadState
	}
	status, ok := statuses[f("finish_status")]
	if !ok {
		return Seat{}, Tournament{}, Stage{}, BadStatus
	}
	var standing *int
	if s := f("standing"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return Seat{}, Tournament{}, Stage{}, BadStanding
		}
		standing = &n
	}
	var score *float64
	if s := f("score"); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return Seat{}, Tournament{}, Stage{}, BadScore
		}
		score = &v
	}
	switch {
	case status == Finished && (score == nil || *score < 0), status == Resigned && (score == nil || *score >= 0):
		return Seat{}, Tournament{}, Stage{}, BadScore
	case status == TimedOut && (standing != nil || score != nil):
		return Seat{}, Tournament{}, Stage{}, BadStanding
	}
	name, banned := strings.CutSuffix(f("player_name"), "(BAN)")
	var moderators []string
	for m := range strings.SplitSeq(f("moderators"), ",") {
		if m = strings.TrimSpace(m); m != "" {
			moderators = append(moderators, m)
		}
	}
	seat := Seat{
		TournamentID: ids["tournament_id"], StageID: ids["stage_id"], StageNumber: ids["stage_number"],
		GroupID: ids["group_id"], SessionID: ids["session_id"], TournamentName: f("tournament_name"),
		GroupName: f("group_name"), ShareCode: f("share_code"), State: f("session_state"),
		StartedAt: times["started_at"], FinishedAt: times["finished_at"], Speed: f("speed"), Timer: f("timer"),
		PlayerID: ids["player_id"], PlayerName: name, Banned: banned, StartingPosition: ids["starting_position"],
		PlayersInGame: ids["players_in_game"], Standing: standing, Score: score, FinishStatus: status,
	}
	t := Tournament{
		ID: ids["tournament_id"], Name: f("tournament_name"), OrganiserID: ids["organiser_id"],
		OrganiserName: f("organiser_name"), Moderators: moderators, Created: times["tournament_created_at"],
		Started: times["tournament_started_at"], Finished: times["tournament_finished_at"],
	}
	st := Stage{ID: ids["stage_id"], TournamentID: ids["tournament_id"], Number: ids["stage_number"],
		Started: times["stage_started_at"], Finished: times["stage_finished_at"]}
	return seat, t, st, ""
}
