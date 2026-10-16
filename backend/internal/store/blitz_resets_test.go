//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"ttacommunity/backend/internal/blitz"
)

// playedRows counts what recording games leaves: games, their import batches, the "Round N" stages and their group
// members.
func playedRows(t *testing.T, a *AdminStore) (games, batches, stages, members int) {
	t.Helper()
	if err := a.pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM games), (SELECT count(*) FROM import_batches WHERE source = 'admin'),
		       (SELECT count(*) FROM stages WHERE label LIKE 'Round %'), (SELECT count(*) FROM group_members)`).Scan(&games, &batches, &stages, &members); err != nil {
		t.Fatalf("counting what was played: %v", err)
	}
	return games, batches, stages, members
}

func TestAResultIsResetAsIfNeverPlayed(t *testing.T) {
	a, _, _, matches := blitzRoundOne(t)
	ctx := context.Background()
	annBob, cyDee := matches[0], matches[1]
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Finished: finishedAt}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", cyDee, "a", "Dee did not show"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	if err := a.ResetBlitzResult(ctx, "us_open", "2026", annBob); err != nil {
		t.Fatalf("ResetBlitzResult(Ann v Bob): %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if m := matchOf(t, ed, annBob); m.Result != "" || m.Game != nil || m.DecidedAt != nil {
		t.Errorf("Ann v Bob once reset = %q, want no result and no game", resultLine(m))
	}
	if m := matchOf(t, ed, cyDee); m.Result != "a" {
		t.Errorf("Cy v Dee = %q, want its ruling kept", resultLine(m))
	}
	if g, b, s, gm := playedRows(t, a); g+b+s+gm != 0 {
		t.Errorf("left after the reset: %d games, %d batches, %d stages, %d group members; want none", g, b, s, gm)
	}
	for range 2 {
		if err := a.ResetBlitzResult(ctx, "us_open", "2026", cyDee); err != nil {
			t.Errorf("ResetBlitzResult(Cy v Dee), a ruling, then nothing to undo: %v", err)
		}
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(180), B: finished(200), Finished: finishedAt}); err != nil {
		t.Errorf("recording Ann v Bob again with the code reset: %v", err)
	}
}

func TestARoundsResultsAreResetTogether(t *testing.T) {
	a, _, _, matches := blitzRoundOne(t)
	ctx := context.Background()
	for i, m := range matches {
		if err := a.RecordBlitzGame(ctx, "us_open", "2026", m, BlitzGame{ShareCode: []string{"ABCD1234", ""}[i], A: finished(250), B: finished(200), Finished: finishedAt}); err != nil {
			t.Fatalf("RecordBlitzGame: %v", err)
		}
	}
	round := roundID(t, a, 1)
	if err := a.ResetBlitzRound(ctx, "us_open", "2026", round); err != nil {
		t.Fatalf("ResetBlitzRound: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if len(ed.Rounds) != 1 || ed.Rounds[0].PublishedAt == nil || len(ed.Rounds[0].Matches) != 2 {
		t.Fatalf("rounds once reset = %v, want round 1 published with its two matches", roundLines(ed.Rounds))
	}
	for _, m := range ed.Rounds[0].Matches {
		if m.Result != "" || m.Game != nil {
			t.Errorf("match %d once reset = %q, want no result", m.ID, resultLine(m))
		}
	}
	if g, b, s, gm := playedRows(t, a); g+b+s+gm != 0 {
		t.Errorf("left after the reset: %d games, %d batches, %d stages, %d group members; want none", g, b, s, gm)
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", round); err != nil {
		t.Errorf("deleting the round once reset: %v", err)
	}
}

func TestOnlyTheLatestRoundIsReset(t *testing.T) {
	a, _, ids, matches := blitzRoundOne(t)
	ctx := context.Background()
	for _, m := range matches {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m, "a", "a forfeit"); err != nil {
			t.Fatalf("RecordBlitzRuling: %v", err)
		}
	}
	r2, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 2, Matches: []BlitzMatch{{A: ids[0], B: &ids[2]}, {A: ids[1]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	tests := []struct {
		name string
		do   func() error
		want error
	}{
		{"a result of an earlier round", func() error { return a.ResetBlitzResult(ctx, "us_open", "2026", matches[0]) }, ErrBlitzNotLatest},
		{"an earlier round", func() error { return a.ResetBlitzRound(ctx, "us_open", "2026", roundID(t, a, 1)) }, ErrBlitzNotLatest},
		{"a bye", func() error { return a.ResetBlitzResult(ctx, "us_open", "2026", r2.Matches[1].ID) }, ErrBlitzBye},
		{"an unknown match", func() error { return a.ResetBlitzResult(ctx, "us_open", "2026", 999) }, ErrNoBlitzMatch},
		{"an unknown round", func() error { return a.ResetBlitzRound(ctx, "us_open", "2026", 999) }, ErrNoBlitzRound},
		{"an unknown edition", func() error { return a.ResetBlitzEdition(ctx, "us_open", "2027") }, ErrNoBlitzEdition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.do(); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAnEditionIsResetToItsEntrants(t *testing.T) {
	a, _, ids, matches := blitzRoundOne(t)
	ctx := context.Background()
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Finished: finishedAt}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", matches[1], "a", "Dee did not show"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	markDropped(t, a, ids[1])
	if _, err := a.SkipBlitzRound(ctx, "us_open", "2026", ids[3]); err != nil {
		t.Fatalf("SkipBlitzRound: %v", err)
	}
	points := blitz.Scoring{Win: 3, Draw: 1, ForfeitWin: 3, Bye: 3}
	if err := a.SetBlitzPoints(ctx, "us_open", "2026", points); err != nil {
		t.Fatalf("SetBlitzPoints: %v", err)
	}
	if err := a.ResetBlitzEdition(ctx, "us_open", "2026"); err != nil {
		t.Fatalf("ResetBlitzEdition: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if len(ed.Rounds) != 0 {
		t.Errorf("rounds once reset = %v, want none", roundLines(ed.Rounds))
	}
	if len(ed.Entrants) != 4 {
		t.Errorf("entrants once reset = %d, want all 4 kept", len(ed.Entrants))
	}
	for _, e := range ed.Entrants {
		if e.Status != "active" || len(e.Skips) != 0 {
			t.Errorf("%s once reset = %s with skips %v, want in with none", e.Name, e.Status, e.Skips)
		}
	}
	if ed.Points != points {
		t.Errorf("points once reset = %+v, want the edition's %+v kept", ed.Points, points)
	}
	if g, b, s, gm := playedRows(t, a); g+b+s+gm != 0 {
		t.Errorf("left after the reset: %d games, %d batches, %d stages, %d group members; want none", g, b, s, gm)
	}
}

func TestAGameInARatingsRunIsResetToo(t *testing.T) {
	tests := []struct {
		name  string
		reset func(a *AdminStore, match, round int64) error
	}{
		{"the result", func(a *AdminStore, match, _ int64) error {
			return a.ResetBlitzResult(context.Background(), "us_open", "2026", match)
		}},
		{"the round", func(a *AdminStore, _, round int64) error {
			return a.ResetBlitzRound(context.Background(), "us_open", "2026", round)
		}},
		{"the edition", func(a *AdminStore, _, _ int64) error {
			return a.ResetBlitzEdition(context.Background(), "us_open", "2026")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, a := blitzFixture(t)
			ctx := context.Background()
			ids, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Ann", Zone: "UTC"}, {Name: "Bob", Zone: "UTC"}})
			if err != nil {
				t.Fatalf("AddBlitzEntrants: %v", err)
			}
			r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}}})
			if err != nil {
				t.Fatalf("AddBlitzRound: %v", err)
			}
			if err := a.RecordBlitzGame(ctx, "us_open", "2026", r.Matches[0].ID, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Finished: finishedAt}); err != nil {
				t.Fatalf("RecordBlitzGame: %v", err)
			}
			ed, err := a.BlitzEdition(ctx, "us_open", "2026")
			if err != nil {
				t.Fatalf("BlitzEdition: %v", err)
			}
			game, run := *matchOf(t, ed, r.Matches[0].ID).GameID, publishedRun(t, pool)
			classifyGame(t, pool, run, game, classified{season: 2026, nameSeason: 2026})
			if _, err := pool.Exec(ctx, `
				INSERT INTO pairwise_matches (run_id, match_id, date, tournament, player_count, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible)
				VALUES ($1, $2, '2026-10-15', 'US Open', 2, 'Ann', 'Bob', 250, 200, 1, 1, false)`, run, game); err != nil {
				t.Fatalf("pairing the game in the run: %v", err)
			}
			if err := tt.reset(a, r.Matches[0].ID, r.ID); err != nil {
				t.Fatalf("resetting %s: %v", tt.name, err)
			}
			var left int
			if err := pool.QueryRow(ctx, `
				SELECT (SELECT count(*) FROM games) + (SELECT count(*) FROM tournament_games) + (SELECT count(*) FROM pairwise_matches)`).Scan(&left); err != nil || left != 0 {
				t.Errorf("the game and the run's rows for it after the reset = %d, %v; want none", left, err)
			}
		})
	}
}

func TestTheAdminLoginDeletesOnlyBlitzGamesNoMatchHolds(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	ids, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Ann", Zone: "UTC"}, {Name: "Bob", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", r.Matches[0].ID, BlitzGame{A: finished(250), B: finished(200), Finished: finishedAt}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	var held int64
	if err := pool.QueryRow(ctx, "SELECT game_id FROM blitz_matches WHERE match_id = $1", r.Matches[0].ID).Scan(&held); err != nil {
		t.Fatalf("finding the game: %v", err)
	}
	var loaded int64
	if err := pool.QueryRow(ctx, `
		WITH b AS (INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id)
		INSERT INTO games (batch_id, source_file, source_line, name, date, state, group_id)
		SELECT b.batch_id, 'matches.csv', 1, 'US Open 2026 Round 1', '2026-10-15', 'played', g.group_id FROM b, games g WHERE g.game_id = $1
		RETURNING game_id`, held).Scan(&loaded); err != nil {
		t.Fatalf("adding a loaded game: %v", err)
	}
	for name, id := range map[string]int64{"a game a match holds": held, "a game the load job loaded": loaded, "no game": 999999} {
		if _, err := a.pool.Exec(ctx, "SELECT delete_blitz_game($1)", id); err == nil {
			t.Errorf("delete_blitz_game(%s) as the admin login succeeded, want it refused", name)
		}
	}
	if g, _, _, _ := playedRows(t, a); g != 2 {
		t.Errorf("games after the refusals = %d, want both kept", g)
	}
}
