package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

// FactsReplaced counts what a replace did: rows written, and rows naming
// someone who is not a player.
type FactsReplaced struct {
	Written int
	Unknown int
}

// replaceAll empties table and runs insert with args in one transaction,
// returning how many rows the insert wrote.
func replaceAll(ctx context.Context, pool *pgxpool.Pool, table, insert string, args ...any) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("starting the %s load: %w", table, err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM "+pgx.Identifier{table}.Sanitize()); err != nil {
		return 0, fmt.Errorf("clearing %s: %w", table, err)
	}
	tag, err := tx.Exec(ctx, insert, args...)
	if err != nil {
		return 0, fmt.Errorf("writing %s: %w", table, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing %s: %w", table, err)
	}
	return int(tag.RowsAffected()), nil
}

// ReplaceDivisions replaces every recorded tournament division, in one
// transaction. A row naming someone who is not a player is counted, not written.
func ReplaceDivisions(ctx context.Context, pool *pgxpool.Pool, divs []tournamentcsv.Division) (FactsReplaced, error) {
	var series, names, players, sources []string
	var seasons []int32
	for _, d := range divs {
		series, seasons, names = append(series, d.Series), append(seasons, int32(d.Season)), append(names, d.Division)
		players, sources = append(players, d.Player), append(sources, d.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_group_divisions", `
		INSERT INTO tournament_group_divisions (series, season, division, player_name, source)
		SELECT d.series, d.season, d.division, p.name, d.source
		FROM unnest($1::text[], $2::int[], $3::text[], $4::text[], $5::text[])
		     AS d(series, season, division, player, source)
		JOIN players p ON p.name = d.player`,
		series, seasons, names, players, sources)
	return FactsReplaced{Written: n, Unknown: len(divs) - n}, err
}

// ReplacePlacements replaces every official placement, in one transaction. A
// row naming someone who is not a player is counted, not written.
func ReplacePlacements(ctx context.Context, pool *pgxpool.Pool, places []tournamentcsv.Placement) (FactsReplaced, error) {
	var series, divisions, players, reasons, sources []string
	var seasons, placeNos []int32
	var points []*float64
	for _, p := range places {
		series, seasons, divisions = append(series, p.Series), append(seasons, int32(p.Season)), append(divisions, p.Division)
		players, placeNos, points = append(players, p.Player), append(placeNos, int32(p.Place)), append(points, p.Points)
		reasons, sources = append(reasons, p.Reason), append(sources, p.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_official_placements", `
		INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		SELECT o.series, o.season, o.division, p.name, o.place, o.points, o.reason, o.source
		FROM unnest($1::text[], $2::int[], $3::text[], $4::text[], $5::int[], $6::float8[], $7::text[], $8::text[])
		     AS o(series, season, division, player, place, points, reason, source)
		JOIN players p ON p.name = o.player`,
		series, seasons, divisions, players, placeNos, points, reasons, sources)
	return FactsReplaced{Written: n, Unknown: len(places) - n}, err
}

// ReplaceCorrections replaces every season correction, in one transaction.
func ReplaceCorrections(ctx context.Context, pool *pgxpool.Pool, fixes []tournamentcsv.Correction) (FactsReplaced, error) {
	var series, reasons, sources []string
	var named []int32
	var seasons []*int32
	for _, c := range fixes {
		var season *int32
		if c.Season != nil {
			s := int32(*c.Season)
			season = &s
		}
		series, named, seasons = append(series, c.Series), append(named, int32(c.NameSeason)), append(seasons, season)
		reasons, sources = append(reasons, c.Reason), append(sources, c.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_season_corrections", `
		INSERT INTO tournament_season_corrections (series, name_season, season, reason, source)
		SELECT * FROM unnest($1::text[], $2::int[], $3::int[], $4::text[], $5::text[])`,
		series, named, seasons, reasons, sources)
	return FactsReplaced{Written: n}, err
}

// ReplaceFinishDates replaces every season's recorded finish date, in one
// transaction.
func ReplaceFinishDates(ctx context.Context, pool *pgxpool.Pool, dates []tournamentcsv.FinishDate) (FactsReplaced, error) {
	var series, days, sources []string
	var seasons []int32
	for _, d := range dates {
		series, seasons = append(series, d.Series), append(seasons, int32(d.Season))
		days, sources = append(days, d.Date.Format(time.DateOnly)), append(sources, d.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_finish_dates", `
		INSERT INTO tournament_finish_dates (series, season, finish_date, source)
		SELECT * FROM unnest($1::text[], $2::int[], $3::date[], $4::text[])`,
		series, seasons, days, sources)
	return FactsReplaced{Written: n}, err
}

// ReplaceHonours replaces every recorded career honour, in one transaction.
// A row naming someone who is not a player is counted, not written.
func ReplaceHonours(ctx context.Context, pool *pgxpool.Pool, honours []tournamentcsv.Honour) (FactsReplaced, error) {
	var series, players, sources []string
	var golds, silvers, bronzes []int32
	var points []*string
	for _, h := range honours {
		series, players, sources = append(series, h.Series), append(players, h.Player), append(sources, h.Source)
		golds, silvers, bronzes = append(golds, int32(h.Gold)), append(silvers, int32(h.Silver)), append(bronzes, int32(h.Bronze))
		points = append(points, h.Points)
	}
	n, err := replaceAll(ctx, pool, "tournament_career_honours", `
		INSERT INTO tournament_career_honours (series, player_name, gold, silver, bronze, points, source)
		SELECT h.series, p.name, h.gold, h.silver, h.bronze, h.points, h.source
		FROM unnest($1::text[], $2::text[], $3::int[], $4::int[], $5::int[], $6::text[], $7::text[])
		     AS h(series, player, gold, silver, bronze, points, source)
		JOIN players p ON p.name = h.player`,
		series, players, golds, silvers, bronzes, points, sources)
	return FactsReplaced{Written: n, Unknown: len(honours) - n}, err
}

// ReplaceCommunityStandings replaces every community leaderboard standing, in
// one transaction. Names are kept as written, players or not.
func ReplaceCommunityStandings(ctx context.Context, pool *pgxpool.Pool, rows []tournamentcsv.Standing) (FactsReplaced, error) {
	var editions, players, sources []string
	var ranks, events []int32
	var points []float64
	for _, r := range rows {
		editions, ranks, players = append(editions, r.Edition), append(ranks, int32(r.Rank)), append(players, r.Player)
		points, events, sources = append(points, r.Points), append(events, int32(r.Events)), append(sources, r.Source)
	}
	n, err := replaceAll(ctx, pool, "community_standings", `
		INSERT INTO community_standings (edition, rank, player_name, points, events, source)
		SELECT * FROM unnest($1::text[], $2::int[], $3::text[], $4::float8[], $5::int[], $6::text[])`,
		editions, ranks, players, points, events, sources)
	return FactsReplaced{Written: n}, err
}

// ReplaceCommunityScores replaces every community leaderboard event score, in
// one transaction. Names are kept as written, players or not.
func ReplaceCommunityScores(ctx context.Context, pool *pgxpool.Pool, rows []tournamentcsv.Score) (FactsReplaced, error) {
	var editions, players, events, sources []string
	var scores []float64
	for _, r := range rows {
		editions, players, events = append(editions, r.Edition), append(players, r.Player), append(events, r.Event)
		scores, sources = append(scores, r.Score), append(sources, r.Source)
	}
	n, err := replaceAll(ctx, pool, "community_scores", `
		INSERT INTO community_scores (edition, player_name, event, score, source)
		SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::float8[], $5::text[])`,
		editions, players, events, scores, sources)
	return FactsReplaced{Written: n}, err
}
