//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// reportsAt is when the tests' players report, inside the sessions signedIn starts.
var reportsAt = signInAt.Add(time.Hour)

// reportsFixture is round one of blitzRoundOne (Ann v Bob, Cy v Dee, published), with Ann and Bob signed in as
// "ann" and "bob" and linked to their members, and a player store on the player login.
func reportsFixture(t *testing.T) (*AdminStore, *PlayerStore, *RatingsReader, []int64, []int64) {
	t.Helper()
	a, reader, ids, matches := blitzRoundOne(t)
	p := NewPlayerStore(asPlayerLogin(t, a.pool.(*pgxpool.Pool)))
	for _, s := range []struct{ subject, name string }{{"100000000000000001", "Ann"}, {"100000000000000002", "Bob"}} {
		token := strings.ToLower(s.name)
		approved(t, a, p, signedIn(t, p, discordID(s.subject, token, s.name), token), s.name)
	}
	return a, p, reader, ids, matches
}

// reported is a game a player reports: a share code, a 50–40 finish.
func reported(code string) BlitzGame {
	return BlitzGame{ShareCode: code, A: finished(50, 1), B: finished(40, 2), Finished: finishedAt}
}

// matchNow reads match id of the edition as the site sees it.
func matchNow(t *testing.T, reader *RatingsReader, id int64) BlitzMatch {
	t.Helper()
	ed, err := reader.BlitzEdition(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	return matchOf(t, ed, id)
}

// reportLine writes where a match's report stands as "<report> by=<entrant> flag=<reason>", "-" for none, naming
// the entrants of ids as blitzWithEntrants does.
func reportLine(m BlitzMatch, ids []int64) string {
	by := "-"
	for i, id := range ids {
		if m.ReportedBy != nil && *m.ReportedBy == id {
			by = []string{"Ann", "Bob", "Cy", "Dee"}[i]
		}
	}
	report := m.Report
	if report == "" {
		report = "-"
	}
	return report + " by=" + by + " flag=" + m.FlagReason
}

func TestAPlayersReportCountsAtOnceAndTheyMayCorrectIt(t *testing.T) {
	a, p, reader, ids, matches := reportsFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	m := matchNow(t, reader, matches[0])
	if got, want := resultLine(m), "a game=REP-1 50/finished 40/finished on 2026-10-15T03:30:00Z"; got != want {
		t.Errorf("the reported result = %q, want %q", got, want)
	}
	if m.Report != "reported" || m.ReportedBy == nil || *m.ReportedBy != ids[0] {
		t.Errorf("the report = %q by %v, want reported by Ann's entry %d", m.Report, m.ReportedBy, ids[0])
	}
	first := *m.GameID

	corrected := BlitzGame{ShareCode: "REP-2", A: finished(40, 1), B: finished(55, 2), Finished: finishedAt}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], corrected, reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame(a correction): %v", err)
	}
	m = matchNow(t, reader, matches[0])
	if got, want := resultLine(m), "b game=REP-2 40/finished 55/finished on 2026-10-15T03:30:00Z"; got != want {
		t.Errorf("the corrected result = %q, want %q", got, want)
	}
	if *m.GameID != first {
		t.Errorf("the correction's game = %d, want the same game %d", *m.GameID, first)
	}
	var source string
	if err := a.pool.QueryRow(ctx, "SELECT b.source FROM games g JOIN import_batches b USING (batch_id) WHERE g.game_id = $1", first).Scan(&source); err != nil || source != "player" {
		t.Errorf("the reported game's source = %q, %v; want player", source, err)
	}
}

func TestTheOpponentConfirmsOrFlagsAReport(t *testing.T) {
	tests := []struct {
		name   string
		answer func(p *PlayerStore, match int64) error
		want   string
	}{
		{"confirmed", func(p *PlayerStore, match int64) error {
			return p.ConfirmBlitzResult(context.Background(), tokenHash("bob"), "us_open", "2026", match, reportsAt)
		}, "confirmed by=Ann flag="},
		{"flagged", func(p *PlayerStore, match int64) error {
			return p.FlagBlitzResult(context.Background(), tokenHash("bob"), "us_open", "2026", match, "  I won on culture  ", reportsAt)
		}, "flagged by=Ann flag=I won on culture"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, p, reader, ids, matches := reportsFixture(t)
			if err := p.ReportBlitzGame(context.Background(), tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
				t.Fatalf("ReportBlitzGame: %v", err)
			}
			if err := tt.answer(p, matches[0]); err != nil {
				t.Fatalf("answering: %v", err)
			}
			if got := reportLine(matchNow(t, reader, matches[0]), ids); got != tt.want {
				t.Errorf("the report = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReportsAreRefused(t *testing.T) {
	annReports := func(t *testing.T, a *AdminStore, p *PlayerStore, matches []int64) {
		if err := p.ReportBlitzGame(context.Background(), tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
			t.Fatalf("ReportBlitzGame(Ann): %v", err)
		}
	}
	tests := []struct {
		name    string
		token   string
		edition string
		match   int // index into the round's matches; -1 for one that does not exist
		game    BlitzGame
		before  func(t *testing.T, a *AdminStore, p *PlayerStore, matches []int64)
		want    error
	}{
		{name: "signed out", token: "nobody", want: ErrNoSession},
		{name: "not linked", token: "cy", before: func(t *testing.T, a *AdminStore, p *PlayerStore, _ []int64) {
			signedIn(t, p, discordID("100000000000000009", "cy", "Cy"), "cy")
		}, want: ErrNotLinked},
		{name: "banned", before: func(t *testing.T, a *AdminStore, _ *PlayerStore, _ []int64) {
			if _, err := a.pool.Exec(context.Background(), "UPDATE members SET banned = true, banned_at = now(), ban_reason = 'cheating' WHERE tta_name = 'Ann'"); err != nil {
				t.Fatalf("banning Ann: %v", err)
			}
		}, want: ErrMemberBanned},
		{name: "another edition", edition: "2025", want: ErrNoBlitzEdition},
		{name: "no such match", match: -1, want: ErrNoBlitzMatch},
		{name: "someone else's match", match: 1, want: ErrNotYourMatch},
		{name: "no share code", game: BlitzGame{A: finished(50, 1), B: finished(40, 2), Finished: finishedAt}, want: ErrBlitzShareCodeNeeded},
		{name: "a game out of bounds", game: BlitzGame{ShareCode: "X", A: finished(-1, 1), B: finished(40, 2), Finished: finishedAt}, want: ErrBadBlitzGame},
		{name: "no turn order", game: BlitzGame{ShareCode: "X", A: finished(50, 0), B: finished(40, 0), Finished: finishedAt}, want: ErrBadBlitzGame},
		{name: "finished after now", game: BlitzGame{ShareCode: "X", A: finished(50, 1), B: finished(40, 2), Finished: reportsAt.Add(time.Minute)}, want: ErrBlitzFinishedAhead},
		{name: "the next round is paired", before: func(t *testing.T, a *AdminStore, p *PlayerStore, matches []int64) {
			for _, m := range matches {
				if err := a.RecordBlitzRuling(context.Background(), "us_open", "2026", m, "a", "ruled"); err != nil {
					t.Fatalf("RecordBlitzRuling: %v", err)
				}
			}
			if _, err := a.AddBlitzRound(context.Background(), "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 8}); err != nil {
				t.Fatalf("AddBlitzRound(2): %v", err)
			}
		}, want: ErrBlitzRoundClosed},
		{name: "the opponent's report", token: "bob", before: annReports, want: ErrBlitzResultLocked},
		{name: "an organiser's game", before: func(t *testing.T, a *AdminStore, _ *PlayerStore, matches []int64) {
			if err := a.RecordBlitzGame(context.Background(), "us_open", "2026", matches[0], reported("ORG")); err != nil {
				t.Fatalf("RecordBlitzGame: %v", err)
			}
		}, want: ErrBlitzResultLocked},
		{name: "an organiser's ruling", before: func(t *testing.T, a *AdminStore, _ *PlayerStore, matches []int64) {
			if err := a.RecordBlitzRuling(context.Background(), "us_open", "2026", matches[0], "b", "ruled"); err != nil {
				t.Fatalf("RecordBlitzRuling: %v", err)
			}
		}, want: ErrBlitzResultLocked},
		{name: "confirmed", before: func(t *testing.T, a *AdminStore, p *PlayerStore, matches []int64) {
			annReports(t, a, p, matches)
			if err := p.ConfirmBlitzResult(context.Background(), tokenHash("bob"), "us_open", "2026", matches[0], reportsAt); err != nil {
				t.Fatalf("ConfirmBlitzResult: %v", err)
			}
		}, want: ErrBlitzResultLocked},
		{name: "flagged", before: func(t *testing.T, a *AdminStore, p *PlayerStore, matches []int64) {
			annReports(t, a, p, matches)
			if err := p.FlagBlitzResult(context.Background(), tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt); err != nil {
				t.Fatalf("FlagBlitzResult: %v", err)
			}
		}, want: ErrBlitzResultLocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _, matches := reportsFixture(t)
			if tt.before != nil {
				tt.before(t, a, p, matches)
			}
			token, edition, match, game := "ann", "2026", int64(9999), reported("REP-9")
			if tt.token != "" {
				token = tt.token
			}
			if tt.edition != "" {
				edition = tt.edition
			}
			if tt.match >= 0 {
				match = matches[tt.match]
			}
			if !tt.game.Finished.IsZero() {
				game = tt.game
			}
			before := ""
			if tt.match >= 0 {
				before = resultLine(matchNow(t, reader, match))
			}
			if err := p.ReportBlitzGame(context.Background(), tokenHash(token), "us_open", edition, match, game, reportsAt); !errors.Is(err, tt.want) {
				t.Errorf("ReportBlitzGame = %v, want %v", err, tt.want)
			}
			if tt.match >= 0 {
				if after := resultLine(matchNow(t, reader, match)); after != before {
					t.Errorf("the match after a refused report = %q, want it unchanged as %q", after, before)
				}
			}
		})
	}
	t.Run("a share code on another game", func(t *testing.T) {
		a, p, _, _, matches := reportsFixture(t)
		if err := a.RecordBlitzGame(context.Background(), "us_open", "2026", matches[1], reported("TAKEN")); err != nil {
			t.Fatalf("RecordBlitzGame(Cy v Dee): %v", err)
		}
		var taken *BlitzShareCodeError
		if err := p.ReportBlitzGame(context.Background(), tokenHash("ann"), "us_open", "2026", matches[0], reported("TAKEN"), reportsAt); !errors.As(err, &taken) {
			t.Errorf("ReportBlitzGame = %v, want a *BlitzShareCodeError", err)
		}
	})
}

func TestAPlayerMayReportAGameFinishedThisMinute(t *testing.T) {
	_, p, _, _, matches := reportsFixture(t)
	game := BlitzGame{ShareCode: "NOW", A: finished(50, 1), B: finished(40, 2), Finished: reportsAt.Truncate(time.Minute)}
	if err := p.ReportBlitzGame(context.Background(), tokenHash("ann"), "us_open", "2026", matches[0], game, reportsAt.Add(59*time.Second)); err != nil {
		t.Errorf("ReportBlitzGame finished this minute = %v, want nil", err)
	}
}

func TestAnswersAreRefused(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		reason string // "" confirms; anything else flags
		report bool   // Ann reports first
		want   error
	}{
		{name: "nothing reported", token: "bob", want: ErrBlitzResultLocked},
		{name: "the reporter confirms their own", token: "ann", report: true, want: ErrBlitzResultLocked},
		{name: "the reporter flags their own", token: "ann", reason: "wrong", report: true, want: ErrBlitzResultLocked},
		{name: "a flag over 500 characters", token: "bob", reason: strings.Repeat("x", 501), report: true, want: ErrBadBlitzFlag},
		{name: "a flag of spaces", token: "bob", reason: "   ", report: true, want: ErrBadBlitzFlag},
		{name: "signed out", token: "nobody", report: true, want: ErrNoSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, p, reader, _, matches := reportsFixture(t)
			ctx := context.Background()
			if tt.report {
				if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
					t.Fatalf("ReportBlitzGame: %v", err)
				}
			}
			var err error
			if tt.reason == "" {
				err = p.ConfirmBlitzResult(ctx, tokenHash(tt.token), "us_open", "2026", matches[0], reportsAt)
			} else {
				err = p.FlagBlitzResult(ctx, tokenHash(tt.token), "us_open", "2026", matches[0], tt.reason, reportsAt)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("answering = %v, want %v", err, tt.want)
			}
			if got := matchNow(t, reader, matches[0]).Report; got != map[bool]string{true: "reported", false: ""}[tt.report] {
				t.Errorf("the report after a refused answer = %q, want it unchanged", got)
			}
		})
	}
}

func TestAnOrganiserResolvesAFlagAndItStaysLocked(t *testing.T) {
	tests := []struct {
		name    string
		resolve func(a *AdminStore, match int64) error
		result  string
	}{
		{"corrected", func(a *AdminStore, match int64) error {
			return a.RecordBlitzGame(context.Background(), "us_open", "2026", match, BlitzGame{ShareCode: "REP-1", A: finished(30, 1), B: finished(60, 2), Finished: finishedAt})
		}, "b"},
		{"ruled", func(a *AdminStore, match int64) error {
			return a.RecordBlitzRuling(context.Background(), "us_open", "2026", match, "draw", "both right")
		}, "draw"},
		{"kept as reported", func(a *AdminStore, match int64) error {
			return a.KeepBlitzReport(context.Background(), "us_open", "2026", match)
		}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _, matches := reportsFixture(t)
			ctx := context.Background()
			if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
				t.Fatalf("ReportBlitzGame: %v", err)
			}
			if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt); err != nil {
				t.Fatalf("FlagBlitzResult: %v", err)
			}
			if err := tt.resolve(a, matches[0]); err != nil {
				t.Fatalf("resolving: %v", err)
			}
			m := matchNow(t, reader, matches[0])
			if m.Result != tt.result || m.Report != "resolved" || m.FlagReason != "wrong" {
				t.Errorf("after resolving = %s %q flag=%q, want %s resolved flag=wrong", m.Result, m.Report, m.FlagReason, tt.result)
			}
			if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); !errors.Is(err, ErrBlitzResultLocked) {
				t.Errorf("reporting a resolved result = %v, want ErrBlitzResultLocked", err)
			}
		})
	}
}

func TestAnOrganisersGameOverAReportResolvesIt(t *testing.T) {
	a, p, reader, _, matches := reportsFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], reported("REP-1")); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if m := matchNow(t, reader, matches[0]); m.Report != "resolved" || m.FlagReason != "" {
		t.Errorf("an organiser's game over a report = %q flag=%q, want resolved with no flag", m.Report, m.FlagReason)
	}
}

func TestOnlyAFlaggedReportIsKept(t *testing.T) {
	a, p, _, _, matches := reportsFixture(t)
	ctx := context.Background()
	if err := a.KeepBlitzReport(ctx, "us_open", "2026", matches[0]); !errors.Is(err, ErrBlitzNotFlagged) {
		t.Errorf("keeping an unreported match = %v, want ErrBlitzNotFlagged", err)
	}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if err := a.KeepBlitzReport(ctx, "us_open", "2026", matches[0]); !errors.Is(err, ErrBlitzNotFlagged) {
		t.Errorf("keeping an unflagged report = %v, want ErrBlitzNotFlagged", err)
	}
}

func TestResettingAMatchOpensItToPlayersAgain(t *testing.T) {
	a, p, reader, ids, matches := reportsFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt); err != nil {
		t.Fatalf("FlagBlitzResult: %v", err)
	}
	if err := a.KeepBlitzReport(ctx, "us_open", "2026", matches[0]); err != nil {
		t.Fatalf("KeepBlitzReport: %v", err)
	}
	if err := a.ResetBlitzResult(ctx, "us_open", "2026", matches[0]); err != nil {
		t.Fatalf("ResetBlitzResult: %v", err)
	}
	if got := reportLine(matchNow(t, reader, matches[0]), ids); got != "- by=- flag=" {
		t.Errorf("the report after a reset = %q, want none", got)
	}
	var games int
	if err := a.pool.QueryRow(ctx, "SELECT count(*) FROM games WHERE share_code = 'REP-1'").Scan(&games); err != nil || games != 0 {
		t.Errorf("games with the reset report's code = %d, %v; want 0", games, err)
	}
	if err := p.ReportBlitzGame(ctx, tokenHash("bob"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Errorf("reporting after a reset = %v, want nil", err)
	}
}

func TestADrawnKnockoutReportSendsTheHigherSeedThrough(t *testing.T) {
	a, p, reader, ids, matches := reportsFixture(t)
	ctx := context.Background()
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 2}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	for _, m := range matches {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m, "a", "ruled"); err != nil {
			t.Fatalf("RecordBlitzRuling(swiss): %v", err)
		}
	}
	final, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2,
		Matches: []BlitzMatch{{A: ids[1], B: &ids[0], Slot: new(0), SeedA: new(2), SeedB: new(1)}}})
	if err != nil {
		t.Fatalf("AddBlitzRound(final): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", final.ID); err != nil {
		t.Fatalf("PublishBlitzRound(final): %v", err)
	}
	drawn := BlitzGame{ShareCode: "KO-1", A: finished(90, 1), B: finished(90, 2), Finished: finishedAt}
	if err := p.ReportBlitzGame(ctx, tokenHash("bob"), "us_open", "2026", final.Matches[0].ID, drawn, reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame(final): %v", err)
	}
	if got := matchNow(t, reader, final.Matches[0].ID).Result; got != "b" {
		t.Errorf("a drawn final's result = %q, want b (Ann, seeded 1st)", got)
	}
}

func TestPlayersReportsAreAudited(t *testing.T) {
	a, p, _, _, matches := reportsFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt); err != nil {
		t.Fatalf("FlagBlitzResult: %v", err)
	}
	rows, err := a.pool.Query(ctx, "SELECT action || ' ' || target || ' ' || (detail ? 'account')::text FROM admin_audit WHERE action LIKE 'blitz.match.%' ORDER BY audit_id")
	if err != nil {
		t.Fatalf("reading the audit: %v", err)
	}
	var got []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("reading the audit: %v", err)
		}
		got = append(got, strings.Replace(line, " "+strconv.FormatInt(matches[0], 10)+" ", " M ", 1))
	}
	if want := "blitz.match.report M true,blitz.match.flag M true"; strings.Join(got, ",") != want {
		t.Errorf("the audit = %q, want %q", strings.Join(got, ","), want)
	}
}

func TestThePlayerLoginWritesGamesOnlyThroughItsFunctions(t *testing.T) {
	a, _, _, _, _ := reportsFixture(t)
	pool := a.pool.(*pgxpool.Pool)
	ctx := context.Background()
	tests := []struct {
		role, function string
		want           bool
	}{
		{"ratings_player", "report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb)", true},
		{"ratings_player", "answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text)", true},
		{"ratings_player", "write_game(bigint, text, text, timestamptz, text, text, text, jsonb, bigint)", false},
		{"ratings_player", "blitz_report_target(bytea, timestamptz, text, text, bigint)", false},
		{"ratings_admin", "write_game(bigint, text, text, timestamptz, text, text, text, jsonb, bigint)", false},
		{"ratings_read", "report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb)", false},
		{"ratings_read", "answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text)", false},
	}
	for _, tt := range tests {
		var got bool
		if err := pool.QueryRow(ctx, "SELECT has_function_privilege($1, $2, 'EXECUTE')", tt.role, tt.function).Scan(&got); err != nil {
			t.Fatalf("has_function_privilege(%s, %s): %v", tt.role, tt.function, err)
		}
		if got != tt.want {
			t.Errorf("%s may execute %s = %v, want %v", tt.role, tt.function, got, tt.want)
		}
	}
	player := asPlayerLogin(t, pool)
	for _, q := range []string{"UPDATE blitz_matches SET report = 'confirmed'", "INSERT INTO import_batches (source) VALUES ('player')"} {
		if _, err := player.Exec(ctx, q); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("ratings_player ran %q: %v, want permission denied", q, err)
		}
	}
}

func TestTheDatabaseRefusesABrokenTurnOrder(t *testing.T) {
	a, _, reader, _, matches := reportsFixture(t)
	player := asPlayerLogin(t, a.pool.(*pgxpool.Pool))
	ctx := context.Background()
	const won, lost = `{"outcome": "finished", "culture": 50, "place": 1`, `{"outcome": "finished", "culture": 40, "place": 2`
	tests := []struct{ name, sides string }{
		{"no positions", "[" + won + "}, " + lost + "}]"},
		{"one position missing", "[" + won + `, "position": 0}, ` + lost + "}]"},
		{"both first", "[" + won + `, "position": 0}, ` + lost + `, "position": 0}]`},
		{"a position past the game's players", "[" + won + `, "position": 0}, ` + lost + `, "position": 2}]`},
		{"a position before the first", "[" + won + `, "position": -1}, ` + lost + `, "position": 0}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			err := player.QueryRow(ctx, "SELECT report_blitz_game($1, $2, 'us_open', '2026', $3, 'BAD-1', $4, 'a', $5)",
				tokenHash("ann"), reportsAt, matches[0], finishedAt, tt.sides).Scan(&got)
			if err == nil || !strings.Contains(err.Error(), "turn order") {
				t.Errorf("report_blitz_game(%s) = %q, %v; want refused for its turn order", tt.sides, got, err)
			}
			if m := matchNow(t, reader, matches[0]); m.Game != nil {
				t.Errorf("the match after a refused report = %q, want no game", resultLine(m))
			}
		})
	}
}

func TestMigration41sDownTakesAGameWithoutTurnOrderAgain(t *testing.T) {
	a, _, _, _, _ := reportsFixture(t)
	ctx := context.Background()
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 40); return err }); err != nil {
		t.Fatalf("migrating down to 40: %v", err)
	}
	var written *int64
	if err := pool.QueryRow(ctx, `
		SELECT w.written FROM tournaments t JOIN series s USING (series_id),
		     write_game(t.tournament_id, 'Round 1', 'US Open 2026 Round 1', $1, 'DOWN-1', 'admin', 'blitz match 1',
		                '[{"name": "Ann", "outcome": "finished", "culture": 50, "place": 1}, {"name": "Bob", "outcome": "finished", "culture": 40, "place": 2}]', NULL) w
		WHERE s.slug = 'us_open' AND t.label = '2026'`, finishedAt).Scan(&written); err != nil || written == nil {
		t.Errorf("write_game without positions after the Down = %v, %v; want a game written", written, err)
	}
}

func TestMigration52sDownTakesAReportFinishedAheadAgain(t *testing.T) {
	a, p, _, _, matches := reportsFixture(t)
	ctx := context.Background()
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 51); return err }); err != nil {
		t.Fatalf("migrating down to 51: %v", err)
	}
	game := BlitzGame{ShareCode: "AHEAD", A: finished(50, 1), B: finished(40, 2), Finished: reportsAt.Add(time.Hour)}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], game, reportsAt); err != nil {
		t.Errorf("ReportBlitzGame finished ahead after the Down = %v, want nil", err)
	}
}

func TestMigration35sDownKeepsAReportedGameAsAdmins(t *testing.T) {
	a, p, _, _, matches := reportsFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 34); return err }); err != nil {
		t.Fatalf("migrating down to 34: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT b.source FROM games g JOIN import_batches b USING (batch_id) WHERE g.share_code = 'REP-1'"); !slices.Equal(got, []string{"admin"}) {
		t.Errorf("the reported game's source after the Down = %q, want admin", got)
	}
	// The old schema's reset, as the code before this migration made it.
	if _, err := pool.Exec(ctx, `
		WITH cleared AS (
			UPDATE blitz_matches m SET result = NULL, game_id = NULL, ruling = '', decided_at = NULL
			FROM blitz_matches old WHERE old.match_id = m.match_id AND m.match_id = $1 RETURNING old.game_id)
		SELECT delete_blitz_game(game_id) FROM cleared`, matches[0]); err != nil {
		t.Errorf("deleting the reported game after the Down: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT share_code FROM games WHERE share_code = 'REP-1'"); len(got) != 0 {
		t.Errorf("games with the reported code after the reset = %q, want none", got)
	}
}

func TestAnEditionSaysWhenTheNextRoundIsPairedBeforeItIsPublished(t *testing.T) {
	a, _, reader, _, matches := reportsFixture(t)
	ctx := context.Background()
	for _, m := range matches {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m, "a", "ruled"); err != nil {
			t.Fatalf("RecordBlitzRuling: %v", err)
		}
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 8}); err != nil {
		t.Fatalf("AddBlitzRound(2): %v", err)
	}
	ed, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if ed.Round == nil || *ed.Round != 1 || ed.PairedRound == nil || *ed.PairedRound != 2 {
		t.Errorf("round %v, paired %v; want 1 published and 2 paired", ed.Round, ed.PairedRound)
	}
}

func TestAdminsEditionListCountsTheFlaggedReports(t *testing.T) {
	a, p, _, _, matches := reportsFixture(t)
	ctx := context.Background()
	count := func() int {
		rows, err := a.BlitzEditions(ctx)
		if err != nil {
			t.Fatalf("BlitzEditions: %v", err)
		}
		for _, r := range rows {
			if r.Slug == "us_open" && r.Edition == "2026" {
				return r.Flagged
			}
		}
		t.Fatal("no us_open 2026 in the list")
		return 0
	}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if got := count(); got != 0 {
		t.Errorf("flagged before a flag = %d, want 0", got)
	}
	if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt); err != nil {
		t.Fatalf("FlagBlitzResult: %v", err)
	}
	if got := count(); got != 1 {
		t.Errorf("flagged = %d, want 1", got)
	}
	if err := a.KeepBlitzReport(ctx, "us_open", "2026", matches[0]); err != nil {
		t.Fatalf("KeepBlitzReport: %v", err)
	}
	if got := count(); got != 0 {
		t.Errorf("flagged once resolved = %d, want 0", got)
	}
}

// linkedToCGE links the member named name to a new CGE account cgeUser, with the player of that name, as the
// extract load and an admin's link would; the admin login cannot add accounts.
func linkedToCGE(t *testing.T, a *AdminStore, name string, cgeUser int) {
	t.Helper()
	ctx := context.Background()
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil
	owner, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	defer owner.Close()
	if _, err := owner.Exec(ctx, "INSERT INTO players (name) SELECT $1 WHERE NOT EXISTS (SELECT FROM players WHERE name = $1)", name); err != nil {
		t.Fatalf("adding player %s: %v", name, err)
	}
	if _, err := owner.Exec(ctx, `
		WITH acc AS (INSERT INTO player_accounts (cge_user, player_id, name) SELECT $2, player_id, $1 FROM players WHERE name = $1 RETURNING cge_user, player_id)
		UPDATE members m SET cge_user = acc.cge_user, player_id = acc.player_id FROM acc WHERE m.tta_name = $1`, name, cgeUser); err != nil {
		t.Fatalf("linking %s to CGE account %d: %v", name, cgeUser, err)
	}
}

// blitzRecordings are the two ways a blitz game reaches the ratings: an organiser records it, or Ann reports it.
var blitzRecordings = []struct {
	name   string
	record func(a *AdminStore, p *PlayerStore, match int64, g BlitzGame) error
}{
	{"recorded by an organiser", func(a *AdminStore, _ *PlayerStore, match int64, g BlitzGame) error {
		return a.RecordBlitzGame(context.Background(), "us_open", "2026", match, g)
	}},
	{"reported by a player", func(_ *AdminStore, p *PlayerStore, match int64, g BlitzGame) error {
		return p.ReportBlitzGame(context.Background(), tokenHash("ann"), "us_open", "2026", match, g, reportsAt)
	}},
}

// linkedToPlayer links the member named member to a new player named player, as a member who renamed on CGE is.
func linkedToPlayer(t *testing.T, a *AdminStore, member, player string) {
	t.Helper()
	ctx := context.Background()
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil
	owner, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	defer owner.Close()
	if _, err := owner.Exec(ctx, `
		WITH p AS (INSERT INTO players (name) VALUES ($2) RETURNING player_id)
		UPDATE members m SET player_id = p.player_id FROM p WHERE m.tta_name = $1`, member, player); err != nil {
		t.Fatalf("linking %s to player %s: %v", member, player, err)
	}
}

func TestABlitzGameSeatsEachSidesMembersPlayer(t *testing.T) {
	for _, tt := range blitzRecordings {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _, matches := reportsFixture(t)
			linkedToPlayer(t, a, "Ann", "Annabel")
			if err := tt.record(a, p, matches[0], reported("CGE-1")); err != nil {
				t.Fatalf("recording the game: %v", err)
			}
			var got []string
			if err := a.pool.QueryRow(context.Background(), `SELECT array_agg(p.name ORDER BY s.seat) FROM seats s JOIN players p USING (player_id)
				WHERE s.game_id = $1`, matchNow(t, reader, matches[0]).GameID).Scan(&got); err != nil {
				t.Fatalf("reading the seats: %v", err)
			}
			if want := []string{"Annabel", "Bob"}; !slices.Equal(got, want) {
				t.Errorf("the seats' players = %q, want %q: Ann's member plays as Annabel", got, want)
			}
		})
	}
}

func TestABlitzGamesSeatsCarryTheirPlayersCGEAccount(t *testing.T) {
	for _, tt := range blitzRecordings {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _, matches := reportsFixture(t)
			seats := func() string {
				t.Helper()
				var got []string
				m := matchNow(t, reader, matches[0])
				if err := a.pool.QueryRow(context.Background(), `SELECT array_agg(p.name || '=' || coalesce(s.cge_user::text, '-') ORDER BY s.seat)
					FROM seats s JOIN players p USING (player_id) WHERE s.game_id = $1`, m.GameID).Scan(&got); err != nil {
					t.Fatalf("reading the seats: %v", err)
				}
				return strings.Join(got, " ")
			}
			linkedToCGE(t, a, "Ann", 1001)
			if err := tt.record(a, p, matches[0], reported("CGE-1")); err != nil {
				t.Fatalf("recording the game: %v", err)
			}
			if got, want := seats(), "Ann=1001 Bob=-"; got != want {
				t.Errorf("the new game's seats = %q, want %q", got, want)
			}
			linkedToCGE(t, a, "Bob", 1002)
			if err := tt.record(a, p, matches[0], reported("CGE-2")); err != nil {
				t.Fatalf("correcting the game: %v", err)
			}
			if got, want := seats(), "Ann=1001 Bob=1002"; got != want {
				t.Errorf("the corrected game's seats = %q, want %q", got, want)
			}
		})
	}
}

func TestMigration63GivesTheBlitzSeatsAlreadyRecordedTheirCGEAccounts(t *testing.T) {
	a, p, reader, _, matches := reportsFixture(t)
	ctx := context.Background()
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 62); return err }); err != nil {
		t.Fatalf("migrating down to 62: %v", err)
	}
	linkedToCGE(t, a, "Ann", 1001)
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("OLD-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame after the Down: %v", err)
	}
	seats := func() string {
		t.Helper()
		var got []string
		if err := pool.QueryRow(ctx, `SELECT array_agg(p.name || '=' || coalesce(s.cge_user::text, '-') ORDER BY s.seat)
			FROM seats s JOIN players p USING (player_id) WHERE s.game_id = $1`,
			matchNow(t, reader, matches[0]).GameID).Scan(&got); err != nil {
			t.Fatalf("reading the seats: %v", err)
		}
		return strings.Join(got, " ")
	}
	if got, want := seats(), "Ann=- Bob=-"; got != want {
		t.Fatalf("the seats after the Down = %q, want %q", got, want)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	if got, want := seats(), "Ann=1001 Bob=-"; got != want {
		t.Errorf("the seats after migration 63 = %q, want %q", got, want)
	}
}
