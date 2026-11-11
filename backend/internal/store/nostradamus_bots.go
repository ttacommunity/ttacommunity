// Nostradamus bots: when a round is published, each bot answers the questions its rule can, from the choices' seeds,
// leaderboard ranks or current Glicko ratings. A question no choice of which has the bot's measure is left for an
// organiser.
package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// botGlickoModel is the rating the Glicko bot reads: the site's main one, over every player count.
const botGlickoModel = "glicko2_daneo"

// botAnswer is a bot's answer to q by its rule ("seed", "leaderboard" or "glicko"): the best of the choices that
// have its measure (the lowest seed or rank, the highest rating), or the worst for a question asking who finishes
// last; as many as the question takes. It is nil when fewer choices than that have the measure.
func botAnswer(rule string, q NostradamusQuestion, glicko map[string]float64) []string {
	type measured struct {
		label string
		value float64
	}
	var ms []measured
	for _, c := range q.Choices {
		switch {
		case rule == "seed" && c.Seed > 0:
			ms = append(ms, measured{c.Label, float64(c.Seed)})
		case rule == "leaderboard" && c.LeaderboardRank > 0:
			ms = append(ms, measured{c.Label, float64(c.LeaderboardRank)})
		case rule == "glicko":
			if r, ok := glicko[strings.ToLower(c.Label)]; ok {
				ms = append(ms, measured{c.Label, -r})
			}
		}
	}
	picks := max(q.Picks, 1)
	if q.Kind == "number" || len(ms) < picks {
		return nil
	}
	slices.SortStableFunc(ms, func(a, b measured) int {
		if q.AsksLast {
			a, b = b, a
		}
		switch {
		case a.value < b.value:
			return -1
		case a.value > b.value:
			return 1
		}
		return 0
	})
	out := make([]string, picks)
	for i := range out {
		out[i] = ms[i].label
	}
	return out
}

// answerForBots writes each bot's answers in a round, by botAnswer, replacing any it had there.
func answerForBots(ctx context.Context, tx pgx.Tx, round int64) error {
	var bots []struct {
		id   int64
		rule string
	}
	rows, err := tx.Query(ctx, `SELECT p.participant_id, p.bot_rule FROM nostradamus_participants p JOIN nostradamus_rounds r USING (edition_id)
		WHERE r.round_id = $1 AND p.bot_rule IS NOT NULL AND p.status = 'accepted'`, round)
	if err != nil {
		return fmt.Errorf("reading the bots: %w", err)
	}
	for rows.Next() {
		var b struct {
			id   int64
			rule string
		}
		if err := rows.Scan(&b.id, &b.rule); err != nil {
			rows.Close()
			return fmt.Errorf("reading the bots: %w", err)
		}
		bots = append(bots, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(bots) == 0 {
		return err
	}
	questions, err := roundQuestions(ctx, tx, round)
	if err != nil {
		return err
	}
	glicko, err := choiceRatings(ctx, tx, round)
	if err != nil {
		return err
	}
	for _, b := range bots {
		if _, err := tx.Exec(ctx, `DELETE FROM nostradamus_answers a USING nostradamus_questions n
			WHERE a.question_id = n.question_id AND n.round_id = $1 AND a.participant_id = $2`, round, b.id); err != nil {
			return fmt.Errorf("clearing a bot's answers: %w", err)
		}
		for _, q := range questions {
			answer := botAnswer(b.rule, q, glicko)
			if answer == nil {
				continue
			}
			if _, err := tx.Exec(ctx, "INSERT INTO nostradamus_answers (question_id, participant_id, answer) VALUES ($1, $2, $3)",
				q.ID, b.id, answer); err != nil {
				return fmt.Errorf("writing a bot's answer: %w", err)
			}
		}
	}
	return nil
}

// roundQuestions reads a round's questions with their choices, in order.
func roundQuestions(ctx context.Context, tx pgx.Tx, round int64) ([]NostradamusQuestion, error) {
	rows, err := tx.Query(ctx, `
		SELECT n.question_id, n.kind, n.picks, n.asks_last, c.label, coalesce(c.seed, 0), coalesce(c.leaderboard_rank, 0)
		FROM nostradamus_questions n JOIN nostradamus_choices c USING (question_id)
		WHERE n.round_id = $1 ORDER BY n.position, c.position`, round)
	if err != nil {
		return nil, fmt.Errorf("reading a round's questions: %w", err)
	}
	defer rows.Close()
	var questions []NostradamusQuestion
	for rows.Next() {
		var q NostradamusQuestion
		var c NostradamusChoice
		if err := rows.Scan(&q.ID, &q.Kind, &q.Picks, &q.AsksLast, &c.Label, &c.Seed, &c.LeaderboardRank); err != nil {
			return nil, fmt.Errorf("reading a round's questions: %w", err)
		}
		if len(questions) == 0 || questions[len(questions)-1].ID != q.ID {
			questions = append(questions, q)
		}
		current := &questions[len(questions)-1]
		current.Choices = append(current.Choices, c)
	}
	return questions, rows.Err()
}

// choiceRatings returns the current Glicko rating of each choice in a round that names a rated player, by lower-case
// name.
func choiceRatings(ctx context.Context, tx pgx.Tx, round int64) (map[string]float64, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT lower(c.label), pr.rating
		FROM nostradamus_choices c
		JOIN nostradamus_questions n USING (question_id)
		JOIN players p ON lower(p.name) = lower(c.label)
		JOIN player_ratings pr ON pr.player_id = p.player_id AND pr.model_type = $2 AND pr.player_count = 0
		     AND pr.run_id = (SELECT run_id FROM current_run WHERE ranking_id = community_ranking())
		WHERE n.round_id = $1`, round, botGlickoModel)
	if err != nil {
		return nil, fmt.Errorf("reading the choices' ratings: %w", err)
	}
	defer rows.Close()
	ratings := map[string]float64{}
	for rows.Next() {
		var name string
		var r float64
		if err := rows.Scan(&name, &r); err != nil {
			return nil, fmt.Errorf("reading the choices' ratings: %w", err)
		}
		ratings[name] = r
	}
	return ratings, rows.Err()
}
