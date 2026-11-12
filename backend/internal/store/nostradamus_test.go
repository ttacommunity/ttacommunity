//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// question is a Nostradamus question; an empty outcome is one not known yet.
type question struct {
	label, kind  string
	points, near int
	outcome      []string
}

// entry is one participant's answers in a round, by question label, and the labels it staked a token on, once each.
type entry struct {
	name    string
	bot     bool
	answers map[string][]string
	tokens  []string
}

// nround is a round of at most three 10-point tokens, by label, with its questions and entries.
type nround struct {
	label     string
	questions []question
	entries   []entry
}

// answered is an entry answering each label in turn, "label=a,b;label=c", with a token on each label given.
func answered(name string, answers string, tokens ...string) entry {
	e := entry{name: name, answers: map[string][]string{}, tokens: tokens}
	for _, a := range strings.Split(answers, ";") {
		label, values, _ := strings.Cut(a, "=")
		e.answers[label] = strings.Split(values, ",")
	}
	return e
}

// bot is answered for a participant that is a bot.
func bot(name string, answers string) entry {
	e := answered(name, answers)
	e.bot = true
	return e
}

// nostradamusFixture migrates a database holding the World Championship's 2031 tournament and its game wc-2031,
// with no rounds yet.
func nostradamusFixture(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := migrated(t)
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('worlds', 'World Championship')`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2031' FROM series WHERE slug = 'worlds'`,
		`INSERT INTO nostradamus_editions (slug, name, tournament_id) SELECT 'wc-2031', 'Nostradamus #9', tournament_id FROM tournaments`,
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return pool
}

// closedAt is when addRounds closes a past round's answers.
var closedAt = time.Date(2031, 4, 1, 12, 0, 0, 0, time.UTC)

// addRounds writes past rounds to game wc-2031, as the one-off import of a past edition does: published, answered,
// then closed. Each participant is added once, linked to the member of its name.
func addRounds(t *testing.T, pool *pgxpool.Pool, rounds ...nround) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i, r := range rounds {
			if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_rounds (edition_id, position, label, name, tokens_max, token_points, closes_at,
				deadline_zone, published_at) SELECT edition_id, $1, $2, 'Stage ' || $2, 3, 10, $3, 'UTC', $3 FROM nostradamus_editions`,
				i, r.label, closedAt); err != nil {
				return err
			}
			for j, q := range r.questions {
				if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_questions (round_id, position, label, kind, points, near_points, picks, outcome)
					SELECT round_id, $2, $3, $4, $5, $6, $7, $8 FROM nostradamus_rounds WHERE label = $1`,
					r.label, j, q.label, q.kind, q.points, q.near, picksOf(q), nullIfEmpty(q.outcome)); err != nil {
					return err
				}
			}
			for _, e := range r.entries {
				if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_participants (edition_id, name, member_id, bot)
					SELECT edition_id, $1, (SELECT min(member_id) FROM members WHERE lower(tta_name) = lower($1)), $2
					FROM nostradamus_editions WHERE slug = 'wc-2031'
					ON CONFLICT (edition_id, lower(name)) DO NOTHING`, e.name, e.bot); err != nil {
					return err
				}
				for label, answer := range e.answers {
					staked := 0
					for _, t := range e.tokens {
						if t == label {
							staked++
						}
					}
					if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_answers (question_id, participant_id, answer, tokens)
						SELECT q.question_id, p.participant_id, $4, $5
						FROM nostradamus_questions q JOIN nostradamus_rounds r USING (round_id), nostradamus_participants p
						WHERE r.label = $1 AND q.label = $2 AND lower(p.name) = lower($3)`,
						r.label, label, e.name, answer, staked); err != nil {
						return err
					}
				}
			}
		}
		_, err := tx.Exec(ctx, "UPDATE nostradamus_rounds SET closed_at = closes_at")
		return err
	})
	if err != nil {
		t.Fatalf("adding rounds: %v", err)
	}
}

// picksOf is how many names q's set takes: as many as its outcome names, else 2; 1 for any other kind.
func picksOf(q question) int {
	switch {
	case q.kind != "set":
		return 1
	case len(q.outcome) > 0:
		return len(q.outcome)
	}
	return 2
}

// boardLines writes the game's standings, read as the site, as "name member total [round points (missed)]".
func boardLines(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	g, err := NewRatingsReader(asReader(t, pool)).NostradamusGame(context.Background(), "wc-2031", time.Now())
	if err != nil {
		t.Fatalf("NostradamusGame: %v", err)
	}
	return standingLines(g.Standings)
}

// standingLines writes standings as boardLines does.
func standingLines(standings []NostradamusStanding) []string {
	var out []string
	for _, s := range standings {
		line := fmt.Sprintf("%s %q %d", s.Name, s.Member, s.Total)
		for _, r := range s.Rounds {
			line += fmt.Sprintf(" %s=%d", r.Label, r.Points)
			if r.Missed {
				line += "(missed)"
			}
		}
		out = append(out, line)
	}
	return out
}

func TestNostradamusScoresEachKindOfAnswer(t *testing.T) {
	pick := question{label: "Q", kind: "pick", points: 4, outcome: []string{"Ann"}}
	tie := question{label: "Q", kind: "pick", points: 4, outcome: []string{"Ann", "Cy"}}
	set := question{label: "Q", kind: "set", points: 6, outcome: []string{"Ann", "Cy"}}
	number := question{label: "Q", kind: "number", points: 8, near: 4, outcome: []string{"3"}}
	tests := []struct {
		name     string
		question question
		answer   string
		tokens   int
		want     int
	}{
		{"a right pick", pick, "Ann", 0, 4},
		{"a right pick in another case", pick, "aNN", 0, 4},
		{"a wrong pick", pick, "Cy", 0, 0},
		{"a right pick with a token", pick, "Ann", 1, 14},
		{"a wrong pick with a token", pick, "Cy", 1, -10},
		{"a right pick with two tokens", pick, "Ann", 2, 24},
		{"a wrong pick with three tokens", pick, "Cy", 3, -30},
		{"either of a tie", tie, "Cy", 0, 4},
		{"a right set in another order", set, "Cy,Ann", 0, 6},
		{"a set with one wrong", set, "Ann,Dee", 0, 0},
		{"a right number", number, "3", 0, 8},
		{"a number one off", number, "2", 0, 4},
		{"a number two off", number, "5", 0, 0},
		{"a number one off with a token, which is not right", number, "4", 1, -6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := nostradamusFixture(t)
			e := answered("Bea", "Q="+tt.answer)
			for range tt.tokens {
				e.tokens = append(e.tokens, "Q")
			}
			addRounds(t, pool, nround{"S1", []question{tt.question}, []entry{e}})
			want := []string{fmt.Sprintf(`Bea "" %d S1=%d`, tt.want, tt.want)}
			if got := boardLines(t, pool); !slices.Equal(got, want) {
				t.Errorf("answering %q: board = %q, want %q", tt.answer, got, want)
			}
		})
	}
}

func TestNostradamusMissedRoundScoresTheLowestBot(t *testing.T) {
	pool := nostradamusFixture(t)
	questions := []question{{label: "A", kind: "pick", points: 4, outcome: []string{"Ann"}}, {label: "B", kind: "pick", points: 2, outcome: []string{"Cy"}}}
	addRounds(t, pool,
		nround{"S1", questions, []entry{answered("Bea", "A=Ann;B=Dee"), bot("Seed", "A=Ann;B=Cy"), bot("Glicko", "A=Dee;B=Cy")}},
		nround{"S2", questions, []entry{answered("Bea", "A=Dee;B=Dee"), answered("Eve", "A=Ann;B=Cy"),
			bot("Seed", "A=Ann;B=Dee"), bot("Glicko", "A=Dee;B=Cy")}})
	want := []string{
		`Seed "" 10 S1=6 S2=4`,
		`Eve "" 8 S1=2(missed) S2=6`,
		`Bea "" 4 S1=4 S2=0`,
		`Glicko "" 4 S1=2 S2=2`,
	}
	if got := boardLines(t, pool); !slices.Equal(got, want) {
		t.Errorf("board = %q, want %q", got, want)
	}
}

func TestNostradamusBoardNamesTheLinkedMember(t *testing.T) {
	pool := nostradamusFixture(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO members (tta_name) VALUES ('Ann')"); err != nil {
		t.Fatal(err)
	}
	questions := []question{{label: "A", kind: "pick", points: 4, outcome: []string{"Cy"}}}
	addRounds(t, pool, nround{"S1", questions, []entry{answered("ann", "A=Cy"), answered("Dee", "A=Dee")}})
	if got, want := boardLines(t, pool), []string{`ann "Ann" 4 S1=4`, `Dee "" 0 S1=0`}; !slices.Equal(got, want) {
		t.Errorf("board = %q, want %q", got, want)
	}
}

func TestNostradamusRulesRefuseWhatBreaksThem(t *testing.T) {
	answer := func(label, name, values string, tokens int) string {
		return fmt.Sprintf(`INSERT INTO nostradamus_answers (question_id, participant_id, answer, tokens)
			SELECT question_id, participant_id, '{%s}', %d FROM nostradamus_questions, nostradamus_participants
			WHERE label = '%s' AND name = '%s'`, values, tokens, label, name)
	}
	addQuestion := func(label, kind string, near int, outcome string) string {
		return fmt.Sprintf(`INSERT INTO nostradamus_questions (round_id, position, label, kind, points, near_points, outcome)
			SELECT round_id, 10, '%s', '%s', 4, %d, %s FROM nostradamus_rounds`, label, kind, near, outcome)
	}
	tests := []struct {
		name    string
		setup   []string
		refused string
	}{
		{"more tokens than the round allows",
			[]string{answer("A", "Bea", "Ann", 1), answer("B", "Bea", "Ann", 2)},
			answer("D", "Bea", "Ann", 1)},
		{"more tokens on one answer than the round allows", nil, answer("A", "Bea", "Ann", 4)},
		{"a negative stake", nil, answer("A", "Bea", "Ann", -1)},
		{"two names for a pick", nil, answer("A", "Bea", "Ann,Cy", 0)},
		{"one name twice in a set", nil, answer("S", "Bea", "Ann,ann", 0)},
		{"a set of the wrong size", nil, answer("S", "Bea", "Ann,Cy,Dee", 0)},
		{"an answer not among the choices", nil, answer("P", "Bea", "Zed", 0)},
		{"an empty name", nil, answer("A", "Bea", `""`, 0)},
		{"a number that is not one", nil, answer("N", "Bea", "three", 0)},
		{"an answer in a closed round", []string{`UPDATE nostradamus_rounds SET closed_at = published_at`}, answer("A", "Bea", "Ann", 0)},
		{"an answer from another game's participant",
			[]string{`INSERT INTO nostradamus_editions (slug, name, tournament_id) SELECT 'wc-2032', 'Nostradamus #10', tournament_id FROM tournaments`,
				`INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'Cy' FROM nostradamus_editions WHERE slug = 'wc-2032'`},
			answer("A", "Cy", "Ann", 0)},
		{"an unknown kind", nil, addQuestion("G", "guess", 0, "NULL")},
		{"near points on a pick", nil, addQuestion("G", "pick", 2, "NULL")},
		{"an outcome that is not a number", nil, addQuestion("G", "number", 0, `'{x}'`)},
		{"published without a deadline", nil, `UPDATE nostradamus_rounds SET closes_at = NULL`},
		{"a name with spaces around it",
			nil, `INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'Cy ' FROM nostradamus_editions WHERE slug = 'wc-2031'`},
		{"one name twice in a game",
			nil, `INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'bea' FROM nostradamus_editions WHERE slug = 'wc-2031'`},
		{"a bot rule on one who is not a bot",
			nil, `INSERT INTO nostradamus_participants (edition_id, name, bot_rule) SELECT edition_id, 'Cy', 'seed' FROM nostradamus_editions WHERE slug = 'wc-2031'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := nostradamusFixture(t)
			addRounds(t, pool, nround{"S1", []question{
				{label: "A", kind: "pick", points: 4}, {label: "B", kind: "pick", points: 4}, {label: "C", kind: "pick", points: 4},
				{label: "D", kind: "pick", points: 4}, {label: "S", kind: "set", points: 4}, {label: "N", kind: "number", points: 4},
				{label: "P", kind: "pick", points: 4},
			}, nil})
			ctx := context.Background()
			for _, q := range append([]string{
				`UPDATE nostradamus_rounds SET closed_at = NULL`,
				`INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'Bea' FROM nostradamus_editions`,
				`INSERT INTO nostradamus_choices (question_id, position, label) SELECT question_id, 0, 'Ann' FROM nostradamus_questions WHERE label = 'P'`,
			}, tt.setup...) {
				if _, err := pool.Exec(ctx, q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			_, err := pool.Exec(ctx, tt.refused)
			var pg *pgconn.PgError
			if !errors.As(err, &pg) || !slices.Contains([]string{"23514", "23505"}, pg.Code) {
				t.Errorf("%s = %v, want a check or unique violation", tt.refused, err)
			}
		})
	}
}

func TestNostradamusMigrationGoesDownAndUp(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	present := func() []string {
		return rowsOf(t, pool, `SELECT relname FROM pg_class WHERE relkind IN ('r', 'v') AND relname LIKE 'nostradamus%'
			UNION SELECT proname FROM pg_proc WHERE proname LIKE '%nostradamus%' ORDER BY 1`)
	}
	want := []string{"my_nostradamus", "nostradamus_answer_counts", "nostradamus_answers", "nostradamus_check_answer", "nostradamus_choices",
		"nostradamus_editions", "nostradamus_member_of", "nostradamus_participants", "nostradamus_points", "nostradamus_questions",
		"nostradamus_round_scores", "nostradamus_rounds", "nostradamus_valid", "request_nostradamus_entry", "submit_nostradamus_answers"}
	if got := present(); !slices.Equal(got, want) {
		t.Fatalf("migrated: %q, want %q", got, want)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 78); return err }); err != nil {
		t.Fatalf("migrating down to 78: %v", err)
	}
	if got := present(); len(got) != 0 {
		t.Errorf("at 78, %q remain, want none", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got := present(); !slices.Equal(got, want) {
		t.Errorf("up again: %q, want %q", got, want)
	}
}

func TestTheSiteLoginCannotReadAnAnswer(t *testing.T) {
	pool := nostradamusFixture(t)
	_, err := asReader(t, pool).Exec(context.Background(), "SELECT * FROM nostradamus_answers")
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42501" {
		t.Errorf("reading answers as the site = %v, want permission denied", err)
	}
}

func TestNostradamusEditionsListsEachGame(t *testing.T) {
	pool := nostradamusFixture(t)
	got, err := NewRatingsReader(asReader(t, pool)).NostradamusEditions(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("NostradamusEditions: %v", err)
	}
	if want := []NostradamusEdition{{Slug: "wc-2031", Name: "Nostradamus #9", Series: "worlds", Tournament: "2031"}}; !slices.Equal(got, want) {
		t.Errorf("NostradamusEditions = %+v, want %+v", got, want)
	}
}

// game is game wc-2031 with an organiser's store, a draft round and its question, and the time it is now.
type game struct {
	pool  *pgxpool.Pool
	admin *AdminStore
	round int64
	q     int64
	now   time.Time
}

// newGame is a fresh game wc-2031 with a draft round R1 (one 10-point token) asking who wins from Ann (seed 2), Cy
// (seed 1) and Dee (no seed), worth 4.
func newGame(t *testing.T) game {
	t.Helper()
	pool := nostradamusFixture(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	ctx := context.Background()
	round, err := a.AddNostradamusRound(ctx, "wc-2031", NostradamusRoundDraft{Label: "R1", Name: "Round 1", TokensMax: ptr(1), TokenPoints: 10})
	if err != nil {
		t.Fatalf("AddNostradamusRound: %v", err)
	}
	q, err := a.AddNostradamusQuestion(ctx, "wc-2031", round, NostradamusQuestion{Label: "Win", Prompt: "Who wins?", Kind: "pick", Points: 4,
		Choices: []NostradamusChoice{{Label: "Ann", Seed: 2}, {Label: "Cy", Seed: 1}, {Label: "Dee"}}})
	if err != nil {
		t.Fatalf("AddNostradamusQuestion: %v", err)
	}
	return game{pool: pool, admin: a, round: round, q: q, now: time.Date(2031, 5, 1, 12, 0, 0, 0, time.UTC)}
}

// publish publishes the game's round, closing a day after its now.
func (g game) publish(t *testing.T) {
	t.Helper()
	if err := g.admin.PublishNostradamusRound(context.Background(), "wc-2031", g.round, g.now.Add(24*time.Hour), "Europe/Berlin", g.now); err != nil {
		t.Fatalf("PublishNostradamusRound: %v", err)
	}
}

// siteGame reads wc-2031 as the site.
func (g game) siteGame(t *testing.T) NostradamusGame {
	t.Helper()
	got, err := NewRatingsReader(asReader(t, g.pool)).NostradamusGame(context.Background(), "wc-2031", time.Now())
	if err != nil {
		t.Fatalf("NostradamusGame: %v", err)
	}
	return got
}

func TestNostradamusADraftIsShownOnlyInAdminUntilPublished(t *testing.T) {
	g := newGame(t)
	if got := g.siteGame(t).Rounds; len(got) != 0 {
		t.Errorf("the site's rounds before publishing = %+v, want none", got)
	}
	admin, err := g.admin.NostradamusGame(context.Background(), "wc-2031", time.Now())
	if err != nil {
		t.Fatalf("admin's NostradamusGame: %v", err)
	}
	if got := admin.Rounds[0].Questions[0].Choices[0]; got != (NostradamusChoice{Label: "Ann", Seed: 2}) {
		t.Errorf("admin's first choice = %+v, want Ann with seed 2", got)
	}
	g.publish(t)
	site := g.siteGame(t)
	if got := site.Rounds[0].Questions[0].Choices[0]; got != (NostradamusChoice{Label: "Ann"}) {
		t.Errorf("the site's first choice = %+v, want Ann without her seed", got)
	}
	if site.Rounds[0].ClosesAt == nil || site.Rounds[0].Zone != "Europe/Berlin" {
		t.Errorf("the site's round closes at %v in %q, want a deadline in Europe/Berlin", site.Rounds[0].ClosesAt, site.Rounds[0].Zone)
	}
}

func TestNostradamusPublishingHasTheBotsAnswer(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	for _, e := range []NostradamusEntrant{{Entrant: BlitzEntrant{Name: "John Seed"}, BotRule: "seed"}, {Entrant: BlitzEntrant{Name: "Glickos"}, BotRule: "glicko"}} {
		if _, err := g.admin.AddNostradamusParticipant(ctx, "wc-2031", e); err != nil {
			t.Fatalf("AddNostradamusParticipant(%s): %v", e.Entrant.Name, err)
		}
	}
	g.publish(t)
	answers, err := g.admin.NostradamusRoundAnswers(ctx, "wc-2031", g.round)
	if err != nil {
		t.Fatalf("NostradamusRoundAnswers: %v", err)
	}
	var got []string
	for _, a := range answers {
		got = append(got, fmt.Sprintf("%s %q", a.Participant, a.Answer))
	}
	if want := []string{`John Seed ["Cy"]`}; !slices.Equal(got, want) {
		t.Errorf("the bots' answers = %q, want John Seed's best seed, and none for Glickos (no rated choice)", got)
	}
}

func TestNostradamusRoundStagesRefuseWhatTheyShould(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	if err := g.admin.SetNostradamusOutcome(ctx, "wc-2031", g.q, []string{"Ann"}); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("an outcome for a draft = %v, want ErrNostradamusRoundState", err)
	}
	if err := g.admin.PublishNostradamusRound(ctx, "wc-2031", g.round, g.now, "UTC", g.now); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("publishing with a deadline of now = %v, want ErrNostradamusRoundState", err)
	}
	empty, err := g.admin.AddNostradamusRound(ctx, "wc-2031", NostradamusRoundDraft{Label: "R2", Name: "Round 2", TokensMax: ptr(1), TokenPoints: 10})
	if err != nil {
		t.Fatalf("AddNostradamusRound: %v", err)
	}
	if err := g.admin.PublishNostradamusRound(ctx, "wc-2031", empty, g.now.Add(time.Hour), "UTC", g.now); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("publishing a round with no question = %v, want ErrNostradamusRoundState", err)
	}
	g.publish(t)
	tests := []struct {
		name string
		err  error
	}{
		{"publishing again", g.admin.PublishNostradamusRound(ctx, "wc-2031", g.round, g.now.Add(time.Hour), "UTC", g.now)},
		{"changing a published question", g.admin.SetNostradamusQuestion(ctx, "wc-2031", g.q, NostradamusQuestion{Label: "Win", Kind: "pick", Points: 9})},
		{"removing a published question", g.admin.RemoveNostradamusQuestion(ctx, "wc-2031", g.q)},
		{"removing a published round", g.admin.RemoveNostradamusRound(ctx, "wc-2031", g.round)},
		{"closing before the deadline", g.admin.CloseNostradamusRound(ctx, "wc-2031", g.round, g.now.Add(time.Hour))},
		{"closing with an outcome missing", g.admin.CloseNostradamusRound(ctx, "wc-2031", g.round, g.now.Add(48*time.Hour))},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, ErrNostradamusRoundState) {
			t.Errorf("%s = %v, want ErrNostradamusRoundState", tt.name, tt.err)
		}
	}
	if err := g.admin.CloseNostradamusRound(ctx, "wc-2099", g.round, g.now); !errors.Is(err, ErrNoNostradamus) {
		t.Errorf("closing another game's round = %v, want ErrNoNostradamus", err)
	}
}

func TestNostradamusAdminEditsADraftsQuestion(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	err := g.admin.SetNostradamusQuestion(ctx, "wc-2031", g.q, NostradamusQuestion{Label: "Top", Prompt: "Who are the top two?", Kind: "set",
		Points: 6, Picks: 2, Choices: []NostradamusChoice{{Label: "Ann"}, {Label: "Cy"}}})
	if err != nil {
		t.Fatalf("SetNostradamusQuestion: %v", err)
	}
	admin, err := g.admin.NostradamusGame(ctx, "wc-2031", time.Now())
	if err != nil {
		t.Fatalf("NostradamusGame: %v", err)
	}
	q := admin.Rounds[0].Questions[0]
	if q.Label != "Top" || q.Picks != 2 || len(q.Choices) != 2 {
		t.Errorf("the question after the change = %+v, want Top, a set of 2 from 2 choices", q)
	}
	if err := g.admin.SetNostradamusQuestion(ctx, "wc-2031", g.q, NostradamusQuestion{Label: "Top", Kind: "pick", Points: 6, Picks: 2}); !errors.Is(err, ErrBadNostradamus) {
		t.Errorf("a pick of 2 = %v, want ErrBadNostradamus", err)
	}
}

// nostradamusPlayers is playersFixture's database with game wc-2031 over worlds 2031 and a round R1 (one 10-point
// token) asking who wins from Ann and Cy, published until a day after now.
func nostradamusPlayers(t *testing.T) (game, *PlayerStore) {
	t.Helper()
	pool, a, p := playersFixture(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2031' FROM series WHERE slug = 'worlds'`,
		`INSERT INTO nostradamus_editions (slug, name, tournament_id)
		 SELECT 'wc-2031', 'Nostradamus #9', t.tournament_id FROM tournaments t JOIN series s USING (series_id) WHERE s.slug = 'worlds' AND t.label = '2031'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	g := game{pool: pool, admin: a, now: signInAt}
	var err error
	if g.round, err = a.AddNostradamusRound(ctx, "wc-2031", NostradamusRoundDraft{Label: "R1", Name: "Round 1", TokensMax: ptr(1), TokenPoints: 10}); err != nil {
		t.Fatalf("AddNostradamusRound: %v", err)
	}
	if g.q, err = a.AddNostradamusQuestion(ctx, "wc-2031", g.round, NostradamusQuestion{Label: "Win", Kind: "pick", Points: 4,
		Choices: []NostradamusChoice{{Label: "Ann"}, {Label: "Cy"}}}); err != nil {
		t.Fatalf("AddNostradamusQuestion: %v", err)
	}
	g.publish(t)
	return g, p
}

func TestNostradamusAPlayerAsksToJoinAndAnswersOnceAccepted(t *testing.T) {
	g, p := nostradamusPlayers(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "bea_d", "Bea"), "token-1")
	approved(t, g.admin, p, l, "Bea")
	hash := tokenHash("token-1")
	answer := []NostradamusGiven{{Question: g.q, Answer: []string{"Cy"}, Tokens: 1}}
	if _, err := p.SubmitNostradamusAnswers(ctx, hash, g.round, answer, g.now); !errors.Is(err, ErrNotEntered) {
		t.Errorf("answering before joining = %v, want ErrNotEntered", err)
	}
	if got, err := p.RequestNostradamusEntry(ctx, hash, "wc-2031", g.now); err != nil || got != "requested" {
		t.Fatalf("RequestNostradamusEntry = %q, %v; want requested", got, err)
	}
	if _, err := p.SubmitNostradamusAnswers(ctx, hash, g.round, answer, g.now); !errors.Is(err, ErrNotEntered) {
		t.Errorf("answering while requested = %v, want ErrNotEntered", err)
	}
	admin, err := g.admin.NostradamusGame(ctx, "wc-2031", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := admin.Participants[0]; got.Name != "Bea" || got.Status != "requested" {
		t.Fatalf("admin's first participant = %+v, want Bea's request", got)
	}
	if err := g.admin.SetNostradamusStatus(ctx, "wc-2031", admin.Participants[0].ID, "accepted"); err != nil {
		t.Fatalf("SetNostradamusStatus: %v", err)
	}
	if n, err := p.SubmitNostradamusAnswers(ctx, hash, g.round, answer, g.now); err != nil || n != 1 {
		t.Fatalf("SubmitNostradamusAnswers = %d, %v; want 1 saved", n, err)
	}
	mine, err := p.MyNostradamus(ctx, hash, "wc-2031", g.now)
	if err != nil {
		t.Fatalf("MyNostradamus: %v", err)
	}
	if got := mine.Answers[g.q]; mine.Status != "accepted" || !slices.Equal(got.Answer, []string{"Cy"}) || got.Tokens != 1 {
		t.Errorf("MyNostradamus = %+v, want accepted with Cy and a token", mine)
	}
	if err := g.admin.SetNostradamusOutcome(ctx, "wc-2031", g.q, []string{"Cy"}); err != nil {
		t.Fatal(err)
	}
	if err := g.admin.CloseNostradamusRound(ctx, "wc-2031", g.round, g.now.Add(25*time.Hour)); err != nil {
		t.Fatalf("CloseNostradamusRound: %v", err)
	}
	site := g.siteGame(t)
	if got, want := standingLines(site.Standings), []string{`Bea "Bea" 14 R1=14`}; !slices.Equal(got, want) {
		t.Errorf("standings after closing = %q, want %q", got, want)
	}
	if got := site.Rounds[0].Questions[0]; !slices.Equal(got.Outcome, []string{"Cy"}) || len(got.Counts) != 1 || got.Counts[0] != (NostradamusCount{Value: "Cy", Answers: 1}) {
		t.Errorf("the closed question on the site = %+v, want its outcome and a poll of Cy 1", got)
	}
}

func TestNostradamusAnswersAreRefusedWhenTheyShouldBe(t *testing.T) {
	g, p := nostradamusPlayers(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "bea_d", "Bea"), "token-1")
	approved(t, g.admin, p, l, "Bea")
	hash := tokenHash("token-1")
	if _, err := g.admin.AddNostradamusParticipant(ctx, "wc-2031", NostradamusEntrant{Entrant: BlitzEntrant{Name: "Bea", Zone: "UTC"}}); err != nil {
		t.Fatalf("adding Bea: %v", err)
	}
	draft, err := g.admin.AddNostradamusRound(ctx, "wc-2031", NostradamusRoundDraft{Label: "R2", Name: "Round 2", TokensMax: ptr(1), TokenPoints: 10})
	if err != nil {
		t.Fatal(err)
	}
	one := func(answer string, tokens int) []NostradamusGiven {
		return []NostradamusGiven{{Question: g.q, Answer: []string{answer}, Tokens: tokens}}
	}
	tests := []struct {
		name    string
		hash    []byte
		round   int64
		answers []NostradamusGiven
		at      time.Time
		want    error
	}{
		{"signed out", tokenHash("no-such-token"), g.round, one("Cy", 0), g.now, ErrNoSession},
		{"a draft round", hash, draft, one("Cy", 0), g.now, ErrRoundNotOpen},
		{"after the deadline", hash, g.round, one("Cy", 0), g.now.Add(24 * time.Hour), ErrPastDeadline},
		{"a name not among the choices", hash, g.round, one("Zed", 0), g.now, ErrBadNostradamus},
		{"a question of another round", hash, g.round, []NostradamusGiven{{Question: g.q + 100, Answer: []string{"Cy"}}}, g.now, ErrBadNostradamus},
		{"more tokens than the round's most", hash, g.round, one("Cy", 2), g.now, ErrBadNostradamus},
		{"one question answered twice", hash, g.round, []NostradamusGiven{{Question: g.q, Answer: []string{"Cy"}},
			{Question: g.q, Answer: []string{"Ann"}}}, g.now, ErrBadNostradamus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := p.SubmitNostradamusAnswers(ctx, tt.hash, tt.round, tt.answers, tt.at); !errors.Is(err, tt.want) {
				t.Errorf("SubmitNostradamusAnswers = %v, want %v", err, tt.want)
			}
		})
	}
	mine, err := p.MyNostradamus(ctx, hash, "wc-2031", g.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine.Answers) != 0 {
		t.Errorf("after only refused answers, MyNostradamus = %+v, want no answers", mine.Answers)
	}
}

func TestNostradamusJoiningLinksAnImportedName(t *testing.T) {
	g, p := nostradamusPlayers(t)
	ctx := context.Background()
	if _, err := g.pool.Exec(ctx, `INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'bea' FROM nostradamus_editions`); err != nil {
		t.Fatal(err)
	}
	l := signedIn(t, p, discordID("100000000000000001", "bea_d", "Bea"), "token-1")
	approved(t, g.admin, p, l, "Bea")
	if got, err := p.RequestNostradamusEntry(ctx, tokenHash("token-1"), "wc-2031", g.now); err != nil || got != "accepted" {
		t.Errorf("RequestNostradamusEntry for an imported name = %q, %v; want accepted, linked", got, err)
	}
	if got, err := p.RequestNostradamusEntry(ctx, tokenHash("token-1"), "wc-2099", g.now); !errors.Is(err, ErrNoNostradamus) {
		t.Errorf("asking to join no game = %q, %v; want ErrNoNostradamus", got, err)
	}
}

func TestNostradamusAdminAddsAndLinksMembers(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	if _, err := g.pool.Exec(ctx, "INSERT INTO members (tta_name) VALUES ('Beatrix')"); err != nil {
		t.Fatal(err)
	}
	id, err := g.admin.AddNostradamusParticipant(ctx, "wc-2031", NostradamusEntrant{Entrant: BlitzEntrant{Name: "Newcomer", DiscordName: "new_one", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddNostradamusParticipant: %v", err)
	}
	if _, err := g.admin.AddNostradamusParticipant(ctx, "wc-2031", NostradamusEntrant{Entrant: BlitzEntrant{Name: "newcomer", Zone: "UTC"}}); !errors.Is(err, ErrBadNostradamus) {
		t.Errorf("adding the same member again = %v, want ErrBadNostradamus", err)
	}
	if err := g.admin.LinkNostradamusParticipant(ctx, "wc-2031", id, "beatrix"); err != nil {
		t.Fatalf("LinkNostradamusParticipant: %v", err)
	}
	admin, err := g.admin.NostradamusGame(ctx, "wc-2031", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := admin.Participants[0]; got.Name != "Newcomer" || got.Member != "Beatrix" || got.Status != "accepted" {
		t.Errorf("the participant = %+v, want Newcomer, accepted, linked to Beatrix", got)
	}
	for _, tt := range []struct {
		name string
		err  error
		want error
	}{
		{"linking no such member", g.admin.LinkNostradamusParticipant(ctx, "wc-2031", id, "Zed"), ErrNoMember},
		{"linking another game's participant", g.admin.LinkNostradamusParticipant(ctx, "wc-2099", id, "Beatrix"), ErrNoNostradamus},
		{"a status that is not one", g.admin.SetNostradamusStatus(ctx, "wc-2031", id, "maybe"), ErrBadNostradamus},
		{"a status for no participant", g.admin.SetNostradamusStatus(ctx, "wc-2031", id+100, "refused"), ErrNoNostradamus},
	} {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s = %v, want %v", tt.name, tt.err, tt.want)
		}
	}
}

func TestNostradamusAdminAddsAGameOverATournament(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	if err := g.admin.AddNostradamusEdition(ctx, NostradamusEdition{Slug: "wc-2031-b", Name: "Side bets", Series: "worlds", Tournament: "2031"}); err != nil {
		t.Fatalf("AddNostradamusEdition: %v", err)
	}
	for _, tt := range []struct {
		name string
		e    NostradamusEdition
		want error
	}{
		{"a slug taken", NostradamusEdition{Slug: "wc-2031", Name: "Again", Series: "worlds", Tournament: "2031"}, ErrBadNostradamus},
		{"a slug not of the pattern", NostradamusEdition{Slug: "WC 2031", Name: "Bad", Series: "worlds", Tournament: "2031"}, ErrBadNostradamus},
		{"no such tournament", NostradamusEdition{Slug: "wc-2099", Name: "Later", Series: "worlds", Tournament: "2099"}, ErrNoTournament},
	} {
		if err := g.admin.AddNostradamusEdition(ctx, tt.e); !errors.Is(err, tt.want) {
			t.Errorf("%s: AddNostradamusEdition = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestNostradamusLinkingARatedPlayerMakesThemAMember(t *testing.T) {
	pool := nostradamusFixture(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO players (name) VALUES ('Rated_Rae')"); err != nil {
		t.Fatal(err)
	}
	addRounds(t, pool, nround{"S1", []question{{label: "A", kind: "pick", points: 4, outcome: []string{"Ann"}}}, []entry{answered("rae", "A=Ann")}})
	admin, err := a.NostradamusGame(ctx, "wc-2031", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.LinkNostradamusParticipant(ctx, "wc-2031", admin.Participants[0].ID, "rated_rae"); err != nil {
		t.Fatalf("linking a rated player who is no member = %v, want them made a member and linked", err)
	}
	if got, want := boardLines(t, pool), []string{`rae "Rated_Rae" 4 S1=4`}; !slices.Equal(got, want) {
		t.Errorf("board = %q, want %q", got, want)
	}
}

// answerAs signs player Bea in on a game with round R1 open, accepted, and saves her answer; it returns her session.
func answerAs(t *testing.T, g game, p *PlayerStore, answer string) []byte {
	t.Helper()
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "bea_d", "Bea"), "token-1")
	approved(t, g.admin, p, l, "Bea")
	hash := tokenHash("token-1")
	if _, err := p.RequestNostradamusEntry(ctx, hash, "wc-2031", g.now); err != nil {
		t.Fatal(err)
	}
	admin, err := g.admin.NostradamusGame(ctx, "wc-2031", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := g.admin.SetNostradamusStatus(ctx, "wc-2031", admin.Participants[0].ID, "accepted"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SubmitNostradamusAnswers(ctx, hash, g.round, []NostradamusGiven{{Question: g.q, Answer: []string{answer}}}, g.now); err != nil {
		t.Fatalf("answering: %v", err)
	}
	return hash
}

func TestNostradamusUnpublishingKeepsTheAnswersThatStillFit(t *testing.T) {
	tests := []struct {
		name   string
		change NostradamusQuestion
		kept   bool
	}{
		{"a question reworded", NostradamusQuestion{Label: "Win", Prompt: "Who will win it all?", Kind: "pick", Points: 5,
			Choices: []NostradamusChoice{{Label: "Ann"}, {Label: "Cy"}}}, true},
		{"its choices changed", NostradamusQuestion{Label: "Win", Kind: "pick", Points: 4,
			Choices: []NostradamusChoice{{Label: "Ann"}, {Label: "Cy"}, {Label: "Dee"}}}, false},
		{"its kind changed", NostradamusQuestion{Label: "Win", Kind: "set", Picks: 2, Points: 4,
			Choices: []NostradamusChoice{{Label: "Ann"}, {Label: "Cy"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, p := nostradamusPlayers(t)
			ctx := context.Background()
			hash := answerAs(t, g, p, "Cy")
			if err := g.admin.UnpublishNostradamusRound(ctx, "wc-2031", g.round); err != nil {
				t.Fatalf("UnpublishNostradamusRound: %v", err)
			}
			if got := g.siteGame(t).Rounds; len(got) != 0 {
				t.Errorf("the site's rounds after unpublishing = %d, want none", len(got))
			}
			if err := g.admin.SetNostradamusQuestion(ctx, "wc-2031", g.q, tt.change); err != nil {
				t.Fatalf("SetNostradamusQuestion: %v", err)
			}
			g.publish(t)
			mine, err := p.MyNostradamus(ctx, hash, "wc-2031", g.now)
			if err != nil {
				t.Fatal(err)
			}
			if _, kept := mine.Answers[g.q]; kept != tt.kept {
				t.Errorf("after %s, the answer kept = %t, want %t", tt.name, kept, tt.kept)
			}
		})
	}
}

func TestNostradamusDeadlinesMoveAndUnpublishingIsRefusedWhenItShouldBe(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	later := g.now.Add(72 * time.Hour)
	if err := g.admin.SetNostradamusDeadline(ctx, "wc-2031", g.round, later, "UTC", g.now); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("a draft's deadline = %v, want ErrNostradamusRoundState (it is set on publishing)", err)
	}
	if err := g.admin.UnpublishNostradamusRound(ctx, "wc-2031", g.round); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("unpublishing a draft = %v, want ErrNostradamusRoundState", err)
	}
	g.publish(t)
	if err := g.admin.SetNostradamusDeadline(ctx, "wc-2031", g.round, later, "Europe/Berlin", g.now); err != nil {
		t.Fatalf("SetNostradamusDeadline: %v", err)
	}
	site := g.siteGame(t).Rounds[0]
	if site.ClosesAt == nil || !site.ClosesAt.Equal(later) || site.Zone != "Europe/Berlin" {
		t.Errorf("the deadline after moving it = %v in %q, want %v in Europe/Berlin", site.ClosesAt, site.Zone, later)
	}
	if err := g.admin.SetNostradamusDeadline(ctx, "wc-2031", g.round, g.now, "UTC", g.now); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("a deadline of now = %v, want ErrNostradamusRoundState", err)
	}
	if err := g.admin.SetNostradamusOutcome(ctx, "wc-2031", g.q, []string{"Ann"}); err != nil {
		t.Fatal(err)
	}
	if err := g.admin.CloseNostradamusRound(ctx, "wc-2031", g.round, later); err != nil {
		t.Fatal(err)
	}
	if err := g.admin.UnpublishNostradamusRound(ctx, "wc-2031", g.round); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("unpublishing a closed round = %v, want ErrNostradamusRoundState", err)
	}
	if err := g.admin.SetNostradamusDeadline(ctx, "wc-2031", g.round, later.Add(time.Hour), "UTC", later); !errors.Is(err, ErrNostradamusRoundState) {
		t.Errorf("a closed round's deadline = %v, want ErrNostradamusRoundState", err)
	}
}

func TestNostradamusEditionsSayWhenARoundIsOpen(t *testing.T) {
	g := newGame(t)
	g.publish(t)
	got, err := NewRatingsReader(asReader(t, g.pool)).NostradamusEditions(context.Background(), g.now)
	if err != nil {
		t.Fatal(err)
	}
	if want := g.now.Add(24 * time.Hour); got[0].OpenUntil == nil || !got[0].OpenUntil.Equal(want) {
		t.Errorf("the game's open round closes at %v, want %v", got[0].OpenUntil, want)
	}
}

func TestNostradamusAPlayerStakesAtLeastTheRoundsLeast(t *testing.T) {
	g, p := nostradamusPlayers(t)
	ctx := context.Background()
	hash := answerAs(t, g, p, "Cy")
	if _, err := g.pool.Exec(ctx, "UPDATE nostradamus_rounds SET tokens_min = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SubmitNostradamusAnswers(ctx, hash, g.round, []NostradamusGiven{{Question: g.q, Answer: []string{"Ann"}}}, g.now); !errors.Is(err, ErrFewTokens) {
		t.Errorf("answers staking no token where one is the least = %v, want ErrFewTokens", err)
	}
	mine, err := p.MyNostradamus(ctx, hash, "wc-2031", g.now)
	if err != nil {
		t.Fatal(err)
	}
	if got := mine.Answers[g.q].Answer; !slices.Equal(got, []string{"Cy"}) {
		t.Errorf("after the refused answers, the answer kept = %q, want the one saved before (Cy)", got)
	}
	if _, err := p.SubmitNostradamusAnswers(ctx, hash, g.round, []NostradamusGiven{{Question: g.q, Answer: []string{"Ann"}, Tokens: 1}}, g.now); err != nil {
		t.Errorf("answers staking the least = %v, want saved", err)
	}
}

func TestNostradamusGamesFinishByTheirMarkOrTheirTournament(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	reader := NewRatingsReader(asReader(t, g.pool))
	state := func() (marked, tournament, finished bool) {
		t.Helper()
		list, err := reader.NostradamusEditions(ctx, g.now)
		if err != nil {
			t.Fatal(err)
		}
		e := list[0]
		return e.MarkedFinished, e.TournamentFinished, e.Finished
	}
	if m, tf, f := state(); m || tf || f {
		t.Errorf("a new game: marked %t, tournament %t, finished %t; want none", m, tf, f)
	}
	if _, err := g.pool.Exec(ctx, `INSERT INTO cge_tournaments (cge_tournament, name, status, series_id, tournament_id, finished)
		SELECT 9001, 'WC 2031 stage 1', 'running', series_id, tournament_id, NULL FROM tournaments`); err != nil {
		t.Fatalf("adding a CGE tournament: %v", err)
	}
	if _, _, f := state(); f {
		t.Error("a game whose CGE tournament is still running is finished, want not")
	}
	if _, err := g.pool.Exec(ctx, "UPDATE cge_tournaments SET finished = '2031-12-01'"); err != nil {
		t.Fatal(err)
	}
	if m, tf, f := state(); m || !tf || !f {
		t.Errorf("once its CGE tournament finished: marked %t, tournament %t, finished %t; want finished by the tournament", m, tf, f)
	}
	if _, err := g.pool.Exec(ctx, "UPDATE cge_tournaments SET finished = NULL"); err != nil {
		t.Fatal(err)
	}
	if err := g.admin.SetNostradamusFinished(ctx, "wc-2031", true, g.now); err != nil {
		t.Fatalf("SetNostradamusFinished: %v", err)
	}
	if m, _, f := state(); !m || !f {
		t.Errorf("once marked: marked %t, finished %t; want both", m, f)
	}
	if err := g.admin.SetNostradamusFinished(ctx, "wc-2031", false, g.now); err != nil {
		t.Fatal(err)
	}
	if _, _, f := state(); f {
		t.Error("unmarked again, the game is finished, want not")
	}
	if err := g.admin.SetNostradamusFinished(ctx, "wc-2099", true, g.now); !errors.Is(err, ErrNoNostradamus) {
		t.Errorf("finishing no game = %v, want ErrNoNostradamus", err)
	}
}

func TestNostradamusGameListsItsAcceptedPlayers(t *testing.T) {
	g := newGame(t)
	ctx := context.Background()
	for _, e := range []NostradamusEntrant{{Entrant: BlitzEntrant{Name: "Bea", Zone: "UTC"}}, {Entrant: BlitzEntrant{Name: "Seed"}, BotRule: "seed"}, {Entrant: BlitzEntrant{Name: "Cy", Zone: "UTC"}}} {
		if _, err := g.admin.AddNostradamusParticipant(ctx, "wc-2031", e); err != nil {
			t.Fatal(err)
		}
	}
	admin, err := g.admin.NostradamusGame(ctx, "wc-2031", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range admin.Participants {
		if p.Name == "Cy" {
			if err := g.admin.SetNostradamusStatus(ctx, "wc-2031", p.ID, "refused"); err != nil {
				t.Fatal(err)
			}
		}
	}
	var got []string
	for _, p := range g.siteGame(t).Players {
		got = append(got, fmt.Sprintf("%s %q %t", p.Name, p.Member, p.Bot))
	}
	if want := []string{`Bea "Bea" false`, `Seed "" true`}; !slices.Equal(got, want) {
		t.Errorf("the site's players = %q, want the accepted ones, people first: %q", got, want)
	}
}
