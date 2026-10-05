package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/gamename"
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

// ReplaceRules replaces every season's recorded rules, in one transaction.
func ReplaceRules(ctx context.Context, pool *pgxpool.Pool, rules []tournamentcsv.Rule) (FactsReplaced, error) {
	var series, s3, s4, tiebreaks, sources []string
	var seasons []int32
	var penalties []float64
	for _, r := range rules {
		series, seasons, penalties = append(series, r.Series), append(seasons, int32(r.Season)), append(penalties, r.TimeoutPenalty)
		s3, s4, tiebreaks, sources = append(s3, r.Scoring3p), append(s4, r.Scoring4p), append(tiebreaks, r.Tiebreaks), append(sources, r.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_season_rules", `
		INSERT INTO tournament_season_rules (series, season, timeout_penalty, scoring_3p, scoring_4p, tiebreaks, source)
		SELECT * FROM unnest($1::text[], $2::int[], $3::float8[], $4::text[], $5::text[], $6::text[], $7::text[])`,
		series, seasons, penalties, s3, s4, tiebreaks, sources)
	return FactsReplaced{Written: n}, err
}

// ReplacePodiums replaces every cup year's recorded podium, in one transaction.
// A row naming someone who is not a player is counted, not written.
func ReplacePodiums(ctx context.Context, pool *pgxpool.Pool, podiums []tournamentcsv.Podium) (FactsReplaced, error) {
	var cups, seasons, finals, players, sources []string
	var places []int32
	var finished []time.Time
	for _, p := range podiums {
		cups, seasons, finals, places = append(cups, p.Cup), append(seasons, p.Season), append(finals, p.Final), append(places, int32(p.Place))
		players, sources, finished = append(players, p.Player), append(sources, p.Source), append(finished, p.Finished)
	}
	n, err := replaceAll(ctx, pool, "tournament_cup_podiums", `
		INSERT INTO tournament_cup_podiums (cup, season, final, place, player_name, source, finished)
		SELECT x.cup, x.season, x.final, x.place, p.name, x.source, x.finished
		FROM unnest($1::text[], $2::text[], $3::text[], $4::int[], $5::text[], $6::text[], $7::date[])
		     AS x(cup, season, final, place, player, source, finished)
		JOIN players p ON p.name = x.player`,
		cups, seasons, finals, places, players, sources, finished)
	return FactsReplaced{Written: n, Unknown: len(podiums) - n}, err
}

// ReplaceSeasonNotes replaces every season's and stage's note, in one transaction.
func ReplaceSeasonNotes(ctx context.Context, pool *pgxpool.Pool, notes []tournamentcsv.SeasonNote) (FactsReplaced, error) {
	var slugs, seasons, stages, texts, sources []string
	for _, n := range notes {
		slugs, seasons, stages = append(slugs, n.Tournament), append(seasons, n.Season), append(stages, n.Stage)
		texts, sources = append(texts, n.Note), append(sources, n.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_season_notes", `
		INSERT INTO tournament_season_notes (tournament, season, stage, note, source)
		SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[])`,
		slugs, seasons, stages, texts, sources)
	return FactsReplaced{Written: n, Unknown: len(notes) - n}, err
}

// ReplaceLadderWins replaces every ladder's top-tier wins, in one
// transaction. A row naming someone who is not a player, or a tournament that
// is not a ladder, is counted, not written.
func ReplaceLadderWins(ctx context.Context, pool *pgxpool.Pool, wins []tournamentcsv.LadderWins) (FactsReplaced, error) {
	var ladders, players, sources []string
	var counts []float64
	for _, w := range wins {
		ladders, players, counts, sources = append(ladders, w.Ladder), append(players, w.Player), append(counts, w.Wins), append(sources, w.Source)
	}
	n, err := replaceAll(ctx, pool, "tournament_ladder_wins", `
		INSERT INTO tournament_ladder_wins (ladder, player_name, wins, source)
		SELECT x.ladder, p.name, x.wins, x.source
		FROM unnest($1::text[], $2::text[], $3::numeric[], $4::text[]) AS x(ladder, player, wins, source)
		JOIN players p ON p.name = x.player
		JOIN tournaments t ON t.slug = x.ladder AND t.kind = 'ladder'`,
		ladders, players, counts, sources)
	return FactsReplaced{Written: n, Unknown: len(wins) - n}, err
}

// ReplaceCGEStages replaces every stage's link to the CGE tournament season
// it was played as, in one transaction. A row naming a stage the catalogue
// lacks is counted, not written.
func ReplaceCGEStages(ctx context.Context, pool *pgxpool.Pool, links []tournamentcsv.CGEStage) (FactsReplaced, error) {
	var slugs, seasons, stages []string
	var cgeT, cgeS []int32
	for _, l := range links {
		slugs, seasons, stages = append(slugs, l.Tournament), append(seasons, l.Season), append(stages, l.Stage)
		cgeT, cgeS = append(cgeT, int32(l.CGETournament)), append(cgeS, int32(l.CGESeason))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return FactsReplaced{}, fmt.Errorf("starting the CGE stages load: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "UPDATE tournament_stages SET cge_tournament = NULL, cge_season = NULL WHERE cge_tournament IS NOT NULL"); err != nil {
		return FactsReplaced{}, fmt.Errorf("clearing CGE stages: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE tournament_stages st SET cge_tournament = x.ct, cge_season = x.cs
		FROM unnest($1::text[], $2::text[], $3::text[], $4::int[], $5::int[]) AS x(slug, season, stage, ct, cs)
		JOIN tournaments t ON t.slug = x.slug
		JOIN tournament_seasons s ON s.tournament_id = t.tournament_id AND s.label = x.season
		WHERE st.season_id = s.season_id AND st.label = x.stage`, slugs, seasons, stages, cgeT, cgeS)
	if err != nil {
		return FactsReplaced{}, fmt.Errorf("writing CGE stages: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return FactsReplaced{}, fmt.Errorf("committing CGE stages: %w", err)
	}
	n := int(tag.RowsAffected())
	return FactsReplaced{Written: n, Unknown: len(links) - n}, nil
}

// ReplaceCGETournaments replaces CGE's catalogue, in one transaction, linking
// each row to the tournament it names. A tournament named that the catalogue
// lacks is added under its CGE name, in no ranking: admin decides its rating.
func ReplaceCGETournaments(ctx context.Context, pool *pgxpool.Pool, rows []tournamentcsv.CGETournament) (FactsReplaced, error) {
	var ids []int32
	var names, statuses, organisers, descriptions, glicko, links, slugs, seasons []string
	var created, started, finished []*time.Time
	for _, c := range rows {
		ids, names, statuses, organisers = append(ids, int32(c.ID)), append(names, c.Name), append(statuses, c.Status), append(organisers, c.Organiser)
		created, started, finished = append(created, c.Created), append(started, c.Started), append(finished, c.Finished)
		descriptions, glicko, links = append(descriptions, c.Description), append(glicko, c.MentionsGlicko), append(links, c.Links)
		slugs, seasons = append(slugs, c.Tournament), append(seasons, c.Season)
	}
	known := map[string]string{}
	for _, t := range gamename.Tournaments {
		known[t.Slug] = t.Name
	}
	var addSlugs, addNames []string
	for _, c := range rows {
		if c.Tournament != "" && !slices.Contains(addSlugs, c.Tournament) {
			name := known[c.Tournament]
			if name == "" {
				name = c.Name
			}
			addSlugs, addNames = append(addSlugs, c.Tournament), append(addNames, name)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return FactsReplaced{}, fmt.Errorf("starting the CGE catalogue load: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO tournaments (slug, name, original_name) SELECT slug, name, name FROM unnest($1::text[], $2::text[]) AS x(slug, name)
		ON CONFLICT (slug) DO NOTHING`, addSlugs, addNames); err != nil {
		return FactsReplaced{}, fmt.Errorf("adding the tournaments CGE's catalogue names: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM cge_tournaments"); err != nil {
		return FactsReplaced{}, fmt.Errorf("clearing cge_tournaments: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO cge_tournaments (cge_tournament, name, status, organiser, created, started, finished,
		                             description, mentions_glicko, links, tournament_id, season)
		SELECT x.id, x.name, x.status, x.organiser, x.created, x.started, x.finished,
		       x.description, x.glicko, x.links, t.tournament_id, x.season
		FROM unnest($1::int[], $2::text[], $3::text[], $4::text[], $5::date[], $6::date[], $7::date[],
		            $8::text[], $9::text[], $10::text[], $11::text[], $12::text[])
		     AS x(id, name, status, organiser, created, started, finished, description, glicko, links, slug, season)
		LEFT JOIN tournaments t ON t.slug = x.slug`,
		ids, names, statuses, organisers, created, started, finished, descriptions, glicko, links, slugs, seasons)
	if err != nil {
		return FactsReplaced{}, fmt.Errorf("writing cge_tournaments: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return FactsReplaced{}, fmt.Errorf("committing cge_tournaments: %w", err)
	}
	return FactsReplaced{Written: int(tag.RowsAffected())}, nil
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
	var sheetRows, ranks, events []int32
	var points []float64
	for _, r := range rows {
		editions, sheetRows, ranks = append(editions, r.Edition), append(sheetRows, int32(r.SheetRow)), append(ranks, int32(r.Rank))
		players, points, events = append(players, r.Player), append(points, r.Points), append(events, int32(r.Events))
		sources = append(sources, r.Source)
	}
	n, err := replaceAll(ctx, pool, "community_standings", `
		INSERT INTO community_standings (edition, sheet_row, rank, player_name, points, events, source)
		SELECT * FROM unnest($1::text[], $2::int[], $3::int[], $4::text[], $5::float8[], $6::int[], $7::text[])`,
		editions, sheetRows, ranks, players, points, events, sources)
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

// ReplaceCommunityPoints replaces every community points table, in one
// transaction. A row for a tournament the catalogue lacks is counted, not written.
func ReplaceCommunityPoints(ctx context.Context, pool *pgxpool.Pool, rows []tournamentcsv.CommunityPoints) (FactsReplaced, error) {
	var tournaments, groups, sources []string
	var editions, places, points []int32
	for _, r := range rows {
		tournaments, editions, groups = append(tournaments, r.Tournament), append(editions, int32(r.FromEdition)), append(groups, r.Group)
		places, points, sources = append(places, int32(r.Place)), append(points, int32(r.Points)), append(sources, r.Source)
	}
	n, err := replaceAll(ctx, pool, "community_points", `
		INSERT INTO community_points (tournament, from_edition, group_name, place, points, source)
		SELECT x.* FROM unnest($1::text[], $2::int[], $3::text[], $4::int[], $5::int[], $6::text[])
		       AS x(tournament, from_edition, group_name, place, points, source)
		JOIN tournaments t ON t.slug = x.tournament`,
		tournaments, editions, groups, places, points, sources)
	return FactsReplaced{Written: n, Unknown: len(rows) - n}, err
}
