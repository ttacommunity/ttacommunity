// Nostradamus editions: reading an edition's board. The rules an answer keeps and how it scores are the database's
// (nostradamus_check_answer, nostradamus_round_scores).
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// ErrNoNostradamus is an edition slug that names none.
var ErrNoNostradamus = errors.New("no such Nostradamus edition")

// NostradamusBoard is an edition's standings: Rounds are the labels of the rounds scored so far, in order.
type NostradamusBoard struct {
	Slug, Name string
	Rounds     []string
	Standings  []NostradamusStanding
}

// NostradamusStanding is a participant's total and points in each scored round. Member is the TTA name of the
// member it is linked to, or empty.
type NostradamusStanding struct {
	Name, Member string
	Bot          bool
	Total        int
	Rounds       []NostradamusRoundScore
}

// NostradamusRoundScore is a participant's points in one round; Missed when it gave no answers there.
type NostradamusRoundScore struct {
	Label  string
	Points int
	Missed bool
}

// NostradamusBoard returns the edition's standings, highest total first, or ErrNoNostradamus.
func (r *RatingsReader) NostradamusBoard(ctx context.Context, slug string) (NostradamusBoard, error) {
	board := NostradamusBoard{Slug: slug}
	var edition int64
	err := r.pool.QueryRow(ctx, "SELECT edition_id, name FROM nostradamus_editions WHERE slug = $1", slug).Scan(&edition, &board.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return NostradamusBoard{}, ErrNoNostradamus
	}
	if err != nil {
		return NostradamusBoard{}, fmt.Errorf("reading Nostradamus edition %s: %w", slug, err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.participant_id, p.name, coalesce(m.tta_name, ''), p.bot, r.label, s.points, s.missed
		FROM nostradamus_round_scores s
		JOIN nostradamus_rounds r USING (round_id)
		JOIN nostradamus_participants p USING (participant_id)
		LEFT JOIN members m ON m.member_id = p.member_id
		WHERE s.edition_id = $1
		ORDER BY sum(s.points) OVER (PARTITION BY p.participant_id) DESC, lower(p.name), r.position`, edition)
	if err != nil {
		return NostradamusBoard{}, fmt.Errorf("reading Nostradamus board %s: %w", slug, err)
	}
	defer rows.Close()
	last := int64(-1)
	for rows.Next() {
		var id int64
		var s NostradamusStanding
		var score NostradamusRoundScore
		if err := rows.Scan(&id, &s.Name, &s.Member, &s.Bot, &score.Label, &score.Points, &score.Missed); err != nil {
			return NostradamusBoard{}, fmt.Errorf("reading Nostradamus board %s: %w", slug, err)
		}
		if id != last {
			board.Standings = append(board.Standings, s)
			last = id
		}
		current := &board.Standings[len(board.Standings)-1]
		current.Rounds = append(current.Rounds, score)
		current.Total += score.Points
		if !slices.Contains(board.Rounds, score.Label) {
			board.Rounds = append(board.Rounds, score.Label)
		}
	}
	if err := rows.Err(); err != nil {
		return NostradamusBoard{}, fmt.Errorf("reading Nostradamus board %s: %w", slug, err)
	}
	return board, nil
}
