//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestDataSummaryCountsTheGamesAndLoads(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2":        day("2024-02-03"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	exec := func(q string) {
		t.Helper()
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec("UPDATE games SET state = 'never_played', date = '2025-01-01' WHERE name = 'Intermezzo S24 - Bronze 1 game 2'")
	exec("UPDATE games SET group_id = NULL WHERE name = 'Wimbledon 2024 Stage 1 - group 1 game 1'")
	var players int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM players").Scan(&players); err != nil {
		t.Fatal(err)
	}

	got, err := NewAdminStore(asAdminLogin(t, pool)).DataSummary(ctx)
	if err != nil {
		t.Fatalf("DataSummary: %v", err)
	}
	if got.Games != 2 || got.NeverPlayed != 1 || got.Unplaced != 1 || got.Players != players {
		t.Errorf("summary = %+v, want 2 played, 1 never played, 1 unplaced, %d players", got, players)
	}
	if got.LatestGame == nil || !got.LatestGame.Equal(day("2024-07-01")) {
		t.Errorf("latest game = %v, want the latest played one, 2024-07-01", got.LatestGame)
	}
	if got.LastLoad == nil || time.Since(*got.LastLoad) > time.Hour || got.LastLoadRefused != 0 {
		t.Errorf("last load = %v with %d refused, want just now and none refused", got.LastLoad, got.LastLoadRefused)
	}
}

func TestDataSummaryOfAnEmptyDatabase(t *testing.T) {
	pool := migrated(t)
	got, err := NewAdminStore(asAdminLogin(t, pool)).DataSummary(context.Background())
	if err != nil {
		t.Fatalf("DataSummary: %v", err)
	}
	got.DatabaseBytes = 0 // an empty database still has a size
	if got != (DataSummary{}) {
		t.Errorf("summary = %+v, want nothing", got)
	}
}

func TestDataSummaryGivesTheDatabasesSizeAndTheRunsTotal(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	for _, b := range []int64{1000, 2000} {
		run := addRun(t, pool, "complete", time.Time{})
		if _, err := pool.Exec(ctx, "UPDATE rating_runs SET result_bytes = $2 WHERE run_id = $1", run, b); err != nil {
			t.Fatal(err)
		}
	}

	got, err := NewAdminStore(asAdminLogin(t, pool)).DataSummary(ctx)
	if err != nil {
		t.Fatalf("DataSummary: %v", err)
	}
	if got.RunsBytes != 3000 || got.DatabaseBytes <= got.RunsBytes {
		t.Errorf("runs %d bytes, database %d, want 3000 and the database larger", got.RunsBytes, got.DatabaseBytes)
	}
}

func TestTheLastLoadIsOurLatestImportNotAGameEnteredByHand(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	for _, q := range []string{
		"INSERT INTO import_batches (source, created_at) VALUES ('load', '2026-10-01T00:00:00Z')",
		"INSERT INTO import_batches (source, created_at) VALUES ('admin', '2026-10-02T00:00:00Z'), ('player', '2026-10-03T00:00:00Z')",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	got, err := NewAdminStore(asAdminLogin(t, pool)).DataSummary(ctx)
	if err != nil {
		t.Fatalf("DataSummary: %v", err)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); got.LastLoad == nil || !got.LastLoad.Equal(want) {
		t.Errorf("last load = %v, want the load's %v", got.LastLoad, want)
	}
}
