// Admin's Nostradamus changes: a game made, rounds drafted with their questions and choices, published (when the
// bots answer), outcomes set and rounds closed; participants added, accepted or refused, and linked to members.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNostradamusRoundState is a change the round's stage refuses: editing a published round, publishing one twice or
// without questions, answering for a bot after the deadline, closing before the deadline or with an outcome missing.
// Its message says which.
var ErrNostradamusRoundState = errors.New("not at this stage of the round")

// roundState is ErrNostradamusRoundState saying why.
func roundState(why string) error {
	return fmt.Errorf("%w: %s", ErrNostradamusRoundState, why)
}

// nostradamusRefusal turns a database refusal of a change (a check, a label taken, a value out of range) into
// ErrBadNostradamus, and leaves any other error as it is.
func nostradamusRefusal(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23514" || pg.Code == "23505" || pg.Code == "22P02" || pg.Code == "23502") {
		return fmt.Errorf("%w: %s", ErrBadNostradamus, pg.Message)
	}
	return err
}

// AddNostradamusEdition makes a game over a series' tournament. It returns ErrNoTournament, or ErrBadNostradamus for a
// slug taken or not of lower-case letters, digits and dashes.
func (a *AdminStore) AddNostradamusEdition(ctx context.Context, e NostradamusEdition) error {
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO nostradamus_editions (slug, name, tournament_id)
		SELECT $1, $2, t.tournament_id FROM tournaments t JOIN series s USING (series_id) WHERE s.slug = $3 AND t.label = $4`,
		e.Slug, strings.TrimSpace(e.Name), e.Series, e.Tournament)
	if err != nil {
		return nostradamusRefusal(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// editionID returns the id of the game of slug, or ErrNoNostradamus.
func editionID(ctx context.Context, db conn, slug string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, "SELECT edition_id FROM nostradamus_editions WHERE slug = $1", slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoNostradamus
	}
	return id, err
}

// draftRound returns the round's id when it is a draft of the game, ErrNoNostradamus when it is not the game's, or
// ErrNostradamusRoundState once it is published.
func draftRound(ctx context.Context, db conn, slug string, round int64) error {
	var published bool
	err := db.QueryRow(ctx, `
		SELECT r.published_at IS NOT NULL FROM nostradamus_rounds r JOIN nostradamus_editions e USING (edition_id)
		WHERE e.slug = $1 AND r.round_id = $2`, slug, round).Scan(&published)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNoNostradamus
	case err != nil:
		return fmt.Errorf("reading a Nostradamus round: %w", err)
	case published:
		return roundState("the round is published; only a draft's questions change")
	}
	return nil
}

// NostradamusRoundDraft is what an organiser sets of a round: its label, name, and tokens with their worth.
type NostradamusRoundDraft struct {
	Label, Name         string
	Tokens, TokenPoints int
}

// AddNostradamusRound adds a draft round after the game's last and returns its id. It returns ErrNoNostradamus, or
// ErrBadNostradamus for a label taken or a value out of range.
func (a *AdminStore) AddNostradamusRound(ctx context.Context, slug string, d NostradamusRoundDraft) (int64, error) {
	edition, err := editionID(ctx, a.pool, slug)
	if err != nil {
		return 0, err
	}
	var id int64
	err = a.pool.QueryRow(ctx, `
		INSERT INTO nostradamus_rounds (edition_id, position, label, name, tokens, token_points)
		SELECT $1, coalesce(max(position) + 1, 0), $2, $3, $4, $5 FROM nostradamus_rounds WHERE edition_id = $1
		RETURNING round_id`, edition, strings.TrimSpace(d.Label), strings.TrimSpace(d.Name), d.Tokens, d.TokenPoints).Scan(&id)
	if err != nil {
		return 0, nostradamusRefusal(err)
	}
	return id, nil
}

// SetNostradamusRound changes a draft round's label, name and tokens. It returns ErrNoNostradamus,
// ErrNostradamusRoundState once published, or ErrBadNostradamus.
func (a *AdminStore) SetNostradamusRound(ctx context.Context, slug string, round int64, d NostradamusRoundDraft) error {
	if err := draftRound(ctx, a.pool, slug, round); err != nil {
		return err
	}
	_, err := a.pool.Exec(ctx, "UPDATE nostradamus_rounds SET label = $2, name = $3, tokens = $4, token_points = $5 WHERE round_id = $1",
		round, strings.TrimSpace(d.Label), strings.TrimSpace(d.Name), d.Tokens, d.TokenPoints)
	return nostradamusRefusal(err)
}

// RemoveNostradamusRound removes a draft round with its questions. It returns ErrNoNostradamus, or
// ErrNostradamusRoundState once published.
func (a *AdminStore) RemoveNostradamusRound(ctx context.Context, slug string, round int64) error {
	if err := draftRound(ctx, a.pool, slug, round); err != nil {
		return err
	}
	if _, err := a.pool.Exec(ctx, "DELETE FROM nostradamus_rounds WHERE round_id = $1", round); err != nil {
		return fmt.Errorf("removing a Nostradamus round: %w", err)
	}
	return nil
}

// AddNostradamusQuestion adds a question, with its choices, after a draft round's last and returns its id. It
// returns ErrNoNostradamus, ErrNostradamusRoundState once published, or ErrBadNostradamus.
func (a *AdminStore) AddNostradamusQuestion(ctx context.Context, slug string, round int64, q NostradamusQuestion) (int64, error) {
	if err := draftRound(ctx, a.pool, slug, round); err != nil {
		return 0, err
	}
	var id int64
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO nostradamus_questions (round_id, position, label, prompt, kind, points, near_points, picks, asks_last)
			SELECT $1, coalesce(max(position) + 1, 0), $2, $3, $4, $5, $6, $7, $8 FROM nostradamus_questions WHERE round_id = $1
			RETURNING question_id`, round, strings.TrimSpace(q.Label), strings.TrimSpace(q.Prompt), q.Kind, q.Points, q.NearPoints,
			max(q.Picks, 1), q.AsksLast).Scan(&id); err != nil {
			return err
		}
		return setChoices(ctx, tx, id, q.Choices)
	})
	if err != nil {
		return 0, nostradamusRefusal(err)
	}
	return id, nil
}

// SetNostradamusQuestion changes a draft round's question and replaces its choices. It returns ErrNoNostradamus,
// ErrNostradamusRoundState once published, or ErrBadNostradamus.
func (a *AdminStore) SetNostradamusQuestion(ctx context.Context, slug string, question int64, q NostradamusQuestion) error {
	round, err := questionRound(ctx, a.pool, slug, question)
	if err != nil {
		return err
	}
	if err := draftRound(ctx, a.pool, slug, round); err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE nostradamus_questions SET label = $2, prompt = $3, kind = $4, points = $5, near_points = $6, picks = $7, asks_last = $8
			WHERE question_id = $1`, question, strings.TrimSpace(q.Label), strings.TrimSpace(q.Prompt), q.Kind, q.Points, q.NearPoints,
			max(q.Picks, 1), q.AsksLast); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM nostradamus_choices WHERE question_id = $1", question); err != nil {
			return err
		}
		return setChoices(ctx, tx, question, q.Choices)
	})
	return nostradamusRefusal(err)
}

// RemoveNostradamusQuestion removes a draft round's question. It returns ErrNoNostradamus, or
// ErrNostradamusRoundState once published.
func (a *AdminStore) RemoveNostradamusQuestion(ctx context.Context, slug string, question int64) error {
	round, err := questionRound(ctx, a.pool, slug, question)
	if err != nil {
		return err
	}
	if err := draftRound(ctx, a.pool, slug, round); err != nil {
		return err
	}
	if _, err := a.pool.Exec(ctx, "DELETE FROM nostradamus_questions WHERE question_id = $1", question); err != nil {
		return fmt.Errorf("removing a Nostradamus question: %w", err)
	}
	return nil
}

// questionRound returns the round of a question of the game, or ErrNoNostradamus.
func questionRound(ctx context.Context, db conn, slug string, question int64) (int64, error) {
	var round int64
	err := db.QueryRow(ctx, `
		SELECT n.round_id FROM nostradamus_questions n JOIN nostradamus_rounds r USING (round_id) JOIN nostradamus_editions e USING (edition_id)
		WHERE e.slug = $1 AND n.question_id = $2`, slug, question).Scan(&round)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoNostradamus
	}
	return round, err
}

// setChoices writes a question's choices in order; a seed or rank of 0 is none.
func setChoices(ctx context.Context, tx pgx.Tx, question int64, choices []NostradamusChoice) error {
	for i, c := range choices {
		if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_choices (question_id, position, label, seed, leaderboard_rank)
			VALUES ($1, $2, $3, nullif($4, 0), nullif($5, 0))`, question, i, strings.TrimSpace(c.Label), c.Seed, c.LeaderboardRank); err != nil {
			return err
		}
	}
	return nil
}

// PublishNostradamusRound opens a draft round for answers until closes, set on the clocks of zone (as the handler
// read it), and has each bot answer the questions its rule can. It returns ErrNoNostradamus, or ErrNostradamusRoundState for a round already published,
// without questions, or a deadline not after now.
func (a *AdminStore) PublishNostradamusRound(ctx context.Context, slug string, round int64, closes time.Time, zone string, now time.Time) error {
	if err := draftRound(ctx, a.pool, slug, round); err != nil {
		return err
	}
	if !closes.After(now) {
		return roundState("the deadline must be after now")
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE nostradamus_rounds SET published_at = $2, closes_at = $3, deadline_zone = $4
			WHERE round_id = $1 AND published_at IS NULL AND EXISTS (SELECT FROM nostradamus_questions n WHERE n.round_id = $1)`,
			round, now, closes, zone)
		if err != nil {
			return nostradamusRefusal(err)
		}
		if tag.RowsAffected() == 0 {
			return roundState("a round needs a question before it is published")
		}
		return answerForBots(ctx, tx, round)
	})
}

// SetNostradamusOutcome sets what happened for a published question of the game, or clears it with none. It returns
// ErrNoNostradamus, ErrNostradamusRoundState for a draft's or a closed round's question, or ErrBadNostradamus for an
// outcome not of its kind.
func (a *AdminStore) SetNostradamusOutcome(ctx context.Context, slug string, question int64, outcome []string) error {
	var published, closed bool
	err := a.pool.QueryRow(ctx, `
		SELECT r.published_at IS NOT NULL, r.closed_at IS NOT NULL
		FROM nostradamus_questions n JOIN nostradamus_rounds r USING (round_id) JOIN nostradamus_editions e USING (edition_id)
		WHERE e.slug = $1 AND n.question_id = $2`, slug, question).Scan(&published, &closed)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNoNostradamus
	case err != nil:
		return fmt.Errorf("reading a Nostradamus question: %w", err)
	case !published:
		return roundState("a draft's question has no outcome yet")
	case closed:
		return roundState("the round is closed")
	}
	_, err = a.pool.Exec(ctx, "UPDATE nostradamus_questions SET outcome = $2 WHERE question_id = $1", question, nullIfEmpty(outcome))
	return nostradamusRefusal(err)
}

// CloseNostradamusRound closes a round once its deadline has passed and every question has an outcome, which shows
// its results and scores on the site. It returns ErrNoNostradamus or ErrNostradamusRoundState saying what is missing.
func (a *AdminStore) CloseNostradamusRound(ctx context.Context, slug string, round int64, now time.Time) error {
	var published, closed, missing bool
	var closes *time.Time
	err := a.pool.QueryRow(ctx, `
		SELECT r.published_at IS NOT NULL, r.closed_at IS NOT NULL, r.closes_at,
		       EXISTS (SELECT FROM nostradamus_questions n WHERE n.round_id = r.round_id AND n.outcome IS NULL)
		FROM nostradamus_rounds r JOIN nostradamus_editions e USING (edition_id) WHERE e.slug = $1 AND r.round_id = $2`,
		slug, round).Scan(&published, &closed, &closes, &missing)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNoNostradamus
	case err != nil:
		return fmt.Errorf("reading a Nostradamus round: %w", err)
	case !published:
		return roundState("the round is not published")
	case closed:
		return roundState("the round is closed already")
	case now.Before(*closes):
		return roundState("the deadline has not passed")
	case missing:
		return roundState("every question needs its outcome first")
	}
	if _, err := a.pool.Exec(ctx, "UPDATE nostradamus_rounds SET closed_at = $2 WHERE round_id = $1", round, now); err != nil {
		return fmt.Errorf("closing a Nostradamus round: %w", err)
	}
	return nil
}

// NostradamusAnswer is one participant's answer to a question, and its points once the outcome is known.
type NostradamusAnswer struct {
	Participant string
	Question    int64
	Answer      []string
	Token       bool
	Points      *int
}

// NostradamusRoundAnswers returns every answer in a round of the game, by participant then question, for admin. It
// returns ErrNoNostradamus for a round not of the game.
func (a *AdminStore) NostradamusRoundAnswers(ctx context.Context, slug string, round int64) ([]NostradamusAnswer, error) {
	var exists bool
	if err := a.pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM nostradamus_rounds r JOIN nostradamus_editions e USING (edition_id)
		WHERE e.slug = $1 AND r.round_id = $2)`, slug, round).Scan(&exists); err != nil {
		return nil, fmt.Errorf("reading a Nostradamus round: %w", err)
	}
	if !exists {
		return nil, ErrNoNostradamus
	}
	rows, err := a.pool.Query(ctx, `
		SELECT p.name, a.question_id, a.answer, a.token,
		       nostradamus_points(n.kind, n.points, n.near_points, n.outcome, a.answer, a.token, r.token_points)
		FROM nostradamus_answers a
		JOIN nostradamus_questions n USING (question_id)
		JOIN nostradamus_rounds r USING (round_id)
		JOIN nostradamus_participants p USING (participant_id)
		WHERE n.round_id = $1
		ORDER BY lower(p.name), n.position`, round)
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus answers: %w", err)
	}
	answers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (NostradamusAnswer, error) {
		var x NostradamusAnswer
		return x, row.Scan(&x.Participant, &x.Question, &x.Answer, &x.Token, &x.Points)
	})
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus answers: %w", err)
	}
	return answers, nil
}

// NostradamusGiven is an answer as given: the question, its names or number, and whether a token is on it.
type NostradamusGiven struct {
	Question int64
	Answer   []string
	Token    bool
}

// SetNostradamusBotAnswers replaces a bot's answers in a published round before its deadline. It returns
// ErrNoNostradamus for a round or participant not of the game, ErrNostradamusRoundState for a participant who is not
// a bot or a round not open, or ErrBadNostradamus for an answer the rules refuse.
func (a *AdminStore) SetNostradamusBotAnswers(ctx context.Context, slug string, round, participant int64, answers []NostradamusGiven, now time.Time) error {
	var bot, open bool
	err := a.pool.QueryRow(ctx, `
		SELECT p.bot, r.published_at IS NOT NULL AND r.closed_at IS NULL AND r.closes_at > $4
		FROM nostradamus_rounds r JOIN nostradamus_editions e USING (edition_id)
		JOIN nostradamus_participants p ON p.edition_id = e.edition_id
		WHERE e.slug = $1 AND r.round_id = $2 AND p.participant_id = $3`, slug, round, participant, now).Scan(&bot, &open)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNoNostradamus
	case err != nil:
		return fmt.Errorf("reading a Nostradamus bot: %w", err)
	case !bot:
		return roundState("only a bot's answers are given by an organiser")
	case !open:
		return roundState("the round is not open for answers")
	}
	err = pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM nostradamus_answers a USING nostradamus_questions n
			WHERE a.question_id = n.question_id AND n.round_id = $1 AND a.participant_id = $2`, round, participant); err != nil {
			return err
		}
		for _, g := range answers {
			tag, err := tx.Exec(ctx, `INSERT INTO nostradamus_answers (question_id, participant_id, answer, token)
				SELECT question_id, $3, $4, $5 FROM nostradamus_questions WHERE question_id = $1 AND round_id = $2`,
				g.Question, round, participant, g.Answer, g.Token)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("%w: question %d is not in the round", ErrBadNostradamus, g.Question)
			}
		}
		return nil
	})
	return nostradamusRefusal(err)
}

// NostradamusEntrant is someone an organiser adds to a game: a member, as a blitz entrant names one (made when new,
// with the details given), or a bot by Entrant.Name with its rule.
type NostradamusEntrant struct {
	Entrant BlitzEntrant
	BotRule string
}

// AddNostradamusParticipant adds an accepted participant to the game and returns its id. It returns
// ErrNoNostradamus, ErrNoMember, ErrMemberBanned, ErrBadMember, or ErrBadNostradamus for one already in the game.
func (a *AdminStore) AddNostradamusParticipant(ctx context.Context, slug string, e NostradamusEntrant) (int64, error) {
	edition, err := editionID(ctx, a.pool, slug)
	if err != nil {
		return 0, err
	}
	var id int64
	err = pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if e.BotRule != "" {
			return tx.QueryRow(ctx, `INSERT INTO nostradamus_participants (edition_id, name, bot, bot_rule) VALUES ($1, $2, true, $3)
				RETURNING participant_id`, edition, strings.TrimSpace(e.Entrant.Name), e.BotRule).Scan(&id)
		}
		member, name, err := entrantMember(ctx, tx, e.Entrant)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO nostradamus_participants (edition_id, name, member_id) VALUES ($1, $2, $3)
			RETURNING participant_id`, edition, name, member).Scan(&id)
	})
	if err != nil {
		return 0, nostradamusRefusal(err)
	}
	return id, nil
}

// SetNostradamusStatus accepts or refuses a participant of the game. It returns ErrNoNostradamus, or
// ErrBadNostradamus for a status that is not one.
func (a *AdminStore) SetNostradamusStatus(ctx context.Context, slug string, participant int64, status string) error {
	tag, err := a.pool.Exec(ctx, `UPDATE nostradamus_participants p SET status = $3 FROM nostradamus_editions e
		WHERE p.edition_id = e.edition_id AND e.slug = $1 AND p.participant_id = $2`, slug, participant, status)
	if err != nil {
		return nostradamusRefusal(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoNostradamus
	}
	return nil
}

// LinkNostradamusParticipant links a participant of the game to the member of that TTA name, ignoring case (made a
// member first when it names a rated player who is none yet), or unlinks it for an empty name. It returns
// ErrNoMember for a name neither a member nor a rated player, ErrMemberBanned, ErrNoNostradamus, or ErrBadNostradamus
// for a member already in the game.
func (a *AdminStore) LinkNostradamusParticipant(ctx context.Context, slug string, participant int64, member string) error {
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id *int64
		if member != "" {
			var known bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT FROM members WHERE lower(tta_name) = lower($1))
				OR EXISTS (SELECT FROM players WHERE lower(name) = lower($1))`, member).Scan(&known); err != nil {
				return fmt.Errorf("finding the member to link: %w", err)
			}
			if !known {
				return ErrNoMember
			}
			found, _, err := entrantMember(ctx, tx, BlitzEntrant{Name: member, Zone: "UTC"})
			if err != nil {
				return err
			}
			id = &found
		}
		tag, err := tx.Exec(ctx, `
			UPDATE nostradamus_participants p SET member_id = $3
			FROM nostradamus_editions e
			WHERE p.edition_id = e.edition_id AND e.slug = $1 AND p.participant_id = $2`, slug, participant, id)
		if err != nil {
			return nostradamusRefusal(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNoNostradamus
		}
		return nil
	})
	return err
}
