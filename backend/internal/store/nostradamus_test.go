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

// entry is one participant's answers in a round, by question label, and the labels it put tokens on.
type entry struct {
	name    string
	bot     bool
	answers map[string][]string
	tokens  []string
}

// nround is a round of three 10-point tokens, by label, with its questions and entries.
type nround struct {
	label     string
	questions []question
	entries   []entry
}

// answered is an entry answering each label in turn, "label=a,b;label=c", with tokens on the labels given.
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

// nostradamusFixture migrates a database holding the World Championship's 2031 tournament and its edition
// wc-2031, with no rounds yet.
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

// addRounds writes rounds to edition wc-2031, adding each participant once and linking it to the member of its
// name, as the one-off import of a past edition does.
func addRounds(t *testing.T, pool *pgxpool.Pool, rounds ...nround) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i, r := range rounds {
			if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_rounds (edition_id, position, label, name, tokens, token_points, closes_at)
				SELECT edition_id, $1, $2, 'Stage ' || $2, 3, 10, $3 FROM nostradamus_editions`,
				i, r.label, time.Date(2031, 4, 1, 12, 0, 0, 0, time.UTC)); err != nil {
				return err
			}
			for j, q := range r.questions {
				var outcome []string
				if len(q.outcome) > 0 {
					outcome = q.outcome
				}
				if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_questions (round_id, position, label, kind, points, near_points, outcome)
					SELECT round_id, $2, $3, $4, $5, $6, $7 FROM nostradamus_rounds WHERE label = $1`,
					r.label, j, q.label, q.kind, q.points, q.near, outcome); err != nil {
					return err
				}
			}
			for _, e := range r.entries {
				if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_participants (edition_id, name, member_id, bot)
					SELECT edition_id, $1, (SELECT min(member_id) FROM members WHERE lower(tta_name) = lower($1)), $2
					FROM nostradamus_editions
					ON CONFLICT (edition_id, lower(name)) DO NOTHING`, e.name, e.bot); err != nil {
					return err
				}
				for label, answer := range e.answers {
					if _, err := tx.Exec(ctx, `INSERT INTO nostradamus_answers (question_id, participant_id, answer, token)
						SELECT q.question_id, p.participant_id, $4, $5
						FROM nostradamus_questions q JOIN nostradamus_rounds r USING (round_id), nostradamus_participants p
						WHERE r.label = $1 AND q.label = $2 AND lower(p.name) = lower($3)`,
						r.label, label, e.name, answer, slices.Contains(e.tokens, label)); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("adding rounds: %v", err)
	}
}

// boardLines writes the edition's board, read as the site, as "name member total [round points (missed)]".
func boardLines(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	board, err := NewRatingsReader(asReader(t, pool)).NostradamusBoard(context.Background(), "wc-2031")
	if err != nil {
		t.Fatalf("NostradamusBoard: %v", err)
	}
	var out []string
	for _, s := range board.Standings {
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
		token    bool
		want     int
	}{
		{"a right pick", pick, "Ann", false, 4},
		{"a right pick in another case", pick, "aNN", false, 4},
		{"a wrong pick", pick, "Cy", false, 0},
		{"a right pick with a token", pick, "Ann", true, 14},
		{"a wrong pick with a token", pick, "Cy", true, -10},
		{"either of a tie", tie, "Cy", false, 4},
		{"a right set in another order", set, "Cy,Ann", false, 6},
		{"a set with one wrong", set, "Ann,Dee", false, 0},
		{"a set with one missing", set, "Ann", false, 0},
		{"a right number", number, "3", false, 8},
		{"a number one off", number, "2", false, 4},
		{"a number two off", number, "5", false, 0},
		{"a number one off with a token, which is not right", number, "4", true, -6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := nostradamusFixture(t)
			e := answered("Bea", "Q="+tt.answer)
			if tt.token {
				e.tokens = []string{"Q"}
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

func TestNostradamusRoundIsScoredOnlyOnceAnOutcomeIsKnown(t *testing.T) {
	pool := nostradamusFixture(t)
	addRounds(t, pool,
		nround{"S1", []question{{label: "A", kind: "pick", points: 4, outcome: []string{"Ann"}}}, []entry{answered("Bea", "A=Ann")}},
		nround{"S2", []question{{label: "A", kind: "pick", points: 4}}, []entry{answered("Bea", "A=Ann")}})
	if got, want := boardLines(t, pool), []string{`Bea "" 4 S1=4`}; !slices.Equal(got, want) {
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

func TestNostradamusBoardOfNoEditionIsRefused(t *testing.T) {
	pool := nostradamusFixture(t)
	if _, err := NewRatingsReader(asReader(t, pool)).NostradamusBoard(context.Background(), "wc-2099"); !errors.Is(err, ErrNoNostradamus) {
		t.Errorf("NostradamusBoard(wc-2099) = %v, want ErrNoNostradamus", err)
	}
}

func TestNostradamusRulesRefuseWhatBreaksThem(t *testing.T) {
	answer := func(label, name, values string, token bool) string {
		return fmt.Sprintf(`INSERT INTO nostradamus_answers (question_id, participant_id, answer, token)
			SELECT question_id, participant_id, '{%s}', %t FROM nostradamus_questions, nostradamus_participants
			WHERE label = '%s' AND name = '%s'`, values, token, label, name)
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
			[]string{answer("A", "Bea", "Ann", true), answer("B", "Bea", "Ann", true), answer("C", "Bea", "Ann", true)},
			answer("D", "Bea", "Ann", true)},
		{"two names for a pick", nil, answer("A", "Bea", "Ann,Cy", false)},
		{"one name twice in a set", nil, answer("S", "Bea", "Ann,ann", false)},
		{"an empty name", nil, answer("A", "Bea", `""`, false)},
		{"a number that is not one", nil, answer("N", "Bea", "three", false)},
		{"an answer from another edition's participant",
			[]string{`INSERT INTO nostradamus_editions (slug, name, tournament_id) SELECT 'wc-2032', 'Nostradamus #10', tournament_id FROM tournaments`,
				`INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'Cy' FROM nostradamus_editions WHERE slug = 'wc-2032'`},
			answer("A", "Cy", "Ann", false)},
		{"an unknown kind", nil, addQuestion("G", "guess", 0, "NULL")},
		{"near points on a pick", nil, addQuestion("G", "pick", 2, "NULL")},
		{"an outcome that is not a number", nil, addQuestion("G", "number", 0, `'{x}'`)},
		{"a name with spaces around it",
			nil, `INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'Cy ' FROM nostradamus_editions`},
		{"one name twice in an edition",
			nil, `INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'bea' FROM nostradamus_editions`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := nostradamusFixture(t)
			addRounds(t, pool, nround{"S1", []question{
				{label: "A", kind: "pick", points: 4}, {label: "B", kind: "pick", points: 4}, {label: "C", kind: "pick", points: 4},
				{label: "D", kind: "pick", points: 4}, {label: "S", kind: "set", points: 4}, {label: "N", kind: "number", points: 4},
			}, []entry{}})
			ctx := context.Background()
			for _, q := range append([]string{`INSERT INTO nostradamus_participants (edition_id, name) SELECT edition_id, 'Bea' FROM nostradamus_editions`}, tt.setup...) {
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
		return rowsOf(t, pool, `SELECT relname FROM pg_class WHERE relkind IN ('r', 'v') AND relname LIKE 'nostradamus%' ORDER BY 1`)
	}
	want := []string{"nostradamus_answers", "nostradamus_editions", "nostradamus_participants", "nostradamus_questions",
		"nostradamus_round_scores", "nostradamus_rounds"}
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

func TestNostradamusEditionsListsEachEdition(t *testing.T) {
	pool := nostradamusFixture(t)
	got, err := NewRatingsReader(asReader(t, pool)).NostradamusEditions(context.Background())
	if err != nil {
		t.Fatalf("NostradamusEditions: %v", err)
	}
	if want := []NostradamusEdition{{Slug: "wc-2031", Name: "Nostradamus #9", Series: "worlds", Tournament: "2031"}}; !slices.Equal(got, want) {
		t.Errorf("NostradamusEditions = %+v, want %+v", got, want)
	}
}

// participantLines writes a participant's rounds as "round points" and each answer as "question answer token
// points", read as the site at now.
func participantLines(t *testing.T, pool *pgxpool.Pool, name string, now time.Time) []string {
	t.Helper()
	p, err := NewRatingsReader(asReader(t, pool)).NostradamusParticipant(context.Background(), "wc-2031", name, now)
	if err != nil {
		t.Fatalf("NostradamusParticipant(%s): %v", name, err)
	}
	var out []string
	for _, r := range p.Rounds {
		out = append(out, fmt.Sprintf("%s %s missed=%t", r.Label, intOrDash(r.Points), r.Missed))
		for _, a := range r.Answers {
			out = append(out, fmt.Sprintf("  %s %q token=%t %s", a.Question.Label, a.Answer, a.Token, intOrDash(a.Points)))
		}
	}
	return out
}

// intOrDash is n, or "-" for none.
func intOrDash(n *int) string {
	if n == nil {
		return "-"
	}
	return fmt.Sprint(*n)
}

func TestNostradamusParticipantShowsTheirAnswersInClosedRounds(t *testing.T) {
	pool := nostradamusFixture(t)
	addRounds(t, pool,
		nround{"S1", []question{{label: "A", kind: "pick", points: 4, outcome: []string{"Ann"}}, {label: "B", kind: "pick", points: 2}},
			[]entry{answered("Bea", "A=Ann", "A"), answered("Cy", "A=Dee;B=Ann")}},
		nround{"S2", []question{{label: "A", kind: "pick", points: 4}}, []entry{answered("Cy", "A=Ann")}})
	if _, err := pool.Exec(context.Background(), "UPDATE nostradamus_rounds SET closes_at = '2031-05-01T12:00:00Z' WHERE label = 'S2'"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, participant string
		now               time.Time
		want              []string
	}{
		{"one who answered", "bea", time.Date(2031, 4, 2, 0, 0, 0, 0, time.UTC), []string{
			"S1 14 missed=false", `  A ["Ann"] token=true 14`}},
		{"an answer whose outcome is not known yet", "Cy", time.Date(2031, 4, 2, 0, 0, 0, 0, time.UTC), []string{
			"S1 0 missed=false", `  A ["Dee"] token=false 0`, `  B ["Ann"] token=false -`}},
		{"a round once it has closed, not scored yet", "Cy", time.Date(2031, 5, 2, 0, 0, 0, 0, time.UTC), []string{
			"S1 0 missed=false", `  A ["Dee"] token=false 0`, `  B ["Ann"] token=false -`, "S2 - missed=false", `  A ["Ann"] token=false -`}},
		{"no answers in a round they missed", "bea", time.Date(2031, 5, 2, 0, 0, 0, 0, time.UTC), []string{
			"S1 14 missed=false", `  A ["Ann"] token=true 14`, "S2 - missed=true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := participantLines(t, pool, tt.participant, tt.now); !slices.Equal(got, tt.want) {
				t.Errorf("%s at %s = %q, want %q", tt.participant, tt.now, got, tt.want)
			}
		})
	}
}

func TestNostradamusParticipantOfNoOneIsRefused(t *testing.T) {
	pool := nostradamusFixture(t)
	addRounds(t, pool, nround{"S1", []question{{label: "A", kind: "pick", points: 4}}, []entry{answered("Bea", "A=Ann")}})
	r := NewRatingsReader(asReader(t, pool))
	for _, tt := range []struct{ edition, name string }{{"wc-2031", "Zed"}, {"wc-2099", "Bea"}} {
		if _, err := r.NostradamusParticipant(context.Background(), tt.edition, tt.name, time.Now()); !errors.Is(err, ErrNoNostradamus) {
			t.Errorf("NostradamusParticipant(%s, %s) = %v, want ErrNoNostradamus", tt.edition, tt.name, err)
		}
	}
}

// adminEdition reads wc-2031 as admin, failing the test on an error.
func adminEdition(t *testing.T, a *AdminStore) AdminNostradamus {
	t.Helper()
	got, err := a.NostradamusForAdmin(context.Background(), "wc-2031")
	if err != nil {
		t.Fatalf("NostradamusForAdmin: %v", err)
	}
	return got
}

func TestNostradamusAdminSetsAnOutcomeThatScoresTheRound(t *testing.T) {
	pool := nostradamusFixture(t)
	addRounds(t, pool, nround{"S1", []question{{label: "A", kind: "set", points: 6}}, []entry{answered("Bea", "A=Ann,Cy")}})
	a := NewAdminStore(asAdminLogin(t, pool))
	question := adminEdition(t, a).Rounds[0].Questions[0].ID
	if err := a.SetNostradamusOutcome(context.Background(), "wc-2031", question, []string{"Cy", "Ann"}); err != nil {
		t.Fatalf("SetNostradamusOutcome: %v", err)
	}
	if got, want := boardLines(t, pool), []string{`Bea "" 6 S1=6`}; !slices.Equal(got, want) {
		t.Errorf("board = %q, want %q", got, want)
	}
	if err := a.SetNostradamusOutcome(context.Background(), "wc-2031", question, nil); err != nil {
		t.Fatalf("SetNostradamusOutcome(nil): %v", err)
	}
	if got := boardLines(t, pool); len(got) != 0 {
		t.Errorf("board after clearing the outcome = %q, want no scored round", got)
	}
}

func TestNostradamusAdminOutcomesAreRefusedWhenTheyShouldBe(t *testing.T) {
	pool := nostradamusFixture(t)
	addRounds(t, pool, nround{"S1", []question{{label: "N", kind: "number", points: 6}}, nil})
	a := NewAdminStore(asAdminLogin(t, pool))
	question := adminEdition(t, a).Rounds[0].Questions[0].ID
	tests := []struct {
		name     string
		edition  string
		question int64
		outcome  []string
		want     error
	}{
		{"a number that is not one", "wc-2031", question, []string{"three"}, ErrBadNostradamus},
		{"two numbers", "wc-2031", question, []string{"3", "4"}, ErrBadNostradamus},
		{"a question of another edition", "wc-2099", question, []string{"3"}, ErrNoNostradamus},
		{"no such question", "wc-2031", question + 100, []string{"3"}, ErrNoNostradamus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetNostradamusOutcome(context.Background(), tt.edition, tt.question, tt.outcome); !errors.Is(err, tt.want) {
				t.Errorf("SetNostradamusOutcome(%q) = %v, want %v", tt.outcome, err, tt.want)
			}
		})
	}
}

func TestNostradamusAdminLinksAParticipantToAMember(t *testing.T) {
	pool := nostradamusFixture(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO members (tta_name) VALUES ('Beatrix')"); err != nil {
		t.Fatal(err)
	}
	addRounds(t, pool, nround{"S1", []question{{label: "A", kind: "pick", points: 4, outcome: []string{"Ann"}}}, []entry{answered("Bea", "A=Ann")}})
	a := NewAdminStore(asAdminLogin(t, pool))
	participant := adminEdition(t, a).Participants[0].ID
	if err := a.LinkNostradamusParticipant(context.Background(), "wc-2031", participant, "beatrix"); err != nil {
		t.Fatalf("LinkNostradamusParticipant: %v", err)
	}
	if got, want := boardLines(t, pool), []string{`Bea "Beatrix" 4 S1=4`}; !slices.Equal(got, want) {
		t.Errorf("board = %q, want %q", got, want)
	}
	if err := a.LinkNostradamusParticipant(context.Background(), "wc-2031", participant, ""); err != nil {
		t.Fatalf("unlinking: %v", err)
	}
	if got, want := boardLines(t, pool), []string{`Bea "" 4 S1=4`}; !slices.Equal(got, want) {
		t.Errorf("board after unlinking = %q, want %q", got, want)
	}
	for _, tt := range []struct {
		name, edition, member string
		participant           int64
		want                  error
	}{
		{"no such member", "wc-2031", "Zed", participant, ErrNoMember},
		{"no such participant", "wc-2031", "Beatrix", participant + 100, ErrNoNostradamus},
		{"a participant of another edition", "wc-2099", "Beatrix", participant, ErrNoNostradamus},
	} {
		if err := a.LinkNostradamusParticipant(context.Background(), tt.edition, tt.participant, tt.member); !errors.Is(err, tt.want) {
			t.Errorf("%s: LinkNostradamusParticipant = %v, want %v", tt.name, err, tt.want)
		}
	}
}
