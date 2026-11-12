// Nostradamus games: the list, and one game as the site sees it (published rounds, results once closed, the
// standings) or as admin does (drafts, outcomes as entered, participants and their status). The rules an answer keeps
// and how it scores are the database's.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNoNostradamus is a game slug, or a round, question or participant of it, that names none.
var ErrNoNostradamus = errors.New("no such Nostradamus game")

// ErrBadNostradamus is a change the rules refuse: an outcome not of its question's kind, a label taken, a value out
// of range. Its message says which where it can.
var ErrBadNostradamus = errors.New("not allowed by the Nostradamus rules")

// NostradamusEdition is a game and the tournament it is played over; OpenUntil is when its first round open for
// answers closes, or nil while none is.
type NostradamusEdition struct {
	Slug, Name, Series, Tournament string
	OpenUntil                      *time.Time
}

// NostradamusChoice is a choice a question offers; Seed and LeaderboardRank, 0 when not given, feed the bots.
type NostradamusChoice struct {
	Label                 string
	Seed, LeaderboardRank int
}

// NostradamusCount is how many answers named a value: a closed round's poll.
type NostradamusCount struct {
	Value   string
	Answers int
}

// NostradamusQuestion is a question. A set takes Picks names; AsksLast marks one about who finishes last. Outcome is
// empty until it is known; Counts are the poll, given for a closed round only.
type NostradamusQuestion struct {
	ID                        int64
	Label, Prompt, Kind       string
	Points, NearPoints, Picks int
	AsksLast                  bool
	Choices                   []NostradamusChoice
	Outcome                   []string
	Counts                    []NostradamusCount
}

// NostradamusRound is a round: a draft until PublishedAt, open until ClosesAt (shown in Zone), scored on the site
// from ClosedAt.
type NostradamusRound struct {
	ID                    int64
	Label, Name, Zone     string
	Tokens, TokenPoints   int
	ClosesAt              *time.Time
	PublishedAt, ClosedAt *time.Time
	Questions             []NostradamusQuestion
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

// NostradamusParticipant is a participant as admin sees it: its member's TTA and Discord names, its bot rule, and
// whether it is requested, accepted or refused.
type NostradamusParticipant struct {
	ID                        int64
	Name, Member, DiscordName string
	Bot                       bool
	BotRule, Status           string
	RequestedAt               *time.Time
}

// NostradamusGame is a game with its rounds, in order, and its standings. Scored are the labels of the rounds in the
// standings. Participants are given to admin only.
type NostradamusGame struct {
	NostradamusEdition
	Rounds       []NostradamusRound
	Scored       []string
	Standings    []NostradamusStanding
	Participants []NostradamusParticipant
}

// NostradamusEditions returns every game, the newest tournament's first, each with its round open at now.
func (r *RatingsReader) NostradamusEditions(ctx context.Context, now time.Time) ([]NostradamusEdition, error) {
	return nostradamusEditions(ctx, r.pool, now)
}

// NostradamusEditions returns every game for admin, the newest tournament's first, each with its round open at now.
func (a *AdminStore) NostradamusEditions(ctx context.Context, now time.Time) ([]NostradamusEdition, error) {
	return nostradamusEditions(ctx, a.pool, now)
}

func nostradamusEditions(ctx context.Context, db conn, now time.Time) ([]NostradamusEdition, error) {
	rows, err := db.Query(ctx, `
		SELECT e.slug, e.name, s.slug, t.label,
		       (SELECT min(r.closes_at) FROM nostradamus_rounds r
		        WHERE r.edition_id = e.edition_id AND r.published_at IS NOT NULL AND r.closed_at IS NULL AND r.closes_at > $1)
		FROM nostradamus_editions e JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		ORDER BY t.tournament_id DESC, e.slug`, now)
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus games: %w", err)
	}
	editions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (NostradamusEdition, error) {
		var e NostradamusEdition
		return e, row.Scan(&e.Slug, &e.Name, &e.Series, &e.Tournament, &e.OpenUntil)
	})
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus games: %w", err)
	}
	return editions, nil
}

// NostradamusGame returns the game as the site shows it: published rounds only, outcomes and polls once a round is
// closed, and the standings over closed rounds. An unknown slug is ErrNoNostradamus.
func (r *RatingsReader) NostradamusGame(ctx context.Context, slug string) (NostradamusGame, error) {
	return nostradamusGame(ctx, r.pool, slug, true)
}

// NostradamusGame returns the game as admin sees it: every round, outcomes as entered, standings over every round
// with an outcome, and the participants. An unknown slug is ErrNoNostradamus.
func (a *AdminStore) NostradamusGame(ctx context.Context, slug string) (NostradamusGame, error) {
	return nostradamusGame(ctx, a.pool, slug, false)
}

func nostradamusGame(ctx context.Context, db conn, slug string, public bool) (NostradamusGame, error) {
	var g NostradamusGame
	var edition int64
	err := db.QueryRow(ctx, `
		SELECT e.edition_id, e.slug, e.name, s.slug, t.label
		FROM nostradamus_editions e JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		WHERE e.slug = $1`, slug).Scan(&edition, &g.Slug, &g.Name, &g.Series, &g.Tournament)
	if errors.Is(err, pgx.ErrNoRows) {
		return NostradamusGame{}, ErrNoNostradamus
	}
	if err != nil {
		return NostradamusGame{}, fmt.Errorf("reading Nostradamus game %s: %w", slug, err)
	}
	if g.Rounds, err = nostradamusRounds(ctx, db, edition, public); err != nil {
		return NostradamusGame{}, err
	}
	if g.Scored, g.Standings, err = nostradamusStandings(ctx, db, edition, public); err != nil {
		return NostradamusGame{}, err
	}
	if !public {
		if g.Participants, err = nostradamusParticipants(ctx, db, edition); err != nil {
			return NostradamusGame{}, err
		}
	}
	return g, nil
}

// nostradamusRounds reads the edition's rounds with their questions and choices: for the site, published rounds
// only, with outcomes and polls for closed ones and no seeds or ranks.
func nostradamusRounds(ctx context.Context, db conn, edition int64, public bool) ([]NostradamusRound, error) {
	rows, err := db.Query(ctx, `
		SELECT r.round_id, r.label, r.name, coalesce(r.deadline_zone, ''), r.tokens, r.token_points, r.closes_at, r.published_at,
		       r.closed_at, n.question_id, n.label, n.prompt, n.kind, n.points, n.near_points, n.picks, n.asks_last,
		       CASE WHEN $2 AND r.closed_at IS NULL THEN '{}' ELSE coalesce(n.outcome, '{}') END,
		       coalesce((SELECT array_agg(c.label ORDER BY c.position) FROM nostradamus_choices c WHERE c.question_id = n.question_id), '{}'),
		       coalesce((SELECT array_agg(CASE WHEN $2 THEN 0 ELSE coalesce(c.seed, 0) END ORDER BY c.position)
		                 FROM nostradamus_choices c WHERE c.question_id = n.question_id), '{}'),
		       coalesce((SELECT array_agg(CASE WHEN $2 THEN 0 ELSE coalesce(c.leaderboard_rank, 0) END ORDER BY c.position)
		                 FROM nostradamus_choices c WHERE c.question_id = n.question_id), '{}'),
		       coalesce((SELECT array_agg(k.value ORDER BY k.answers DESC, lower(k.value)) FROM nostradamus_answer_counts k
		                 WHERE k.question_id = n.question_id AND r.closed_at IS NOT NULL), '{}'),
		       coalesce((SELECT array_agg(k.answers ORDER BY k.answers DESC, lower(k.value)) FROM nostradamus_answer_counts k
		                 WHERE k.question_id = n.question_id AND r.closed_at IS NOT NULL), '{}')
		FROM nostradamus_rounds r
		LEFT JOIN nostradamus_questions n USING (round_id)
		WHERE r.edition_id = $1 AND (r.published_at IS NOT NULL OR NOT $2)
		ORDER BY r.position, n.position`, edition, public)
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus rounds: %w", err)
	}
	defer rows.Close()
	var rounds []NostradamusRound
	for rows.Next() {
		var r NostradamusRound
		var id *int64
		var label, prompt, kind *string
		var points, near, picks *int
		var last *bool
		var q NostradamusQuestion
		var choices, values []string
		var seeds, ranks, answers []int
		if err := rows.Scan(&r.ID, &r.Label, &r.Name, &r.Zone, &r.Tokens, &r.TokenPoints, &r.ClosesAt, &r.PublishedAt, &r.ClosedAt,
			&id, &label, &prompt, &kind, &points, &near, &picks, &last, &q.Outcome, &choices, &seeds, &ranks, &values, &answers); err != nil {
			return nil, fmt.Errorf("reading Nostradamus rounds: %w", err)
		}
		if len(rounds) == 0 || rounds[len(rounds)-1].ID != r.ID {
			rounds = append(rounds, r)
		}
		if id == nil {
			continue
		}
		q.ID, q.Label, q.Prompt, q.Kind, q.Points, q.NearPoints, q.Picks, q.AsksLast = *id, *label, *prompt, *kind, *points, *near, *picks, *last
		for i, c := range choices {
			q.Choices = append(q.Choices, NostradamusChoice{Label: c, Seed: seeds[i], LeaderboardRank: ranks[i]})
		}
		for i, v := range values {
			q.Counts = append(q.Counts, NostradamusCount{Value: v, Answers: answers[i]})
		}
		current := &rounds[len(rounds)-1]
		current.Questions = append(current.Questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading Nostradamus rounds: %w", err)
	}
	return rounds, nil
}

// nostradamusStandings reads each accepted participant's points by round, highest total first: for the site, over
// closed rounds only.
func nostradamusStandings(ctx context.Context, db conn, edition int64, public bool) ([]string, []NostradamusStanding, error) {
	rows, err := db.Query(ctx, `
		SELECT p.participant_id, p.name, coalesce(m.tta_name, ''), p.bot, r.label, s.points, s.missed
		FROM nostradamus_round_scores s
		JOIN nostradamus_rounds r USING (round_id)
		JOIN nostradamus_participants p USING (participant_id)
		LEFT JOIN members m ON m.member_id = p.member_id
		WHERE s.edition_id = $1 AND (s.closed OR NOT $2)
		ORDER BY sum(s.points) OVER (PARTITION BY p.participant_id) DESC, lower(p.name), r.position`, edition, public)
	if err != nil {
		return nil, nil, fmt.Errorf("reading Nostradamus standings: %w", err)
	}
	defer rows.Close()
	var scored []string
	var standings []NostradamusStanding
	last := int64(-1)
	for rows.Next() {
		var id int64
		var s NostradamusStanding
		var score NostradamusRoundScore
		if err := rows.Scan(&id, &s.Name, &s.Member, &s.Bot, &score.Label, &score.Points, &score.Missed); err != nil {
			return nil, nil, fmt.Errorf("reading Nostradamus standings: %w", err)
		}
		if id != last {
			standings = append(standings, s)
			last = id
		}
		current := &standings[len(standings)-1]
		current.Rounds = append(current.Rounds, score)
		current.Total += score.Points
		if !slices.Contains(scored, score.Label) {
			scored = append(scored, score.Label)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading Nostradamus standings: %w", err)
	}
	return scored, standings, nil
}

// nostradamusParticipants reads every participant of the edition, requests first, then by name.
func nostradamusParticipants(ctx context.Context, db conn, edition int64) ([]NostradamusParticipant, error) {
	rows, err := db.Query(ctx, `
		SELECT p.participant_id, p.name, coalesce(m.tta_name, ''), coalesce(m.discord_name, ''), p.bot, coalesce(p.bot_rule, ''),
		       p.status, p.requested_at
		FROM nostradamus_participants p LEFT JOIN members m ON m.member_id = p.member_id
		WHERE p.edition_id = $1
		ORDER BY p.status <> 'requested', p.bot, lower(p.name)`, edition)
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus participants: %w", err)
	}
	people, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (NostradamusParticipant, error) {
		var p NostradamusParticipant
		return p, row.Scan(&p.ID, &p.Name, &p.Member, &p.DiscordName, &p.Bot, &p.BotRule, &p.Status, &p.RequestedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("reading Nostradamus participants: %w", err)
	}
	return people, nil
}

// nullIfEmpty is values, or nil for SQL's null when there are none.
func nullIfEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}
