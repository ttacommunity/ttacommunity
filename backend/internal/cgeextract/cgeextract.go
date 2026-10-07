// Package cgeextract reads CGE's data extract (one row per player per game, every public tournament) and the
// settings files beside it, and works out each seat's place as CGE records it.
package cgeextract

import "time"

// The finish statuses CGE records for a seat; "" for a game not finished.
const (
	Finished = "finished"
	Resigned = "resigned"
	TimedOut = "timed_out"
)

// Seat is one player in one game of the extract, with the game's place in its tournament.
type Seat struct {
	TournamentID, StageID, StageNumber, GroupID, SessionID int
	TournamentName, GroupName, ShareCode, State            string
	StartedAt, FinishedAt                                  *time.Time
	Speed, Timer                                           string
	PlayerID                                               int
	PlayerName                                             string
	Banned                                                 bool
	StartingPosition, PlayersInGame                        int
	Standing                                               *int
	Score                                                  *float64
	FinishStatus                                           string
}

// Places returns each seat's place, in seat order: finishers by CGE's standing (ties as CGE has them), then
// resigners, the last to resign highest (CGE marks the first -9, the next -8), then timed-out seats, tied last.
func Places(seats []Seat) []int {
	var finishers, resigners int
	for _, s := range seats {
		switch s.FinishStatus {
		case Finished:
			finishers++
		case Resigned:
			resigners++
		}
	}
	places := make([]int, len(seats))
	for i, s := range seats {
		ahead := 0
		for _, o := range seats {
			switch {
			case s.FinishStatus == Finished && o.FinishStatus == Finished:
				if *o.Standing < *s.Standing {
					ahead++
				}
			case s.FinishStatus == Resigned && o.FinishStatus == Resigned:
				if *o.Score > *s.Score {
					ahead++
				}
			}
		}
		switch s.FinishStatus {
		case Finished:
			places[i] = 1 + ahead
		case Resigned:
			places[i] = 1 + finishers + ahead
		default:
			places[i] = 1 + finishers + resigners
		}
	}
	return places
}
