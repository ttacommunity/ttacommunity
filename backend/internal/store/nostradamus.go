// Nostradamus editions: the site's reads (the editions, a board, a participant's answers) and admin's (an edition's
// questions, an outcome set, a participant linked). The rules an answer keeps and how it scores are the database's.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNoNostradamus is an edition slug, or a question or participant of it, that names none.
var ErrNoNostradamus = errors.New("no such Nostradamus edition")

// ErrBadNostradamus is an outcome that is not of its question's kind.
var ErrBadNostradamus = errors.New("not an outcome of the question's kind")

// NostradamusEdition is an edition and the tournament it is played over.
type NostradamusEdition struct {
	Slug, Name, Series, Tournament string
}

// NostradamusEditions returns every edition, the newest tournament's first.
func (r *RatingsReader) NostradamusEditions(ctx context.Context) ([]NostradamusEdition, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.slug, e.name, s.slug, t.label
		FROM nostradamus_editions e JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		ORDER BY t.tournament_id DESC, e.slug`)
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus editions: %w", err)
	}
	editions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (NostradamusEdition, error) {
		var e NostradamusEdition
		return e, row.Scan(&e.Slug, &e.Name, &e.Series, &e.Tournament)
	})
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus editions: %w", err)
	}
	return editions, nil
}

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

// NostradamusQuestion is a question as the site and admin show it; Outcome is empty until it is known.
type NostradamusQuestion struct {
	ID                 int64
	Label, Prompt      string
	Kind               string
	Points, NearPoints int
	Outcome            []string
}

// NostradamusAnswer is a participant's answer to a question, and its points once the outcome is known.
type NostradamusAnswer struct {
	Question NostradamusQuestion
	Answer   []string
	Token    bool
	Points   *int
}

// NostradamusParticipantRound is a closed round as one participant played it. Points are the round's once it is
// scored; Missed when they gave no answers, so Answers is empty.
type NostradamusParticipantRound struct {
	Label, Name string
	ClosesAt    time.Time
	Points      *int
	Missed      bool
	Answers     []NostradamusAnswer
}

// NostradamusParticipant is one participant's rounds: only those closed, since answers are hidden until then.
type NostradamusParticipant struct {
	Name, Member string
	Bot          bool
	Rounds       []NostradamusParticipantRound
}

// NostradamusParticipant returns the named participant's answers in the edition's rounds closed by now, the name
// matched ignoring case, or ErrNoNostradamus.
func (r *RatingsReader) NostradamusParticipant(ctx context.Context, slug, name string, now time.Time) (NostradamusParticipant, error) {
	var p NostradamusParticipant
	var id int64
	err := r.pool.QueryRow(ctx, `
		SELECT p.participant_id, p.name, coalesce(m.tta_name, ''), p.bot
		FROM nostradamus_participants p JOIN nostradamus_editions e USING (edition_id)
		LEFT JOIN members m ON m.member_id = p.member_id
		WHERE e.slug = $1 AND lower(p.name) = lower($2)`, slug, name).Scan(&id, &p.Name, &p.Member, &p.Bot)
	if errors.Is(err, pgx.ErrNoRows) {
		return NostradamusParticipant{}, ErrNoNostradamus
	}
	if err != nil {
		return NostradamusParticipant{}, fmt.Errorf("reading a Nostradamus participant: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.round_id, r.label, r.name, r.closes_at, s.points,
		       NOT EXISTS (SELECT FROM nostradamus_answers a JOIN nostradamus_questions n USING (question_id)
		                   WHERE n.round_id = r.round_id AND a.participant_id = p.participant_id),
		       n.question_id, n.label, n.prompt, n.kind, n.points, n.near_points, coalesce(n.outcome, '{}'),
		       a.answer, coalesce(a.token, false),
		       nostradamus_points(n.kind, n.points, n.near_points, n.outcome, a.answer, a.token, r.token_points)
		FROM nostradamus_participants p
		JOIN nostradamus_rounds r USING (edition_id)
		LEFT JOIN nostradamus_round_scores s ON s.round_id = r.round_id AND s.participant_id = p.participant_id
		LEFT JOIN nostradamus_questions n ON n.round_id = r.round_id
		LEFT JOIN nostradamus_answers a ON a.question_id = n.question_id AND a.participant_id = p.participant_id
		WHERE p.participant_id = $1 AND r.closes_at <= $2 AND (a.answer IS NOT NULL OR n.question_id IS NULL OR
		      NOT EXISTS (SELECT FROM nostradamus_answers x JOIN nostradamus_questions y USING (question_id)
		                  WHERE y.round_id = r.round_id AND x.participant_id = p.participant_id))
		ORDER BY r.position, n.position`, id, now)
	if err != nil {
		return NostradamusParticipant{}, fmt.Errorf("reading a Nostradamus participant's answers: %w", err)
	}
	defer rows.Close()
	last := int64(-1)
	for rows.Next() {
		var round int64
		var pr NostradamusParticipantRound
		var question *int64
		var q NostradamusQuestion
		var label, prompt, kind *string
		var points, near *int
		var a NostradamusAnswer
		if err := rows.Scan(&round, &pr.Label, &pr.Name, &pr.ClosesAt, &pr.Points, &pr.Missed,
			&question, &label, &prompt, &kind, &points, &near, &q.Outcome, &a.Answer, &a.Token, &a.Points); err != nil {
			return NostradamusParticipant{}, fmt.Errorf("reading a Nostradamus participant's answers: %w", err)
		}
		if round != last {
			p.Rounds = append(p.Rounds, pr)
			last = round
		}
		if pr.Missed || question == nil {
			continue
		}
		q.ID, q.Label, q.Prompt, q.Kind, q.Points, q.NearPoints = *question, *label, *prompt, *kind, *points, *near
		a.Question = q
		current := &p.Rounds[len(p.Rounds)-1]
		current.Answers = append(current.Answers, a)
	}
	if err := rows.Err(); err != nil {
		return NostradamusParticipant{}, fmt.Errorf("reading a Nostradamus participant's answers: %w", err)
	}
	return p, nil
}

// AdminNostradamusRound is a round with every question, for admin.
type AdminNostradamusRound struct {
	Label, Name string
	ClosesAt    time.Time
	Questions   []NostradamusQuestion
}

// AdminNostradamusParticipant is a participant and the TTA name of the member it is linked to, or empty.
type AdminNostradamusParticipant struct {
	ID           int64
	Name, Member string
	Bot          bool
}

// AdminNostradamus is an edition as admin sees it: its rounds' questions with their outcomes, and its participants.
type AdminNostradamus struct {
	Slug, Name   string
	Rounds       []AdminNostradamusRound
	Participants []AdminNostradamusParticipant
}

// NostradamusForAdmin returns the edition's rounds, in order, and its participants by name, or ErrNoNostradamus.
func (a *AdminStore) NostradamusForAdmin(ctx context.Context, slug string) (AdminNostradamus, error) {
	e := AdminNostradamus{Slug: slug}
	var edition int64
	err := a.pool.QueryRow(ctx, "SELECT edition_id, name FROM nostradamus_editions WHERE slug = $1", slug).Scan(&edition, &e.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminNostradamus{}, ErrNoNostradamus
	}
	if err != nil {
		return AdminNostradamus{}, fmt.Errorf("reading Nostradamus edition %s: %w", slug, err)
	}
	rows, err := a.pool.Query(ctx, `
		SELECT r.round_id, r.label, r.name, r.closes_at, n.question_id, n.label, n.prompt, n.kind, n.points, n.near_points,
		       coalesce(n.outcome, '{}')
		FROM nostradamus_rounds r JOIN nostradamus_questions n USING (round_id)
		WHERE r.edition_id = $1 ORDER BY r.position, n.position`, edition)
	if err != nil {
		return AdminNostradamus{}, fmt.Errorf("reading Nostradamus questions: %w", err)
	}
	defer rows.Close()
	last := int64(-1)
	for rows.Next() {
		var round int64
		var r AdminNostradamusRound
		var q NostradamusQuestion
		if err := rows.Scan(&round, &r.Label, &r.Name, &r.ClosesAt, &q.ID, &q.Label, &q.Prompt, &q.Kind, &q.Points, &q.NearPoints, &q.Outcome); err != nil {
			return AdminNostradamus{}, fmt.Errorf("reading Nostradamus questions: %w", err)
		}
		if round != last {
			e.Rounds = append(e.Rounds, r)
			last = round
		}
		current := &e.Rounds[len(e.Rounds)-1]
		current.Questions = append(current.Questions, q)
	}
	if err := rows.Err(); err != nil {
		return AdminNostradamus{}, fmt.Errorf("reading Nostradamus questions: %w", err)
	}
	people, err := a.pool.Query(ctx, `
		SELECT p.participant_id, p.name, coalesce(m.tta_name, ''), p.bot
		FROM nostradamus_participants p LEFT JOIN members m ON m.member_id = p.member_id
		WHERE p.edition_id = $1 ORDER BY lower(p.name)`, edition)
	if err != nil {
		return AdminNostradamus{}, fmt.Errorf("reading Nostradamus participants: %w", err)
	}
	e.Participants, err = pgx.CollectRows(people, func(row pgx.CollectableRow) (AdminNostradamusParticipant, error) {
		var p AdminNostradamusParticipant
		return p, row.Scan(&p.ID, &p.Name, &p.Member, &p.Bot)
	})
	if err != nil {
		return AdminNostradamus{}, fmt.Errorf("reading Nostradamus participants: %w", err)
	}
	return e, nil
}

// SetNostradamusOutcome sets what happened for a question of the edition, or clears it with none. It returns
// ErrNoNostradamus for a question not in the edition, or ErrBadNostradamus for an outcome not of its kind.
func (a *AdminStore) SetNostradamusOutcome(ctx context.Context, slug string, question int64, outcome []string) error {
	tag, err := a.pool.Exec(ctx, `
		UPDATE nostradamus_questions n SET outcome = $3
		FROM nostradamus_rounds r JOIN nostradamus_editions e USING (edition_id)
		WHERE n.round_id = r.round_id AND e.slug = $1 AND n.question_id = $2`, slug, question, nullIfEmpty(outcome))
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23514" {
		return ErrBadNostradamus
	}
	if err != nil {
		return fmt.Errorf("setting a Nostradamus outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoNostradamus
	}
	return nil
}

// LinkNostradamusParticipant links a participant of the edition to the member of that TTA name, ignoring case, or
// unlinks it for an empty name. It returns ErrNoMember or ErrNoNostradamus.
func (a *AdminStore) LinkNostradamusParticipant(ctx context.Context, slug string, participant int64, member string) error {
	var id *int64
	if member != "" {
		if err := a.pool.QueryRow(ctx, "SELECT min(member_id) FROM members WHERE lower(tta_name) = lower($1)", member).Scan(&id); err != nil {
			return fmt.Errorf("finding the member to link: %w", err)
		}
		if id == nil {
			return ErrNoMember
		}
	}
	tag, err := a.pool.Exec(ctx, `
		UPDATE nostradamus_participants p SET member_id = $3
		FROM nostradamus_editions e
		WHERE p.edition_id = e.edition_id AND e.slug = $1 AND p.participant_id = $2`, slug, participant, id)
	if err != nil {
		return fmt.Errorf("linking a Nostradamus participant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoNostradamus
	}
	return nil
}

// nullIfEmpty is values, or nil for SQL's null when there are none.
func nullIfEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}
