// Command load seeds the ratings database from a directory of match CSVs and
// places each game in the tournament catalogue, sets players' countries from
// CSVs with Player and Code columns, or replaces one kind of recorded
// tournament fact from one CSV.
// It checks every row first, and writes no matches if the database has some.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgegames"
	"ttacommunity/backend/internal/countrycsv"
	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/tournamentcsv"
)

func main() {
	reset := flag.Bool("reset", false, "delete every match, refusal and rating run first")
	migrate := flag.Bool("migrate", false, "only apply the ratings migrations, and load nothing")
	countries := flag.Bool("countries", false, "set players' countries from the CSV files named, a later file winning")
	classify := flag.Bool("classify", false, "only place unplaced games in the tournament catalogue, by their names")
	cgeGames := flag.String("cge-games", "", "attach CGE's facts (seats, CGE users, finish dates) from the CSV named to the games already loaded")
	facts := flag.String("facts", "", "replace every recorded tournament fact of one kind (divisions, placements, corrections, dates, honours, rules, podiums, cge_tournaments, community or community_scores) with the CSV file named")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: load [-reset] <directory of match CSVs>")
		fmt.Fprintln(os.Stderr, "       load -migrate")
		fmt.Fprintln(os.Stderr, "       load -classify")
		fmt.Fprintln(os.Stderr, "       load -cge-games <CSV file>")
		fmt.Fprintln(os.Stderr, "       load -countries <CSV file>...")
		fmt.Fprintln(os.Stderr, "       load -facts divisions|placements|corrections|dates|honours|rules|podiums|cge_tournaments|community|community_scores <CSV file>")
		fmt.Fprintln(os.Stderr, "The database is RATINGS_DATABASE_URL.")
		flag.PrintDefaults()
	}
	flag.Parse()
	one := !*reset && btoi(*migrate)+btoi(*classify)+btoi(*cgeGames != "")+btoi(*countries)+btoi(*facts != "") == 1
	var err error
	switch {
	case *migrate && one && flag.NArg() == 0:
		err = migrateOnly()
	case *classify && one && flag.NArg() == 0:
		err = classifyOnly()
	case *cgeGames != "" && one && flag.NArg() == 0:
		err = attachCGEGames(*cgeGames)
	case *countries && one && flag.NArg() > 0:
		err = setCountries(flag.Args())
	case *facts != "" && one && flag.NArg() == 1:
		err = replaceFacts(*facts, flag.Arg(0))
	case !*migrate && !*classify && *cgeGames == "" && !*countries && *facts == "" && flag.NArg() == 1:
		err = run(*reset, flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(64)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
}

// migrateOnly applies every ratings migration not yet applied.
func migrateOnly() error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	fmt.Println("ratings migrations applied")
	return nil
}

// classifyOnly places every unplaced game in the tournament catalogue, after
// applying any pending migration.
func classifyOnly() error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	placed, err := store.ClassifyMatches(ctx, pool)
	if err != nil {
		return err
	}
	reportClassified(os.Stdout, placed)
	return nil
}

// reportClassified writes how many games were placed and each tournament
// name no rule reads. It names tournaments, never a player.
func reportClassified(w io.Writer, c store.Classified) {
	fmt.Fprintf(w, "placed in the catalogue: %d\n", c.Placed)
	if len(c.Unplaced) == 0 {
		return
	}
	fmt.Fprintf(w, "names no rule reads: %d (their games stay unplaced)\n", len(c.Unplaced))
	for _, n := range c.Unplaced {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

// attachCGEGames reads the file first, then records CGE's facts on the games
// already loaded and adds the games we lack (store.AddCGEGames), after
// applying any pending migration.
func attachCGEGames(file string) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	res, err := cgegames.Read(file, f)
	f.Close()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	got, err := store.AttachCGEGames(ctx, pool, res.Games)
	if err != nil {
		return err
	}
	added, err := store.AddCGEGames(ctx, pool, res.Games)
	if err != nil {
		return err
	}
	reportAttached(os.Stdout, file, res, added, got)
	return nil
}

// reportAttached writes how many CGE games matched ours, which did not (by
// CGE tournament and stage), which of our seasons each stage matched, and
// what was added. It names tournaments and seasons, never a player.
func reportAttached(w io.Writer, file string, res cgegames.Result, added store.Added, got store.Attached) {
	fmt.Fprintf(w, "matched to our games: %d\n", got.Matched)
	fmt.Fprintf(w, "never played, set apart: %d\n", got.NeverPlayed)
	fmt.Fprintf(w, "no game of ours: %d (CGE %s)\n", len(got.Unmatched), byStage(got.Unmatched))
	fmt.Fprintln(w, "where each CGE stage matched:")
	for _, l := range got.Seasons {
		fmt.Fprintf(w, "  CGE %d/%d -> season %s: %d games\n", l.CGETournament, l.CGESeason, l.Season, l.Games)
	}
	fmt.Fprintf(w, "refused: %d\n", len(res.Refused))
	for _, r := range res.Refused {
		fmt.Fprintf(w, "  %s:%d %s\n", file, r.Line, r.Reason)
	}
	fmt.Fprintf(w, "added: %d (%d of them never played)\n", added.Added, added.NeverPlayed)
	fmt.Fprintf(w, "already held: %d\n", added.AlreadyHeld)
	fmt.Fprintf(w, "placing not settled, not added: %d\n", added.Unsettled)
	fmt.Fprintf(w, "may be a copy of ours, not added: %d (CGE %s)\n", len(added.Conflicts), byStage(added.Conflicts))
}

// byStage counts games by CGE tournament and stage: "19/2: 1, 21/7: 2".
func byStage(games []cgegames.Game) string {
	counts := map[string]int{}
	for _, g := range games {
		counts[fmt.Sprintf("%d/%d", g.CGETournament, g.CGESeason)]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s: %d", k, counts[k])
	}
	return strings.Join(parts, ", ")
}

// setCountries reads every file first, then sets the countries of known
// players in one statement, after applying any pending migration.
func setCountries(files []string) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	res, err := countrycsv.ReadFiles(os.DirFS("/"), absolute(files))
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	set, err := store.SetCountries(ctx, pool, res.Countries)
	if err != nil {
		return err
	}
	reportCountries(os.Stdout, files, res, set)
	return nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// replaceFacts reads the file first, then replaces every recorded fact of
// one kind in one transaction, after applying any pending migration.
func replaceFacts(kind, file string) error {
	switch kind {
	case "divisions":
		return loadFacts(kind, file, tournamentcsv.ReadDivisions, store.ReplaceDivisions)
	case "placements":
		return loadFacts(kind, file, tournamentcsv.ReadPlacements, store.ReplacePlacements)
	case "corrections":
		return loadFacts(kind, file, tournamentcsv.ReadCorrections, store.ReplaceCorrections)
	case "dates":
		return loadFacts(kind, file, tournamentcsv.ReadFinishDates, store.ReplaceFinishDates)
	case "honours":
		return loadFacts(kind, file, tournamentcsv.ReadHonours, store.ReplaceHonours)
	case "rules":
		return loadFacts(kind, file, tournamentcsv.ReadRules, store.ReplaceRules)
	case "podiums":
		return loadFacts(kind, file, tournamentcsv.ReadPodiums, store.ReplacePodiums)
	case "cge_tournaments":
		return loadFacts(kind, file, tournamentcsv.ReadCGETournaments, store.ReplaceCGETournaments)
	case "community":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityStandings, store.ReplaceCommunityStandings)
	case "community_scores":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityScores, store.ReplaceCommunityScores)
	}
	return fmt.Errorf("unknown kind of fact %q: want divisions, placements, corrections, dates, honours, rules, podiums, cge_tournaments, community or community_scores", kind)
}

func loadFacts[T any](
	kind, file string,
	read func(string, io.Reader) (tournamentcsv.Result[T], error),
	replace func(context.Context, *pgxpool.Pool, []T) (store.FactsReplaced, error),
) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	res, err := read(file, f)
	f.Close()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	replaced, err := replace(ctx, pool, res.Facts)
	if err != nil {
		return err
	}
	reportFacts(os.Stdout, kind, file, res.Rows, res.Refused, replaced)
	return nil
}

// reportFacts writes what was read and written. It names the file, lines
// and reasons, never a player.
func reportFacts(w io.Writer, kind, file string, rows int, refused []tournamentcsv.Refusal, replaced store.FactsReplaced) {
	fmt.Fprintf(w, "file: %s\n", file)
	fmt.Fprintf(w, "rows read: %d\n", rows)
	fmt.Fprintf(w, "%s written: %d\n", kind, replaced.Written)
	fmt.Fprintf(w, "not a player: %d\n", replaced.Unknown)
	fmt.Fprintf(w, "refused: %d\n", len(refused))
	for _, r := range refused {
		fmt.Fprintf(w, "  %s:%d %s\n", r.File, r.Line, r.Reason)
	}
}

// absolute turns each path into one os.DirFS("/") can open.
func absolute(files []string) []string {
	out := make([]string, len(files))
	for i, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		out[i] = strings.TrimPrefix(abs, "/")
	}
	return out
}

// reportCountries writes what was read and set. It names files, lines and
// reasons, never a player.
func reportCountries(w io.Writer, files []string, res countrycsv.Result, set store.CountriesSet) {
	fmt.Fprintf(w, "files: %s\n", strings.Join(files, ", "))
	fmt.Fprintf(w, "rows read: %d\n", res.Rows)
	fmt.Fprintf(w, "countries set: %d\n", set.Set)
	fmt.Fprintf(w, "not a player: %d\n", set.Unknown)
	fmt.Fprintf(w, "no country: %d\n", res.None)
	fmt.Fprintf(w, "refused: %d\n", len(res.Refused))
	for _, r := range res.Refused {
		fmt.Fprintf(w, "  %s:%d %s\n", r.File, r.Line, r.Reason)
	}
}

func run(reset bool, dir string) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	res, err := matchcsv.CheckFiles(os.DirFS(dir))
	if err != nil {
		return fmt.Errorf("checking %s: %w", dir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()

	if reset {
		err = store.ResetRatings(ctx, pool)
	} else {
		err = store.MigrateRatings(ctx, pool)
	}
	if err != nil {
		return err
	}
	loaded, err := store.LoadMatches(ctx, pool, res.Matches, res.Refused)
	if err != nil {
		return fmt.Errorf("loading: %w (to start again from empty: -reset, or --reset with scripts/ratings-db.sh)", err)
	}
	report(os.Stdout, res, loaded)
	placed, err := store.ClassifyMatches(ctx, pool)
	if err != nil {
		return fmt.Errorf("placing the loaded games: %w", err)
	}
	reportClassified(os.Stdout, placed)
	return nil
}

// report writes what was read, loaded and refused. It names files and
// reasons, never a player.
func report(w io.Writer, res matchcsv.Result, loaded store.Loaded) {
	fmt.Fprintf(w, "files: %d (%s)\n", len(res.Files), strings.Join(res.Files, ", "))
	fmt.Fprintf(w, "rows read: %d\n", res.Rows)
	fmt.Fprintf(w, "loaded: %d matches, %d players (batch %d)\n", loaded.Matches, loaded.Players, loaded.BatchID)
	fmt.Fprintf(w, "refused: %d\n", loaded.Refused)

	byReason := map[matchcsv.Reason]int{}
	for _, r := range res.Refused {
		byReason[r.Reason]++
	}
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, string(r))
	}
	slices.Sort(reasons)
	for _, r := range reasons {
		fmt.Fprintf(w, "  %-16s %d\n", r, byReason[matchcsv.Reason(r)])
	}
	if loaded.Refused > 0 {
		fmt.Fprintln(w, "Each refused row, with its file, line and reason, is in the refused_rows table.")
	}
}
