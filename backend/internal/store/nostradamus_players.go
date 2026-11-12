// A signed-in player's Nostradamus: asking to join a game, reading their status and answers, and answering an open
// round. Each goes through a database function that checks the session; the player login writes nothing else.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrNotEntered is a player not accepted in the game whose round they answer.
var ErrNotEntered = errors.New("you are not entered in this game yet")

// ErrRoundNotOpen is a round not open for answers: not published yet, or closed.
var ErrRoundNotOpen = errors.New("the round is not open for answers")

// ErrFewTokens is answers staking fewer tokens in all than the round's least.
var ErrFewTokens = errors.New("you have staked fewer tokens than this round asks for")

// ErrPastDeadline is an answer after the round's deadline.
var ErrPastDeadline = errors.New("the round's deadline has passed")

// nostradamusPlayerError maps a player function's refusal to its error, or nil for none it knows.
func nostradamusPlayerError(refusal string) error {
	switch refusal {
	case "signed-out":
		return ErrNoSession
	case "not-linked":
		return ErrNotLinked
	case "banned":
		return ErrMemberBanned
	case "no-edition":
		return ErrNoNostradamus
	case "not-entered":
		return ErrNotEntered
	case "not-open":
		return ErrRoundNotOpen
	case "past-deadline":
		return ErrPastDeadline
	case "bad":
		return ErrBadNostradamus
	case "few-tokens":
		return ErrFewTokens
	}
	return nil
}

// RequestNostradamusEntry asks for the session's member to play the game, and returns their status: requested,
// accepted or refused. It returns ErrNoSession, ErrNotLinked, ErrMemberBanned or ErrNoNostradamus.
func (s *PlayerStore) RequestNostradamusEntry(ctx context.Context, tokenHash []byte, slug string, now time.Time) (string, error) {
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT request_nostradamus_entry($1, $2, $3)", tokenHash, now, slug).Scan(&got); err != nil {
		return "", fmt.Errorf("asking to join a Nostradamus game: %w", err)
	}
	if err := nostradamusPlayerError(got); err != nil {
		return "", err
	}
	return got, nil
}

// MyNostradamus is a player's place in a game: their status (none until they ask) and their answers by question.
type MyNostradamus struct {
	Status  string
	Answers map[int64]NostradamusGiven
}

// MyNostradamus returns the session's member's status and answers in the game. It returns ErrNoSession,
// ErrNotLinked or ErrMemberBanned.
func (s *PlayerStore) MyNostradamus(ctx context.Context, tokenHash []byte, slug string, now time.Time) (MyNostradamus, error) {
	rows, err := s.pool.Query(ctx, "SELECT status, question_id, answer, tokens FROM my_nostradamus($1, $2, $3)", tokenHash, now, slug)
	if err != nil {
		return MyNostradamus{}, fmt.Errorf("reading your Nostradamus answers: %w", err)
	}
	defer rows.Close()
	mine := MyNostradamus{Answers: map[int64]NostradamusGiven{}}
	for rows.Next() {
		var question *int64
		var answer []string
		var tokens *int
		if err := rows.Scan(&mine.Status, &question, &answer, &tokens); err != nil {
			return MyNostradamus{}, fmt.Errorf("reading your Nostradamus answers: %w", err)
		}
		if question != nil {
			mine.Answers[*question] = NostradamusGiven{Question: *question, Answer: answer, Tokens: *tokens}
		}
	}
	if err := rows.Err(); err != nil {
		return MyNostradamus{}, fmt.Errorf("reading your Nostradamus answers: %w", err)
	}
	if err := nostradamusPlayerError(mine.Status); err != nil {
		return MyNostradamus{}, err
	}
	return mine, nil
}

// SubmitNostradamusAnswers replaces the session's member's answers in an open round, all or none, and returns how
// many it saved. It returns ErrNoSession, ErrNotLinked, ErrMemberBanned, ErrRoundNotOpen, ErrNotEntered,
// ErrPastDeadline, ErrFewTokens, or ErrBadNostradamus for an answer the rules refuse.
func (s *PlayerStore) SubmitNostradamusAnswers(ctx context.Context, tokenHash []byte, round int64, answers []NostradamusGiven, now time.Time) (int, error) {
	type given struct {
		Question int64    `json:"question"`
		Answer   []string `json:"answer"`
		Tokens   int      `json:"tokens"`
	}
	list := make([]given, len(answers))
	for i, a := range answers {
		list[i] = given(a)
		if list[i].Answer == nil {
			list[i].Answer = []string{}
		}
	}
	body, err := json.Marshal(list)
	if err != nil {
		return 0, fmt.Errorf("submitting Nostradamus answers: %w", err)
	}
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT submit_nostradamus_answers($1, $2, $3, $4)", tokenHash, now, round, body).Scan(&got); err != nil {
		return 0, fmt.Errorf("submitting Nostradamus answers: %w", err)
	}
	if n, ok := strings.CutPrefix(got, "saved:"); ok {
		return strconv.Atoi(n)
	}
	if err := nostradamusPlayerError(got); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("submitting Nostradamus answers: unexpected %q", got)
}
