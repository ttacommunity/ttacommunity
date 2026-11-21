//go:build integration

package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// bestOfThree is reportsFixture's edition taken to a published knockout of four after one Swiss round: Ann (seed 1)
// v Bob (seed 4) and Cy (seed 2) v Dee (seed 3), the round offering best of 3, or one game when bestOf is 1. Ann and
// Bob are signed in. It returns the stores, the entrants' ids, the knockout round's id and its two matches' ids.
func bestOfThree(t *testing.T, bestOf int) (*AdminStore, *PlayerStore, *RatingsReader, []int64, int64, []int64) {
	t.Helper()
	a, p, reader, ids, swiss := reportsFixture(t)
	ctx := context.Background()
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 4}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	for _, m := range swiss {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m, "a", "ruled"); err != nil {
			t.Fatalf("RecordBlitzRuling: %v", err)
		}
	}
	ko := func(x, y int64, slot, seedX, seedY int) BlitzMatch {
		return BlitzMatch{A: x, B: &y, Slot: new(slot), SeedA: new(seedX), SeedB: new(seedY)}
	}
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2, Matches: []BlitzMatch{ko(ids[0], ids[1], 0, 1, 4), ko(ids[2], ids[3], 1, 2, 3)}})
	if err != nil {
		t.Fatalf("AddBlitzRound(knockout): %v", err)
	}
	if err := a.SetBlitzRoundBestOf(ctx, "us_open", "2026", r.ID, bestOf); err != nil {
		t.Fatalf("SetBlitzRoundBestOf(%d): %v", bestOf, err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound(knockout): %v", err)
	}
	return a, p, reader, ids, r.ID, []int64{r.Matches[0].ID, r.Matches[1].ID}
}

// won is a game side a won (a), side b won (b), or a draw.
func won(code, by string) BlitzGame {
	g := BlitzGame{ShareCode: code, A: finished(45, 1), B: finished(45, 2), Finished: finishedAt}
	switch by {
	case "a":
		g.A.Culture = 50
	case "b":
		g.B.Culture = 50
	}
	return g
}

// agreed makes match a best of 3 as its players agree it: Ann proposes, Bob accepts.
func agreed(t *testing.T, p *PlayerStore, match int64) {
	t.Helper()
	for _, s := range []struct{ token, answer string }{{"ann", "propose"}, {"bob", "accept"}} {
		if err := p.AnswerBlitzBestOf(context.Background(), tokenHash(s.token), "us_open", "2026", match, s.answer, reportsAt); err != nil {
			t.Fatalf("AnswerBlitzBestOf(%s, %s): %v", s.token, s.answer, err)
		}
	}
}

// digit writes a number from 0 to 9.
func digit(n int) string { return string(rune('0' + n)) }

// gamesLine writes a match's games as "<number>:<result>/<report>" in order, "-" for no report.
func gamesLine(m BlitzMatch) string {
	var out []string
	for _, g := range m.Games {
		report := g.Report
		if report == "" {
			report = "-"
		}
		out = append(out, digit(g.Number)+":"+g.Result+"/"+report)
	}
	return strings.Join(out, " ")
}

func TestABestOfThreeIsWonAtTwoPoints(t *testing.T) {
	tests := []struct {
		name  string
		games []string
		want  string
	}{
		{"one game is not enough", []string{"a"}, ""},
		{"a drawn first game is not enough", []string{"draw"}, ""},
		{"two straight wins", []string{"a", "a"}, "a"},
		{"one each goes to a third", []string{"a", "b"}, ""},
		{"the third decides", []string{"a", "b", "b"}, "b"},
		{"a win and a draw is one and a half", []string{"a", "draw"}, ""},
		{"a win and two draws is two", []string{"a", "draw", "draw"}, "a"},
		{"draw, win, draw makes two for the lower seed", []string{"draw", "b", "draw"}, "b"},
		{"level after three goes to the higher seed", []string{"draw", "draw", "draw"}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _, _, matches := bestOfThree(t, 3)
			agreed(t, p, matches[0])
			for i, by := range tt.games {
				if err := a.RecordBlitzGame(context.Background(), "us_open", "2026", matches[0], i+1, won("BO3-"+digit(i+1), by)); err != nil {
					t.Fatalf("RecordBlitzGame(game %d, %s): %v", i+1, by, err)
				}
			}
			if got := matchNow(t, reader, matches[0]).Result; got != tt.want {
				t.Errorf("the match after %v = %q, want %q", tt.games, got, tt.want)
			}
		})
	}
}

func TestAMatchTakesOnlyTheGamesItNeeds(t *testing.T) {
	tests := []struct {
		name   string
		bestOf int
		agree  bool
		before []string
		number int
		want   error
	}{
		{"a second game of a one-game round", 1, false, []string{"a"}, 2, ErrNoBlitzGame},
		{"a second game before the players agree", 3, false, []string{"a"}, 2, ErrNoBlitzGame},
		{"game 2 before game 1", 3, true, nil, 2, ErrNoBlitzGame},
		{"a fourth game", 3, true, []string{"a", "b", "a"}, 4, ErrNoBlitzGame},
		{"a third game after two straight wins", 3, true, []string{"a", "a"}, 3, ErrBlitzGameNotNeeded},
		{"a third game at one each", 3, true, []string{"a", "b"}, 3, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, _, _, _, matches := bestOfThree(t, tt.bestOf)
			if tt.agree {
				agreed(t, p, matches[0])
			}
			ctx := context.Background()
			for i, by := range tt.before {
				if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], i+1, won("NEED-"+digit(i+1), by)); err != nil {
					t.Fatalf("RecordBlitzGame(game %d): %v", i+1, err)
				}
			}
			if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], tt.number, won("NEED-X", "a")); !errors.Is(err, tt.want) {
				t.Errorf("RecordBlitzGame(game %d) = %v, want %v", tt.number, err, tt.want)
			}
		})
	}
}

func TestPlayersAgreeToPlayBestOfThree(t *testing.T) {
	tests := []struct {
		name  string
		steps [][2]string // token, answer
		want  string      // "<games> proposed=<entrant>"
	}{
		{"a proposal waits for the opponent", [][2]string{{"ann", "propose"}}, "1 proposed=Ann"},
		{"the opponent accepts", [][2]string{{"ann", "propose"}, {"bob", "accept"}}, "3 proposed=-"},
		{"the opponent declines", [][2]string{{"ann", "propose"}, {"bob", "decline"}}, "1 proposed=-"},
		{"the proposer withdraws", [][2]string{{"ann", "propose"}, {"ann", "withdraw"}}, "1 proposed=-"},
		{"both propose", [][2]string{{"ann", "propose"}, {"bob", "propose"}}, "3 proposed=-"},
		{"a declined proposal is made again", [][2]string{{"ann", "propose"}, {"bob", "decline"}, {"bob", "propose"}}, "1 proposed=Bob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, p, reader, ids, _, matches := bestOfThree(t, 3)
			for _, s := range tt.steps {
				if err := p.AnswerBlitzBestOf(context.Background(), tokenHash(s[0]), "us_open", "2026", matches[0], s[1], reportsAt); err != nil {
					t.Fatalf("AnswerBlitzBestOf(%s, %s): %v", s[0], s[1], err)
				}
			}
			m := matchNow(t, reader, matches[0])
			by := "-"
			if m.ProposedBy != nil {
				by = map[int64]string{ids[0]: "Ann", ids[1]: "Bob"}[*m.ProposedBy]
			}
			if got := digit(m.BestOf) + " proposed=" + by; got != tt.want {
				t.Errorf("after %v: %q, want %q", tt.steps, got, tt.want)
			}
		})
	}
}

func TestBestOfThreeAnswersAreRefused(t *testing.T) {
	tests := []struct {
		name   string
		bestOf int
		before func(t *testing.T, a *AdminStore, p *PlayerStore, match int64)
		token  string
		answer string
		want   error
	}{
		{"a proposal in a one-game round", 1, nil, "ann", "propose", ErrBlitzBestOfNotOffered},
		{"an answer that is none", 3, nil, "ann", "maybe", ErrBadBlitzBestOf},
		{"accepting with no proposal", 3, nil, "bob", "accept", ErrBadBlitzBestOf},
		{"accepting your own proposal", 3, func(t *testing.T, _ *AdminStore, p *PlayerStore, m int64) { propose(t, p, "ann", m) }, "ann", "accept", ErrBadBlitzBestOf},
		{"withdrawing the opponent's proposal", 3, func(t *testing.T, _ *AdminStore, p *PlayerStore, m int64) { propose(t, p, "ann", m) }, "bob", "withdraw", ErrBadBlitzBestOf},
		{"proposing again once agreed", 3, func(t *testing.T, _ *AdminStore, p *PlayerStore, m int64) { agreed(t, p, m) }, "ann", "propose", ErrBlitzBestOfClosed},
		{"proposing once game 1 is reported", 3, func(t *testing.T, _ *AdminStore, p *PlayerStore, m int64) {
			if err := p.ReportBlitzGame(context.Background(), tokenHash("bob"), "us_open", "2026", m, 1, reported("CLOSED-1"), reportsAt); err != nil {
				t.Fatalf("ReportBlitzGame: %v", err)
			}
		}, "ann", "propose", ErrBlitzBestOfClosed},
		{"proposing once an organiser ruled", 3, func(t *testing.T, a *AdminStore, _ *PlayerStore, m int64) {
			if err := a.RecordBlitzRuling(context.Background(), "us_open", "2026", m, "a", "ruled"); err != nil {
				t.Fatalf("RecordBlitzRuling: %v", err)
			}
		}, "ann", "propose", ErrBlitzBestOfClosed},
		{"someone else's match", 3, nil, "ann", "propose", ErrNotYourMatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, _, _, _, matches := bestOfThree(t, tt.bestOf)
			match := matches[0]
			if tt.want == ErrNotYourMatch {
				match = matches[1]
			}
			if tt.before != nil {
				tt.before(t, a, p, match)
			}
			if err := p.AnswerBlitzBestOf(context.Background(), tokenHash(tt.token), "us_open", "2026", match, tt.answer, reportsAt); !errors.Is(err, tt.want) {
				t.Errorf("AnswerBlitzBestOf(%s, %s) = %v, want %v", tt.token, tt.answer, err, tt.want)
			}
		})
	}
}

// propose has the player of token propose best of 3 on match.
func propose(t *testing.T, p *PlayerStore, token string, match int64) {
	t.Helper()
	if err := p.AnswerBlitzBestOf(context.Background(), tokenHash(token), "us_open", "2026", match, "propose", reportsAt); err != nil {
		t.Fatalf("AnswerBlitzBestOf(%s, propose): %v", token, err)
	}
}

func TestAProposalLapsesWhenGameOneIsReported(t *testing.T) {
	_, p, reader, _, _, matches := bestOfThree(t, 3)
	propose(t, p, "ann", matches[0])
	if err := p.ReportBlitzGame(context.Background(), tokenHash("bob"), "us_open", "2026", matches[0], 1, reported("LAPSE-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if m := matchNow(t, reader, matches[0]); m.ProposedBy != nil || m.BestOf != 1 || m.Result != "a" {
		t.Errorf("after game 1: proposed by %v, best of %d, result %q; want no proposal, one game, a", m.ProposedBy, m.BestOf, m.Result)
	}
}

func TestPlayersReportEachGameOfABestOfThree(t *testing.T) {
	_, p, reader, _, _, matches := bestOfThree(t, 3)
	agreed(t, p, matches[0])
	ctx := context.Background()
	steps := []struct {
		token  string
		number int
		game   BlitzGame
	}{{"ann", 1, won("EACH-1", "a")}, {"bob", 2, won("EACH-2", "b")}, {"ann", 3, won("EACH-3", "a")}}
	for _, s := range steps {
		if err := p.ReportBlitzGame(ctx, tokenHash(s.token), "us_open", "2026", matches[0], s.number, s.game, reportsAt); err != nil {
			t.Fatalf("ReportBlitzGame(%s, game %d): %v", s.token, s.number, err)
		}
	}
	if err := p.ConfirmBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], 3, reportsAt); err != nil {
		t.Fatalf("ConfirmBlitzResult(game 3): %v", err)
	}
	if err := p.FlagBlitzResult(ctx, tokenHash("ann"), "us_open", "2026", matches[0], 2, "I won game 2", reportsAt); err != nil {
		t.Fatalf("FlagBlitzResult(game 2): %v", err)
	}
	m := matchNow(t, reader, matches[0])
	if got, want := m.Result+" "+gamesLine(m), "a 1:a/reported 2:b/flagged 3:a/confirmed"; got != want {
		t.Errorf("the match = %q, want %q", got, want)
	}
	if err := p.ConfirmBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], 2, reportsAt); !errors.Is(err, ErrBlitzResultLocked) {
		t.Errorf("Bob confirming his own game 2 = %v, want ErrBlitzResultLocked", err)
	}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], 4, won("EACH-4", "a"), reportsAt); !errors.Is(err, ErrNoBlitzGame) {
		t.Errorf("reporting a game 4 = %v, want ErrNoBlitzGame", err)
	}
}

func TestARulingDecidesTheWholeMatch(t *testing.T) {
	a, p, reader, _, _, matches := bestOfThree(t, 3)
	agreed(t, p, matches[0])
	ctx := context.Background()
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], 1, won("RULE-1", "a")); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", matches[0], "b", "Ann left"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	if m := matchNow(t, reader, matches[0]); m.Result != "b" || m.Ruling != "Ann left" {
		t.Errorf("the match = %q ruled %q, want b ruled", m.Result, m.Ruling)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], 2, won("RULE-2", "a")); err != nil {
		t.Fatalf("RecordBlitzGame(game 2 after the ruling): %v", err)
	}
	if m := matchNow(t, reader, matches[0]); m.Result != "a" || m.Ruling != "" {
		t.Errorf("after an organiser's game 2 = %q ruled %q, want the games' a and no ruling", m.Result, m.Ruling)
	}
}

func TestAnOrganiserSetsARoundsAndAMatchsGames(t *testing.T) {
	a, p, reader, _, round, matches := bestOfThree(t, 3)
	ctx := context.Background()
	if err := a.SetBlitzMatchBestOf(ctx, "us_open", "2026", matches[1], 3); err != nil {
		t.Fatalf("SetBlitzMatchBestOf(3): %v", err)
	}
	if m := matchNow(t, reader, matches[1]); m.BestOf != 3 {
		t.Errorf("Cy v Dee set to best of 3 = %d games", m.BestOf)
	}
	if err := a.SetBlitzRoundBestOf(ctx, "us_open", "2026", round, 1); !errors.Is(err, ErrBlitzBestOfAgreed) {
		t.Errorf("a round back to one game with a match agreed = %v, want ErrBlitzBestOfAgreed", err)
	}
	agreed(t, p, matches[0])
	for i, by := range []string{"a", "b"} {
		if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], i+1, won("SET-"+digit(i+1), by)); err != nil {
			t.Fatalf("RecordBlitzGame(game %d): %v", i+1, err)
		}
	}
	if err := a.SetBlitzMatchBestOf(ctx, "us_open", "2026", matches[0], 1); !errors.Is(err, ErrBlitzBestOfPlayed) {
		t.Errorf("a match with two games back to one = %v, want ErrBlitzBestOfPlayed", err)
	}
}

func TestBestOfIsRefused(t *testing.T) {
	a, _, _, _, swissMatches := reportsFixture(t)
	ctx := context.Background()
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	swiss := ed.Rounds[0].ID
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"a Swiss round of 3", func() error { return a.SetBlitzRoundBestOf(ctx, "us_open", "2026", swiss, 3) }, ErrBlitzBestOfNotOffered},
		{"a round of 2", func() error { return a.SetBlitzRoundBestOf(ctx, "us_open", "2026", swiss, 2) }, ErrBadBlitzBestOf},
		{"a round that is none", func() error { return a.SetBlitzRoundBestOf(ctx, "us_open", "2026", 999999, 3) }, ErrNoBlitzRound},
		{"a Swiss match of 3", func() error { return a.SetBlitzMatchBestOf(ctx, "us_open", "2026", swissMatches[0], 3) }, ErrBlitzBestOfNotOffered},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestResettingABestOfThreeDeletesItsGamesAndKeepsTheAgreement(t *testing.T) {
	a, p, reader, _, _, matches := bestOfThree(t, 3)
	agreed(t, p, matches[0])
	ctx := context.Background()
	for i, by := range []string{"a", "a"} {
		if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], i+1, won("RESET-"+digit(i+1), by)); err != nil {
			t.Fatalf("RecordBlitzGame(game %d): %v", i+1, err)
		}
	}
	if err := a.ResetBlitzResult(ctx, "us_open", "2026", matches[0]); err != nil {
		t.Fatalf("ResetBlitzResult: %v", err)
	}
	if m := matchNow(t, reader, matches[0]); m.Result != "" || len(m.Games) != 0 || m.BestOf != 3 {
		t.Errorf("after the reset: result %q, %d games, best of %d; want none, none, 3", m.Result, len(m.Games), m.BestOf)
	}
	if got := rowsOf(t, a.pool.(*pgxpool.Pool), "SELECT share_code FROM games WHERE share_code LIKE 'RESET-%'"); len(got) != 0 {
		t.Errorf("games left after the reset = %q, want none", got)
	}
}

func TestTheNextKnockoutRoundWaitsForABestOfThreeToBeDecided(t *testing.T) {
	a, p, _, ids, _, matches := bestOfThree(t, 3)
	agreed(t, p, matches[0])
	ctx := context.Background()
	for i, by := range []string{"a", "b"} {
		if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], i+1, won("WAIT-"+digit(i+1), by)); err != nil {
			t.Fatalf("RecordBlitzGame(game %d): %v", i+1, err)
		}
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", matches[1], "a", "ruled"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	final := BlitzRound{Kind: "knockout", Seed: 3, Matches: []BlitzMatch{{A: ids[0], B: &ids[2], Slot: new(0), SeedA: new(1), SeedB: new(2)}}}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", final); !errors.Is(err, ErrBlitzRoundUndecided) {
		t.Errorf("pairing the final at one game each = %v, want ErrBlitzRoundUndecided", err)
	}
}

func TestMigration102sDownRefusesAMatchWithASecondGame(t *testing.T) {
	a, p, _, _, _, matches := bestOfThree(t, 3)
	agreed(t, p, matches[0])
	ctx := context.Background()
	for i, by := range []string{"a", "b"} {
		if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], i+1, won("DOWN-"+digit(i+1), by)); err != nil {
			t.Fatalf("RecordBlitzGame(game %d): %v", i+1, err)
		}
	}
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	err = withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 101); return err })
	if err == nil || !strings.Contains(err.Error(), "more than one game") {
		t.Errorf("migrating down to 101 with a second game = %v, want a refusal naming more than one game", err)
	}
	if got := rowsOf(t, pool, "SELECT count(*)::text FROM blitz_match_games"); len(got) != 1 || got[0] != "2" {
		t.Errorf("the match games after the refused Down = %q, want both kept", got)
	}
}

func TestAReplacementTakesAMatchWithoutItsGamesOrProposal(t *testing.T) {
	tests := []struct {
		name   string
		before func(t *testing.T, a *AdminStore, p *PlayerStore, match int64)
		want   error
	}{
		{"a proposal by the player who dropped lapses", func(t *testing.T, _ *AdminStore, p *PlayerStore, m int64) { propose(t, p, "bob", m) }, nil},
		{"a best of 3 with a game played stays", func(t *testing.T, a *AdminStore, p *PlayerStore, m int64) {
			agreed(t, p, m)
			if err := a.RecordBlitzGame(context.Background(), "us_open", "2026", m, 1, won("REPLACE-1", "a")); err != nil {
				t.Fatalf("RecordBlitzGame: %v", err)
			}
		}, ErrBlitzMatchDecided},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, ids, round, matches := bestOfThree(t, 3)
			ctx := context.Background()
			tt.before(t, a, p, matches[0])
			eve, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Eve", Zone: "UTC"}})
			if err != nil {
				t.Fatalf("AddBlitzEntrants: %v", err)
			}
			if err := a.SetBlitzEntrant(ctx, "us_open", "2026", ids[1], BlitzEntrantChange{Status: new("dropped")}); err != nil {
				t.Fatalf("SetBlitzEntrant(Bob dropped): %v", err)
			}
			if err := a.ReplaceBlitzPlayer(ctx, "us_open", "2026", round, matches[0], ids[1], eve[0]); !errors.Is(err, tt.want) {
				t.Fatalf("ReplaceBlitzPlayer = %v, want %v", err, tt.want)
			}
			if m := matchNow(t, reader, matches[0]); tt.want == nil && (m.ProposedBy != nil || *m.B != eve[0]) {
				t.Errorf("after the replacement: b %d, proposed by %v; want Eve and no proposal", *m.B, m.ProposedBy)
			}
		})
	}
}

func TestANewRoundIsOneGameAMatchAsSaved(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	r, err := a.AddBlitzRound(context.Background(), "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if m := r.Matches[0]; r.BestOf != 1 || m.BestOf != 1 || m.Games == nil || len(m.Games) != 0 {
		t.Errorf("the new round: best of %d, its match best of %d with games %v; want 1, 1 and none", r.BestOf, m.BestOf, m.Games)
	}
}
